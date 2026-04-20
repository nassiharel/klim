// Package favorites manages the user's favorites list stored
// in ~/.config/clim/favorites/favorites.yaml. Each entry is a tool name.
package favorites

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// favFile is the on-disk YAML structure.
type favFile struct {
	Tools []string `yaml:"tools"`
}

// StoragePath returns the path to the favorites file.
func StoragePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "clim", "favorites", "favorites.yaml"), nil
}

// Load reads all favorite tool names from disk. Returns an empty (non-nil)
// slice if the file doesn't exist yet.
func Load() ([]string, error) {
	path, err := StoragePath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("reading favorites: %w", err)
	}

	var f favFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing favorites: %w", err)
	}

	if f.Tools == nil {
		return []string{}, nil
	}
	return f.Tools, nil
}

// Save writes favorite tool names to disk atomically.
func Save(names []string) error {
	path, err := StoragePath()
	if err != nil {
		return err
	}

	sorted := make([]string, len(names))
	copy(sorted, names)
	sort.Strings(sorted)

	f := favFile{Tools: sorted}
	data, err := yaml.Marshal(&f)
	if err != nil {
		return fmt.Errorf("marshalling favorites: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	header := "# clim — Favorites\n# Managed by clim; safe to edit manually.\n\n"

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(header+string(data)), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(path)
		if retryErr := os.Rename(tmp, path); retryErr != nil {
			_ = os.Remove(tmp)
			return retryErr
		}
	}
	return nil
}

// Add adds a tool name to favorites (no-op if already present).
func Add(name string) error {
	names, err := Load()
	if err != nil {
		return err
	}
	for _, n := range names {
		if n == name {
			return nil
		}
	}
	return Save(append(names, name))
}

// Remove removes a tool name from favorites (no-op if absent).
func Remove(name string) error {
	names, err := Load()
	if err != nil {
		return err
	}
	filtered := names[:0]
	for _, n := range names {
		if n != name {
			filtered = append(filtered, n)
		}
	}
	return Save(filtered)
}

// Contains checks whether a tool name is in the favorites list.
func Contains(name string) (bool, error) {
	names, err := Load()
	if err != nil {
		return false, err
	}
	for _, n := range names {
		if n == name {
			return true, nil
		}
	}
	return false, nil
}

// Toggle adds the name if absent, removes if present.
// Returns true if the name was added, false if removed.
func Toggle(name string) (bool, error) {
	names, err := Load()
	if err != nil {
		return false, err
	}

	for i, n := range names {
		if n == name {
			// Remove.
			names = append(names[:i], names[i+1:]...)
			return false, Save(names)
		}
	}

	// Add.
	return true, Save(append(names, name))
}

// Set builds a map[string]bool from the favorites list for quick lookups.
func Set() (map[string]bool, error) {
	names, err := Load()
	if err != nil {
		return nil, err
	}
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m, nil
}
