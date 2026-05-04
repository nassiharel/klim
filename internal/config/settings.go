package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SettingType describes what kind of value a Setting holds. The TUI
// and the web UI use this to pick the right editor (checkbox, text
// input, dropdown, etc.) and to validate input.
type SettingType int

const (
	// SettingBool — true/false. Editor: checkbox.
	SettingBool SettingType = iota
	// SettingString — free-text. Editor: text input.
	SettingString
	// SettingInt — non-negative integer; 0 means "auto / default".
	SettingInt
	// SettingDuration — time.Duration parsed via time.ParseDuration
	// (so values like "30s", "24h" work).
	SettingDuration
	// SettingChoice — string with a fixed Choices list. Editor: select.
	SettingChoice
)

// Setting is one editable config field. The Get/Set closures make
// each field self-describing — adding a new field is a one-place
// change here, and every UI surface picks it up automatically.
//
// Only one Get/Set pair is populated per Setting (the one matching
// Type). Helper methods Display/Raw/SetFromString hide the dispatch.
type Setting struct {
	Section string // section header (Logging, Marketplace, etc.). Empty = continue previous.
	Label   string // user-facing label.
	Key     string // stable form/JSON key, slug-style (lowercase + underscores).
	Type    SettingType
	Help    string   // short hint shown next to the input.
	Choices []string // for SettingChoice.

	GetBool     func(*Config) bool
	SetBool     func(*Config, bool)
	GetString   func(*Config) string
	SetString   func(*Config, string)
	GetInt      func(*Config) int
	SetInt      func(*Config, int)
	GetDuration func(*Config) time.Duration
	SetDuration func(*Config, time.Duration)
}

// Display returns the user-facing rendering of the current value.
// Strings render as "(default)" when empty, ints as "auto" when 0 —
// the same conventions the TUI's config editor used to bake in
// per-render.
func (s Setting) Display(cfg *Config) string {
	switch s.Type {
	case SettingBool:
		if s.GetBool(cfg) {
			return "true"
		}
		return "false"
	case SettingString:
		v := s.GetString(cfg)
		if v == "" {
			return "(default)"
		}
		return v
	case SettingInt:
		v := s.GetInt(cfg)
		if v == 0 {
			return "auto"
		}
		return strconv.Itoa(v)
	case SettingDuration:
		return s.GetDuration(cfg).String()
	case SettingChoice:
		return s.GetString(cfg)
	}
	return ""
}

// Raw returns the underlying value as a string suitable for editing
// (what to pre-populate an input with). Booleans return "true"/"false";
// empty strings and zero ints return "" so placeholders show through.
func (s Setting) Raw(cfg *Config) string {
	switch s.Type {
	case SettingBool:
		if s.GetBool(cfg) {
			return "true"
		}
		return "false"
	case SettingString, SettingChoice:
		return s.GetString(cfg)
	case SettingInt:
		v := s.GetInt(cfg)
		if v == 0 {
			return ""
		}
		return strconv.Itoa(v)
	case SettingDuration:
		return s.GetDuration(cfg).String()
	}
	return ""
}

// SetFromString parses raw and applies it to cfg. Returns a typed
// error on bad input so the UI can show "invalid duration" etc.
//
// For SettingBool, accepts "true"/"false"/"on"/"off"/"1"/"0" and
// the empty string (which means "false" — HTML checkboxes omit the
// value entirely when unchecked, so the default has to be false).
func (s Setting) SetFromString(cfg *Config, raw string) error {
	raw = strings.TrimSpace(raw)
	switch s.Type {
	case SettingBool:
		v, err := parseBool(raw)
		if err != nil {
			return err
		}
		s.SetBool(cfg, v)
	case SettingString:
		s.SetString(cfg, raw) // empty = default; same as TUI
	case SettingInt:
		if raw == "" {
			s.SetInt(cfg, 0) // 0 = auto
			return nil
		}
		v, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("%s: not an integer: %q", s.Label, raw)
		}
		if v < 0 {
			return fmt.Errorf("%s: must be >= 0", s.Label)
		}
		s.SetInt(cfg, v)
	case SettingDuration:
		v, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("%s: invalid duration %q (use 30s, 24h, etc.)", s.Label, raw)
		}
		s.SetDuration(cfg, v)
	case SettingChoice:
		valid := false
		for _, c := range s.Choices {
			if c == raw {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("%s: %q is not one of %v", s.Label, raw, s.Choices)
		}
		s.SetString(cfg, raw)
	default:
		return fmt.Errorf("%s: unsupported setting type", s.Label)
	}
	return nil
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(s) {
	case "", "false", "off", "0", "no":
		return false, nil
	case "true", "on", "1", "yes":
		return true, nil
	}
	return false, fmt.Errorf("not a boolean: %q", s)
}

// AllSettings returns every editable config field, in the order they
// should appear in any UI. Adding a field requires updating this list
// and the corresponding Config struct field; nothing else changes —
// both the TUI editor and the web /config page pick it up
// automatically.
func AllSettings() []Setting {
	return []Setting{
		// --- Logging ---
		{
			Section: "Logging", Label: "Log Level", Key: "log_level", Type: SettingChoice,
			Help:      "Verbosity for the on-disk log file.",
			Choices:   []string{"debug", "info", "warn", "error"},
			GetString: func(c *Config) string { return c.Logging.Level },
			SetString: func(c *Config, v string) { c.Logging.Level = v },
		},
		{
			Label: "Log to File", Key: "log_file", Type: SettingBool,
			Help:    "Write structured logs to ~/.config/clim/logs/clim.log.",
			GetBool: func(c *Config) bool { return c.Logging.File },
			SetBool: func(c *Config, v bool) { c.Logging.File = v },
		},
		{
			Label: "Verbose Logging", Key: "log_verbose", Type: SettingBool,
			Help:    "Also stream logs to stderr (loud — use --verbose for one-off invocations instead).",
			GetBool: func(c *Config) bool { return c.Logging.Verbose },
			SetBool: func(c *Config, v bool) { c.Logging.Verbose = v },
		},
		// --- Marketplace ---
		{
			Section: "Marketplace", Label: "Catalog URL", Key: "marketplace_url", Type: SettingString,
			Help:      "Where to fetch marketplace.yaml. Leave blank for the default published catalog.",
			GetString: func(c *Config) string { return c.Marketplace.URL },
			SetString: func(c *Config, v string) { c.Marketplace.URL = v },
		},
		{
			Label: "Auto Refresh", Key: "marketplace_auto_refresh", Type: SettingBool,
			Help:    "Re-fetch the catalog on each invocation if older than the refresh interval.",
			GetBool: func(c *Config) bool { return c.Marketplace.AutoRefresh },
			SetBool: func(c *Config, v bool) { c.Marketplace.AutoRefresh = v },
		},
		{
			Label: "Refresh Interval", Key: "marketplace_refresh_interval", Type: SettingDuration,
			Help:        "Used when Auto Refresh is on. Examples: 1h, 24h.",
			GetDuration: func(c *Config) time.Duration { return c.Marketplace.RefreshInterval.Duration },
			SetDuration: func(c *Config, v time.Duration) { c.Marketplace.RefreshInterval = Duration{Duration: v} },
		},
		// --- Performance ---
		{
			Section: "Performance", Label: "Concurrency", Key: "performance_concurrency", Type: SettingInt,
			Help:   "Maximum concurrent package-manager queries. 0 = auto-pick based on CPU count.",
			GetInt: func(c *Config) int { return c.Performance.Concurrency },
			SetInt: func(c *Config, v int) { c.Performance.Concurrency = v },
		},
		{
			Label: "Command Timeout", Key: "performance_command_timeout", Type: SettingDuration,
			Help:        "How long any single package-manager subprocess can run before clim gives up.",
			GetDuration: func(c *Config) time.Duration { return c.Performance.CommandTimeout.Duration },
			SetDuration: func(c *Config, v time.Duration) { c.Performance.CommandTimeout = Duration{Duration: v} },
		},
		// --- UI ---
		{
			Section: "UI", Label: "Default Tab", Key: "ui_default_tab", Type: SettingChoice,
			Help:      "Which tab the TUI opens on launch.",
			Choices:   []string{"installed", "updates", "marketplace", "backup", "dashboard", "config"},
			GetString: func(c *Config) string { return c.UI.DefaultTab },
			SetString: func(c *Config, v string) { c.UI.DefaultTab = v },
		},
		{
			Label: "Show Path", Key: "ui_show_path", Type: SettingBool,
			Help:    "Display each tool's filesystem path next to its name.",
			GetBool: func(c *Config) bool { return c.UI.ShowPath },
			SetBool: func(c *Config, v bool) { c.UI.ShowPath = v },
		},
		{
			Label: "Sidebar Right", Key: "ui_sidebar_right", Type: SettingBool,
			Help:    "Render the filter sidebar on the right (default: left).",
			GetBool: func(c *Config) bool { return c.UI.SidebarRight },
			SetBool: func(c *Config, v bool) { c.UI.SidebarRight = v },
		},
		// --- Defaults (consumed by clim install / upgrade / remove) ---
		{
			Section: "Defaults", Label: "Preferred Source", Key: "defaults_preferred_source", Type: SettingChoice,
			Help:      "Package manager that clim install / upgrade / remove prefers when available. Empty = OS-priority fallback.",
			Choices:   []string{"", "winget", "choco", "scoop", "brew", "apt", "snap", "npm"},
			GetString: func(c *Config) string { return c.Defaults.PreferredSource },
			SetString: func(c *Config, v string) { c.Defaults.PreferredSource = v },
		},
	}
}

// SettingByKey looks up a Setting by its stable Key. Used by the
// web UI when applying form-submitted values, since form keys arrive
// in arbitrary order and may include extras (e.g. a CSRF token).
func SettingByKey(key string) (Setting, bool) {
	for _, s := range AllSettings() {
		if s.Key == key {
			return s, true
		}
	}
	return Setting{}, false
}
