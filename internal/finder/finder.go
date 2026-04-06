package finder

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/nassiharel/clim/internal/registry"
)

// Cached PATHEXT extensions (Windows), computed once.
var cachedPathExts struct {
	once sync.Once
	exts []string
}

// toolRef links a binary name to its tool index.
type toolRef struct {
	toolIdx int
	binName string
}

// match represents a found binary that matches a tool.
type match struct {
	toolIdx  int
	pathDir  string            // the original PATH directory (before symlink resolution)
	instance registry.Instance // Path is the resolved (EvalSymlinks) path
}

// FindAll locates all installations of each curated tool across PATH.
// It populates the Instances field of each non-disabled tool in-place.
// Returns an error if PATH is empty or not set.
func FindAll(tools []registry.Tool) error {
	pathDirs := pathDirectories()
	if len(pathDirs) == 0 {
		return errors.New("PATH is empty or not set")
	}

	// Phase 1: Build a map of all binary names we're looking for.
	// On Windows, file names are case-insensitive, so we normalise keys to
	// lowercase. On Unix/macOS they are case-sensitive and kept as-is.
	wantedBins := make(map[string][]toolRef) // normalised binary name → tool refs
	for i := range tools {
		if tools[i].Disabled {
			continue
		}
		for _, bin := range tools[i].BinaryNames {
			key := normaliseName(bin)
			wantedBins[key] = append(wantedBins[key], toolRef{toolIdx: i, binName: bin})
		}
	}

	// Phase 2: Scan PATH dirs concurrently, collect matches.
	numWorkers := runtime.NumCPU()
	if numWorkers > len(pathDirs) {
		numWorkers = len(pathDirs)
	}
	if numWorkers < 1 {
		numWorkers = 1
	}

	dirCh := make(chan string, len(pathDirs))
	for _, d := range pathDirs {
		dirCh <- d
	}
	close(dirCh)

	var mu sync.Mutex
	var matches []match
	var wg sync.WaitGroup

	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for dir := range dirCh {
				scanDir(dir, wantedBins, func(m match) {
					mu.Lock()
					matches = append(matches, m)
					mu.Unlock()
				})
			}
		}()
	}
	wg.Wait()

	// Phase 3: Assign matches to tools, preserving PATH order and deduplicating.
	// Build per-tool seen sets and ordered instances.
	type seenKey struct {
		toolIdx int
		path    string
	}
	seen := make(map[seenKey]struct{})

	// Sort matches by PATH directory order to preserve precedence.
	dirOrder := make(map[string]int, len(pathDirs))
	for i, d := range pathDirs {
		dirOrder[d] = i
	}

	// Group by tool, keeping PATH order.
	toolMatches := make(map[int][]match)
	for _, m := range matches {
		toolMatches[m.toolIdx] = append(toolMatches[m.toolIdx], m)
	}

	for i := range tools {
		if tools[i].Disabled {
			continue
		}
		tms := toolMatches[i]
		// Sort by PATH order using the original scanning directory.
		// Falls back to max index for entries not in dirOrder (e.g. LookPath fallback)
		// to keep them after all PATH-scanned entries.
		fallback := len(pathDirs)
		sort.SliceStable(tms, func(a, b int) bool {
			oa, ok := dirOrder[tms[a].pathDir]
			if !ok {
				oa = fallback
			}
			ob, ok := dirOrder[tms[b].pathDir]
			if !ok {
				ob = fallback
			}
			return oa < ob
		})
		for _, m := range tms {
			key := seenKey{toolIdx: i, path: m.instance.Path}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			tools[i].Instances = append(tools[i].Instances, m.instance)
		}
	}

	// Phase 4: Fallback via exec.LookPath for tools with no instances.
	for i := range tools {
		if tools[i].Disabled || len(tools[i].Instances) > 0 {
			continue
		}
		for _, binName := range tools[i].BinaryNames {
			path, err := exec.LookPath(binName)
			if err != nil {
				continue
			}
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil || resolved == "" {
				resolved = path
			}
			tools[i].Instances = append(tools[i].Instances, registry.Instance{
				Path:   resolved,
				Source: detectSource(resolved),
			})
			break
		}
	}

	return nil
}

// scanDir scans one directory for any of the wanted binary names and reports matches.
func scanDir(dir string, wantedBins map[string][]toolRef, emit func(match)) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	// Build a set of entry names for O(1) lookup.
	// On Windows names are case-insensitive; on Unix they are case-sensitive.
	entryNames := make(map[string]string, len(entries)) // normalised → original name
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		entryNames[normaliseName(e.Name())] = e.Name()
	}

	// Check each wanted binary against this directory's entries.
	for wantedKey, refs := range wantedBins {
		candidates := binaryCandidateNames(wantedKey)
		for _, cand := range candidates {
			if _, exists := entryNames[cand]; !exists {
				continue
			}
			fullPath := filepath.Join(dir, entryNames[cand])
			resolved, err := filepath.EvalSymlinks(fullPath)
			if err != nil {
				continue
			}
			info, err := os.Stat(resolved)
			if err != nil || info.IsDir() {
				continue
			}
			for _, ref := range refs {
				emit(match{
					toolIdx: ref.toolIdx,
					pathDir: dir,
					instance: registry.Instance{
						Path:   resolved,
						Source: detectSource(resolved),
					},
				})
			}
			break // first matching candidate wins for this binary name
		}
	}
}

// normaliseName returns a file name suitable for map-key comparison.
// On Windows file systems are case-insensitive, so we lowercase the name.
// On Unix/macOS names are case-sensitive and returned as-is.
func normaliseName(name string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(name)
	}
	return name
}

// binaryCandidateNames returns the normalised file names to look for in a directory.
func binaryCandidateNames(name string) []string {
	if runtime.GOOS == "windows" {
		var candidates []string
		for _, ext := range getPathExtensions() {
			candidates = append(candidates, name+ext)
		}
		candidates = append(candidates, name)
		return candidates
	}
	return []string{name}
}

func pathDirectories() []string {
	pathEnv := os.Getenv("PATH")
	if pathEnv == "" {
		return nil
	}
	raw := strings.Split(pathEnv, string(os.PathListSeparator))
	seen := make(map[string]struct{}, len(raw))
	dirs := make([]string, 0, len(raw))
	for _, d := range raw {
		if d = strings.TrimSpace(d); d != "" {
			if _, exists := seen[d]; !exists {
				seen[d] = struct{}{}
				dirs = append(dirs, d)
			}
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
// Normalises backslashes to forward slashes for cross-platform matching.
func detectSource(path string) registry.InstallSource {
	// Use ReplaceAll instead of filepath.ToSlash so Windows-style paths
	// are normalised to forward slashes on every platform (ToSlash only
	// replaces os.PathSeparator, which is '/' on Unix — leaving
	// backslashes untouched).
	lower := strings.ToLower(strings.ReplaceAll(path, `\`, "/"))

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
		// ~/.local/bin is a general user-level directory (XDG standard) used by
		// pip, pipx, cargo, go install, and manual copies. Attribute as manual
		// since we can't reliably determine the actual installer.
		return registry.SourceManual

	// Windows: winget installs to many locations beyond Program Files.
	case strings.Contains(lower, "program files"):
		return registry.SourceWinget
	case strings.Contains(lower, "8wekyb3d8bbwe"):
		// 8wekyb3d8bbwe is the package family suffix for Microsoft Store (MSIX)
		// sideloaded packages, used by WinGet-distributed tools like fzf, bat, fd.
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
