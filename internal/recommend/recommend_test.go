package recommend

import (
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/registry"
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

func TestRelated_FocusTagsOnly(t *testing.T) {
	pkg := registry.PackageIDs{Brew: "x", Winget: "x", Apt: "x"}
	// Focus on jq. Both fx and yq share the "json" tag with jq.
	// kubectl shares no tags with jq, so even though it's not installed
	// it must not appear in jq's related list.
	tools := []registry.Tool{
		{
			Name:      "jq",
			Tags:      []string{"json", "cli"},
			Instances: []registry.Instance{{Path: "/jq"}},
		},
		{Name: "fx", Tags: []string{"json", "tui"}, Packages: pkg},
		{Name: "yq", Tags: []string{"json", "yaml"}, Packages: pkg},
		{Name: "kubectl", Tags: []string{"kubernetes"}, Packages: pkg},
	}
	recs := Related(tools[0], tools, 5)
	if len(recs) != 2 {
		t.Fatalf("got %d recs, want 2 (fx, yq) — kubectl shares no tag with jq", len(recs))
	}
	got := []string{tools[recs[0].ToolIdx].Name, tools[recs[1].ToolIdx].Name}
	for _, n := range got {
		if n != "fx" && n != "yq" {
			t.Errorf("unexpected related tool %q (kubectl should not appear)", n)
		}
	}
}

func TestRelated_IgnoresGlobalInstalledSet(t *testing.T) {
	// Regression for the bug where /tools/jq showed different tools than
	// the TUI's "You might also like": the web previously fed Compute()
	// (which biases toward overall installed-tag frequency) then
	// post-filtered. Related must score purely off the focus tool's
	// tags so the order is identical regardless of what else is
	// installed.
	pkg := registry.PackageIDs{Brew: "x", Winget: "x", Apt: "x"}
	tools := []registry.Tool{
		{Name: "jq", Tags: []string{"json"}, Instances: []registry.Instance{{Path: "/jq"}}},
		// fx shares 1 tag with jq.
		{Name: "fx", Tags: []string{"json"}, Packages: pkg},
		// yq shares 1 tag with jq AND has a "kubernetes" tag matching
		// kubectl. Compute would boost yq because of the kubernetes
		// overlap with installed kubectl. Related must NOT.
		{Name: "yq", Tags: []string{"json", "kubernetes"}, Packages: pkg},
		// kubectl is installed; it inflates the kubernetes tag freq
		// in Compute but is irrelevant to Related (which only looks
		// at the focus tool).
		{Name: "kubectl", Tags: []string{"kubernetes"}, Instances: []registry.Instance{{Path: "/k"}}},
	}
	recs := Related(tools[0], tools, 5)
	if len(recs) != 2 {
		t.Fatalf("got %d recs, want 2", len(recs))
	}
	// fx and yq each match jq on exactly 1 tag, so they tie on score
	// and are sorted alphabetically.
	if tools[recs[0].ToolIdx].Name != "fx" || tools[recs[1].ToolIdx].Name != "yq" {
		t.Errorf("got %s, %s; want fx, yq (alphabetical tiebreaker on equal score)",
			tools[recs[0].ToolIdx].Name, tools[recs[1].ToolIdx].Name)
	}
	// Both should report 100% match since they share the same number
	// of tags with the focus.
	if recs[0].MatchPct != 100 || recs[1].MatchPct != 100 {
		t.Errorf("expected both at 100%% match, got %d%% / %d%%", recs[0].MatchPct, recs[1].MatchPct)
	}
}

func TestRelated_SkipsInstalledAndFocusItself(t *testing.T) {
	pkg := registry.PackageIDs{Brew: "x", Winget: "x", Apt: "x"}
	tools := []registry.Tool{
		{Name: "jq", Tags: []string{"json"}, Instances: []registry.Instance{{Path: "/jq"}}},
		// Already installed — must be skipped.
		{Name: "fx", Tags: []string{"json"}, Instances: []registry.Instance{{Path: "/fx"}}},
		// Same name as focus — must be skipped (defensive: prevents
		// duplicate catalog entries from polluting the related list).
		{Name: "jq", Tags: []string{"json"}, Packages: pkg},
		// Eligible.
		{Name: "yq", Tags: []string{"json"}, Packages: pkg},
	}
	recs := Related(tools[0], tools, 5)
	if len(recs) != 1 || tools[recs[0].ToolIdx].Name != "yq" {
		t.Fatalf("got %d recs (first=%v); want exactly yq", len(recs), recs)
	}
}

func TestRelated_NoTagsReturnsNil(t *testing.T) {
	tools := []registry.Tool{
		{Name: "jq", Instances: []registry.Instance{{Path: "/jq"}}},
		{Name: "yq", Tags: []string{"yaml"}, Packages: registry.PackageIDs{Brew: "yq"}},
	}
	if got := Related(tools[0], tools, 5); got != nil {
		t.Errorf("focus with no tags should return nil, got %d", len(got))
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
