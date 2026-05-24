// Package score computes a 0–100 environment health score by combining
// tool freshness, doctor diagnostics, audit findings, compliance status,
// and source management into a single metric.
package score

import (
	"fmt"
	"net/url"

	"github.com/nassiharel/klim/internal/compliance"
	"github.com/nassiharel/klim/internal/doctor"
	"github.com/nassiharel/klim/internal/registry"
)

// Category represents one scoring dimension.
type Category struct {
	Name    string `json:"name"`
	Points  int    `json:"points"`
	MaxPts  int    `json:"max_points"`
	Status  string `json:"status"` // "ok", "warning", "error"
	Details string `json:"details,omitempty"`
}

// Result holds the complete score breakdown.
type Result struct {
	Total      int        `json:"total"`
	MaxTotal   int        `json:"max_total"`
	Grade      string     `json:"grade"`
	Categories []Category `json:"categories"`
}

// Input holds all inputs for score computation.
type Input struct {
	Tools         []registry.Tool
	DoctorIssues  []doctor.Issue
	AuditWarnings int
	AuditInfos    int
	CompResult    *compliance.Result
	ComplianceErr string
}

// Compute calculates the environment health score.
func Compute(input Input) Result {
	var cats []Category

	cats = append(cats,
		scoreToolFreshness(input.Tools),
		scoreDoctorHealth(input.DoctorIssues),
		scoreAuditClean(input.AuditWarnings, input.AuditInfos),
		scoreCompliance(input.CompResult, input.ComplianceErr),
		scoreManagedSources(input.Tools),
	)

	total, maxTotal := 0, 0
	for _, c := range cats {
		total += c.Points
		maxTotal += c.MaxPts
	}

	return Result{
		Total:      total,
		MaxTotal:   maxTotal,
		Grade:      grade(total, maxTotal),
		Categories: cats,
	}
}

// BadgeURL returns a shields.io badge URL for the score. The color
// uses BadgeColor so any caller building their own score badge stays
// in sync with this canonical URL builder.
func BadgeURL(r Result) string {
	pct := 0
	if r.MaxTotal > 0 {
		pct = r.Total * 100 / r.MaxTotal
	}
	color := BadgeColor(pct)
	label := url.PathEscape("klim score")
	value := url.PathEscape(fmt.Sprintf("%d/%d %s", r.Total, r.MaxTotal, r.Grade))
	return fmt.Sprintf("https://img.shields.io/badge/%s-%s-%s", label, value, color)
}

// BadgeColor returns the Shields.io color name for the given
// percentage 0..100. Public so other packages can render
// score-equivalent badges without duplicating the threshold table
// (and silently drifting from `klim score --badge`'s colors).
func BadgeColor(pct int) string {
	switch {
	case pct < 50:
		return "red"
	case pct < 70:
		return "orange"
	case pct < 85:
		return "yellow"
	case pct < 95:
		return "yellowgreen"
	}
	return "brightgreen"
}

func grade(points, maximum int) string {
	if maximum == 0 {
		return "?"
	}
	pct := points * 100 / maximum
	switch {
	case pct >= 95:
		return "A+"
	case pct >= 90:
		return "A"
	case pct >= 80:
		return "B"
	case pct >= 70:
		return "C"
	case pct >= 60:
		return "D"
	default:
		return "F"
	}
}

// --- Scoring functions (each returns a Category with points) ---

// Tool freshness: 30 points. Lose points per outdated tool.
func scoreToolFreshness(tools []registry.Tool) Category {
	const maxPts = 30
	var installed, outdated int
	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		installed++
		if t.HasUpdate() {
			outdated++
		}
	}
	if installed == 0 {
		return Category{Name: "Tools up to date", Points: maxPts, MaxPts: maxPts, Status: "ok", Details: "No tools installed"}
	}

	pct := (installed - outdated) * 100 / installed
	points := maxPts * pct / 100
	status := "ok"
	details := fmt.Sprintf("%d/%d tools current", installed-outdated, installed)
	if outdated > 0 {
		status = "warning"
		details = fmt.Sprintf("%d tool(s) have updates available", outdated)
	}
	return Category{Name: "Tools up to date", Points: points, MaxPts: maxPts, Status: status, Details: details}
}

// Doctor health: 25 points. Errors cost 5 each, warnings cost 2.
func scoreDoctorHealth(issues []doctor.Issue) Category {
	const maxPts = 25
	errs, warns, _ := doctor.CountBySeverity(issues)
	penalty := errs*5 + warns*2
	points := maxPts - penalty
	if points < 0 {
		points = 0
	}
	status := "ok"
	details := "No issues"
	if errs > 0 {
		status = "error"
		details = fmt.Sprintf("%d error(s), %d warning(s)", errs, warns)
	} else if warns > 0 {
		status = "warning"
		details = fmt.Sprintf("%d warning(s)", warns)
	}
	return Category{Name: "Doctor health", Points: points, MaxPts: maxPts, Status: status, Details: details}
}

// Audit findings: 20 points. Warnings cost 3, infos cost 1.
func scoreAuditClean(warns, infos int) Category {
	const maxPts = 20
	penalty := warns*3 + infos*1
	points := maxPts - penalty
	if points < 0 {
		points = 0
	}
	status := "ok"
	details := "No findings"
	if warns > 0 {
		status = "warning"
		details = fmt.Sprintf("%d warning(s), %d info(s)", warns, infos)
	} else if infos > 0 {
		details = fmt.Sprintf("%d info(s)", infos)
	}
	return Category{Name: "Audit clean", Points: points, MaxPts: maxPts, Status: status, Details: details}
}

// Compliance: 15 points. Full marks if compliant or no policy.
// Zero points if policy exists but failed to load.
func scoreCompliance(result *compliance.Result, loadErr string) Category {
	const maxPts = 15
	if result == nil {
		if loadErr != "" {
			return Category{Name: "Compliance", Points: 0, MaxPts: maxPts, Status: "error", Details: "Policy load failed: " + loadErr}
		}
		return Category{Name: "Compliance", Points: maxPts, MaxPts: maxPts, Status: "ok", Details: "No policy configured"}
	}
	if result.Compliant && len(result.Violations) == 0 {
		return Category{Name: "Compliance", Points: maxPts, MaxPts: maxPts, Status: "ok", Details: "All tools comply"}
	}
	var errors, warnings int
	for _, v := range result.Violations {
		if v.Severity == "error" {
			errors++
		} else {
			warnings++
		}
	}
	penalty := errors*5 + warnings*2
	points := maxPts - penalty
	if points < 0 {
		points = 0
	}
	status := "warning"
	if errors > 0 {
		status = "error"
	}
	return Category{Name: "Compliance", Points: points, MaxPts: maxPts, Status: status, Details: fmt.Sprintf("%d error(s), %d warning(s)", errors, warnings)}
}

// Managed sources: 10 points. Lose 3 per manual/unmanaged tool.
func scoreManagedSources(tools []registry.Tool) Category {
	const maxPts = 10
	var unmanaged int
	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		primary := t.PrimaryInstance()
		if primary != nil && primary.Source == registry.SourceManual {
			unmanaged++
		}
	}
	penalty := unmanaged * 3
	points := maxPts - penalty
	if points < 0 {
		points = 0
	}
	status := "ok"
	details := "All tools from managed sources"
	if unmanaged > 0 {
		status = "warning"
		details = fmt.Sprintf("%d unmanaged tool(s)", unmanaged)
	}
	return Category{Name: "Managed sources", Points: points, MaxPts: maxPts, Status: status, Details: details}
}
