package finder

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/nassiharel/clim/internal/registry"
)

// Cached PATHEXT extensions (Windows), computed once.
var cachedPathExts struct {
	once sync.Once
	exts []string
}

// FindAll locates all installations of each curated tool across PATH.
func FindAll(tools []registry.Tool) {
	pathDirs := pathDirectories()

	for i := range tools {
		tools[i].Instances = findInstances(&tools[i], pathDirs)
	}
}

func findInstances(tool *registry.Tool, pathDirs []string) []registry.Instance {
	seen := make(map[string]bool)
	var instances []registry.Instance

	for _, dir := range pathDirs {
		for _, binName := range tool.BinaryNames {
			for _, fullPath := range binaryCandidates(dir, binName) {
				resolved, err := filepath.EvalSymlinks(fullPath)
				if err != nil {
					continue
				}
				info, err := os.Stat(resolved)
				if err != nil || info.IsDir() {
					continue
				}
				if seen[resolved] {
					continue
				}
				seen[resolved] = true

				instances = append(instances, registry.Instance{
					Path:   resolved,
					Source: detectSource(resolved),
				})
			}
		}
	}

	// Fallback: exec.LookPath for edge cases (e.g. Windows App Execution Aliases).
	if len(instances) == 0 {
		for _, binName := range tool.BinaryNames {
			path, err := exec.LookPath(binName)
			if err != nil {
				continue
			}
			resolved, _ := filepath.EvalSymlinks(path)
			if resolved == "" {
				resolved = path
			}
			instances = append(instances, registry.Instance{
				Path:   resolved,
				Source: detectSource(resolved),
			})
			break
		}
	}

	return instances
}

// binaryCandidates returns candidate paths without stat-checking;
// the caller handles stat + symlink resolution.
func binaryCandidates(dir, name string) []string {
	if runtime.GOOS == "windows" {
		var candidates []string
		for _, ext := range getPathExtensions() {
			candidates = append(candidates, filepath.Join(dir, name+ext))
		}
		candidates = append(candidates, filepath.Join(dir, name))
		return candidates
	}
	return []string{filepath.Join(dir, name)}
}

func pathDirectories() []string {
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return nil
	}
	raw := strings.Split(pathEnv, string(os.PathListSeparator))
	dirs := make([]string, 0, len(raw))
	for _, d := range raw {
		if d = strings.TrimSpace(d); d != "" {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

func getPathExtensions() []string {
	cachedPathExts.once.Do(func() {
		env := os.Getenv("PATHEXT")
		if env == "" {
			cachedPathExts.exts = []string{".exe", ".cmd", ".bat", ".com"}
			return
		}
		parts := strings.Split(env, ";")
		exts := make([]string, 0, len(parts))
		for _, p := range parts {
			if p = strings.TrimSpace(p); p != "" {
				exts = append(exts, strings.ToLower(p))
			}
		}
		cachedPathExts.exts = exts
	})
	return cachedPathExts.exts
}

// detectSource infers the install source from the binary's resolved path.
// Uses forward slashes only (filepath.ToSlash) for consistent matching.
func detectSource(path string) registry.InstallSource {
	lower := strings.ToLower(filepath.ToSlash(path))

	switch {
	case strings.Contains(lower, "chocolatey/"):
		return registry.SourceChoco
	case strings.Contains(lower, "scoop/"):
		return registry.SourceScoop
	case strings.HasPrefix(lower, "/opt/homebrew/") ||
		strings.Contains(lower, "/homebrew/cellar/") ||
		strings.HasPrefix(lower, "/usr/local/cellar/"):
		return registry.SourceBrew
	case strings.HasPrefix(lower, "/snap/"):
		return registry.SourceSnap
	case strings.HasPrefix(lower, "/usr/bin/") || strings.HasPrefix(lower, "/usr/lib/"):
		return registry.SourceApt
	case strings.Contains(lower, "roaming/npm/") ||
		strings.Contains(lower, "node_modules/") ||
		strings.Contains(lower, "/lib/node_modules/"):
		return registry.SourceNPM
	case strings.Contains(lower, "/go/bin/"):
		return registry.SourceGo
	case strings.Contains(lower, ".cargo/bin/"):
		return registry.SourceCargo
	case strings.Contains(lower, ".local/bin/"):
		return registry.SourcePip

	// Windows: winget installs to many locations beyond Program Files.
	case strings.Contains(lower, "program files"):
		return registry.SourceWinget
	case strings.Contains(lower, "8wekyb3d8bbwe"):
		// WinGet source packages (e.g. fzf, bat, fd, helm via winget)
		return registry.SourceWinget
	case strings.Contains(lower, "microsoft/windowsapps/"):
		// Windows Store / App Execution Aliases (e.g. python)
		return registry.SourceWinget
	case strings.Contains(lower, "appdata/local/programs/"):
		// Per-user installs (VS Code, Azure Dev CLI, etc.)
		return registry.SourceWinget
	case strings.Contains(lower, "programdata/"):
		return registry.SourceWinget

	default:
		return registry.SourceManual
	}
}
