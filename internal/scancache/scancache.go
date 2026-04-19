// Package scancache persists the result of a full tool scan (PATH discovery
// plus package-manager version resolution) to a YAML file on disk. The TUI
// and CLI load this cache on startup so users don't pay the cost of running
// dozens of subprocess queries (winget/brew/npm/etc.) every time clim runs.
//
// The catalog itself is still loaded via the catalog package — this cache
// only stores the dynamic, per-host data: which tools are installed, where
// their binaries live, their installed versions, and their latest versions
// as reported by package managers.
//
// The cache is invalidated explicitly by the user (TUI "r" key, CLI
// --refresh flag) and automatically after any mutating action (install,
// upgrade, remove).
package scancache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/registry"
)

// cacheVersion is bumped whenever the on-disk schema changes so older
// caches written by prior versions are ignored instead of mis-parsed.
const cacheVersion = 1

// Entry captures the dynamic scan data for a single tool: install
// locations and both the installed and latest versions.
type Entry struct {
	Instances  []instanceYAML `yaml:"instances,omitempty"`
	Latest     string         `yaml:"latest,omitempty"`
	LatestFrom string         `yaml:"latest_from,omitempty"`
}

type instanceYAML struct {
	Path    string `yaml:"path"`
	Version string `yaml:"version,omitempty"`
	Source  string `yaml:"source,omitempty"`
}

// file is the on-disk representation.
type file struct {
	Version   int              `yaml:"version"`
	SavedAt   time.Time        `yaml:"saved_at"`
	ToolCount int              `yaml:"tool_count"`
	Tools     map[string]Entry `yaml:"tools"`
}

// Path returns the absolute path to the scan cache file.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "clim", "scan-cache.yaml"), nil
}

// Exists reports whether the cache file is present on disk.
func Exists() bool {
	path, err := Path()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// Save writes the given tools to the cache file. Only fields produced by
// scanning (Instances, Latest, LatestFrom) are persisted — the catalog
// portion is re-loaded from the marketplace cache on each run.
func Save(tools []registry.Tool) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr != nil {
		return fmt.Errorf("creating cache dir: %w", mkErr)
	}

	entries := make(map[string]Entry, len(tools))
	for _, t := range tools {
		// Skip tools with no scan data to keep the cache compact. A missing
		// entry on Apply() means "not installed, no latest known", which is
		// the correct state for these tools.
		if len(t.Instances) == 0 && t.Latest == "" && t.LatestFrom == "" {
			continue
		}
		e := Entry{
			Latest:     t.Latest,
			LatestFrom: t.LatestFrom,
		}
		for _, inst := range t.Instances {
			e.Instances = append(e.Instances, instanceYAML{
				Path:    inst.Path,
				Version: inst.Version,
				Source:  string(inst.Source),
			})
		}
		entries[t.Name] = e
	}

	f := file{
		Version:   cacheVersion,
		SavedAt:   time.Now().UTC(),
		ToolCount: len(tools),
		Tools:     entries,
	}

	data, err := yaml.Marshal(&f)
	if err != nil {
		return fmt.Errorf("marshalling cache: %w", err)
	}

	return atomicWrite(path, data, 0o644)
}

// Load reads the cache file and returns a map keyed by tool name along
// with the time the cache was written. Returns os.ErrNotExist when no
// cache is present, and a regular error if the file exists but is
// unreadable or has an incompatible schema.
func Load() (map[string]Entry, time.Time, error) {
	path, err := Path()
	if err != nil {
		return nil, time.Time{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	var f file
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, time.Time{}, fmt.Errorf("parsing cache: %w", err)
	}
	if f.Version != cacheVersion {
		return nil, time.Time{}, fmt.Errorf("cache schema version %d unsupported (want %d)", f.Version, cacheVersion)
	}
	return f.Tools, f.SavedAt, nil
}

// Apply overlays cached scan data onto the catalog tools. Tools not present
// in the cache are left untouched (they will look "not installed"), which
// is the correct behaviour for newly-added catalog entries.
func Apply(tools []registry.Tool, entries map[string]Entry) []registry.Tool {
	for i := range tools {
		entry, ok := entries[tools[i].Name]
		if !ok {
			continue
		}
		tools[i].Latest = entry.Latest
		tools[i].LatestFrom = entry.LatestFrom
		if len(entry.Instances) == 0 {
			tools[i].Instances = nil
			continue
		}
		insts := make([]registry.Instance, 0, len(entry.Instances))
		for _, e := range entry.Instances {
			insts = append(insts, registry.Instance{
				Path:    e.Path,
				Version: e.Version,
				Source:  registry.InstallSource(e.Source),
			})
		}
		tools[i].Instances = insts
	}
	return tools
}

// Delete removes the cache file. A missing file is not an error.
func Delete() error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// atomicWrite writes data to a temp file in the same directory then renames
// it over the target, matching the catalog package's approach.
func atomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		// Windows: Rename fails if destination exists. Remove and retry.
		if removeErr := os.Remove(path); removeErr == nil {
			if retryErr := os.Rename(tmpPath, path); retryErr == nil {
				return nil
			}
		}
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
