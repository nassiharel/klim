package finder

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nassiharel/clim/internal/registry"
)

// FindAll locates all installations of each curated tool across PATH.
// For each tool, it finds every matching binary in PATH directories,
// resolves symlinks, and identifies the primary (first in PATH).
func FindAll(tools []registry.Tool) {
	pathDirs := pathDirectories()

	for i := range tools {
		tools[i].Instances = findInstances(&tools[i], pathDirs)
	}
}

// findInstances searches all PATH directories for a tool's binary names.
// Returns all found instances; the first is marked as primary.
func findInstances(tool *registry.Tool, pathDirs []string) []registry.Instance {
	seen := make(map[string]bool) // resolved path → already added
	var instances []registry.Instance

	for _, dir := range pathDirs {
		for _, binName := range tool.BinaryNames {
			candidates := binaryCandidates(dir, binName)
			for _, fullPath := range candidates {
				resolved, err := filepath.EvalSymlinks(fullPath)
				if err != nil {
					continue
				}

				info, err := os.Stat(resolved)
				if err != nil || info.IsDir() {
					continue
				}

				// Deduplicate by resolved path.
				if seen[resolved] {
					continue
				}
				seen[resolved] = true

				inst := registry.Instance{
					Path:      resolved,
					IsPrimary: len(instances) == 0,
					Source:    detectSource(resolved),
				}
				instances = append(instances, inst)
			}
		}
	}

	// If nothing found via manual PATH walk, try exec.LookPath as fallback
	// (handles edge cases like Windows App Execution Aliases).
	if len(instances) == 0 {
		for _, binName := range tool.BinaryNames {
			path, err := exec.LookPath(binName)
			if err != nil {
				continue
			}
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				resolved = path
			}
			instances = append(instances, registry.Instance{
				Path:      resolved,
				IsPrimary: true,
				Source:    detectSource(resolved),
			})
			break // first match wins
		}
	}

	return instances
}

// binaryCandidates returns possible full paths for a binary name in a directory.
// On Windows, appends PATHEXT extensions; on Unix, returns the name as-is.
func binaryCandidates(dir, name string) []string {
	if runtime.GOOS == "windows" {
		var candidates []string
		// Try with each PATHEXT extension.
		for _, ext := range pathExtensions() {
			candidate := filepath.Join(dir, name+ext)
			if _, err := os.Stat(candidate); err == nil {
				candidates = append(candidates, candidate)
			}
		}
		// Also try the exact name (might already have extension).
		exact := filepath.Join(dir, name)
		if _, err := os.Stat(exact); err == nil {
			candidates = append(candidates, exact)
		}
		return candidates
	}

	// Unix: just the name.
	candidate := filepath.Join(dir, name)
	if _, err := os.Stat(candidate); err == nil {
		return []string{candidate}
	}
	return nil
}

// pathDirectories returns all directories in PATH, in order.
func pathDirectories() []string {
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return nil
	}

	raw := strings.Split(pathEnv, string(os.PathListSeparator))
	var dirs []string
	for _, d := range raw {
		d = strings.TrimSpace(d)
		if d != "" {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// pathExtensions returns Windows PATHEXT extensions (lowercased).
func pathExtensions() []string {
	env := os.Getenv("PATHEXT")
	if env == "" {
		return []string{".exe", ".cmd", ".bat", ".com"}
	}
	parts := strings.Split(env, ";")
	exts := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			exts = append(exts, strings.ToLower(p))
		}
	}
	return exts
}

// detectSource infers the install source from the binary's file path.
func detectSource(path string) string {
	lower := strings.ToLower(filepath.ToSlash(path))

	// Windows package managers.
	if strings.Contains(lower, "chocolatey/") || strings.Contains(lower, "chocolatey\\") {
		return "choco"
	}
	if strings.Contains(lower, "scoop/") || strings.Contains(lower, "scoop\\") {
		return "scoop"
	}

	// macOS Homebrew.
	if strings.HasPrefix(lower, "/opt/homebrew/") || strings.Contains(lower, "/homebrew/cellar/") ||
		strings.HasPrefix(lower, "/usr/local/cellar/") {
		return "brew"
	}

	// Linux package managers.
	if strings.HasPrefix(lower, "/snap/") || strings.HasPrefix(lower, "/snap/bin/") {
		return "snap"
	}

	// Debian/Ubuntu system packages.
	if strings.HasPrefix(lower, "/usr/bin/") || strings.HasPrefix(lower, "/usr/lib/") {
		return "apt"
	}

	// npm global installs.
	if strings.Contains(lower, "roaming/npm/") || strings.Contains(lower, "node_modules/") ||
		strings.Contains(lower, "/lib/node_modules/") {
		return "npm"
	}

	// Go installs.
	if strings.Contains(lower, "/go/bin/") || strings.Contains(lower, "\\go\\bin\\") {
		return "go"
	}

	// Cargo/Rust installs.
	if strings.Contains(lower, ".cargo/bin/") || strings.Contains(lower, ".cargo\\bin\\") {
		return "cargo"
	}

	// pip / user-local installs.
	if strings.Contains(lower, ".local/bin/") || strings.Contains(lower, ".local\\bin\\") {
		return "pip"
	}

	// Windows Program Files — likely winget or MSI install.
	if strings.Contains(lower, "program files") || strings.Contains(lower, "programdata") {
		return "winget"
	}

	return "manual"
}
