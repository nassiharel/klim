package finder

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/nassiharel/klim/internal/registry"
)

// ErrEmptyPATH is returned when the PATH environment variable is empty or not set.
var ErrEmptyPATH = errors.New("PATH is empty or not set")

// Cached PATHEXT extensions (Windows), computed once.
var cachedPathExts struct {
	once sync.Once
	exts []string
}

// Cached dpkg availability check, computed once.
var cachedHasDpkg struct {
	once      sync.Once
	available bool
}

func hasDpkg() bool {
	cachedHasDpkg.once.Do(func() {
		_, err := exec.LookPath("dpkg")
		cachedHasDpkg.available = err == nil
	})
	return cachedHasDpkg.available
}

// ToolFinder abstracts tool discovery on the filesystem.
type ToolFinder interface {
	// FindAll scans PATH (and a curated set of per-user install
	// roots on Windows) and populates each tool's Instances. The
	// ctx argument must be non-nil; callers that don't have a
	// meaningful context should pass context.Background(). FindAll
	// returns ctx.Err() when ctx is cancelled mid-scan, ErrEmptyPATH
	// when no PATH directories are available (including Windows
	// registry PATH directories) and Phase 5 produced no results,
	// or nil on success.
	FindAll(ctx context.Context, tools []registry.Tool) error
}

// PathFinder is the default ToolFinder that scans PATH directories.
type PathFinder struct{}

// NewFinder returns the default PATH-based tool finder.
func NewFinder() ToolFinder {
	return &PathFinder{}
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

// FindAll is a convenience wrapper around the default PathFinder.
func FindAll(ctx context.Context, tools []registry.Tool) error {
	return defaultFinder.FindAll(ctx, tools)
}

var defaultFinder ToolFinder = &PathFinder{}

// FindAll implements ToolFinder by scanning PATH directories.
func (pf *PathFinder) FindAll(ctx context.Context, tools []registry.Tool) error {
	pathDirs := pathDirectories()
	if len(pathDirs) == 0 {
		// No PATH to scan, but Phase 5 still applies: winget GUI apps
		// under %LOCALAPPDATA%\Programs (Freelens, etc.) live outside
		// any PATH dir and would otherwise be invisible in shells,
		// services, or CI environments launching klim with a stripped
		// PATH. We suppress ErrEmptyPATH only when Phase 5 actually
		// added an instance — service callers discard the tool slice
		// on error, so signalling "empty PATH" would lose those
		// instances. We track the addition count rather than counting
		// non-empty Instances so a caller that pre-populated the slice
		// (e.g. tests, RefreshTool replays) doesn't accidentally
		// suppress ErrEmptyPATH when Phase 5 found nothing new.
		added := scanExtraInstallRoots(ctx, tools)
		// Honour cancellation explicitly: it's strictly more
		// informative than ErrEmptyPATH and matches the non-empty
		// PATH branch's behaviour.
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if added > 0 {
			slog.Info("PATH was empty; Phase 5 fallback recovered installs",
				"tools_total", len(tools), "added", added)
			return nil
		}
		return ErrEmptyPATH
	}

	// On Windows, update the process PATH so exec.LookPath (Phase 4) also
	// sees directories added after launch (e.g. by winget/scoop installs
	// that modify the registry PATH but not the inherited environment).
	if runtime.GOOS == "windows" {
		_ = os.Setenv("PATH", strings.Join(pathDirs, string(os.PathListSeparator)))
	}

	// Phase 1: Build a map of all binary names we're looking for.
	// On Windows, file names are case-insensitive, so we normalise keys to
	// lowercase. On Unix/macOS they are case-sensitive and kept as-is.
	wantedBins := make(map[string][]toolRef) // normalised binary name → tool refs
	for i := range tools {
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
				if ctx.Err() != nil {
					return
				}
				scanDir(dir, wantedBins, func(m match) {
					mu.Lock()
					matches = append(matches, m)
					mu.Unlock()
				})
			}
		}()
	}
	wg.Wait()

	if ctx.Err() != nil {
		return ctx.Err()
	}

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
		// gosec G602 false positive: i is bounded by `range tools`,
		// so &tools[i] is always safe. Taking the pointer once
		// scopes the suppression to a single line and lets the
		// remaining mutations read cleanly.
		t := &tools[i] //nolint:gosec // G602: i is bounded by range tools.
		// Clear previous instances so rescans don't accumulate duplicates.
		t.Instances = nil

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
			t.Instances = append(t.Instances, m.instance)
		}
	}

	// Phase 4: Fallback via exec.LookPath for tools with no instances.
	for i := range tools {
		// gosec G602 false positive: same justification as Phase 3.
		t := &tools[i] //nolint:gosec // G602: i is bounded by range tools.
		if len(t.Instances) > 0 {
			continue
		}
		for _, binName := range t.BinaryNames {
			path, err := exec.LookPath(binName)
			if err != nil {
				continue
			}
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil || resolved == "" {
				resolved = path
			}
			t.Instances = append(t.Instances, registry.Instance{
				Path:   resolved,
				Source: detectSource(resolved),
			})
			break
		}
	}

	// Phase 5: Walk well-known per-user install roots one level deep for
	// tools the previous phases missed. GUI apps installed via winget
	// (Freelens, etc.) live under %LOCALAPPDATA%\Programs\<App>\<App>.exe
	// and never expose a binary on PATH, so PATH scanning alone leaves
	// them invisible to klim. extraInstallRoots() returns nothing on
	// Linux/macOS, making this a no-op there. Returned count is ignored
	// here because Phase 5 instances are surfaced via tools directly.
	_ = scanExtraInstallRoots(ctx, tools)

	// Surface ctx cancellation that happened during Phase 4 or 5.
	// scanExtraInstallRootsAt returns early on cancellation but
	// doesn't propagate the error itself, and Phase 4 doesn't check
	// ctx at all — so without this check FindAll could report
	// success on a partially-completed scan.
	if ctx.Err() != nil {
		return ctx.Err()
	}

	found := 0
	for _, t := range tools {
		if len(t.Instances) > 0 {
			found++
		}
	}
	slog.Info("PATH scan complete", "dirs", len(pathDirs), "tools_found", found, "tools_total", len(tools))

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

// BinaryCandidateNames returns the OS-aware list of file names to
// look for when resolving a binary by its catalog name. On Windows
// the list expands by PATHEXT (.exe, .cmd, .bat, .com, …) so a tool
// listed as "go" matches "go.exe" / "go.cmd" / etc. Exported so the
// scancache fast-path uses the same expansion the full scan does.
func BinaryCandidateNames(name string) []string {
	return binaryCandidateNames(name)
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

// PathDirectories returns the deduplicated list of directories the
// finder will scan on this OS. Equivalent to splitting $PATH but
// also merging Windows registry PATH entries that the current
// process may not have inherited (e.g. winget portable installs
// that update PATH after launch). Exported so scancache (and other
// fast-path callers) can consult the same view of PATH the finder
// uses.
func PathDirectories() []string {
	return pathDirectories()
}

// DetectSource maps a resolved binary path to the install source
// klim believes owns it (winget, brew, apt, scoop, choco, npm,
// manual, …). Exported so callers re-resolving a path outside the
// main scan loop (notably scancache.Apply when recovering a stale
// cached entry via PATH lookup) classify the source the same way.
func DetectSource(path string) registry.InstallSource {
	return detectSource(path)
}

func pathDirectories() []string {
	// Start with the process PATH, then merge any directories from the
	// Windows registry that the current process hasn't picked up yet
	// (e.g. winget portable installs that modify PATH after launch).
	pathEnv := os.Getenv("PATH")
	if extra := registryPATH(); extra != "" {
		pathEnv = pathEnv + string(os.PathListSeparator) + extra
	}
	if pathEnv == "" {
		return nil
	}
	raw := strings.Split(pathEnv, string(os.PathListSeparator))
	seen := make(map[string]struct{}, len(raw))
	dirs := make([]string, 0, len(raw))
	for _, d := range raw {
		if d = strings.TrimSpace(d); d != "" {
			// Normalize for dedup: trim trailing slashes on all platforms,
			// lowercase only on Windows (case-insensitive filesystem).
			key := strings.TrimRight(d, `\/`)
			if runtime.GOOS == "windows" {
				key = strings.ToLower(key)
			}
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
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
	case strings.Contains(lower, "roaming/npm/") ||
		strings.Contains(lower, "node_modules/") ||
		strings.Contains(lower, "/lib/node_modules/"):
		return registry.SourceNPM
	case strings.HasPrefix(lower, "/usr/bin/") || strings.HasPrefix(lower, "/usr/lib/"):
		// System package manager path — could be apt, dnf, pacman, etc.
		// Only attribute to apt if dpkg is available (Debian/Ubuntu).
		if hasDpkg() {
			return registry.SourceApt
		}
		return registry.SourceManual
	case strings.Contains(lower, "/go/bin/"):
		return registry.SourceGo
	case strings.Contains(lower, ".cargo/bin/"):
		return registry.SourceCargo
	case strings.Contains(lower, ".local/bin/"):
		// ~/.local/bin is a general user-level directory (XDG standard) used by
		// pip, pipx, cargo, go install, and manual copies. Attribute as manual
		// since we can't reliably determine the actual installer.
		return registry.SourceManual

	// Windows: binaries bundled inside Git for Windows — not independently managed.
	// Excludes /git/cmd/ where git.exe itself lives (managed by the installer).
	case strings.Contains(lower, "/git/usr/bin/") ||
		strings.Contains(lower, "/git/mingw64/bin/"):
		return registry.SourceManual

	// Windows: WinGet stores its package metadata under
	// %LOCALAPPDATA%\Microsoft\WinGet\Packages\<id_<source>>. Binaries
	// living there are unambiguously winget-managed.
	case strings.Contains(lower, "/microsoft/winget/packages/"):
		return registry.SourceWinget
	case strings.Contains(lower, "8wekyb3d8bbwe"):
		// 8wekyb3d8bbwe is the package family suffix for Microsoft Store (MSIX)
		// sideloaded packages, used by WinGet-distributed tools like fzf, bat, fd.
		return registry.SourceWinget
	case strings.Contains(lower, "microsoft/windowsapps/"):
		// Windows Store / App Execution Aliases (e.g. python)
		return registry.SourceWinget

	// AppData\Local\Programs and Program Files are *predominantly*
	// winget territory on modern Windows: most managed MSIs (VS
	// Code, Azure Dev CLI, machine-wide installs, etc.) land there
	// via winget. Calling them SourceManual previously regressed
	// the happy path for legitimately winget-managed tools — they
	// stopped getting upgrade/remove actions and lost winget
	// latest-version resolution.
	//
	// Path alone genuinely is ambiguous (Cursor, GitHub Desktop,
	// Docker Desktop, manual MSIs, and Chocolatey use these
	// directories too), so when we're wrong, the user hits a
	// "winget reports no installed package" error. The friendlier
	// hint we surface for that exact exit code (see
	// internal/tui/action_hints.go) is the safety net for the
	// false-positive case; classifying here as SourceWinget keeps
	// the much more common winget-actually-owns-it case working
	// without a manual rescan.
	case strings.Contains(lower, "appdata/local/programs/"):
		return registry.SourceWinget
	case strings.Contains(lower, "program files"):
		return registry.SourceWinget

	default:
		return registry.SourceManual
	}
}

// scanExtraInstallRoots is the Phase 5 fallback: for every tool still
// without instances, walk extraInstallRoots() one level deep looking
// for a binary that matches one of the tool's BinaryNames. The first
// match per tool wins. Each emitted instance gets its source classified
// by detectSource so the resolver picks the right package manager
// (e.g. winget for paths under %LOCALAPPDATA%\Programs).
//
// Returns the number of instances Phase 5 appended so callers can
// distinguish "Phase 5 found something" from "tools already had
// instances from earlier phases".
//
// This is intentionally narrow:
//   - only runs for tools the regular PATH/LookPath phases missed
//   - only walks roots curated as "winget-user-scope friendly"
//   - bails out cheaply when a root doesn't exist
func scanExtraInstallRoots(ctx context.Context, tools []registry.Tool) int {
	return scanExtraInstallRootsAt(ctx, tools, extraInstallRootsFn(), nil)
}

// extraInstallRootsFn is the function Phase 5 calls to discover the
// list of extra install roots. Production points at extraInstallRoots
// (defined per-OS in path_*.go); tests swap it out to inject a fake
// root so the FindAll empty-PATH branch can be exercised end-to-end.
var extraInstallRootsFn = extraInstallRoots

// scanExtraInstallRootsAt is the testable form of scanExtraInstallRoots:
// callers supply the install roots explicitly so unit tests can exercise
// the directory walk on any OS without depending on real install paths.
//
// The optional visit callback fires for every subdir actually read.
// Production callers pass nil; tests use it to assert that the walker
// short-circuits once every pending tool is resolved. Threading the
// visitor as a parameter (rather than a package-level var) keeps the
// production state immutable and avoids data-race risk if tests are
// ever parallelised.
//
// Walks lazily: each subdir is read on demand, every still-pending tool
// is matched against that subdir's entries (BinaryNames in declared
// order), then resolved tools are dropped from the pending set. The
// outer loop breaks the moment the pending set drains, so subdirs that
// alphabetically follow the last match are never read at all. ctx
// cancellation is honoured between roots and between subdirs. ctx
// must be non-nil; callers should pass context.Background() when no
// real cancellation context is available.
//
// Note: this design picks subdir-order over cross-subdir BinaryNames
// priority — i.e. for a tool declaring BinaryNames [python, python3],
// finding "python3" in subdir "A" wins over "python" in later subdir
// "B". That's deliberate: %LOCALAPPDATA%\Programs subdirs are app-name
// directories that don't ship multiple-named binaries of the same tool,
// so the priority distinction is theoretical here, while reading every
// root up-front purely to honour it would defeat the early-exit
// guarantee callers (and reviewers) reasonably expect.
func scanExtraInstallRootsAt(ctx context.Context, tools []registry.Tool, roots []string, visit func(string)) int {
	if ctx.Err() != nil {
		return 0
	}
	if len(roots) == 0 {
		return 0
	}

	pending := make([]int, 0, len(tools))
	for i := range tools {
		if len(tools[i].Instances) == 0 && len(tools[i].BinaryNames) > 0 {
			pending = append(pending, i)
		}
	}
	if len(pending) == 0 {
		return 0
	}

	added := 0
walk:
	for _, root := range roots {
		if root == "" {
			continue
		}
		if ctx.Err() != nil {
			return added
		}
		rootEntries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, re := range rootEntries {
			if !re.IsDir() {
				continue
			}
			if ctx.Err() != nil {
				return added
			}
			sub := filepath.Join(root, re.Name())
			files, err := os.ReadDir(sub)
			if err != nil {
				continue
			}
			if visit != nil {
				// Fire only after a successful ReadDir so the
				// callback's contract — "every subdir actually
				// read" — holds even when a sibling is
				// permission-denied or otherwise unreadable.
				visit(sub)
			}
			entries := make(map[string]string, len(files))
			for _, f := range files {
				if f.IsDir() {
					continue
				}
				entries[normaliseName(f.Name())] = f.Name()
			}
			if len(entries) == 0 {
				continue
			}

			// Filter pending in place: keep tools that did NOT match
			// in this subdir; matched tools get an instance appended
			// and are dropped from future iterations. Reading and
			// writing through pending[:n] is safe because n is
			// always <= the current read index.
			n := 0
			for _, idx := range pending {
				matched := false
				for _, bin := range tools[idx].BinaryNames {
					if matched {
						break
					}
					for _, cand := range binaryCandidateNames(normaliseName(bin)) {
						orig, ok := entries[cand]
						if !ok {
							continue
						}
						full := filepath.Join(sub, orig)
						// Fall back to the unresolved path when EvalSymlinks
						// fails (permission errors, unusual reparse points)
						// — matches Phase 4's behaviour and keeps the tool
						// detectable instead of dropping it silently.
						resolved, err := filepath.EvalSymlinks(full)
						if err != nil || resolved == "" {
							resolved = full
						}
						info, err := os.Stat(resolved)
						if err != nil || info.IsDir() {
							continue
						}
						tools[idx].Instances = append(tools[idx].Instances, registry.Instance{
							Path:   resolved,
							Source: detectSource(resolved),
						})
						matched = true
						added++
						break
					}
				}
				if !matched {
					pending[n] = idx
					n++
				}
			}
			pending = pending[:n]
			if len(pending) == 0 {
				break walk
			}
		}
	}
	return added
}
