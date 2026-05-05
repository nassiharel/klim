// Package doctor runs environment health checks and reports diagnostic
// issues. It inspects the user's PATH, installed tools, package manager
// availability, and scan cache freshness to surface problems and suggest
// fixes.
package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/scancache"
)

// Severity classifies the importance of a diagnostic issue.
type Severity string

// Severity constants.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Category constants group issues by topic.
const (
	CategoryPATH  = "PATH"
	CategoryTools = "Tools"
	CategoryPM    = "Package Managers"
	CategoryCache = "Cache"
)

// Issue represents a single diagnostic finding.
type Issue struct {
	Severity Severity `json:"severity"`
	Category string   `json:"category"`
	Title    string   `json:"title"`
	Detail   string   `json:"detail"`
	Fix      string   `json:"fix,omitempty"`
}

// ScanMeta provides context about the resolved tool data.
type ScanMeta struct {
	FromCache bool
	CacheAge  time.Duration
}

// Diagnose runs all health checks and returns found issues.
func Diagnose(tools []registry.Tool, meta ScanMeta) []Issue {
	var issues []Issue
	issues = append(issues, checkDuplicatePATH()...)
	issues = append(issues, checkBrokenPATH()...)
	issues = append(issues, checkPATHShadowing(tools)...)
	issues = append(issues, checkUserWritablePathOrder()...)
	issues = append(issues, checkMultipleInstallations(tools)...)
	issues = append(issues, checkMissingPMs(tools)...)
	issues = append(issues, checkStaleCache()...)
	issues = append(issues, checkUnresolvedVersions(tools)...)
	issues = append(issues, checkOutdatedSummary(tools, meta)...)
	return issues
}

// HasErrors reports whether any issue has error severity.
func HasErrors(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == SeverityError {
			return true
		}
	}
	return false
}

// CountBySeverity returns (errors, warnings, infos).
func CountBySeverity(issues []Issue) (int, int, int) {
	var e, w, i int
	for _, issue := range issues {
		switch issue.Severity {
		case SeverityError:
			e++
		case SeverityWarning:
			w++
		case SeverityInfo:
			i++
		}
	}
	return e, w, i
}

// --- Individual checks ---

// checkDuplicatePATH detects duplicate entries in PATH.
func checkDuplicatePATH() []Issue {
	raw := os.Getenv("PATH")
	if raw == "" {
		return nil
	}
	dirs := filepath.SplitList(raw)
	seen := make(map[string]pathEntry) // normalized path → first occurrence
	var issues []Issue

	for i, dir := range dirs {
		norm := normalizePath(dir)
		if norm == "" {
			continue
		}
		if first, ok := seen[norm]; ok {
			detail := fmt.Sprintf("%q (position %d) duplicates %q (position %d)", dir, i+1, first.raw, first.index+1)
			issues = append(issues, Issue{
				Severity: SeverityWarning,
				Category: CategoryPATH,
				Title:    "Duplicate PATH entry",
				Detail:   detail,
				Fix:      "Remove the duplicate entry from your PATH",
			})
		} else {
			seen[norm] = pathEntry{raw: dir, index: i}
		}
	}
	return issues
}

type pathEntry struct {
	raw   string
	index int
}

// checkBrokenPATH detects PATH entries that don't exist or aren't directories.
func checkBrokenPATH() []Issue {
	raw := os.Getenv("PATH")
	if raw == "" {
		return nil
	}
	dirs := filepath.SplitList(raw)
	var issues []Issue

	for _, dir := range dirs {
		cleaned := strings.TrimSpace(dir)
		if cleaned == "" {
			continue
		}
		cleaned = filepath.Clean(cleaned)
		info, err := os.Stat(cleaned)
		switch {
		case os.IsNotExist(err):
			issues = append(issues, Issue{
				Severity: SeverityWarning,
				Category: CategoryPATH,
				Title:    "Missing PATH directory",
				Detail:   fmt.Sprintf("%q does not exist", dir),
				Fix:      "Remove this entry from your PATH",
			})
		case os.IsPermission(err):
			issues = append(issues, Issue{
				Severity: SeverityWarning,
				Category: CategoryPATH,
				Title:    "Inaccessible PATH directory",
				Detail:   fmt.Sprintf("%q exists but permission denied", dir),
				Fix:      "Fix permissions or remove from PATH",
			})
		case err == nil && !info.IsDir():
			issues = append(issues, Issue{
				Severity: SeverityWarning,
				Category: CategoryPATH,
				Title:    "Non-directory in PATH",
				Detail:   fmt.Sprintf("%q is a file, not a directory", dir),
				Fix:      "Remove this entry from your PATH",
			})
		}
	}
	return issues
}

// checkMultipleInstallations detects tools with multiple instances that have
// conflicting versions. Same tool at multiple paths with identical versions
// is normal and not flagged.
func checkMultipleInstallations(tools []registry.Tool) []Issue {
	var issues []Issue
	for _, t := range tools {
		if len(t.Instances) < 2 {
			continue
		}

		// Collect unique resolved versions (skip empty/unresolved).
		versions := make(map[string][]string) // version → list of paths
		for _, inst := range t.Instances {
			if inst.Version == "" {
				continue
			}
			versions[inst.Version] = append(versions[inst.Version], inst.Path)
		}

		if len(versions) <= 1 {
			continue
		}

		// Build detail showing version → paths (sorted for stable output).
		versionKeys := make([]string, 0, len(versions))
		for v := range versions {
			versionKeys = append(versionKeys, v)
		}
		sort.Strings(versionKeys)

		var parts []string
		for _, v := range versionKeys {
			parts = append(parts, fmt.Sprintf("  %s: %s", v, strings.Join(versions[v], ", ")))
		}
		primary := t.PrimaryInstance()
		fix := ""
		if primary != nil {
			fix = fmt.Sprintf("Active version is %s from %s", primary.Version, primary.Path)
		}

		issues = append(issues, Issue{
			Severity: SeverityError,
			Category: CategoryTools,
			Title:    fmt.Sprintf("%s has %d installations with different versions", t.DisplayName, len(t.Instances)),
			Detail:   strings.Join(parts, "\n"),
			Fix:      fix,
		})
	}
	return issues
}

// checkMissingPMs flags package managers that have packages for installed
// tools but aren't available on the system.
func checkMissingPMs(tools []registry.Tool) []Issue {
	pmStatus := registry.AllPMStatusForOS()
	unavailable := make(map[registry.InstallSource]bool)
	for _, s := range pmStatus {
		if !s.Available {
			unavailable[s.Source] = true
		}
	}
	if len(unavailable) == 0 {
		return nil
	}

	// Count tools that have package IDs for unavailable PMs.
	neededCount := make(map[registry.InstallSource]int)
	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		for pm := range unavailable {
			if hasPkgID(t.Packages, pm) {
				neededCount[pm]++
			}
		}
	}

	// Sort PM keys for stable output ordering.
	pmKeys := make([]registry.InstallSource, 0, len(neededCount))
	for pm := range neededCount {
		if neededCount[pm] > 0 {
			pmKeys = append(pmKeys, pm)
		}
	}
	sort.Slice(pmKeys, func(i, j int) bool { return pmKeys[i] < pmKeys[j] })

	var issues []Issue
	for _, pm := range pmKeys {
		count := neededCount[pm]
		issues = append(issues, Issue{
			Severity: SeverityInfo,
			Category: CategoryPM,
			Title:    fmt.Sprintf("%s is not installed", pm),
			Detail:   fmt.Sprintf("%d of your installed tools have %s packages available", count, pm),
			Fix:      fmt.Sprintf("Install %s to get version tracking and updates for these tools", pm),
		})
	}
	return issues
}

// checkStaleCache warns if the scan cache is older than 7 days.
func checkStaleCache() []Issue {
	path, err := scancache.Path()
	if err != nil {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	age := time.Since(info.ModTime())
	if age < 7*24*time.Hour {
		return nil
	}
	days := int(age.Hours() / 24)
	return []Issue{{
		Severity: SeverityInfo,
		Category: CategoryCache,
		Title:    "Scan cache is stale",
		Detail:   fmt.Sprintf("Last scan was %d days ago", days),
		Fix:      "Run clim with --refresh or press r in the TUI to rescan",
	}}
}

// checkUnresolvedVersions flags installed tools where version couldn't be
// determined. Only flags tools installed via tracked sources (not manual).
func checkUnresolvedVersions(tools []registry.Tool) []Issue {
	var unresolved []string
	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		primary := t.PrimaryInstance()
		if primary == nil {
			continue
		}
		if primary.Version == "" && primary.Source != registry.SourceManual {
			unresolved = append(unresolved, t.Name)
		}
	}
	if len(unresolved) == 0 {
		return nil
	}
	detail := strings.Join(unresolved, ", ")
	if len(unresolved) > 10 {
		detail = strings.Join(unresolved[:10], ", ") + fmt.Sprintf(" and %d more", len(unresolved)-10)
	}
	return []Issue{{
		Severity: SeverityWarning,
		Category: CategoryTools,
		Title:    fmt.Sprintf("%d installed tool(s) with unknown version", len(unresolved)),
		Detail:   detail,
		Fix:      "Try running with --refresh for a fresh version scan",
	}}
}

// checkOutdatedSummary reports a summary of tools with available updates.
func checkOutdatedSummary(tools []registry.Tool, meta ScanMeta) []Issue {
	var outdated int
	for _, t := range tools {
		if t.HasUpdate() {
			outdated++
		}
	}
	if outdated == 0 {
		return nil
	}
	detail := fmt.Sprintf("%d tool(s) have newer versions available", outdated)
	if meta.FromCache {
		detail += " (based on cached data)"
	}
	return []Issue{{
		Severity: SeverityInfo,
		Category: CategoryTools,
		Title:    fmt.Sprintf("%d update(s) available", outdated),
		Detail:   detail,
		Fix:      "Switch to the Updates tab or run clim list to see details",
	}}
}

// checkPATHShadowing detects tools with multiple Instances on PATH and
// reports which path "wins" (first match per shell-style PATH lookup).
// Mostly an info-level diagnostic, but bumps to warning when the
// winning path is in a user-writable directory ahead of a system one
// — that's a privilege-escalation pattern (an attacker with write
// access to your bin/ dir can shadow sudo, kubectl, etc.).
func checkPATHShadowing(tools []registry.Tool) []Issue {
	var issues []Issue
	for _, t := range tools {
		if len(t.Instances) < 2 {
			continue
		}
		// Build "tool found at A, B, C" — A wins.
		paths := make([]string, 0, len(t.Instances))
		for _, inst := range t.Instances {
			paths = append(paths, inst.Path)
		}
		winner := paths[0]
		shadowed := paths[1:]

		// Severity: info by default; warning when the winner sits in a
		// user-writable dir that precedes a system one for any of the
		// shadowed paths.
		sev := SeverityInfo
		if winnerIsUserWritableShadowingSystem(winner, shadowed) {
			sev = SeverityWarning
		}
		issues = append(issues, Issue{
			Severity: sev,
			Category: CategoryPATH,
			Title:    t.DisplayName + " shadowed on PATH",
			Detail:   fmt.Sprintf("Active: %s\nShadowed: %s", winner, strings.Join(shadowed, ", ")),
			Fix:      "Reorder your PATH so the trusted location comes first, or remove duplicate copies",
		})
	}
	return issues
}

// winnerIsUserWritableShadowingSystem reports whether the active path
// sits in a user-writable directory while any of the shadowed paths
// is in a system directory. Best-effort; missing files / unreadable
// dirs return false.
func winnerIsUserWritableShadowingSystem(winner string, shadowed []string) bool {
	if !isUserWritableDir(filepath.Dir(winner)) {
		return false
	}
	for _, p := range shadowed {
		if isSystemDir(filepath.Dir(p)) {
			return true
		}
	}
	return false
}

// checkUserWritablePathOrder walks PATH left-to-right and flags
// user-writable directories that precede system directories. This is
// a privilege-escalation hardening check: if `~/.local/bin` is ahead
// of `/usr/local/bin` on PATH, an attacker who lands a file in
// `~/.local/bin/sudo` shadows the real sudo.
func checkUserWritablePathOrder() []Issue {
	raw := os.Getenv("PATH")
	if raw == "" {
		return nil
	}
	parts := filepath.SplitList(raw)
	var issues []Issue
	seenUserWritable := []string{}
	for _, dir := range parts {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		switch {
		case isUserWritableDir(dir):
			seenUserWritable = append(seenUserWritable, dir)
		case isSystemDir(dir) && len(seenUserWritable) > 0:
			issues = append(issues, Issue{
				Severity: SeverityInfo,
				Category: CategoryPATH,
				Title:    "User-writable PATH dir precedes system dir",
				Detail: fmt.Sprintf("System dir: %s\nUser-writable dirs ahead of it: %s",
					dir, strings.Join(seenUserWritable, ", ")),
				Fix: "Move user-writable bin dirs after system dirs to avoid shadowing trusted binaries",
			})
			// Only report the first system dir; one warning per
			// problematic ordering is enough to make the point.
			return issues
		}
	}
	return issues
}

// hasPathPrefix reports whether dir is the same path as parent or a
// descendant of it, with proper path-boundary handling. Avoids false
// positives like HasPrefix("/home/joel/bin", "/home/joe"). Both inputs
// are cleaned; comparison is case-insensitive on Windows (filesystem
// semantics) and exact elsewhere.
func hasPathPrefix(dir, parent string) bool {
	if parent == "" {
		return false
	}
	dirClean := filepath.Clean(dir)
	parentClean := filepath.Clean(parent)
	if runtime.GOOS == "windows" {
		dirClean = strings.ToLower(dirClean)
		parentClean = strings.ToLower(parentClean)
	}
	if dirClean == parentClean {
		return true
	}
	rel, err := filepath.Rel(parentClean, dirClean)
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

// isUserWritableDir reports whether dir is in a location an
// unprivileged user can drop binaries into. This is intentionally a
// heuristic, not an authoritative permission check:
//
//   - World-writable dirs (mode bits include 0o002) always count.
//   - Dirs under $HOME with the owner-write bit set count, on the
//     assumption that the calling user owns their own home tree.
//     We don't syscall to compare the dir's stat UID against EUID
//     (would require platform-specific syscall.Stat_t and pull in
//     extra build constraints) — accuracy isn't worth the
//     complexity for what's already a hardening *suggestion*.
//   - On Windows, the heuristic is "lives under USERPROFILE".
//
// False positives just inflate severity on the PATH-shadowing
// diagnostic; they don't break installs or fail builds.
func isUserWritableDir(dir string) bool {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return false
	}
	info, err := os.Stat(dir) // #nosec G304 -- dir originates from PATH; checking PATH integrity is the purpose.
	if err != nil || !info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		// Heuristic: directories under the user profile are typically
		// user-writable. The ACL machinery to check properly isn't
		// worth the complexity for what's already a hardening
		// suggestion.
		return hasPathPrefix(dir, os.Getenv("USERPROFILE"))
	}
	mode := info.Mode().Perm()
	// World-writable always counts.
	if mode&0o002 != 0 {
		return true
	}
	// Owner-writable counts when current user owns the dir. On non-
	// Windows we can't easily get the file owner without syscalls; a
	// good-enough heuristic is "user dir under HOME".
	if hasPathPrefix(dir, os.Getenv("HOME")) {
		return mode&0o200 != 0
	}
	return false
}

// isSystemDir reports whether dir is one of the canonical OS-trusted
// PATH entries. Hard-coded list keeps the check predictable; missing
// a non-standard system dir just means we don't flag a potential
// shadowing — preferable to false positives.
func isSystemDir(dir string) bool {
	dir = filepath.Clean(strings.TrimSpace(dir))
	switch runtime.GOOS {
	case "windows":
		dl := strings.ToLower(dir)
		for _, sys := range []string{
			`c:\windows`, `c:\windows\system32`, `c:\windows\syswow64`,
			`c:\program files`, `c:\program files (x86)`,
		} {
			if strings.HasPrefix(dl, sys) {
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

// normalizePath normalizes a path for deduplication. On Windows, it
// lowercases and cleans the path. On Unix, it just cleans it.
func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return p
}

// hasPkgID checks whether a tool has a package ID for the given source.
func hasPkgID(p registry.PackageIDs, source registry.InstallSource) bool {
	switch source {
	case registry.SourceWinget:
		return p.Winget != ""
	case registry.SourceChoco:
		return p.Choco != ""
	case registry.SourceScoop:
		return p.Scoop != ""
	case registry.SourceBrew:
		return p.Brew != ""
	case registry.SourceApt:
		return p.Apt != ""
	case registry.SourceSnap:
		return p.Snap != ""
	case registry.SourceNPM:
		return p.NPM != ""
	}
	return false
}
