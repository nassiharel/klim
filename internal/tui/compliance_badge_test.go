package tui

import (
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/compliance"
	"github.com/nassiharel/klim/internal/config"
	"github.com/nassiharel/klim/internal/registry"
)

// modelWithCompliance returns a Model wired with a compliance index
// built against the supplied policy + tools. Hides the boilerplate so
// each assertion stays focused on the behavior under test.
func modelWithCompliance(t *testing.T, policy *compliance.Policy, tools []registry.Tool, blockInstalls bool) Model {
	t.Helper()
	idx := compliance.BuildIndex(policy, tools)
	cfg := config.Default()
	cfg.Compliance.BlockInstalls = blockInstalls
	return Model{
		complianceIndex: idx,
		tools:           tools,
		cfg:             cfg,
	}
}

// strippedANSI returns s with lipgloss color escape sequences removed
// so we can assert on visible text without coupling to the exact ANSI
// codes lipgloss emits.
func strippedANSI(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == 0x1b:
			in = true
		case in && r == 'm':
			in = false
		case in:
			// drop
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func TestComplianceBadge_Violation(t *testing.T) {
	policy := &compliance.Policy{
		Name:         "Test Policy",
		BlockedTools: []string{"badtool"},
	}
	tools := []registry.Tool{
		{Name: "badtool", DisplayName: "Bad Tool"},
	}
	m := modelWithCompliance(t, policy, tools, false)

	got := strippedANSI(m.complianceBadge("badtool"))
	if got != "✗ blocked" {
		t.Errorf("violation badge = %q, want %q", got, "✗ blocked")
	}
}

func TestComplianceBadge_Warning(t *testing.T) {
	policy := &compliance.Policy{
		Name:            "Test Policy",
		AllowedLicenses: []string{"MIT"},
	}
	tools := []registry.Tool{
		{
			Name:       "warntool",
			Instances:  []registry.Instance{{Path: "/usr/bin/warntool", Version: "1.0", Source: registry.SourceBrew}},
			GitHubInfo: &registry.GitHubInfo{License: "BSD-3-Clause"},
		},
	}
	m := modelWithCompliance(t, policy, tools, false)

	got := strippedANSI(m.complianceBadge("warntool"))
	if got != "⚠ policy" {
		t.Errorf("warning badge = %q, want %q", got, "⚠ policy")
	}
}

func TestComplianceBadge_CompliantReturnsEmpty(t *testing.T) {
	policy := &compliance.Policy{Name: "Test Policy"}
	tools := []registry.Tool{
		{Name: "git", Instances: []registry.Instance{{Path: "/usr/bin/git", Version: "2.43.0", Source: registry.SourceBrew}}},
	}
	m := modelWithCompliance(t, policy, tools, false)

	if got := m.complianceBadge("git"); got != "" {
		t.Errorf("expected empty badge for compliant tool, got %q", got)
	}
}

func TestComplianceBadge_NoPolicyReturnsEmpty(t *testing.T) {
	m := Model{} // no policy, no index
	if got := m.complianceBadge("anything"); got != "" {
		t.Errorf("expected empty badge with no policy, got %q", got)
	}
}

func TestComplianceVerdictChip_Violation(t *testing.T) {
	policy := &compliance.Policy{
		Name:         "Test Policy",
		BlockedTools: []string{"badtool"},
	}
	tools := []registry.Tool{
		{Name: "badtool", DisplayName: "Bad Tool"},
	}
	m := modelWithCompliance(t, policy, tools, false)

	got := strippedANSI(m.complianceVerdictChip("badtool"))
	if !strings.Contains(got, "POLICY VIOLATION") {
		t.Errorf("verdict chip should say POLICY VIOLATION, got %q", got)
	}
}

func TestComplianceVerdictChip_CompliantHidden(t *testing.T) {
	policy := &compliance.Policy{Name: "Test Policy"}
	tools := []registry.Tool{
		{Name: "git", Instances: []registry.Instance{{Path: "/usr/bin/git", Version: "2.43.0", Source: registry.SourceBrew}}},
	}
	m := modelWithCompliance(t, policy, tools, false)

	if got := m.complianceVerdictChip("git"); got != "" {
		t.Errorf("compliant tool should have no verdict chip, got %q", got)
	}
}

// TestRenderToolComplianceSection_ShowsViolationAndPolicyName guards
// the "page-level summary" use case: opening detail for a blocked
// tool must clearly explain why it's blocked AND name the policy.
func TestRenderToolComplianceSection_ShowsViolationAndPolicyName(t *testing.T) {
	policy := &compliance.Policy{
		Name:         "Engineering Policy",
		BlockedTools: []string{"badtool"},
	}
	tools := []registry.Tool{
		{Name: "badtool", DisplayName: "Bad Tool"},
	}
	m := modelWithCompliance(t, policy, tools, false)

	out := strippedANSI(m.renderToolComplianceSection(tools[0]))
	for _, want := range []string{"Violation", "Engineering Policy", "blocked by policy"} {
		if !strings.Contains(out, want) {
			t.Errorf("section should contain %q, got:\n%s", want, out)
		}
	}
}

// TestRenderToolComplianceSection_ShowsBlockInstallNotice asserts the
// install-gate notice fires only when BlockInstalls is enabled. Without
// this gate, users would see an alarming "install blocked" message
// even when their config still allows the install.
func TestRenderToolComplianceSection_ShowsBlockInstallNotice(t *testing.T) {
	policy := &compliance.Policy{
		Name:         "Test Policy",
		BlockedTools: []string{"badtool"},
	}
	tools := []registry.Tool{
		{Name: "badtool", DisplayName: "Bad Tool"},
	}

	// BlockInstalls=true → notice appears.
	mOn := modelWithCompliance(t, policy, tools, true)
	if got := strippedANSI(mOn.renderToolComplianceSection(tools[0])); !strings.Contains(got, "install/upgrade blocked") {
		t.Errorf("BlockInstalls=true should show install-blocked notice, got:\n%s", got)
	}

	// BlockInstalls=false → still shows the violation but no install gate.
	mOff := modelWithCompliance(t, policy, tools, false)
	if got := strippedANSI(mOff.renderToolComplianceSection(tools[0])); strings.Contains(got, "install/upgrade blocked") {
		t.Errorf("BlockInstalls=false should NOT show install-blocked notice, got:\n%s", got)
	}
}

// TestRenderToolComplianceSection_ReturnsEmptyWithoutPolicy ensures the
// detail page stays uncluttered for the common "no policy configured"
// case. The divider in renderDetailBody only renders when this returns
// a non-empty string, so empty == hidden divider.
func TestRenderToolComplianceSection_ReturnsEmptyWithoutPolicy(t *testing.T) {
	m := Model{cfg: config.Default()} // no index
	if got := m.renderToolComplianceSection(registry.Tool{Name: "git"}); got != "" {
		t.Errorf("expected empty section without policy, got %q", got)
	}
}
