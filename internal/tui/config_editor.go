package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/config"
	"github.com/nassiharel/klim/internal/registry"
)

// configSetting and the SettingType constants are aliases for the
// shared schema defined in internal/config. The TUI keeps its own
// rendering helpers (currentValue, rawValue, settingLineOffset) but
// the field definitions live in one place so the web UI stays in
// sync.
type configSetting = config.Setting

const (
	settingBool     = config.SettingBool
	settingString   = config.SettingString
	settingInt      = config.SettingInt
	settingDuration = config.SettingDuration
	settingChoice   = config.SettingChoice
)

// configSavedMsg is sent after config is saved to disk.
type configSavedMsg struct {
	err error
}

// allConfigSettings returns the list of editable settings (delegates
// to the shared schema).
func allConfigSettings() []configSetting {
	return config.AllSettings()
}

// rawValue returns the underlying value for editing (not the display string).

// settingLineOffset estimates the rendered line offset for a setting at index idx,
// accounting for section headers (2 lines each: blank + header) and 1 line per setting.
// This keeps configScroll in sync with the actual rendered line count.
func settingLineOffset(settings []configSetting, idx int) int {
	line := 0
	currentSection := ""
	for i := 0; i <= idx && i < len(settings); i++ {
		if settings[i].Section != "" && settings[i].Section != currentSection {
			currentSection = settings[i].Section
			line += 2 // blank line + section header
		}
		if i < idx {
			line++ // setting line
		}
	}
	return line
}

func (m Model) handleKeyConfigEditor(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	settings := allConfigSettings()

	// If editing a text field, handle that first.
	if m.configEditing {
		switch msg.String() {
		case "esc":
			m.configEditing = false
			m.statusMsg = ""
			return m, nil
		case "enter":
			m.configEditing = false
			val := strings.TrimSpace(m.configEditInput.Value())
			if m.configCursor < len(settings) && m.cfg != nil {
				s := settings[m.configCursor]
				switch s.Type {
				case settingString:
					s.SetString(m.cfg, val) // empty = default
				case settingInt:
					if val == "" {
						s.SetInt(m.cfg, 0) // 0 = auto
					} else if v, err := strconv.Atoi(val); err == nil {
						s.SetInt(m.cfg, v)
					} else {
						m.statusMsg = "✗ Invalid number: " + val
						return m, nil
					}
				case settingDuration:
					if v, err := time.ParseDuration(val); err == nil {
						s.SetDuration(m.cfg, v)
					} else {
						m.statusMsg = fmt.Sprintf("✗ Invalid duration: %s (use e.g. 30s, 24h)", val)
						return m, nil
					}
				}
				m.statusMsg = "Modified (press S to save)"
			}
			return m, nil
		default:
			var cmd tea.Cmd
			m.configEditInput, cmd = m.configEditInput.Update(msg)
			return m, cmd
		}
	}

	// Global keys — tab switching, quit.
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "right", "tab":
		m.activeTab = (m.activeTab + 1) % tabCount
		m.cursor = 0
		m.dashboardScroll = 0
		m.discoverSubTab = discoverTools
		m.applyFilter()
		if m.activeTab == tabProject {
			return m, projectLoadListCmd(m.tools)
		}
		return m, nil
	case "left", "shift+tab":
		m.activeTab = (m.activeTab + tabCount - 1) % tabCount
		m.cursor = 0
		m.dashboardScroll = 0
		m.discoverSubTab = discoverTools
		m.applyFilter()
		if m.activeTab == tabProject {
			return m, projectLoadListCmd(m.tools)
		}
		return m, nil
	case "1":
		m.activeTab = tabInstalled
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "2":
		m.activeTab = tabFavorites
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "3":
		m.activeTab = tabUpdates
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "4":
		m.activeTab = tabDiscover
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "5":
		m.activeTab = tabBackup
		m.cursor = 0
		return m, nil
	case "6":
		m.activeTab = tabProject
		m.cursor = 0
		m.projectCursor = 0
		m.projectView = projectViewList
		return m, projectLoadListCmd(m.tools)
	case "7":
		m.activeTab = tabDashboard
		m.cursor = 0
		m.dashboardScroll = 0
		m.myBackupFiles = scanBackupsDir()
		return m, nil
	case "8":
		m.activeTab = tabConfig
		m.cursor = 0
		return m, nil
	}

	// Config-specific keys.
	switch msg.String() {
	case "up", "k":
		if m.configCursor > 0 {
			m.configCursor--
			// Skip section headers.
			for m.configCursor > 0 && settings[m.configCursor].Label == "" {
				m.configCursor--
			}
		} else if m.configScroll > 0 {
			// At top of settings — scroll preamble into view.
			m.configScroll--
		}
		m.autoScrollConfig(settings)
	case "down", "j":
		if m.configCursor < len(settings)-1 {
			m.configCursor++
		} else {
			// At bottom of settings — allow scrolling past.
			m.configScroll++
		}
		m.autoScrollConfig(settings)
	case "pgup":
		page := m.height - 6
		if page < 1 {
			page = 1
		}
		m.configScroll -= page
		if m.configScroll < 0 {
			m.configScroll = 0
		}
	case "pgdown":
		page := m.height - 6
		if page < 1 {
			page = 1
		}
		m.configScroll += page
	case "home", "g":
		m.configScroll = 0
		m.configCursor = 0
	case "enter", "space":
		if m.configCursor < len(settings) && m.cfg != nil {
			s := settings[m.configCursor]
			switch s.Type {
			case settingBool:
				// Toggle.
				s.SetBool(m.cfg, !s.GetBool(m.cfg))
				m.statusMsg = "Modified (press S to save)"
			case settingChoice:
				// Cycle through choices. Fall back to first if current unknown.
				current := s.GetString(m.cfg)
				found := false
				for i, c := range s.Choices {
					if c == current {
						s.SetString(m.cfg, s.Choices[(i+1)%len(s.Choices)])
						found = true
						break
					}
				}
				if !found && len(s.Choices) > 0 {
					s.SetString(m.cfg, s.Choices[0])
				}
				m.statusMsg = "Modified (press S to save)"
			case settingString, settingInt, settingDuration:
				// Enter edit mode with raw value.
				m.configEditing = true
				m.configEditInput.SetValue(s.Raw(m.cfg))
				m.configEditInput.SetWidth(40)
				return m, m.configEditInput.Focus()
			}
		}
		return m, nil
	case "S":
		// Save config to disk.
		if m.cfg != nil {
			return m, saveConfigCmd(m.cfg)
		}
	case "r":
		// Reset to defaults.
		if m.cfg != nil {
			def := config.Default()
			*m.cfg = *def
			m.statusMsg = "Reset to defaults (press S to save)"
		}
		return m, nil
	case "u":
		// Check for klim self-update. We never auto-install from the
		// TUI — replacing the running binary mid-render is fine on
		// Linux/macOS but adds a Windows-only "running file is locked"
		// failure mode that's better surfaced from the dedicated
		// `klim update` command. So this is check-only here, and the
		// status line points users to the CLI command if an upgrade
		// is available.
		m.statusMsg = "Checking for klim updates..."
		return m, checkSelfUpdateCmd()
	}
	return m, nil
}

func saveConfigCmd(cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		err := config.Save(cfg)
		return configSavedMsg{err: err}
	}
}

// --- Rendering ---

// configPreambleLines returns the number of rendered lines in renderConfigView
// before the editable settings section (version info, paths, package managers).
func (m Model) configPreambleLines() int {
	// 1 blank + Version + OS + Go + 1 blank + Config + Log + Last Scan + 1 blank + "Package Managers" header
	lines := 10
	lines += len(registry.AllPMStatusForOS())
	// Config warnings section: 1 blank + header + 1 blank + N warnings.
	if len(m.configWarnings) > 0 {
		lines += 3 + len(m.configWarnings)
	}
	return lines
}

// autoScrollConfig keeps the cursor-selected setting visible within the
// scrollable config view. Uses the same visible-rows calculation as renderView.
func (m *Model) autoScrollConfig(settings []configSetting) {
	preamble := m.configPreambleLines()
	cursorLine := preamble + settingLineOffset(settings, m.configCursor)

	// Match renderView's visible rows: height - headerRows(4: title+tabs+rule+blank) - gap(1) - footer.
	visibleRows := m.height - 5 - m.footerHeight()
	if visibleRows < 5 {
		visibleRows = 5
	}

	// Scroll up if cursor above viewport.
	if cursorLine < m.configScroll {
		m.configScroll = cursorLine
	}
	// Scroll down if cursor below viewport.
	if cursorLine >= m.configScroll+visibleRows {
		m.configScroll = cursorLine - visibleRows + 1
	}
}

func (m Model) renderConfigEditor() string {
	var b strings.Builder
	settings := allConfigSettings()

	b.WriteString("\n  " + detailTitleStyle.Render("Settings") + "\n")

	currentSection := ""
	for i, s := range settings {
		// Section header.
		if s.Section != "" && s.Section != currentSection {
			currentSection = s.Section
			b.WriteString("\n  " + dashSection.Render(currentSection) + "\n")
		}

		cursor := "  "
		if i == m.configCursor {
			cursor = "▸ "
		}

		label := dashLabel.Render(fixedWidth(s.Label, 20))
		var value string

		if m.cfg != nil {
			val := s.Display(m.cfg)
			switch s.Type {
			case settingBool:
				if s.GetBool(m.cfg) {
					value = upToDateStyle.Render("● true")
				} else {
					value = dimVersion.Render("○ false")
				}
			case settingChoice:
				value = buttonStyle.Render(val)
			default:
				value = nameStyle.Render(val)
			}
		} else {
			value = dimVersion.Render("(no config)")
		}

		// If currently editing this field, show input.
		if m.configEditing && i == m.configCursor {
			value = m.configEditInput.View()
		}

		line := cursor + label + "  " + value
		if i == m.configCursor && !m.configEditing {
			w := lipgloss.Width(line)
			if w < m.width {
				line += strings.Repeat(" ", m.width-w)
			}
			line = selectedRowStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	return b.String()
}
