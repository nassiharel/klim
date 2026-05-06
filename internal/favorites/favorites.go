// Package favorites manages the user's favorites list stored
// in ~/.config/klim/favorites/favorites.yaml. Each entry is a tool name.
package favorites

import (
	"sort"

	"github.com/nassiharel/klim/internal/fileutil"
	"github.com/nassiharel/klim/internal/paths"
)

// favFile is the on-disk YAML structure.
type favFile struct {
	Tools []string `yaml:"tools"`
}

const yamlHeader = "# klim — Favorites\n# Managed by klim; safe to edit manually.\n\n"

// StoragePath returns the path to the favorites file.
func StoragePath() (string, error) {
	return paths.Favorites()
}

// Load reads all favorite tool names from disk. Returns an empty (non-nil)
// slice if the file doesn't exist yet.
func Load() ([]string, error) {
	path, err := paths.Favorites()
	if err != nil {
		return nil, err
	}

	var f favFile
	found, err := fileutil.ReadYAML(path, &f)
	if err != nil {
		return nil, err
	}
	if !found || f.Tools == nil {
		return []string{}, nil
	}
	return f.Tools, nil
}

// Save writes favorite tool names to disk atomically.
func Save(names []string) error {
	path, err := paths.Favorites()
	if err != nil {
		return err
	}

	sorted := make([]string, len(names))
	copy(sorted, names)
	sort.Strings(sorted)

	return fileutil.WriteYAML(path, &favFile{Tools: sorted}, yamlHeader)
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
