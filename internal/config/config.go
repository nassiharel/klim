// Package config manages the clim configuration file (config.yaml).
// All values have sensible defaults — the config file is optional.
package config

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/fileutil"
	"github.com/nassiharel/clim/internal/paths"
)

// DefaultMarketplaceURL is the canonical marketplace.yaml location on GitHub.
// The marketplace branch is auto-published by CI from individual files in
// marketplace/tools/ and marketplace/packs/ on the main branch.
const DefaultMarketplaceURL = "https://raw.githubusercontent.com/nassiharel/clim/marketplace/marketplace.yaml"

// Config holds all clim configuration.
type Config struct {
	Logging     LoggingConfig     `yaml:"logging"`
	Marketplace MarketplaceConfig `yaml:"marketplace"`
	Performance PerformanceConfig `yaml:"performance"`
	UI          UIConfig          `yaml:"ui"`
}

// LoggingConfig controls log output.
type LoggingConfig struct {
	Level   string `yaml:"level"`   // debug, info, warn, error; default: debug
	File    bool   `yaml:"file"`    // write to ~/.config/clim/clim.log; default: true
	Verbose bool   `yaml:"verbose"` // also log to stderr; default: false
}

// MarketplaceConfig controls marketplace catalog behavior.
type MarketplaceConfig struct {
	URL             string   `yaml:"url"`
	AutoRefresh     bool     `yaml:"auto_refresh"`
	RefreshInterval Duration `yaml:"refresh_interval"`
}

// PerformanceConfig tunes concurrency and timeouts.
type PerformanceConfig struct {
	Concurrency    int      `yaml:"concurrency"`
	CommandTimeout Duration `yaml:"command_timeout"`
}

// UIConfig controls user interface preferences.
type UIConfig struct {
	DefaultTab   string `yaml:"default_tab"`
	ShowPath     bool   `yaml:"show_path"`
	SidebarRight bool   `yaml:"sidebar_right"` // true = filter sidebar on right side
}

// Duration wraps time.Duration for YAML marshaling as a human-readable string
// (e.g. "10s", "24h") instead of nanoseconds.
type Duration struct {
	time.Duration
}

// MarshalYAML encodes a Duration as a string like "10s" or "24h".
func (d Duration) MarshalYAML() (interface{}, error) {
	return d.String(), nil
}

// UnmarshalYAML decodes a Duration from a string like "10s" or "24h".
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}
	d.Duration = parsed
	return nil
}

// Default returns a Config with all defaults populated.
func Default() *Config {
	return &Config{
		Logging: LoggingConfig{
			Level:   "debug",
			File:    true,
			Verbose: false,
		},
		Marketplace: MarketplaceConfig{
			URL:             DefaultMarketplaceURL,
			AutoRefresh:     false,
			RefreshInterval: Duration{24 * time.Hour},
		},
		Performance: PerformanceConfig{
			Concurrency:    0, // 0 = auto (runtime.NumCPU)
			CommandTimeout: Duration{30 * time.Second},
		},
		UI: UIConfig{
			DefaultTab: "installed",
			ShowPath:   true,
		},
	}
}

// Path returns the path to config.yaml.
func Path() (string, error) {
	return paths.Config()
}

// Load reads config.yaml. If the file doesn't exist, it writes a default
// config and returns the defaults. Returns an error only if the file exists
// but is unreadable or has invalid YAML. Warnings (e.g. unknown fields) are
// returned separately and do not prevent loading.
func Load() (*Config, error) {
	cfg, _, err := LoadWithWarnings()
	return cfg, err
}

// LoadWithWarnings reads config.yaml and returns the config plus any warnings
// about unknown/misspelled fields. The config is still usable even when
// warnings are present — unknown fields are simply ignored.
func LoadWithWarnings() (*Config, []string, error) {
	path, err := paths.Config()
	if err != nil {
		return Default(), nil, nil
	}

	// Start from defaults, then overlay the file values.
	cfg := Default()
	found, err := fileutil.ReadYAML(path, cfg)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		// First run — write defaults so user can discover the file.
		_ = Save(cfg)
		return cfg, nil, nil
	}

	// Check for unknown fields via strict decode.
	var warnings []string
	data, readErr := os.ReadFile(path)
	if readErr == nil {
		warnings = detectUnknownFields(data)
	}

	// Validate known field values.
	warnings = append(warnings, cfg.Validate()...)

	return cfg, warnings, nil
}

// detectUnknownFields attempts a strict YAML decode that rejects unknown keys.
// Returns human-readable warnings for each unknown field found.
func detectUnknownFields(data []byte) []string {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var strict Config
	if err := dec.Decode(&strict); err != nil {
		// Extract the useful part of the error message.
		msg := err.Error()
		// yaml.v3 errors for unknown fields look like:
		// "line N: field foo not found in type config.Config"
		if strings.Contains(msg, "not found in type") {
			return []string{"config.yaml: " + msg}
		}
		// Other strict errors — report as-is.
		return []string{"config.yaml: " + msg}
	}
	return nil
}

// Validate checks field values and returns warnings for invalid/suspicious values.
func (c *Config) Validate() []string {
	var w []string

	// Logging level.
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
		// ok
	default:
		w = append(w, fmt.Sprintf("logging.level: unknown value %q (expected debug/info/warn/error)", c.Logging.Level))
	}

	// UI default tab.
	validTabs := map[string]bool{
		"installed": true, "favorites": true, "updates": true,
		"marketplace": true, "backup": true, "dashboard": true, "config": true,
	}
	if c.UI.DefaultTab != "" && !validTabs[c.UI.DefaultTab] {
		w = append(w, fmt.Sprintf("ui.default_tab: unknown value %q", c.UI.DefaultTab))
	}

	// Performance.
	if c.Performance.Concurrency < 0 {
		w = append(w, fmt.Sprintf("performance.concurrency: negative value %d", c.Performance.Concurrency))
	}
	if c.Performance.CommandTimeout.Duration < 0 {
		w = append(w, "performance.command_timeout: negative duration")
	}

	// Marketplace URL.
	if c.Marketplace.URL != "" && !strings.HasPrefix(c.Marketplace.URL, "http") {
		w = append(w, fmt.Sprintf("marketplace.url: %q doesn't look like a URL", c.Marketplace.URL))
	}

	return w
}

// MustLoad calls Load and panics on error (corrupt YAML).
// Missing file is not an error — defaults are returned silently.
func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		panic(fmt.Sprintf("clim: %v", err))
	}
	return cfg
}

const configHeader = "# clim — Configuration\n# All values are optional. Defaults are shown below.\n# Restart clim after editing for changes to take effect.\n\n"

// Save writes the config to config.yaml atomically.
func Save(cfg *Config) error {
	path, err := paths.Config()
	if err != nil {
		return err
	}
	return fileutil.WriteYAML(path, cfg, configHeader)
}
