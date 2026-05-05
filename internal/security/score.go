// Package security computes a per-tool security verdict by aggregating
// signals from internal/audit, internal/vuln, internal/compliance, and
// the tool's installed source.
package security

import (
	"strings"

	"github.com/nassiharel/clim/internal/audit"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/vuln"
)

// Status is the per-tool security verdict.
type Status int

// Status values, low to high severity.
const (
	StatusUnknown Status = iota
	StatusClean
	StatusWatch
	StatusRisk
)

// String returns the canonical machine-readable label.
func (s Status) String() string {
	switch s {
	case StatusClean:
		return "clean"
	case StatusWatch:
		return "watch"
	case StatusRisk:
		return "risk"
	}
	return "unknown"
}

// Glyph returns the single character used in the TUI badge column.
func (s Status) Glyph() string {
	switch s {
	case StatusClean:
		return "●"
	case StatusWatch:
		return "◐"
	case StatusRisk:
		return "✗"
	}
	return "○"
}

// Verdict is the score result for a single tool.
type Verdict struct {
	Tool    string
	Status  Status
	Reasons []string
}

// Score evaluates one tool given catalog metadata, audit findings,
// and any vuln matches for it.
func Score(t registry.Tool, findings []audit.Finding, match *vuln.Match) Verdict {
	v := Verdict{Tool: t.Name}
	if !t.IsInstalled() {
		return v
	}

	if match != nil && len(match.Vulnerabilities) > 0 {
		v.Status = upgrade(v.Status, StatusRisk)
		topSev := match.MaxSeverity()
		v.Reasons = append(v.Reasons,
			"known vulnerability"+severitySuffix(topSev)+" — "+vulnIDsSummary(match.Vulnerabilities))
	}

	for _, f := range findings {
		if f.Tool != t.Name {
			continue
		}
		switch f.Category {
		case "Archived":
			v.Status = upgrade(v.Status, StatusRisk)
			v.Reasons = append(v.Reasons, "upstream archived")
		case "Unmanaged":
			v.Status = upgrade(v.Status, StatusWatch)
			v.Reasons = append(v.Reasons, "unmanaged install")
		case "Stale":
			v.Status = upgrade(v.Status, StatusWatch)
			v.Reasons = append(v.Reasons, "stale upstream")
		case "Outdated":
			v.Status = upgrade(v.Status, StatusWatch)
			v.Reasons = append(v.Reasons, "outdated")
		case "No Version":
			v.Status = upgrade(v.Status, StatusWatch)
			v.Reasons = append(v.Reasons, "no version detected")
		}
	}

	if v.Status == StatusUnknown {
		v.Status = StatusClean
	}
	return v
}

func upgrade(current, candidate Status) Status {
	if candidate > current {
		return candidate
	}
	return current
}

func severitySuffix(s vuln.Severity) string {
	if s == vuln.SeverityUnknown {
		return ""
	}
	return " (" + string(s) + ")"
}

func vulnIDsSummary(vulns []vuln.Vulnerability) string {
	const limit = 3
	if len(vulns) == 0 {
		return ""
	}
	ids := make([]string, 0, limit+1)
	for i, v := range vulns {
		if i >= limit {
			break
		}
		ids = append(ids, v.ID)
	}
	out := strings.Join(ids, ", ")
	if len(vulns) > limit {
		out += " +" + itoa(len(vulns)-limit) + " more"
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	out := ""
	for n > 0 {
		out = string(rune('0'+n%10)) + out
		n /= 10
	}
	return out
}

// Index gives O(1) lookup of a tool's verdict by name.
type Index struct {
	verdicts map[string]Verdict
}

// BuildIndex constructs an Index from a tool list, audit findings, and
// the optional vuln report.
func BuildIndex(tools []registry.Tool, findings []audit.Finding, vulnReport *vuln.Report) *Index {
	matches := map[string]*vuln.Match{}
	if vulnReport != nil {
		for i := range vulnReport.Matches {
			m := &vulnReport.Matches[i]
			matches[m.Tool] = m
		}
	}
	idx := &Index{verdicts: make(map[string]Verdict, len(tools))}
	for _, t := range tools {
		idx.verdicts[t.Name] = Score(t, findings, matches[t.Name])
	}
	return idx
}

// Verdict returns the per-tool verdict. If the tool isn't in the
// index (no Score call ever ran for it), returns a Verdict with the
// tool name populated and StatusUnknown so callers don't lose the
// identity in error messages or rendering.
func (i *Index) Verdict(toolName string) Verdict {
	if i == nil {
		return Verdict{Tool: toolName}
	}
	v, ok := i.verdicts[toolName]
	if !ok {
		return Verdict{Tool: toolName}
	}
	return v
}

// Counts returns (clean, watch, risk, unknown).
func (i *Index) Counts() (clean, watch, risk, unknown int) {
	if i == nil {
		return
	}
	for _, v := range i.verdicts {
		switch v.Status {
		case StatusClean:
			clean++
		case StatusWatch:
			watch++
		case StatusRisk:
			risk++
		default:
			unknown++
		}
	}
	return
}
