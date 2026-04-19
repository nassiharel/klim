package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nassiharel/clim/internal/config"
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

// --- Key handling ---

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
					s.setString(m.cfg, val)
				case settingInt:
					if v, err := strconv.Atoi(val); err == nil {
						s.setInt(m.cfg, v)
					} else {
						m.statusMsg = fmt.Sprintf("✗ Invalid number: %s", val)
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
		return m, nil
	case "left", "shift+tab":
		m.activeTab = (m.activeTab + tabCount - 1) % tabCount
		m.cursor = 0
		m.dashboardScroll = 0
		m.discoverSubTab = discoverTools
		m.applyFilter()
		return m, nil
	case "1":
		m.activeTab = tabInstalled
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "2":
		m.activeTab = tabUpdates
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "3":
		m.activeTab = tabDiscover
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "4":
		m.activeTab = tabBackup
		m.cursor = 0
		return m, nil
	case "5":
		m.activeTab = tabDashboard
		m.cursor = 0
		m.dashboardScroll = 0
		m.myBackupFiles = scanBackupsDir()
		return m, nil
	case "6":
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
		}
		// Auto-scroll: keep cursor visible.
		if m.configCursor < m.configScroll {
			m.configScroll = m.configCursor
		}
	case "down", "j":
		if m.configCursor < len(settings)-1 {
			m.configCursor++
		}
		// Auto-scroll: keep cursor visible (estimate ~15 visible setting rows).
		visibleSettings := m.height/2 - 4
		if visibleSettings < 5 {
			visibleSettings = 5
		}
		if m.configCursor >= m.configScroll+visibleSettings {
			m.configScroll = m.configCursor - visibleSettings + 1
		}
	case "enter", "space":
		if m.configCursor < len(settings) && m.cfg != nil {
			s := settings[m.configCursor]
			switch s.typ {
			case settingBool:
				// Toggle.
				s.setBool(m.cfg, !s.getBool(m.cfg))
				m.statusMsg = "Modified (press S to save)"
			case settingChoice:
				// Cycle through choices.
				current := s.getString(m.cfg)
				for i, c := range s.choices {
					if c == current {
						next := s.choices[(i+1)%len(s.choices)]
						s.setString(m.cfg, next)
						break
					}
				}
				m.statusMsg = "Modified (press S to save)"
			case settingString, settingInt, settingDuration:
				// Enter edit mode.
				m.configEditing = true
				m.configEditInput.SetValue(s.currentValue(m.cfg))
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
