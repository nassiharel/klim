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
	"os/exec"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/fileutil"
	"github.com/nassiharel/clim/internal/finder"
	"github.com/nassiharel/clim/internal/paths"
	"github.com/nassiharel/clim/internal/registry"
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
// If it doesn't, we try to recover the instance via a PATH search of
// the tool's binary names — that handles the common "external
// upgrade rotated the versioned dir under brew/winget" pattern where
// the tool is still installed and on PATH but at a different
// resolved path. Recovery uses finder.PathDirectories() so we see
// the same merged PATH (process + Windows registry) the finder
// would see during a full scan, and finder.DetectSource() so a
// recovered path gets re-classified rather than inheriting a now-
// stale cached source. Recovered paths are deduped within the
// per-tool loop so multiple stale instances all pointing at the
// same live binary collapse into one.
//
// Without this guard a tool uninstalled outside clim's own install/
// upgrade/remove flow would either appear "installed" forever
// (false positive) or vanish on a benign upgrade (false negative).
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
		seenPaths := make(map[string]struct{}, len(entry.Instances))
		recoveryAttempted := false
		var recoveredPath string
		var recoveredOK bool
		for _, e := range entry.Instances {
			if pathExists(e.Path) {
				if _, dup := seenPaths[e.Path]; dup {
					continue
				}
				seenPaths[e.Path] = struct{}{}
				insts = append(insts, registry.Instance{
					Path:    e.Path,
					Version: e.Version,
					Source:  registry.InstallSource(e.Source),
				})
				continue
			}
			// Stale cached path. Try to recover via PATH lookup —
			// once per tool — so multiple stale instances don't
			// each append the same recovered binary. The recovered
			// instance's source comes from a fresh DetectSource on
			// the resolved path, not the stale cached source.
			if !recoveryAttempted {
				recoveredPath, recoveredOK = lookupOnPATH(tools[i].BinaryNames)
				recoveryAttempted = true
			}
			if !recoveredOK {
				continue
			}
			if _, dup := seenPaths[recoveredPath]; dup {
				continue
			}
			seenPaths[recoveredPath] = struct{}{}
			insts = append(insts, registry.Instance{
				Path:    recoveredPath,
				Version: e.Version, // best-effort; next full scan refreshes
				Source:  finder.DetectSource(recoveredPath),
			})
		}
		tools[i].Instances = insts
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

// lookupOnPATH returns the resolved path of the first binary name
// found on the same merged PATH the finder uses, or ("", false)
// when none are present. Used by Apply as a fallback when the
// cached path is stale (e.g. external brew upgrade rotated the
// versioned directory).
//
// We delegate the candidate-name expansion to finder.BinaryCandidateNames
// so PATHEXT-driven Windows extensions (.exe / .cmd / .bat / .com /
// …) are handled identically to the main scan. Hardcoding a smaller
// list would silently drop tools whose command resolves via a less
// common PATHEXT entry, defeating the recovery's purpose.
//
// Walking finder.PathDirectories() rather than relying on
// exec.LookPath alone matters on Windows: registry PATH entries
// that winget/scoop installs add post-launch are visible through
// the finder helper but not through the inherited process PATH.
//
// Loop nesting is `for dir { for name { for candidate } }` to
// match finder.scanDir's PATH-order semantics: the earliest PATH
// dir always wins, regardless of which binary alias resolves
// first. The previous nesting (`for name { for dir }`) could
// recover a later-PATH `python3` instead of an earlier-PATH
// `python` for tools with multiple binary names.
func lookupOnPATH(binaryNames []string) (string, bool) {
	if len(binaryNames) == 0 {
		return "", false
	}
	dirs := finder.PathDirectories()
	if len(dirs) == 0 {
		// Fall back to exec.LookPath so we still recover when the
		// finder helper produces an empty list (e.g. minimal test
		// environments).
		return lookupViaExec(binaryNames)
	}
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
				return resolved, true
			}
		}
	}
	return "", false
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
