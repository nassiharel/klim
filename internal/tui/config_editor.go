package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/registry"
)

// configSettingType describes what kind of value a setting holds.
type configSettingType int

const (
	settingBool configSettingType = iota
	settingString
	settingInt
	settingDuration
	settingChoice
)

// configSetting represents one editable config field.
type configSetting struct {
	section string // section header (Logging, Marketplace, etc.)
	label   string // display name
	key     string // unique key for identification
	typ     configSettingType
	choices []string // for settingChoice type

	// Get/set through the config pointer.
	getBool     func(*config.Config) bool
	setBool     func(*config.Config, bool)
	getString   func(*config.Config) string
	setString   func(*config.Config, string)
	getInt      func(*config.Config) int
	setInt      func(*config.Config, int)
	getDuration func(*config.Config) time.Duration
	setDuration func(*config.Config, time.Duration)
}

// configSavedMsg is sent after config is saved to disk.
type configSavedMsg struct {
	err error
}

// allConfigSettings returns the list of editable settings.
func allConfigSettings() []configSetting {
	return []configSetting{
		// --- Logging ---
		{section: "Logging", label: "Log Level", key: "log_level", typ: settingChoice,
			choices:   []string{"debug", "info", "warn", "error"},
			getString: func(c *config.Config) string { return c.Logging.Level },
			setString: func(c *config.Config, v string) { c.Logging.Level = v },
		},
		{section: "", label: "Log to File", key: "log_file", typ: settingBool,
			getBool: func(c *config.Config) bool { return c.Logging.File },
			setBool: func(c *config.Config, v bool) { c.Logging.File = v },
		},
		{section: "", label: "Verbose Logging", key: "log_verbose", typ: settingBool,
			getBool: func(c *config.Config) bool { return c.Logging.Verbose },
			setBool: func(c *config.Config, v bool) { c.Logging.Verbose = v },
		},
		// --- Marketplace ---
		{section: "Marketplace", label: "Catalog URL", key: "marketplace_url", typ: settingString,
			getString: func(c *config.Config) string { return c.Marketplace.URL },
			setString: func(c *config.Config, v string) { c.Marketplace.URL = v },
		},
		{section: "", label: "Auto Refresh", key: "auto_refresh", typ: settingBool,
			getBool: func(c *config.Config) bool { return c.Marketplace.AutoRefresh },
			setBool: func(c *config.Config, v bool) { c.Marketplace.AutoRefresh = v },
		},
		{section: "", label: "Refresh Interval", key: "refresh_interval", typ: settingDuration,
			getDuration: func(c *config.Config) time.Duration { return c.Marketplace.RefreshInterval.Duration },
			setDuration: func(c *config.Config, v time.Duration) { c.Marketplace.RefreshInterval = config.Duration{Duration: v} },
		},
		// --- Performance ---
		{section: "Performance", label: "Concurrency", key: "concurrency", typ: settingInt,
			getInt: func(c *config.Config) int { return c.Performance.Concurrency },
			setInt: func(c *config.Config, v int) { c.Performance.Concurrency = v },
		},
		{section: "", label: "Command Timeout", key: "cmd_timeout", typ: settingDuration,
			getDuration: func(c *config.Config) time.Duration { return c.Performance.CommandTimeout.Duration },
			setDuration: func(c *config.Config, v time.Duration) { c.Performance.CommandTimeout = config.Duration{Duration: v} },
		},
		// --- UI ---
		{section: "UI", label: "Default Tab", key: "default_tab", typ: settingChoice,
			choices:   []string{"installed", "updates", "marketplace", "backup", "dashboard", "config"},
			getString: func(c *config.Config) string { return c.UI.DefaultTab },
			setString: func(c *config.Config, v string) { c.UI.DefaultTab = v },
		},
		{section: "", label: "Show Path", key: "show_path", typ: settingBool,
			getBool: func(c *config.Config) bool { return c.UI.ShowPath },
			setBool: func(c *config.Config, v bool) { c.UI.ShowPath = v },
		},
		{section: "", label: "Sidebar Right", key: "sidebar_right", typ: settingBool,
			getBool: func(c *config.Config) bool { return c.UI.SidebarRight },
			setBool: func(c *config.Config, v bool) { c.UI.SidebarRight = v },
		},
	}
}

// currentValue returns the display string for a setting.
func (s configSetting) currentValue(cfg *config.Config) string {
	switch s.typ {
	case settingBool:
		if s.getBool(cfg) {
			return "true"
		}
		return "false"
	case settingString:
		v := s.getString(cfg)
		if v == "" {
			return "(default)"
		}
		return v
	case settingInt:
		v := s.getInt(cfg)
		if v == 0 {
			return "auto"
		}
		return strconv.Itoa(v)
	case settingDuration:
		return s.getDuration(cfg).String()
	case settingChoice:
		return s.getString(cfg)
	}
	return ""
}

// rawValue returns the underlying value for editing (not the display string).

// settingLineOffset estimates the rendered line offset for a setting at index idx,
// accounting for section headers (2 lines each: blank + header) and 1 line per setting.
// This keeps configScroll in sync with the actual rendered line count.
func settingLineOffset(settings []configSetting, idx int) int {
	line := 0
	currentSection := ""
	for i := 0; i <= idx && i < len(settings); i++ {
		if settings[i].section != "" && settings[i].section != currentSection {
			currentSection = settings[i].section
			line += 2 // blank line + section header
		}
		if i < idx {
			line++ // setting line
		}
	}
	return line
}

func (s configSetting) rawValue(cfg *config.Config) string {
	switch s.typ {
	case settingString:
		return s.getString(cfg)
	case settingInt:
		v := s.getInt(cfg)
		if v == 0 {
			return ""
		}
		return strconv.Itoa(v)
	case settingDuration:
		return s.getDuration(cfg).String()
	default:
		return ""
	}
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
				switch s.typ {
				case settingString:
					s.setString(m.cfg, val) // empty = default
				case settingInt:
					if val == "" {
						s.setInt(m.cfg, 0) // 0 = auto
					} else if v, err := strconv.Atoi(val); err == nil {
						s.setInt(m.cfg, v)
					} else {
						m.statusMsg = "✗ Invalid number: " + val
						return m, nil
					}
				case settingDuration:
					if v, err := time.ParseDuration(val); err == nil {
						s.setDuration(m.cfg, v)
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
			for m.configCursor > 0 && settings[m.configCursor].label == "" {
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
			switch s.typ {
			case settingBool:
				// Toggle.
				s.setBool(m.cfg, !s.getBool(m.cfg))
				m.statusMsg = "Modified (press S to save)"
			case settingChoice:
				// Cycle through choices. Fall back to first if current unknown.
				current := s.getString(m.cfg)
				found := false
				for i, c := range s.choices {
					if c == current {
						s.setString(m.cfg, s.choices[(i+1)%len(s.choices)])
						found = true
						break
					}
				}
				if !found && len(s.choices) > 0 {
					s.setString(m.cfg, s.choices[0])
				}
				m.statusMsg = "Modified (press S to save)"
			case settingString, settingInt, settingDuration:
				// Enter edit mode with raw value.
				m.configEditing = true
				m.configEditInput.SetValue(s.rawValue(m.cfg))
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
		if s.section != "" && s.section != currentSection {
			currentSection = s.section
			b.WriteString("\n  " + dashSection.Render(currentSection) + "\n")
		}

		cursor := "  "
		if i == m.configCursor {
			cursor = "▸ "
		}

		label := dashLabel.Render(fixedWidth(s.label, 20))
		var value string

		if m.cfg != nil {
			val := s.currentValue(m.cfg)
			switch s.typ {
			case settingBool:
				if s.getBool(m.cfg) {
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
