// Package pathconflict derives a structured view of PATH-shadowing and
// version-conflict situations from an already-scanned tool slice.
//
// Inputs come from registry.Tool.Instances, which the finder
// populates in PATH precedence order (first instance wins). The
// analyzer reads os.Getenv("PATH") and does best-effort os.Stat
// calls on PATH entries to flag missing / duplicate / user-writable
// directories — i.e. it is NOT a fully-pure transform; it touches
// the filesystem (read-only) and the process environment. The
// IO is bounded by the size of $PATH and never writes; failures
// degrade gracefully (a stat error just means the entry isn't
// flagged as user-writable).
//
// The companion `doctor` package keeps emitting text Issue records
// for PATH shadowing — those are good for the long-form Issues
// list. This package returns the underlying structured model so a
// visualization can render it richly (tables, two-pane layouts,
// version diffs).
package pathconflict

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/nassiharel/klim/internal/registry"
)

// Report is the analyzer's output. Both views are derived from the
// same set of tool instances; together they let a UI render either a
// tool-centric or PATH-centric layout without re-walking the data.
type Report struct {
	ByTool []ToolView `json:"by_tool"`
	ByDir  []DirView  `json:"by_dir"`
}

// ToolView captures one tool that has at least two PATH instances.
// Active is the binary that actually resolves when the user runs the
// command; Shadowed lists every other instance in PATH order.
type ToolView struct {
	Name            string         `json:"name"`
	DisplayName     string         `json:"display_name"`
	Active          InstanceView   `json:"active"`
	Shadowed        []InstanceView `json:"shadowed"`
	VersionConflict bool           `json:"version_conflict"`
	// PrivilegeRisk is true when the winning copy sits in a
	// user-writable dir that shadows a copy in a system dir — the
	// classic local-priv-esc setup.
	PrivilegeRisk bool `json:"privilege_risk"`
}

// InstanceView is one resolved binary location with the metadata the
// renderer needs (version + which PM owns it).
type InstanceView struct {
	Path    string                 `json:"path"`
	Version string                 `json:"version"`
	Source  registry.InstallSource `json:"source"`
	// UninstallCmd is the human-readable command that would remove
	// this specific copy via its PM. Empty for manual installs.
	UninstallCmd string `json:"uninstall_cmd,omitempty"`
}

// DirView is one PATH directory with the tools it provides. ToolEntry
// records whether this dir wins or loses for each tool — the same
// tool may appear in multiple DirViews, marked Active in the
// first-occurrence dir and Shadowed everywhere else.
type DirView struct {
	Dir         string      `json:"dir"`
	Order       int         `json:"order"` // 1-based PATH position
	Exists      bool        `json:"exists"`
	IsDir       bool        `json:"is_dir"`
	Tools       []ToolEntry `json:"tools"`
	UserWrite   bool        `json:"user_writable"`
	SystemDir   bool        `json:"system_dir"`
	Duplicate   bool        `json:"duplicate"` // duplicates a previous PATH entry
	DuplicateOf string      `json:"duplicate_of,omitempty"`
}

// ToolEntry is one (tool, instance-in-this-dir) pairing inside a DirView.
type ToolEntry struct {
	Name        string                 `json:"name"`
	DisplayName string                 `json:"display_name"`
	Version     string                 `json:"version"`
	Source      registry.InstallSource `json:"source"`
	Active      bool                   `json:"active"` // wins PATH lookup
}

// Analyze derives the Report from the tool slice. Reads PATH at call
// time (single os.Getenv lookup) to build the ByDir view; the ByTool
// view depends only on the tools' Instances field.
func Analyze(tools []registry.Tool) Report {
	return Report{
		ByTool: analyzeByTool(tools),
		ByDir:  analyzeByDir(tools),
	}
}

func analyzeByTool(tools []registry.Tool) []ToolView {
	views := make([]ToolView, 0)
	for _, t := range tools {
		if len(t.Instances) < 2 {
			continue
		}
		active := makeInstanceView(t, t.Instances[0])
		shadowed := make([]InstanceView, 0, len(t.Instances)-1)
		for _, inst := range t.Instances[1:] {
			shadowed = append(shadowed, makeInstanceView(t, inst))
		}
		v := ToolView{
			Name:        t.Name,
			DisplayName: fallbackName(t),
			Active:      active,
			Shadowed:    shadowed,
		}
		v.VersionConflict = versionsDiffer(t.Instances)
		v.PrivilegeRisk = winnerIsUserWritableShadowingSystem(t.Instances)
		views = append(views, v)
	}
	sort.Slice(views, func(i, j int) bool {
		// Prioritise the more interesting rows: version conflicts
		// first, then priv-esc shadowing, then everything else
		// alphabetical.
		ai, aj := views[i], views[j]
		if ai.VersionConflict != aj.VersionConflict {
			return ai.VersionConflict
		}
		if ai.PrivilegeRisk != aj.PrivilegeRisk {
			return ai.PrivilegeRisk
		}
		return strings.ToLower(ai.DisplayName) < strings.ToLower(aj.DisplayName)
	})
	return views
}

func analyzeByDir(tools []registry.Tool) []DirView {
	raw := os.Getenv("PATH")
	if raw == "" {
		return nil
	}
	parts := filepath.SplitList(raw)

	// Index tools' instances by the directory their binary lives in
	// (normalised for case on Windows). One dir may contain multiple
	// tools; one tool may have multiple instances across dirs.
	type pathEntry struct {
		tool    *registry.Tool
		inst    registry.Instance
		isFirst bool // first occurrence across PATH = the winner
	}
	dirIndex := make(map[string][]pathEntry)
	for ti := range tools {
		t := &tools[ti] //nolint:gosec // G602: ti bounded by range tools.
		for ii, inst := range t.Instances {
			d := normalizeDir(filepath.Dir(inst.Path))
			dirIndex[d] = append(dirIndex[d], pathEntry{
				tool:    t,
				inst:    inst,
				isFirst: ii == 0,
			})
		}
	}

	views := make([]DirView, 0, len(parts))
	seenDir := make(map[string]int) // normalised dir → first DirView.Order
	for i, raw := range parts {
		dir := strings.TrimSpace(raw)
		if dir == "" {
			continue
		}
		dv := DirView{
			Dir:       dir,
			Order:     i + 1,
			UserWrite: isUserWritableDir(dir),
			SystemDir: isSystemDir(dir),
		}
		if info, err := os.Stat(dir); err == nil { //nolint:gosec // G703: dir originates from $PATH; auditing PATH is the point.
			dv.Exists = true
			dv.IsDir = info.IsDir()
		}
		norm := normalizeDir(dir)
		if prior, ok := seenDir[norm]; ok {
			dv.Duplicate = true
			dv.DuplicateOf = parts[prior-1]
		} else {
			seenDir[norm] = i + 1
		}
		for _, ent := range dirIndex[norm] {
			dv.Tools = append(dv.Tools, ToolEntry{
				Name:        ent.tool.Name,
				DisplayName: fallbackName(*ent.tool),
				Version:     ent.inst.Version,
				Source:      ent.inst.Source,
				Active:      ent.isFirst,
			})
		}
		sort.Slice(dv.Tools, func(a, b int) bool {
			return strings.ToLower(dv.Tools[a].DisplayName) < strings.ToLower(dv.Tools[b].DisplayName)
		})
		views = append(views, dv)
	}
	return views
}

// HasConflicts reports whether the report contains anything worth
// surfacing prominently (version mismatch or privilege risk). Callers
// can use it to colour the tab header / exit non-zero in CI.
func (r Report) HasConflicts() bool {
	for _, v := range r.ByTool {
		if v.VersionConflict || v.PrivilegeRisk {
			return true
		}
	}
	return false
}

// CountShadowed returns the total number of shadowed instances across
// every tool. Useful for summary lines like "12 shadowed copies".
func (r Report) CountShadowed() int {
	n := 0
	for _, v := range r.ByTool {
		n += len(v.Shadowed)
	}
	return n
}

func makeInstanceView(t registry.Tool, inst registry.Instance) InstanceView {
	iv := InstanceView{
		Path:    inst.Path,
		Version: inst.Version,
		Source:  inst.Source,
	}
	// Only PM-owned instances expose an uninstall command. Manual
	// installs (`SourceManual`, or anything without a PkgID) fall
	// through to the empty string so the UI shows "manual — open
	// location" instead of a misleading suggestion.
	if inst.Source != registry.SourceManual {
		if cmd := t.Packages.RemoveCmd(inst.Source); cmd != "" {
			iv.UninstallCmd = cmd
		}
	}
	return iv
}

func versionsDiffer(instances []registry.Instance) bool {
	var first string
	for _, inst := range instances {
		if inst.Version == "" {
			continue
		}
		if first == "" {
			first = inst.Version
			continue
		}
		if !registry.VersionsMatch(first, inst.Version) {
			return true
		}
	}
	return false
}

func winnerIsUserWritableShadowingSystem(instances []registry.Instance) bool {
	if len(instances) < 2 {
		return false
	}
	winnerDir := filepath.Dir(instances[0].Path)
	if !isUserWritableDir(winnerDir) {
		return false
	}
	for _, inst := range instances[1:] {
		if isSystemDir(filepath.Dir(inst.Path)) {
			return true
		}
	}
	return false
}

func fallbackName(t registry.Tool) string {
	if t.DisplayName != "" {
		return t.DisplayName
	}
	return t.Name
}

func normalizeDir(p string) string {
	p = filepath.Clean(strings.TrimSpace(p))
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return p
}

// isUserWritableDir mirrors the heuristic used in internal/doctor.
// Duplicated here (not exported from doctor) because doctor depends on
// scancache and we want this package free of that transitive weight.
func isUserWritableDir(dir string) bool {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return false
	}
	info, err := os.Stat(dir) //nolint:gosec // G304: dir originates from PATH; auditing PATH is the point.
	if err != nil || !info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return hasPathPrefix(dir, os.Getenv("USERPROFILE"))
	}
	mode := info.Mode().Perm()
	if mode&0o002 != 0 {
		return true
	}
	if hasPathPrefix(dir, os.Getenv("HOME")) {
		return mode&0o200 != 0
	}
	return false
}

func isSystemDir(dir string) bool {
	dir = filepath.Clean(strings.TrimSpace(dir))
	switch runtime.GOOS {
	case "windows":
		// Use hasPathPrefix (which goes through filepath.Rel) so
		// that look-alike siblings like C:\WindowsApps and
		// C:\Windows.old are NOT classified as system dirs even
		// though they share the "c:\windows" string prefix.
		for _, sys := range []string{
			`c:\windows`, `c:\windows\system32`, `c:\windows\syswow64`,
			`c:\program files`, `c:\program files (x86)`,
		} {
			if hasPathPrefix(dir, sys) {
				return true
			}
		}
		return false
	default:
		switch dir {
		case "/usr/local/sbin", "/usr/local/bin",
			"/usr/sbin", "/usr/bin",
			"/sbin", "/bin",
			"/opt/homebrew/bin", "/opt/homebrew/sbin":
			return true
		}
		return false
	}
}

func hasPathPrefix(dir, parent string) bool {
	if parent == "" {
		return false
	}
	dc := filepath.Clean(dir)
	pc := filepath.Clean(parent)
	if runtime.GOOS == "windows" {
		dc = strings.ToLower(dc)
		pc = strings.ToLower(pc)
	}
	if dc == pc {
		return true
	}
	rel, err := filepath.Rel(pc, dc)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if strings.HasPrefix(rel, "..") {
		return false
	}
	return true
}
