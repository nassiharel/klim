// Package score computes a 0–100 environment health score by combining
// tool freshness, doctor diagnostics, audit findings, compliance status,
// and cache health into a single metric.
package score

import (
	"fmt"
	"net/url"

	"github.com/nassiharel/clim/internal/audit"
	"github.com/nassiharel/clim/internal/compliance"
	"github.com/nassiharel/clim/internal/doctor"
	"github.com/nassiharel/clim/internal/registry"
)

// Category represents one scoring dimension.
type Category struct {
	Name     string `json:"name"`
	Points   int    `json:"points"`
	MaxPts   int    `json:"max_points"`
	Status   string `json:"status"` // "ok", "warning", "error"
	Details  string `json:"details,omitempty"`
}

// Result holds the complete score breakdown.
type Result struct {
	Total      int        `json:"total"`
	MaxTotal   int        `json:"max_total"`
	Grade      string     `json:"grade"`
	Categories []Category `json:"categories"`
}

// Compute calculates the environment health score.
func Compute(tools []registry.Tool, doctorIssues []doctor.Issue, compResult *compliance.Result) Result {
	var cats []Category

	cats = append(cats, scoreToolFreshness(tools))
	cats = append(cats, scoreDoctorHealth(doctorIssues))
	cats = append(cats, scoreAuditFindings(tools))
	cats = append(cats, scoreCompliance(compResult))
	cats = append(cats, scoreManagedSources(tools))

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

// BadgeURL returns a shields.io badge URL for the score.
func BadgeURL(r Result) string {
	color := "brightgreen"
	switch {
	case r.Total < 50:
		color = "red"
	case r.Total < 70:
		color = "orange"
	case r.Total < 85:
		color = "yellow"
	case r.Total < 95:
		color = "yellowgreen"
	}
	label := url.PathEscape("clim score")
	value := url.PathEscape(fmt.Sprintf("%d/%d %s", r.Total, r.MaxTotal, r.Grade))
	return fmt.Sprintf("https://img.shields.io/badge/%s-%s-%s", label, value, color)
}

func grade(points, max int) string {
	if max == 0 {
		return "?"
	}
	pct := points * 100 / max
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

// Audit findings: 20 points. Warnings cost 2, based on audit.Analyze.
func scoreAuditFindings(tools []registry.Tool) Category {
	const maxPts = 20
	findings, _ := audit.Analyze(tools)
	warns, infos := audit.CountBySeverity(findings)
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
func scoreCompliance(result *compliance.Result) Category {
	const maxPts = 15
	if result == nil {
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
