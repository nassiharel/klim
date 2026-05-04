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
