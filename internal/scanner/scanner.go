package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/registry"
)

// ScanPATH walks every directory in the system PATH, collects unique
// executable basenames (first occurrence wins, matching shell precedence),
// resolves symlinks, and applies the config filter.
//
// Returns tools sorted alphabetically by Name, with Path populated
// and Version empty (to be filled in later by the detector).
func ScanPATH(cfg config.Config) ([]registry.Tool, error) {
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return nil, nil
	}

	dirs := strings.Split(pathEnv, string(os.PathListSeparator))
	seen := make(map[string]bool) // lowercase name → already added
	var tools []registry.Tool

	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}

		// Skip OS system directories unless explicitly enabled.
		if !cfg.ScanSystemDirs && isSystemDir(dir) {
			continue
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			// Skip unreadable / non-existent directories silently.
			continue
		}

		for _, entry := range entries {
			// Skip directories and irregular files.
			if entry.IsDir() {
				continue
			}

			fullPath := filepath.Join(dir, entry.Name())

			// Resolve symlinks to get the actual file info.
			resolved, err := filepath.EvalSymlinks(fullPath)
			if err != nil {
				// Broken symlink — skip.
				continue
			}

			info, err := os.Stat(resolved)
			if err != nil {
				continue
			}

			// Must be a regular file.
			if !info.Mode().IsRegular() {
				continue
			}

			// Must be executable on this platform.
			if !isExecutable(entry.Name(), info) {
				continue
			}

			// Normalize the name (strip PATHEXT suffix on Windows).
			name := normalizeName(entry.Name())

			// Deduplicate: first occurrence in PATH wins.
			lowerName := strings.ToLower(name)
			if seen[lowerName] {
				continue
			}

			// Apply include/exclude filter.
			if !cfg.ApplyFilter(name) {
				seen[lowerName] = true // still mark as seen to avoid re-checking
				continue
			}

			seen[lowerName] = true
			tools = append(tools, registry.Tool{
				Name: name,
				Path: resolved,
			})
		}
	}

	// Sort alphabetically by name (case-insensitive).
	sort.Slice(tools, func(i, j int) bool {
		return strings.ToLower(tools[i].Name) < strings.ToLower(tools[j].Name)
	})

	return tools, nil
}
