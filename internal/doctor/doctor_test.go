package doctor

import (
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/registry"
)

func TestDiagnose_healthy(t *testing.T) {
	tools := []registry.Tool{
		{
			Name:        "kubectl",
			DisplayName: "kubectl",
			Instances:   []registry.Instance{{Path: "/usr/local/bin/kubectl", Version: "1.28.0", Source: "brew"}},
			Latest:      "1.28.0",
		},
	}
	issues := Diagnose(tools, ScanMeta{})
	// Should not flag any tool-level errors when there's one instance, version is current.
	for _, i := range issues {
		if i.Category == CategoryTools && i.Severity == SeverityError {
			t.Errorf("unexpected tool error: %s", i.Title)
		}
	}
}

func TestCheckMultipleInstallations_sameVersion(t *testing.T) {
	tools := []registry.Tool{
		{
			Name:        "go",
			DisplayName: "Go",
			Instances: []registry.Instance{
				{Path: "/usr/local/go/bin/go", Version: "1.22.0", Source: "manual"},
				{Path: "/opt/go/bin/go", Version: "1.22.0", Source: "manual"},
			},
		},
	}
	issues := checkMultipleInstallations(tools)
	if len(issues) != 0 {
		t.Errorf("expected no issues for same-version instances, got %d: %v", len(issues), issues)
	}
}

func TestCheckMultipleInstallations_differentVersions(t *testing.T) {
	tools := []registry.Tool{
		{
			Name:        "node",
			DisplayName: "Node.js",
			Instances: []registry.Instance{
				{Path: "/usr/local/bin/node", Version: "20.11.0", Source: "brew"},
				{Path: "/usr/bin/node", Version: "18.19.0", Source: "apt"},
			},
		},
	}
	issues := checkMultipleInstallations(tools)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Severity != SeverityError {
		t.Errorf("expected error, got %s", issues[0].Severity)
	}
	if !strings.Contains(issues[0].Title, "Node.js") {
		t.Errorf("expected title to contain 'Node.js', got %q", issues[0].Title)
	}
}

func TestCheckMultipleInstallations_singleInstance(t *testing.T) {
	tools := []registry.Tool{
		{
			Name:      "jq",
			Instances: []registry.Instance{{Path: "/usr/bin/jq", Version: "1.6"}},
		},
	}
	issues := checkMultipleInstallations(tools)
	if len(issues) != 0 {
		t.Errorf("expected no issues for single instance, got %d", len(issues))
	}
}

func TestCheckUnresolvedVersions(t *testing.T) {
	tools := []registry.Tool{
		{
			Name:      "tool-a",
			Instances: []registry.Instance{{Path: "/bin/a", Version: "", Source: "brew"}},
		},
		{
			Name:      "tool-b",
			Instances: []registry.Instance{{Path: "/bin/b", Version: "1.0", Source: "brew"}},
		},
		{
			Name:      "tool-c",
			Instances: []registry.Instance{{Path: "/bin/c", Version: "", Source: registry.SourceManual}},
		},
	}
	issues := checkUnresolvedVersions(tools)
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if !strings.Contains(issues[0].Detail, "tool-a") {
		t.Errorf("expected tool-a in detail, got %q", issues[0].Detail)
	}
	// tool-c is manual, should not be flagged.
	if strings.Contains(issues[0].Detail, "tool-c") {
		t.Errorf("manual tool should not be flagged")
	}
}

func TestCheckOutdatedSummary(t *testing.T) {
	tools := []registry.Tool{
		{
			Name:      "terraform",
			Instances: []registry.Instance{{Path: "/bin/tf", Version: "1.5.0"}},
			Latest:    "1.7.0",
		},
		{
			Name:      "kubectl",
			Instances: []registry.Instance{{Path: "/bin/k", Version: "1.28.0"}},
			Latest:    "1.28.0",
		},
	}
	issues := checkOutdatedSummary(tools, ScanMeta{})
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if !strings.Contains(issues[0].Title, "1 update") {
		t.Errorf("expected '1 update' in title, got %q", issues[0].Title)
	}
}

func TestCheckOutdatedSummary_none(t *testing.T) {
	tools := []registry.Tool{
		{
			Name:      "kubectl",
			Instances: []registry.Instance{{Path: "/bin/k", Version: "1.28.0"}},
			Latest:    "1.28.0",
		},
	}
	issues := checkOutdatedSummary(tools, ScanMeta{})
	if len(issues) != 0 {
		t.Errorf("expected no issues, got %d", len(issues))
	}
}

func TestCountBySeverity(t *testing.T) {
	issues := []Issue{
		{Severity: SeverityError},
		{Severity: SeverityWarning},
		{Severity: SeverityWarning},
		{Severity: SeverityInfo},
		{Severity: SeverityInfo},
		{Severity: SeverityInfo},
	}
	e, w, i := CountBySeverity(issues)
	if e != 1 || w != 2 || i != 3 {
		t.Errorf("CountBySeverity = (%d, %d, %d), want (1, 2, 3)", e, w, i)
	}
}

func TestHasErrors(t *testing.T) {
	if HasErrors([]Issue{{Severity: SeverityWarning}}) {
		t.Error("expected false for warnings only")
	}
	if !HasErrors([]Issue{{Severity: SeverityError}}) {
		t.Error("expected true for errors")
	}
	if HasErrors(nil) {
		t.Error("expected false for nil")
	}
}

func TestCheckMissingPMs_noInstalledTools(t *testing.T) {
	tools := []registry.Tool{
		{
			Name:     "kubectl",
			Packages: registry.PackageIDs{Brew: "kubernetes-cli"},
			// Not installed.
		},
	}
	issues := checkMissingPMs(tools)
	// Should not flag anything if the tool isn't installed.
	for _, i := range issues {
		if i.Category == CategoryPM {
			t.Errorf("should not flag PMs for non-installed tools: %s", i.Title)
		}
	}
}
