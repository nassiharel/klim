package security

import (
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/audit"
	"github.com/nassiharel/klim/internal/registry"
	"github.com/nassiharel/klim/internal/vuln"
)

func installedTool(name string) registry.Tool {
	return registry.Tool{
		Name: name,
		Instances: []registry.Instance{
			{Path: "/usr/bin/" + name, Version: "1.0", Source: registry.SourceApt},
		},
	}
}

func TestScore_NotInstalledIsUnknown(t *testing.T) {
	v := Score(registry.Tool{Name: "ghost"}, nil, nil)
	if v.Status != StatusUnknown {
		t.Errorf("status = %v, want Unknown", v.Status)
	}
}

func TestScore_CleanWhenNoSignals(t *testing.T) {
	v := Score(installedTool("git"), nil, nil)
	if v.Status != StatusClean {
		t.Errorf("status = %v, want Clean", v.Status)
	}
}

func TestScore_VulnPromotesToRisk(t *testing.T) {
	match := &vuln.Match{
		Tool: "node",
		Vulnerabilities: []vuln.Vulnerability{
			{ID: "GHSA-aaa", Severity: vuln.SeverityHigh},
		},
	}
	v := Score(installedTool("node"), nil, match)
	if v.Status != StatusRisk {
		t.Errorf("status = %v, want Risk", v.Status)
	}
	if len(v.Reasons) == 0 || !strings.Contains(v.Reasons[0], "GHSA-aaa") {
		t.Errorf("reasons = %v", v.Reasons)
	}
}

func TestScore_OutdatedIsWatch(t *testing.T) {
	findings := []audit.Finding{{Tool: "git", Category: "Outdated"}}
	v := Score(installedTool("git"), findings, nil)
	if v.Status != StatusWatch {
		t.Errorf("status = %v, want Watch", v.Status)
	}
}

func TestScore_ArchivedIsRisk(t *testing.T) {
	findings := []audit.Finding{{Tool: "old", Category: "Archived"}}
	v := Score(installedTool("old"), findings, nil)
	if v.Status != StatusRisk {
		t.Errorf("status = %v, want Risk for archived", v.Status)
	}
}

func TestScore_RiskWinsOverWatch(t *testing.T) {
	findings := []audit.Finding{{Tool: "node", Category: "Outdated"}}
	match := &vuln.Match{
		Tool:            "node",
		Vulnerabilities: []vuln.Vulnerability{{ID: "GHSA-x", Severity: vuln.SeverityCritical}},
	}
	v := Score(installedTool("node"), findings, match)
	if v.Status != StatusRisk {
		t.Errorf("status = %v, want Risk", v.Status)
	}
}

func TestBuildIndex_Counts(t *testing.T) {
	tools := []registry.Tool{
		installedTool("a"), installedTool("b"), installedTool("c"), installedTool("d"),
	}
	findings := []audit.Finding{
		{Tool: "b", Category: "Outdated"},
		{Tool: "c", Category: "Archived"},
	}
	vulnReport := &vuln.Report{
		Matches: []vuln.Match{
			{Tool: "d", Vulnerabilities: []vuln.Vulnerability{{ID: "GHSA-x", Severity: vuln.SeverityHigh}}},
		},
	}
	idx := BuildIndex(tools, findings, vulnReport)
	clean, watch, risk, unknown := idx.Counts()
	if clean != 1 || watch != 1 || risk != 2 || unknown != 0 {
		t.Errorf("counts = %d/%d/%d/%d, want 1/1/2/0", clean, watch, risk, unknown)
	}
}

func TestStatus_GlyphsAreDistinct(t *testing.T) {
	seen := map[string]Status{}
	for _, s := range []Status{StatusUnknown, StatusClean, StatusWatch, StatusRisk} {
		g := s.Glyph()
		if other, dup := seen[g]; dup {
			t.Errorf("glyph %q dup: %v vs %v", g, other, s)
		}
		seen[g] = s
	}
}
