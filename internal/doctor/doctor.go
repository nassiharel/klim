// Package doctor runs environment health checks and reports diagnostic
// issues. It inspects the user's PATH, installed tools, package manager
// availability, and scan cache freshness to surface problems and suggest
// fixes.
package doctor

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nassiharel/klim/internal/registry"
	"github.com/nassiharel/klim/internal/scancache"
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
	// Action carries an optional structured remediation. The Health
	// → Issues TUI dispatches on Action.Kind to provide one-key
	// fixes (copy a command, trigger a rescan, jump to Updates,
	// etc.). CLI output only renders the legacy `Fix` summary, so
	// Action stays purely additive.
	//
	// Pointer rather than embedded struct so JSON `omitempty` can
	// actually drop the field — encoding/json doesn't treat
	// zero-value structs as empty.
	Action *Action `json:"action,omitempty"`
}

// ScanMeta provides context about the resolved tool data.
type ScanMeta struct {
	FromCache bool
	CacheAge  time.Duration
}

// Diagnose runs all health checks and returns found issues.
func Diagnose(tools []registry.Tool, meta ScanMeta) []Issue {
	var issues []Issue
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
		issue := Issue{
			Severity: SeverityInfo,
			Category: CategoryPM,
			Title:    fmt.Sprintf("%s is not installed", pm),
			Detail:   fmt.Sprintf("%d of your installed tools have %s packages available", count, pm),
			Fix:      fmt.Sprintf("Install %s to get version tracking and updates for these tools", pm),
		}
		if cmd := installPMCommand(pm); cmd != "" {
			issue.Action = &Action{
				Kind:    ActionCopyCommand,
				Label:   fmt.Sprintf("Copy %s install command", pm),
				Command: cmd,
				Target:  string(pm),
			}
		}
		issues = append(issues, issue)
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
		Fix:      "Run klim with --refresh or press r in the TUI to rescan",
		Action: &Action{
			Kind:  ActionRescan,
			Label: "Rescan now",
		},
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
		Action: &Action{
			Kind:  ActionRescan,
			Label: "Rescan now",
		},
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
		Fix:      "Switch to the Updates tab or run klim tool list to see details",
		Action: &Action{
			Kind:  ActionJumpUpdates,
			Label: "Jump to Updates tab",
		},
	}}
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
