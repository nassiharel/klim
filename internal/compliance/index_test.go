package compliance

import (
	"testing"

	"github.com/nassiharel/clim/internal/registry"
)

func TestIndex_NoPolicy(t *testing.T) {
	idx := BuildIndex(nil, nil)
	if idx.HasPolicy() {
		t.Error("expected HasPolicy=false")
	}
	if idx.Status("git") != StatusUnknown {
		t.Errorf("status = %v, want Unknown", idx.Status("git"))
	}
	if ok, _ := idx.CanInstall("anything"); !ok {
		t.Error("expected CanInstall=true with no policy")
	}
}

func TestIndex_BlockedTool(t *testing.T) {
	policy := &Policy{
		Name:         "test",
		BlockedTools: []string{"badtool"},
	}
	tools := []registry.Tool{
		{Name: "git", Instances: []registry.Instance{{Source: "brew", Version: "2.50"}}},
		{Name: "badtool", Instances: []registry.Instance{{Source: "brew", Version: "1.0"}}},
	}
	idx := BuildIndex(policy, tools)

	if idx.Status("git") != StatusCompliant {
		t.Errorf("git status = %v, want Compliant", idx.Status("git"))
	}
	if idx.Status("badtool") != StatusViolation {
		t.Errorf("badtool status = %v, want Violation", idx.Status("badtool"))
	}
	ok, reason := idx.CanInstall("badtool")
	if ok {
		t.Error("expected CanInstall=false for blocked tool")
	}
	if reason == "" {
		t.Error("expected non-empty block reason")
	}
}

func TestIndex_AllowedSources(t *testing.T) {
	policy := &Policy{
		Name:           "test",
		AllowedSources: []string{"brew", "winget"},
	}
	tools := []registry.Tool{
		{Name: "good", Instances: []registry.Instance{{Source: "brew", Version: "1.0"}}},
		{Name: "bad", Instances: []registry.Instance{{Source: "scoop", Version: "1.0"}}},
	}
	idx := BuildIndex(policy, tools)

	if idx.Status("good") != StatusCompliant {
		t.Errorf("good status = %v, want Compliant", idx.Status("good"))
	}
	if idx.Status("bad") != StatusViolation {
		t.Errorf("bad status = %v, want Violation", idx.Status("bad"))
	}
}

func TestIndex_BlockedLicense(t *testing.T) {
	policy := &Policy{
		Name:            "test",
		BlockedLicenses: []string{"GPL-3.0"},
	}
	tools := []registry.Tool{
		{Name: "gpltool", Instances: []registry.Instance{{Source: "brew", Version: "1.0"}},
			GitHubInfo: &registry.GitHubInfo{License: "GPL-3.0"}},
	}
	idx := BuildIndex(policy, tools)
	if idx.Status("gpltool") != StatusViolation {
		t.Errorf("gpltool status = %v, want Violation", idx.Status("gpltool"))
	}
}

func TestIndex_AllowedLicenseWarning(t *testing.T) {
	policy := &Policy{
		Name:            "test",
		AllowedLicenses: []string{"MIT", "Apache-2.0"},
	}
	tools := []registry.Tool{
		{Name: "tool", Instances: []registry.Instance{{Source: "brew", Version: "1.0"}},
			GitHubInfo: &registry.GitHubInfo{License: "BSD-3-Clause"}},
	}
	idx := BuildIndex(policy, tools)
	if idx.Status("tool") != StatusWarning {
		t.Errorf("status = %v, want Warning", idx.Status("tool"))
	}
}

func TestIndex_Counts(t *testing.T) {
	policy := &Policy{
		Name:            "test",
		BlockedTools:    []string{"bad"},
		AllowedLicenses: []string{"MIT"},
	}
	tools := []registry.Tool{
		{Name: "good", Instances: []registry.Instance{{Source: "brew", Version: "1.0"}},
			GitHubInfo: &registry.GitHubInfo{License: "MIT"}},
		{Name: "bad", Instances: []registry.Instance{{Source: "brew", Version: "1.0"}}},
		{Name: "warn", Instances: []registry.Instance{{Source: "brew", Version: "1.0"}},
			GitHubInfo: &registry.GitHubInfo{License: "BSD"}},
	}
	idx := BuildIndex(policy, tools)
	c, w, v := idx.Counts()
	if c != 1 || w != 1 || v != 1 {
		t.Errorf("counts = %d/%d/%d, want 1/1/1", c, w, v)
	}
}

// TestIndex_RequiredMissing_StillInstallable guards against a regression
// where required-but-not-installed tools were marked StatusViolation +
// CanInstall=false, which made it impossible to install the very tools
// the policy was demanding the user install.
func TestIndex_RequiredMissing_StillInstallable(t *testing.T) {
	policy := &Policy{
		Name: "requires-git",
		RequiredTools: []RequiredTool{
			{Name: "git"},
		},
	}
	// git is in the catalog but not installed.
	tools := []registry.Tool{
		{Name: "git"},
	}
	idx := BuildIndex(policy, tools)

	// Status surfaces the violation so the doctor view / badge stays honest…
	if got := idx.Status("git"); got != StatusViolation {
		t.Errorf("Status(git) = %v, want StatusViolation (so doctor surfaces it)", got)
	}
	// …but CanInstall must permit the install — installing IS the fix.
	ok, reason := idx.CanInstall("git")
	if !ok {
		t.Errorf("CanInstall(git) = false (%q); should be true so the user can install the required tool", reason)
	}
}

// TestIndex_BlockedToolVsRequiredMissing verifies the two violation
// kinds behave differently w.r.t. CanInstall.
func TestIndex_BlockedToolVsRequiredMissing(t *testing.T) {
	policy := &Policy{
		Name:         "mixed",
		BlockedTools: []string{"banned"},
		RequiredTools: []RequiredTool{
			{Name: "needed"},
		},
	}
	tools := []registry.Tool{
		{Name: "banned"},
		{Name: "needed"},
	}
	idx := BuildIndex(policy, tools)

	if ok, _ := idx.CanInstall("banned"); ok {
		t.Error("blocked tool must remain CanInstall=false")
	}
	if ok, _ := idx.CanInstall("needed"); !ok {
		t.Error("required-missing tool must be CanInstall=true so the user can install it")
	}
}

func TestIndex_ApplyVulnSeverities(t *testing.T) {
	policy := &Policy{
		Name:            "test",
		MaxVulnSeverity: "high",
	}
	tools := []registry.Tool{
		{Name: "node"},
		{Name: "git"},
	}
	idx := BuildIndex(policy, tools)
	idx.ApplyVulnSeverities(map[string]string{
		"node": "CRITICAL", // >= HIGH → block
		"git":  "MEDIUM",   // <  HIGH → allow
	})
	if ok, reason := idx.CanInstall("node"); ok {
		t.Errorf("CRITICAL CVE on node should block install (threshold=HIGH); reason=%q", reason)
	}
	if ok, _ := idx.CanInstall("git"); !ok {
		t.Errorf("MEDIUM CVE on git should NOT block install when threshold=HIGH")
	}
}

func TestIndex_ApplyVulnSeverities_NoThresholdNoOp(t *testing.T) {
	policy := &Policy{Name: "test"} // no MaxVulnSeverity set
	idx := BuildIndex(policy, []registry.Tool{{Name: "node"}})
	idx.ApplyVulnSeverities(map[string]string{"node": "CRITICAL"})
	if ok, _ := idx.CanInstall("node"); !ok {
		t.Error("with no MaxVulnSeverity, vuln data should NOT block installs")
	}
}

func TestIndex_ApplyVulnSeverities_BadThresholdNoOp(t *testing.T) {
	policy := &Policy{Name: "test", MaxVulnSeverity: "bogus"}
	idx := BuildIndex(policy, []registry.Tool{{Name: "node"}})
	idx.ApplyVulnSeverities(map[string]string{"node": "CRITICAL"})
	if ok, _ := idx.CanInstall("node"); !ok {
		t.Error("invalid threshold should be a no-op (config.Validate warns separately)")
	}
}
