package audit

import (
	"strings"
	"testing"
	"time"

	"github.com/nassiharel/klim/internal/registry"
)

// installedTool builds a registry.Tool with one Instance populated so
// IsInstalled() returns true and PrimaryInstance() points at it.
func installedTool(name, version string, source registry.InstallSource, latest string) registry.Tool {
	return registry.Tool{
		Name:   name,
		Latest: latest,
		Instances: []registry.Instance{
			{Path: "/usr/local/bin/" + name, Version: version, Source: source},
		},
	}
}

func TestAnalyze_NoInstalledToolsReturnsNoFindings(t *testing.T) {
	tools := []registry.Tool{
		{Name: "git"}, // not installed (no Instances)
	}
	findings, licenses := Analyze(tools)
	if len(findings) != 0 {
		t.Fatalf("findings: want 0, got %d", len(findings))
	}
	if len(licenses) != 0 {
		t.Fatalf("licenses: want empty, got %v", licenses)
	}
}

func TestAnalyze_ManualSourceWarning(t *testing.T) {
	tools := []registry.Tool{
		installedTool("oddtool", "1.0.0", registry.SourceManual, ""),
	}
	findings, _ := Analyze(tools)
	if len(findings) == 0 {
		t.Fatalf("want at least one finding")
	}
	if findings[0].Category != "Unmanaged" {
		t.Fatalf("category: want Unmanaged, got %s", findings[0].Category)
	}
	if findings[0].Severity != "warning" {
		t.Fatalf("severity: want warning, got %s", findings[0].Severity)
	}
	if !strings.Contains(findings[0].Message, "/usr/local/bin/oddtool") {
		t.Fatalf("message should reference the install path, got %q", findings[0].Message)
	}
}

func TestAnalyze_NoVersionWarning(t *testing.T) {
	// Source is non-manual but version is empty → "No Version" warning.
	tools := []registry.Tool{
		installedTool("git", "", registry.SourceBrew, ""),
	}
	findings, _ := Analyze(tools)
	var found bool
	for _, f := range findings {
		if f.Category == "No Version" {
			found = true
			if f.Severity != "warning" {
				t.Fatalf("severity for No Version: want warning, got %s", f.Severity)
			}
		}
	}
	if !found {
		t.Fatalf("expected No Version finding, got %+v", findings)
	}

	// Manual source with empty version: should NOT trigger No Version
	// (the unmanaged warning already covers it).
	tools = []registry.Tool{installedTool("oddtool", "", registry.SourceManual, "")}
	findings, _ = Analyze(tools)
	for _, f := range findings {
		if f.Category == "No Version" {
			t.Fatalf("manual+no-version should not raise a No Version finding")
		}
	}
}

func TestAnalyze_ArchivedAndStale(t *testing.T) {
	twoYearsAgo := time.Now().Add(-2 * 365 * 24 * time.Hour).Format(time.RFC3339)
	threeMonthsAgo := time.Now().Add(-90 * 24 * time.Hour).Format(time.RFC3339)

	cases := []struct {
		name      string
		gh        *registry.GitHubInfo
		wantArch  bool
		wantStale bool
	}{
		{"archived + recent", &registry.GitHubInfo{Archived: true, PushedAt: threeMonthsAgo}, true, false},
		{"archived + ancient", &registry.GitHubInfo{Archived: true, PushedAt: twoYearsAgo}, true, true},
		{"active but stale", &registry.GitHubInfo{Archived: false, PushedAt: twoYearsAgo}, false, true},
		{"active and recent", &registry.GitHubInfo{Archived: false, PushedAt: threeMonthsAgo}, false, false},
		{"malformed pushed_at", &registry.GitHubInfo{Archived: false, PushedAt: "not-a-date"}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tool := installedTool("kubectl", "1.30.0", registry.SourceBrew, "")
			tool.GitHubInfo = tc.gh
			findings, _ := Analyze([]registry.Tool{tool})
			var sawArch, sawStale bool
			for _, f := range findings {
				switch f.Category {
				case "Archived":
					sawArch = true
				case "Stale":
					sawStale = true
				}
			}
			if sawArch != tc.wantArch {
				t.Errorf("archived: want %v, got %v (findings=%+v)", tc.wantArch, sawArch, findings)
			}
			if sawStale != tc.wantStale {
				t.Errorf("stale: want %v, got %v (findings=%+v)", tc.wantStale, sawStale, findings)
			}
		})
	}
}

func TestAnalyze_OutdatedFinding(t *testing.T) {
	tool := installedTool("kubectl", "1.30.0", registry.SourceBrew, "1.31.0")
	findings, _ := Analyze([]registry.Tool{tool})
	var msg string
	for _, f := range findings {
		if f.Category == "Outdated" {
			if f.Severity != "info" {
				t.Errorf("outdated severity: want info, got %s", f.Severity)
			}
			msg = f.Message
		}
	}
	if msg == "" {
		t.Fatalf("expected Outdated finding, got %+v", findings)
	}
	if !strings.Contains(msg, "1.30.0") || !strings.Contains(msg, "1.31.0") {
		t.Errorf("outdated message should include both versions, got %q", msg)
	}
}

func TestAnalyze_LicenseInventory(t *testing.T) {
	mit := &registry.GitHubInfo{License: "MIT"}
	apache := &registry.GitHubInfo{License: "Apache-2.0"}
	tools := []registry.Tool{
		func() registry.Tool { t := installedTool("a", "1", registry.SourceBrew, ""); t.GitHubInfo = mit; return t }(),
		func() registry.Tool { t := installedTool("b", "1", registry.SourceBrew, ""); t.GitHubInfo = mit; return t }(),
		func() registry.Tool { t := installedTool("c", "1", registry.SourceBrew, ""); t.GitHubInfo = apache; return t }(),
		installedTool("d", "1", registry.SourceBrew, ""), // no GitHubInfo => Unknown
	}
	_, licenses := Analyze(tools)
	if licenses["MIT"] != 2 {
		t.Errorf("MIT: want 2, got %d (full map: %v)", licenses["MIT"], licenses)
	}
	if licenses["Apache-2.0"] != 1 {
		t.Errorf("Apache-2.0: want 1, got %d", licenses["Apache-2.0"])
	}
	if licenses["Unknown"] != 1 {
		t.Errorf("Unknown: want 1, got %d", licenses["Unknown"])
	}
}

func TestAnalyze_FindingsSortedWarningsBeforeInfos(t *testing.T) {
	twoYearsAgo := time.Now().Add(-2 * 365 * 24 * time.Hour).Format(time.RFC3339)
	tools := []registry.Tool{
		// Tool that yields an info-level Stale finding only.
		func() registry.Tool {
			t := installedTool("aardvark", "1", registry.SourceBrew, "")
			t.GitHubInfo = &registry.GitHubInfo{PushedAt: twoYearsAgo}
			return t
		}(),
		// Tool that yields a warning-level Unmanaged finding.
		installedTool("zylophone", "1", registry.SourceManual, ""),
	}
	findings, _ := Analyze(tools)
	if len(findings) < 2 {
		t.Fatalf("want at least 2 findings, got %d", len(findings))
	}
	if findings[0].Severity != "warning" {
		t.Errorf("findings[0] should be warning (sorted first), got %s", findings[0].Severity)
	}
	// Within same severity, sorted alphabetically by tool name.
	// Construct two same-severity findings to test that explicitly.
	tools = []registry.Tool{
		installedTool("zoo", "1", registry.SourceManual, ""),
		installedTool("apple", "1", registry.SourceManual, ""),
	}
	findings, _ = Analyze(tools)
	if findings[0].Tool != "apple" {
		t.Errorf("alphabetical sort: want apple first, got %s (full=%+v)", findings[0].Tool, findings)
	}
}

func TestCountBySeverity(t *testing.T) {
	findings := []Finding{
		{Severity: "warning"}, {Severity: "warning"}, {Severity: "info"},
		{Severity: "info"}, {Severity: "info"}, {Severity: "unknown"},
	}
	w, i := CountBySeverity(findings)
	if w != 2 {
		t.Errorf("warnings: want 2, got %d", w)
	}
	if i != 3 {
		t.Errorf("infos: want 3, got %d", i)
	}
}

func TestCountBySeverity_EmptySlice(t *testing.T) {
	w, i := CountBySeverity(nil)
	if w != 0 || i != 0 {
		t.Errorf("empty: want (0,0), got (%d,%d)", w, i)
	}
}

func TestSeverityRank_ContractWarningBeforeInfoBeforeUnknown(t *testing.T) {
	if severityRank("warning") >= severityRank("info") {
		t.Errorf("warning should rank before info")
	}
	if severityRank("info") >= severityRank("anything-else") {
		t.Errorf("info should rank before unknown severities")
	}
}
