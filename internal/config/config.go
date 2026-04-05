package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Config holds user preferences for clim, persisted to disk as JSON.
type Config struct {
	// Include, if non-empty, is an allowlist — only binaries whose names
	// appear here are shown. When set, Exclude is ignored.
	Include []string `json:"include,omitempty"`

	// Exclude is a denylist — binaries whose names appear here are hidden.
	// Ignored when Include is non-empty.
	Exclude []string `json:"exclude,omitempty"`

	// VersionTimeout is the maximum number of seconds to wait for a
	// binary's --version output. Defaults to 5 if zero or unset.
	VersionTimeout int `json:"version_timeout,omitempty"`

	// ScanSystemDirs, when true, includes OS system directories in the scan
	// (e.g., C:\Windows\System32 on Windows, /sbin on Linux).
	// By default (false), system directories are skipped to show only
	// developer-relevant tools.
	ScanSystemDirs bool `json:"scan_system_dirs,omitempty"`
}

// DefaultPath returns the platform-appropriate config file path,
// typically ~/.config/clim/config.json on Linux/macOS or
// %APPDATA%/clim/config.json on Windows.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "clim", "config.json"), nil
}

// Load reads config from the given path. If the file does not exist,
// it returns a zero-value Config (no error).
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Config{}, nil
		}
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// Save writes the config to the given path, creating parent directories
// as needed. The JSON is indented for human readability.
func Save(path string, cfg Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// ApplyFilter returns true if the given binary name should be included
// in the output. If Include is non-empty, only names in Include pass.
// Otherwise, names in Exclude are rejected. If both are empty, everything passes.
func (c Config) ApplyFilter(name string) bool {
	if len(c.Include) > 0 {
		return containsIgnoreCase(c.Include, name)
	}
	if len(c.Exclude) > 0 {
		return !containsIgnoreCase(c.Exclude, name)
	}
	return true
}

// EffectiveTimeout returns the version detection timeout in seconds,
// defaulting to 5 if not explicitly configured.
func (c Config) EffectiveTimeout() int {
	if c.VersionTimeout > 0 {
		return c.VersionTimeout
	}
	return 5
}

// containsIgnoreCase checks if the slice contains the value (case-insensitive).
func containsIgnoreCase(slice []string, value string) bool {
	lower := strings.ToLower(value)
	return slices.ContainsFunc(slice, func(s string) bool {
		return strings.ToLower(s) == lower
	})
}
