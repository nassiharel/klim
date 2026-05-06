// Package scancache persists the result of a full tool scan (PATH discovery
// plus package-manager version resolution) to a YAML file on disk. The TUI
// and CLI load this cache on startup so users don't pay the cost of running
// dozens of subprocess queries (winget/brew/npm/etc.) every time klim runs.
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
	"os/exec"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/klim/internal/fileutil"
	"github.com/nassiharel/klim/internal/finder"
	"github.com/nassiharel/klim/internal/paths"
	"github.com/nassiharel/klim/internal/registry"
)

// cacheVersion is bumped whenever the on-disk schema changes so older
// caches written by prior versions are ignored instead of misparsed.
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
	return paths.ScanCache()
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
	if mkErr := fileutil.EnsureDir(path); mkErr != nil {
		return fmt.Errorf("creating cache dir: %w", mkErr)
	}

	entries := make(map[string]Entry, len(tools))
	for _, t := range tools {
		// Only cache installed tools. Not-installed tools have no useful
		// per-host state worth persisting — a missing cache entry on
		// Apply() already means "not installed", which is correct.
		if !t.IsInstalled() {
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
		ToolCount: len(entries),
		Tools:     entries,
	}

	data, err := yaml.Marshal(&f)
	if err != nil {
		return fmt.Errorf("marshalling cache: %w", err)
	}

	return fileutil.AtomicWrite(path, data, 0o644)
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
	var f file
	found, err := fileutil.ReadYAML(path, &f)
	if err != nil {
		return nil, time.Time{}, err
	}
	if !found {
		return nil, time.Time{}, os.ErrNotExist
	}
	if f.Version != cacheVersion {
		return nil, time.Time{}, fmt.Errorf("cache schema version %d unsupported (want %d)", f.Version, cacheVersion)
	}
	return f.Tools, f.SavedAt, nil
}

// Apply overlays cached scan data onto the catalog tools. Tools not present
// in the cache are left untouched (they will look "not installed"), which
// is the correct behaviour for newly-added catalog entries.
//
// For each cached instance we verify the recorded path still exists.
// If it doesn't, we recover the tool by walking the same merged PATH
// the finder would use during a full scan and collecting every
// matching binary (mirrors finder.scanDir which emits every match
// per tool, not just the first). That handles:
//   - external upgrade rotates a versioned dir under brew/winget
//     (the binary is still on PATH at a different resolved path)
//   - tool has multiple installed copies and several moved at once
//
// Recovered paths are deduped within the per-tool loop so multiple
// stale entries collapsing to the same live binary appear once.
// Recovered instances get a fresh source classification via
// finder.DetectSource (the cached source could now be wrong if the
// binary was reinstalled via a different manager).
//
// When no original path stat-checks AND no recovery match anywhere
// on PATH, the tool's Latest/LatestFrom are also cleared — leaving
// stale 'latest version' metadata on a now-uninstalled tool would
// have the UI confidently render upgrade hints for something the
// user can't even run.
func Apply(tools []registry.Tool, entries map[string]Entry) []registry.Tool {
	for i := range tools {
		t := &tools[i] //nolint:gosec // G602: i is bounded by range; gosec's flow analysis can't see that.
		entry, ok := entries[t.Name]
		if !ok {
			continue
		}
		t.Latest = entry.Latest
		t.LatestFrom = entry.LatestFrom
		if len(entry.Instances) == 0 {
			t.Instances = nil
			t.Latest = ""
			t.LatestFrom = ""
			continue
		}
		insts := make([]registry.Instance, 0, len(entry.Instances))
		seenPaths := make(map[string]struct{}, len(entry.Instances))

		// First pass: keep instances whose cached path still
		// exists. Tracking which entries were stale lets us know
		// whether to bother walking PATH for recovery.
		anyStale := false
		for _, e := range entry.Instances {
			if !pathExists(e.Path) {
				anyStale = true
				continue
			}
			if _, dup := seenPaths[e.Path]; dup {
				continue
			}
			seenPaths[e.Path] = struct{}{}
			insts = append(insts, registry.Instance{
				Path:    e.Path,
				Version: e.Version,
				Source:  registry.InstallSource(e.Source),
			})
		}

		// Second pass: if any cached path was stale, walk PATH and
		// append every matching binary that we haven't already
		// recorded. Doing it once (rather than per-stale-instance)
		// keeps the scan cheap, and collecting all matches lets a
		// tool with multiple cached installs recover several at
		// once — the previous "first match wins" approach silently
		// dropped distinct later-PATH installations.
		if anyStale {
			for _, recovered := range lookupAllOnPATH(t.BinaryNames) {
				if _, dup := seenPaths[recovered]; dup {
					continue
				}
				seenPaths[recovered] = struct{}{}
				insts = append(insts, registry.Instance{
					Path:   recovered,
					Source: finder.DetectSource(recovered),
					// Version is intentionally empty: a fresh scan
					// runs the per-PM resolver to populate it.
					// Carrying a stale cached version here would
					// be misleading for a binary at a new path.
				})
			}
		}

		t.Instances = insts
		if len(insts) == 0 {
			// Every cached instance vanished and no recovery
			// found a replacement — the tool's gone. Clear stale
			// metadata so the UI doesn't show upgrade hints for
			// something uninstalled.
			t.Latest = ""
			t.LatestFrom = ""
		}
	}
	return tools
}

// pathExists is a tiny wrapper around os.Stat used by Apply to drop
// cached instances whose binary has been removed since the last
// scan. Symlinks resolve through Stat (vs Lstat) so a broken symlink
// is treated as gone.
func pathExists(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// lookupAllOnPATH walks every directory in finder.PathDirectories()
// and returns every binary that matches one of binaryNames (with
// PATHEXT expansion on Windows), in PATH order. Resolved through
// EvalSymlinks. Each path is returned at most once. Mirrors
// finder.scanDir which emits every match per tool, so the scancache
// fast-path can recover multiple cached installations that all
// moved to new resolved paths in one pass.
func lookupAllOnPATH(binaryNames []string) []string {
	if len(binaryNames) == 0 {
		return nil
	}
	dirs := finder.PathDirectories()
	if len(dirs) == 0 {
		// Test/edge environments: fall back to exec.LookPath so we
		// still produce something useful. Returns at most one
		// match — exec.LookPath doesn't expose siblings.
		if path, ok := lookupViaExec(binaryNames); ok {
			return []string{path}
		}
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, 2)
	for _, dir := range dirs {
		for _, name := range binaryNames {
			for _, candidate := range finder.BinaryCandidateNames(name) {
				full := filepath.Join(dir, candidate)
				info, err := os.Stat(full)
				if err != nil || info.IsDir() {
					continue
				}
				resolved, err := filepath.EvalSymlinks(full)
				if err != nil || resolved == "" {
					resolved = full
				}
				if _, dup := seen[resolved]; dup {
					continue
				}
				seen[resolved] = struct{}{}
				out = append(out, resolved)
			}
		}
	}
	return out
}

func lookupViaExec(binaryNames []string) (string, bool) {
	for _, name := range binaryNames {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || resolved == "" {
			resolved = path
		}
		return resolved, true
	}
	return "", false
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
