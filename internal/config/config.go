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
// Otherwise, names in both the user Exclude list and the built-in default
// exclude list are rejected.
func (c Config) ApplyFilter(name string) bool {
	if len(c.Include) > 0 {
		return containsIgnoreCase(c.Include, name)
	}
	if containsIgnoreCase(c.Exclude, name) {
		return false
	}
	if defaultExcluded(name) {
		return false
	}
	return true
}

// defaultExcluded returns true if the name matches a built-in exclude pattern.
// These are internal/helper binaries that developers rarely invoke directly.
// The list uses case-insensitive exact match and prefix match (for families
// like "docker-credential-*", "git-*" internals, "lens-cli-*").
func defaultExcluded(name string) bool {
	lower := strings.ToLower(name)

	// Exact matches — internal helpers, installers, support binaries.
	if defaultExcludeSet[lower] {
		return true
	}

	// Prefix matches — families of internal binaries.
	for _, prefix := range defaultExcludePrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}

	return false
}

// defaultExcludeSet contains binaries that are internal helpers, installers,
// or support tools — not directly invoked by developers.
//
// Organised by category for maintainability.
var defaultExcludeSet = func() map[string]bool {
	names := []string{
		// --- Runtime internals / crash handlers ---
		"createdump",

		// --- Installers / uninstallers ---
		"unins000", "install_tools",

		// --- Package manager internals ---
		"refreshenv", // chocolatey

		// --- Git internals (plumbing, not user-facing) ---
		"git-receive-pack", "git-upload-pack", "git-upload-archive",
		"git-gui", "gitk", "scalar",
		"start-ssh-agent", "start-ssh-pageant",

		// --- Docker internals ---
		"extension-admin", "hub-tool", "local-sandboxesd",
		"com.docker.cli",

		// --- .NET internals ---
		"dnx", "dnu", "createdump",

		// --- Node.js internals ---
		"nodevars", "install_tools",

		// --- SQL tools (DBA, not dev) ---
		"bcp", "sqlcmd", "sqllocaldb",

		// --- Misc support binaries ---
		"corepack",
	}

	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}()

// defaultExcludePrefixes hides families of internal binaries by prefix.
var defaultExcludePrefixes = []string{
	"docker-credential-", // docker-credential-desktop, docker-credential-ecr-login, ...
	"docker-index",       // docker-index
	"lens-cli-",          // lens-cli-windows-amd64, lens-cli-windows-arm64, ...
	"code-tunnel",        // code-tunnel, code-tunnel-insiders
}

// containsIgnoreCase checks if the slice contains the value (case-insensitive).
func containsIgnoreCase(slice []string, value string) bool {
	lower := strings.ToLower(value)
	return slices.ContainsFunc(slice, func(s string) bool {
		return strings.ToLower(s) == lower
	})
}
