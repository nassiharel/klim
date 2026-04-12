// Package config manages the clim configuration file (config.yaml).
// All values have sensible defaults — the config file is optional.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all clim configuration.
type Config struct {
	GitHub      GitHubConfig      `yaml:"github"`
	Marketplace MarketplaceConfig `yaml:"marketplace"`
	Performance PerformanceConfig `yaml:"performance"`
	UI          UIConfig          `yaml:"ui"`
}

// GitHubConfig configures the GitHub repository used for self-update
// and marketplace catalog fetching.
type GitHubConfig struct {
	Owner  string `yaml:"owner"`
	Repo   string `yaml:"repo"`
	Branch string `yaml:"branch"`
}

// MarketplaceConfig controls marketplace catalog behavior.
type MarketplaceConfig struct {
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
	DefaultTab string `yaml:"default_tab"`
	ShowPath   bool   `yaml:"show_path"`
}

// Duration wraps time.Duration for YAML marshaling as a human-readable string
// (e.g. "10s", "24h") instead of nanoseconds.
type Duration struct {
	time.Duration
}

// MarshalYAML encodes a Duration as a string like "10s" or "24h".
func (d Duration) MarshalYAML() (interface{}, error) {
	return d.Duration.String(), nil
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
		GitHub: GitHubConfig{
			Owner:  "nassiharel",
			Repo:   "clim",
			Branch: "main",
		},
		Marketplace: MarketplaceConfig{
			AutoRefresh:     false,
			RefreshInterval: Duration{24 * time.Hour},
		},
		Performance: PerformanceConfig{
			Concurrency:    0, // 0 = auto (runtime.NumCPU)
			CommandTimeout: Duration{10 * time.Second},
		},
		UI: UIConfig{
			DefaultTab: "installed",
			ShowPath:   true,
		},
	}
}

// Path returns the path to config.yaml.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "clim", "config.yaml"), nil
}

// Load reads config.yaml. If the file doesn't exist, it writes a default
// config and returns the defaults. Returns an error only if the file exists
// but is unreadable or has invalid YAML.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return Default(), nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// First run — write defaults so user can discover the file.
			cfg := Default()
			_ = writeDefault(path, cfg)
			return cfg, nil
		}
		return Default(), nil // unreadable — fall back to defaults
	}

	// Start from defaults, then overlay the file values.
	cfg := Default()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config.yaml: %w", err)
	}

	return cfg, nil
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

// defaultConfigTemplate is the commented YAML written on first run.
const defaultConfigTemplate = `# clim — Configuration
# All values are optional. Defaults are shown below.
# Restart clim after editing for changes to take effect.

# GitHub repository for self-update and marketplace catalog.
github:
  owner: nassiharel
  repo: clim
  branch: main

# Marketplace settings.
marketplace:
  auto_refresh: false    # auto-fetch latest catalog on startup
  refresh_interval: 24h  # minimum time between auto-refreshes

# Performance tuning.
performance:
  concurrency: 0         # max parallel version checks (0 = auto)
  command_timeout: 10s   # timeout for package manager subprocess calls

# UI preferences.
ui:
  default_tab: installed # startup tab: installed, updates, marketplace, disabled, backup, config
  show_path: true        # show PATH column in list output
`

func writeDefault(path string, _ *Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(defaultConfigTemplate), 0o644)
}
