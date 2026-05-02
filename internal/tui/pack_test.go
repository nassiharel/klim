package tui

import (
	"strings"
	"testing"

	"github.com/nassiharel/clim/internal/registry"
)

func TestBuildPackInstallItems(t *testing.T) {
	tools := []registry.Tool{
		{
			Name:        "git",
			DisplayName: "Git",
			Instances:   []registry.Instance{{Path: "/usr/bin/git", Version: "2.43.0"}},
			Packages:    registry.PackageIDs{Winget: "Git.Git", Brew: "git", Apt: "git"},
		},
		{
			Name:        "gh",
			DisplayName: "GitHub CLI",
			// Not installed — no instances.
			Packages: registry.PackageIDs{Winget: "GitHub.cli", Brew: "gh", Apt: "gh"},
		},
		{
			Name:        "fzf",
			DisplayName: "fzf",
			// Not installed.
			Packages: registry.PackageIDs{Winget: "junegunn.fzf", Brew: "fzf"},
		},
	}

	// Stub all PMs as available so BestInstallSource works deterministically.
	registry.SetPMAvailableFunc(func(_ registry.InstallSource) bool { return true })
	t.Cleanup(func() { registry.SetPMAvailableFunc(nil) })

	pack := registry.Pack{
		Name:        "test-pack",
		DisplayName: "Test Pack",
		ToolNames:   []string{"git", "gh", "fzf", "unknown-tool"},
	}

	items := buildPackInstallItems(tools, pack)

	if len(items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(items))
	}

	// git — already installed → skipped.
	if items[0].status != packItemSkipped {
		t.Errorf("git: status = %d, want skipped", items[0].status)
	}
	if items[0].errMsg != "already installed" {
		t.Errorf("git: errMsg = %q, want 'already installed'", items[0].errMsg)
	}

	// gh — not installed, has packages → pending.
	if items[1].status != packItemPending {
		t.Errorf("gh: status = %d, want pending", items[1].status)
	}
	if len(items[1].cmdArgs) == 0 {
		t.Error("gh: expected non-empty cmdArgs")
	}

	// fzf — not installed, has packages → pending.
	if items[2].status != packItemPending {
		t.Errorf("fzf: status = %d, want pending", items[2].status)
	}

	// unknown-tool — not in catalog → skipped.
	if items[3].status != packItemSkipped {
		t.Errorf("unknown-tool: status = %d, want skipped", items[3].status)
	}
	if items[3].errMsg != "not in catalog" {
		t.Errorf("unknown-tool: errMsg = %q, want 'not in catalog'", items[3].errMsg)
	}
}

func TestBuildPackRemoveItems(t *testing.T) {
	tools := []registry.Tool{
		{
			Name:        "git",
			DisplayName: "Git",
			Instances:   []registry.Instance{{Path: "/usr/bin/git", Source: registry.SourceBrew}},
			Packages:    registry.PackageIDs{Brew: "git"},
		},
		{
			Name:        "gh",
			DisplayName: "GitHub CLI",
			// Not installed.
			Packages: registry.PackageIDs{Brew: "gh"},
		},
	}

	pack := registry.Pack{
		Name:      "test-pack",
		ToolNames: []string{"git", "gh", "missing"},
	}

	items := buildPackRemoveItems(tools, pack)

	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}

	// git — installed, has remove args → pending.
	if items[0].status != packItemPending {
		t.Errorf("git: status = %d, want pending", items[0].status)
	}
	if len(items[0].cmdArgs) == 0 {
		t.Error("git: expected non-empty cmdArgs for removal")
	}

	// gh — not installed → skipped.
	if items[1].status != packItemSkipped {
		t.Errorf("gh: status = %d, want skipped", items[1].status)
	}

	// missing — not in catalog → skipped.
	if items[2].status != packItemSkipped {
		t.Errorf("missing: status = %d, want skipped", items[2].status)
	}
}

func TestCountPackSkipped(t *testing.T) {
	items := []packItem{
		{status: packItemSkipped},
		{status: packItemPending},
		{status: packItemSkipped},
		{status: packItemDone},
		{status: packItemSkipped},
	}

	got := countPackSkipped(items)
	if got != 3 {
		t.Errorf("countPackSkipped() = %d, want 3", got)
	}
}

func TestCountPackSkipped_Empty(t *testing.T) {
	got := countPackSkipped(nil)
	if got != 0 {
		t.Errorf("countPackSkipped(nil) = %d, want 0", got)
	}
}

func TestPackSummary_Success(t *testing.T) {
	m := Model{
		packItems: []packItem{
			{status: packItemDone},
			{status: packItemDone},
			{status: packItemSkipped, errMsg: "already installed"},
		},
	}
	s := m.packSummary()
	if s != "✓ 2 succeeded, 1 skipped" {
		t.Errorf("unexpected summary: %q", s)
	}
}

func TestPackSummary_Cancelled(t *testing.T) {
	m := Model{
		packCancelled: true,
		packItems: []packItem{
			{status: packItemDone},
			{status: packItemSkipped, errMsg: "cancelled"},
		},
	}
	s := m.packSummary()
	if s != "⚠ Cancelled — 1 succeeded, 1 skipped" {
		t.Errorf("unexpected summary: %q", s)
	}
}

func TestPackSkipDoesNotDoubleCount(t *testing.T) {
	// Build a model with pack items simulating an in-progress operation.
	m := Model{
		packItems: []packItem{
			{name: "a", status: packItemRunning, cmdArgs: []string{"echo", "a"}},
			{name: "b", status: packItemPending, cmdArgs: []string{"echo", "b"}},
		},
		packInstalling: true,
		packDone:       0,
	}

	// User presses "s" to skip — should mark next pending item.
	skipped := false
	for i := range m.packItems {
		if m.packItems[i].status == packItemPending {
			m.packItems[i].status = packItemSkipped
			m.packItems[i].errMsg = "skipped"
			m.packDone++
			skipped = true
			break
		}
	}
	if !skipped {
		t.Fatal("expected to skip a pending item")
	}
	if m.packDone != 1 {
		t.Errorf("after skip: expected done=1, got %d", m.packDone)
	}
	if m.packItems[1].status != packItemSkipped {
		t.Errorf("expected item b to be skipped, got %d", m.packItems[1].status)
	}

	// packItemDoneMsg arrives for running item a — should increment done.
	if m.packItems[0].status == packItemRunning {
		m.packItems[0].status = packItemDone
		m.packDone++
	}
	if m.packDone != 2 {
		t.Errorf("after complete: expected done=2, got %d", m.packDone)
	}

	// Verify no double-count: item b was already skipped, won't be touched.
	if m.packItems[1].status != packItemSkipped {
		t.Errorf("skipped item should stay skipped, got %d", m.packItems[1].status)
	}
}

func TestBuildPackInstallItems_AllInstalled(t *testing.T) {
	tools := []registry.Tool{
		{
			Name:      "git",
			Instances: []registry.Instance{{Path: "/usr/bin/git"}},
		},
		{
			Name:      "gh",
			Instances: []registry.Instance{{Path: "/usr/bin/gh"}},
		},
	}

	pack := registry.Pack{
		Name:      "all-installed",
		ToolNames: []string{"git", "gh"},
	}

	items := buildPackInstallItems(tools, pack)

	for _, item := range items {
		if item.status != packItemSkipped {
			t.Errorf("%s: status = %d, want skipped (all installed)", item.name, item.status)
		}
	}
}

func TestBuildPackRemoveItems_NoneInstalled(t *testing.T) {
	tools := []registry.Tool{
		{Name: "git"}, // no instances
		{Name: "gh"},  // no instances
	}

	pack := registry.Pack{
		Name:      "none-installed",
		ToolNames: []string{"git", "gh"},
	}

	items := buildPackRemoveItems(tools, pack)

	for _, item := range items {
		if item.status != packItemSkipped {
			t.Errorf("%s: status = %d, want skipped (none installed)", item.name, item.status)
		}
	}
}

func TestBuildPackInstallItems_AllToolsNotInCatalog(t *testing.T) {
	tools := []registry.Tool{
		{Name: "git", Packages: registry.PackageIDs{Winget: "Git.Git"}},
	}

	pack := registry.Pack{
		Name:      "phantom-pack",
		ToolNames: []string{"nonexistent-a", "nonexistent-b", "nonexistent-c"},
	}

	items := buildPackInstallItems(tools, pack)

	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(items))
	}
	for _, item := range items {
		if item.status != packItemSkipped {
			t.Errorf("%s: status = %d, want skipped", item.name, item.status)
		}
		if item.errMsg != "not in catalog" {
			t.Errorf("%s: errMsg = %q, want 'not in catalog'", item.name, item.errMsg)
		}
	}
}

func TestBuildPackRemoveItems_AllToolsNotInCatalog(t *testing.T) {
	tools := []registry.Tool{
		{Name: "git", Instances: []registry.Instance{{Path: "/usr/bin/git"}}},
	}

	pack := registry.Pack{
		Name:      "phantom-pack",
		ToolNames: []string{"nonexistent-a", "nonexistent-b"},
	}

	items := buildPackRemoveItems(tools, pack)

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	for _, item := range items {
		if item.status != packItemSkipped {
			t.Errorf("%s: status = %d, want skipped", item.name, item.status)
		}
		if item.errMsg != "not in catalog" {
			t.Errorf("%s: errMsg = %q, want 'not in catalog'", item.name, item.errMsg)
		}
	}
}

func TestBuildPackInstallItems_MixedCatalogAndMissing(t *testing.T) {
	registry.SetPMAvailableFunc(func(_ registry.InstallSource) bool { return true })
	t.Cleanup(func() { registry.SetPMAvailableFunc(nil) })

	tools := []registry.Tool{
		{Name: "git", Packages: registry.PackageIDs{Winget: "Git.Git", Brew: "git"}},
	}

	pack := registry.Pack{
		Name:      "mixed-pack",
		ToolNames: []string{"git", "phantom-tool"},
	}

	items := buildPackInstallItems(tools, pack)

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	// git — not installed, in catalog → pending.
	if items[0].status != packItemPending {
		t.Errorf("git: status = %d, want pending", items[0].status)
	}
	// phantom-tool — not in catalog → skipped.
	if items[1].status != packItemSkipped {
		t.Errorf("phantom-tool: status = %d, want skipped", items[1].status)
	}
	if items[1].errMsg != "not in catalog" {
		t.Errorf("phantom-tool: errMsg = %q, want 'not in catalog'", items[1].errMsg)
	}
}

// --- Recommendation tests ---

func TestComputeRecommendations_Basic(t *testing.T) {
	pkg := registry.PackageIDs{Brew: "x", Winget: "x"} // at least one PM so HasAnyPackageForOS passes
	tools := []registry.Tool{
		{Name: "kubectl", Tags: []string{"kubernetes", "k8s", "cluster"}, Instances: []registry.Instance{{Path: "/usr/bin/kubectl"}}},
		{Name: "helm", Tags: []string{"kubernetes", "k8s", "charts"}, Instances: []registry.Instance{{Path: "/usr/bin/helm"}}},
		{Name: "stern", Tags: []string{"kubernetes", "k8s", "logs", "tail"}, Packages: pkg},
		{Name: "k9s", Tags: []string{"kubernetes", "k8s", "tui"}, Packages: pkg},
		{Name: "ansible", Tags: []string{"automation", "ssh", "agentless"}, Packages: pkg},
	}

	recs := computeRecommendations(tools)

	if len(recs) == 0 {
		t.Fatal("expected recommendations, got none")
	}

	recNames := make(map[string]bool)
	for _, r := range recs {
		recNames[tools[r.toolIdx].Name] = true
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
	if recs[0].score < recs[len(recs)-1].score {
		t.Error("recommendations should be sorted by score descending")
	}
}

func TestComputeRecommendations_ScoreOrdering(t *testing.T) {
	pkg := registry.PackageIDs{Brew: "x", Winget: "x"}
	tools := []registry.Tool{
		{Name: "git", Tags: []string{"vcs", "scm"}, Instances: []registry.Instance{{Path: "/usr/bin/git"}}},
		{Name: "kubectl", Tags: []string{"kubernetes", "k8s"}, Instances: []registry.Instance{{Path: "/usr/bin/kubectl"}}},
		{Name: "helm", Tags: []string{"kubernetes", "k8s", "charts"}, Packages: pkg},
		{Name: "gh", Tags: []string{"vcs", "github"}, Packages: pkg},
	}

	recs := computeRecommendations(tools)

	if len(recs) < 2 {
		t.Fatalf("expected at least 2 recs, got %d", len(recs))
	}
	if tools[recs[0].toolIdx].Name != "helm" {
		t.Errorf("expected helm first, got %s", tools[recs[0].toolIdx].Name)
	}
}

func TestComputeRecommendations_NoInstalledTools(t *testing.T) {
	tools := []registry.Tool{
		{Name: "git", Tags: []string{"vcs"}},
		{Name: "kubectl", Tags: []string{"kubernetes"}},
	}

	recs := computeRecommendations(tools)

	if len(recs) != 0 {
		t.Errorf("expected 0 recommendations when nothing installed, got %d", len(recs))
	}
}

func TestComputeRecommendations_AllInstalled(t *testing.T) {
	tools := []registry.Tool{
		{Name: "git", Tags: []string{"vcs"}, Instances: []registry.Instance{{Path: "/usr/bin/git"}}},
		{Name: "gh", Tags: []string{"vcs"}, Instances: []registry.Instance{{Path: "/usr/bin/gh"}}},
	}

	recs := computeRecommendations(tools)

	if len(recs) != 0 {
		t.Errorf("expected 0 recommendations when all installed, got %d", len(recs))
	}
}

func TestComputeRecommendations_ReasonContainsToolNames(t *testing.T) {
	pkg := registry.PackageIDs{Brew: "x", Winget: "x"}
	tools := []registry.Tool{
		{Name: "kubectl", Tags: []string{"kubernetes"}, Instances: []registry.Instance{{Path: "/usr/bin/kubectl"}}},
		{Name: "stern", Tags: []string{"kubernetes", "logs"}, Packages: pkg},
	}

	recs := computeRecommendations(tools)

	if len(recs) != 1 {
		t.Fatalf("expected 1 rec, got %d", len(recs))
	}
	if recs[0].reason == "" {
		t.Error("expected non-empty reason")
	}
	if !strings.Contains(recs[0].reason, "kubectl") {
		t.Errorf("reason %q should mention kubectl", recs[0].reason)
	}
}
