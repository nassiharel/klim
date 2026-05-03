package recommend

import (
	"strings"
	"testing"

	"github.com/nassiharel/clim/internal/registry"
)

func TestCompute_Basic(t *testing.T) {
	pkg := registry.PackageIDs{Brew: "x", Winget: "x", Apt: "x"} // pass HasAnyPackageForOS on every test runner
	tools := []registry.Tool{
		{Name: "kubectl", Tags: []string{"kubernetes", "k8s", "cluster"}, Instances: []registry.Instance{{Path: "/usr/bin/kubectl"}}},
		{Name: "helm", Tags: []string{"kubernetes", "k8s", "charts"}, Instances: []registry.Instance{{Path: "/usr/bin/helm"}}},
		{Name: "stern", Tags: []string{"kubernetes", "k8s", "logs", "tail"}, Packages: pkg},
		{Name: "k9s", Tags: []string{"kubernetes", "k8s", "tui"}, Packages: pkg},
		{Name: "ansible", Tags: []string{"automation", "ssh", "agentless"}, Packages: pkg},
	}

	recs := Compute(tools)

	if len(recs) == 0 {
		t.Fatal("expected recommendations, got none")
	}

	recNames := make(map[string]bool)
	for _, r := range recs {
		recNames[tools[r.ToolIdx].Name] = true
	}
	if !recNames["stern"] {
		t.Error("expected stern in recommendations")
	}
	if !recNames["k9s"] {
		t.Error("expected k9s in recommendations")
	}
	if recNames["ansible"] {
		t.Error("ansible should not be recommended (no tag overlap)")
	}
	if recs[0].Score < recs[len(recs)-1].Score {
		t.Error("recommendations should be sorted by score descending")
	}
}

func TestCompute_ScoreOrdering(t *testing.T) {
	pkg := registry.PackageIDs{Brew: "x", Winget: "x", Apt: "x"}
	tools := []registry.Tool{
		{Name: "git", Tags: []string{"vcs", "scm"}, Instances: []registry.Instance{{Path: "/usr/bin/git"}}},
		{Name: "kubectl", Tags: []string{"kubernetes", "k8s"}, Instances: []registry.Instance{{Path: "/usr/bin/kubectl"}}},
		{Name: "helm", Tags: []string{"kubernetes", "k8s", "charts"}, Packages: pkg},
		{Name: "gh", Tags: []string{"vcs", "github"}, Packages: pkg},
	}

	recs := Compute(tools)

	if len(recs) < 2 {
		t.Fatalf("expected at least 2 recs, got %d", len(recs))
	}
	if tools[recs[0].ToolIdx].Name != "helm" {
		t.Errorf("expected helm first, got %s", tools[recs[0].ToolIdx].Name)
	}
}

func TestCompute_NoInstalledTools(t *testing.T) {
	tools := []registry.Tool{
		{Name: "git", Tags: []string{"vcs"}},
		{Name: "kubectl", Tags: []string{"kubernetes"}},
	}

	recs := Compute(tools)

	if len(recs) != 0 {
		t.Errorf("expected 0 recommendations when nothing installed, got %d", len(recs))
	}
}

func TestCompute_AllInstalled(t *testing.T) {
	tools := []registry.Tool{
		{Name: "git", Tags: []string{"vcs"}, Instances: []registry.Instance{{Path: "/usr/bin/git"}}},
		{Name: "gh", Tags: []string{"vcs"}, Instances: []registry.Instance{{Path: "/usr/bin/gh"}}},
	}

	recs := Compute(tools)

	if len(recs) != 0 {
		t.Errorf("expected 0 recommendations when all installed, got %d", len(recs))
	}
}

func TestCompute_ReasonContainsToolNames(t *testing.T) {
	pkg := registry.PackageIDs{Brew: "x", Winget: "x", Apt: "x"}
	tools := []registry.Tool{
		{Name: "kubectl", Tags: []string{"kubernetes"}, Instances: []registry.Instance{{Path: "/usr/bin/kubectl"}}},
		{Name: "stern", Tags: []string{"kubernetes", "logs"}, Packages: pkg},
	}

	recs := Compute(tools)

	if len(recs) != 1 {
		t.Fatalf("expected 1 rec, got %d", len(recs))
	}
	if recs[0].Reason == "" {
		t.Error("expected non-empty reason")
	}
	if !strings.Contains(recs[0].Reason, "kubectl") {
		t.Errorf("reason %q should mention kubectl", recs[0].Reason)
	}
}

func TestCompute_RespectsMaxCap(t *testing.T) {
	// Build a fixture with one installed tool tagged "shared" and lots
	// of candidate tools all tagged the same way. Compute should cap
	// at Max.
	pkg := registry.PackageIDs{Brew: "x", Winget: "x", Apt: "x"}
	tools := []registry.Tool{
		{Name: "anchor", Tags: []string{"shared"}, Instances: []registry.Instance{{Path: "/anchor"}}},
	}
	for i := 0; i < Max+10; i++ {
		tools = append(tools, registry.Tool{
			Name:     "candidate-" + string(rune('a'+(i%26))) + "-" + string(rune('0'+(i%10))) + "-" + string(rune('a'+i%26)),
			Tags:     []string{"shared"},
			Packages: pkg,
		})
	}
	recs := Compute(tools)
	if len(recs) > Max {
		t.Errorf("recs=%d, want <= %d", len(recs), Max)
	}
}
