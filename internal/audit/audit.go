// Package audit provides shared security audit logic used by both the
// CLI (clim audit) and TUI (Doctor→Audit sub-tab).
package audit

import (
	"fmt"
	"sort"
	"time"

	"github.com/nassiharel/clim/internal/registry"
)

// Finding represents a single audit issue.
type Finding struct {
	Severity string `json:"severity"` // "warning", "info"
	Tool     string `json:"tool"`
	Category string `json:"category"`
	Message  string `json:"message"`
}

// Analyze runs all audit checks on installed tools and returns findings
// plus a license inventory.
func Analyze(tools []registry.Tool) ([]Finding, map[string]int) {
	var findings []Finding
	licenses := make(map[string]int)

	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		primary := t.PrimaryInstance()
		if primary == nil {
			continue
		}

		if primary.Source == registry.SourceManual {
			findings = append(findings, Finding{
				Severity: "warning",
				Tool:     t.Name,
				Category: "Unmanaged",
				Message:  fmt.Sprintf("Installed from unknown source at %s — not tracked by any package manager", primary.Path),
			})
		}

		if primary.Version == "" && primary.Source != registry.SourceManual {
			findings = append(findings, Finding{
				Severity: "warning",
				Tool:     t.Name,
				Category: "No Version",
				Message:  "Version could not be determined — cannot verify security status",
			})
		}

		if t.GitHubInfo != nil && t.GitHubInfo.Archived {
			findings = append(findings, Finding{
				Severity: "warning",
				Tool:     t.Name,
				Category: "Archived",
				Message:  "Upstream repository is archived — no longer receiving security updates",
			})
		}

		if t.GitHubInfo != nil && t.GitHubInfo.PushedAt != "" {
			if pushed, err := time.Parse(time.RFC3339, t.GitHubInfo.PushedAt); err == nil {
				age := time.Since(pushed)
				if age > 365*24*time.Hour {
					months := int(age.Hours() / 24 / 30)
					findings = append(findings, Finding{
						Severity: "info",
						Tool:     t.Name,
						Category: "Stale",
						Message:  fmt.Sprintf("Last upstream activity was %d months ago", months),
					})
				}
			}
		}

		if t.HasUpdate() {
			findings = append(findings, Finding{
				Severity: "info",
				Tool:     t.Name,
				Category: "Outdated",
				Message:  fmt.Sprintf("Update available: %s → %s", primary.Version, t.Latest),
			})
		}

		if t.GitHubInfo != nil && t.GitHubInfo.License != "" {
			licenses[t.GitHubInfo.License]++
		} else {
			licenses["Unknown"]++
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity < findings[j].Severity
		}
		return findings[i].Tool < findings[j].Tool
	})

	return findings, licenses
}

// CountBySeverity returns (warnings, infos).
func CountBySeverity(findings []Finding) (int, int) {
	var w, i int
	for _, f := range findings {
		switch f.Severity {
		case "warning":
			w++
		case "info":
			i++
		}
	}
	return w, i
}
