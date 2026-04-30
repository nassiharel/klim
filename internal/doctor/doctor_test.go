package doctor

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nassiharel/clim/internal/registry"
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
	if issues[0].Severity != SeverityWarning {
		t.Errorf("expected warning, got %s", issues[0].Severity)
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

func TestCheckBrokenPATH(t *testing.T) {
	// Create a temp dir with a file (non-directory) to test that case.
	tmpDir := t.TempDir()
	fakePath := filepath.Join(tmpDir, "not-a-dir")
	if err := os.WriteFile(fakePath, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	nonExist := filepath.Join(tmpDir, "does-not-exist")

	sep := string(os.PathListSeparator)
	t.Setenv("PATH", tmpDir+sep+fakePath+sep+nonExist)

	issues := checkBrokenPATH()

	var foundNonDir, foundMissing bool
	for _, i := range issues {
		if strings.Contains(i.Title, "Non-directory") {
			foundNonDir = true
		}
		if strings.Contains(i.Title, "Missing") {
			foundMissing = true
		}
	}
	if !foundNonDir {
		t.Error("expected non-directory PATH issue")
	}
	if !foundMissing {
		t.Error("expected missing PATH issue")
	}
}

func TestCheckDuplicatePATH(t *testing.T) {
	tmpDir := t.TempDir()
	sep := string(os.PathListSeparator)

	path := tmpDir + sep + tmpDir
	t.Setenv("PATH", path)

	issues := checkDuplicatePATH()
	if len(issues) != 1 {
		t.Fatalf("expected 1 duplicate issue, got %d", len(issues))
	}
	if issues[0].Severity != SeverityWarning {
		t.Errorf("expected warning severity, got %s", issues[0].Severity)
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"  ", ""},
		{"/usr/local/bin", filepath.Clean("/usr/local/bin")},
		{"/usr/local/bin/", filepath.Clean("/usr/local/bin")},
	}
	for _, tt := range tests {
		got := normalizePath(tt.input)
		if runtime.GOOS == "windows" {
			tt.want = strings.ToLower(tt.want)
		}
		if got != tt.want {
			t.Errorf("normalizePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
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
