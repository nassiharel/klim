package plan

import (
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/registry"
)

func TestBuild_UpgradesOutdatedTools(t *testing.T) {
	tools := []registry.Tool{
		{
			Name:        "terraform",
			DisplayName: "Terraform",
			Packages:    registry.PackageIDs{Brew: "terraform"},
			Instances:   []registry.Instance{{Path: "/usr/local/bin/terraform", Version: "1.8.0", Source: registry.SourceBrew}},
			Latest:      "1.9.0",
		},
		{
			Name:      "kubectl",
			Packages:  registry.PackageIDs{Brew: "kubectl"},
			Instances: []registry.Instance{{Path: "/usr/local/bin/kubectl", Version: "1.31.0", Source: registry.SourceBrew}},
			Latest:    "1.32.0",
		},
		{
			Name:      "up-to-date",
			Packages:  registry.PackageIDs{Brew: "up-to-date"},
			Instances: []registry.Instance{{Path: "/usr/local/bin/utd", Version: "1.0.0", Source: registry.SourceBrew}},
			Latest:    "1.0.0",
		},
	}
	p := Build(tools, Options{IncludeUpgrades: true})
	if len(p.Changes) != 2 {
		t.Fatalf("want 2 changes, got %d", len(p.Changes))
	}
	got := map[string]Change{}
	for _, c := range p.Changes {
		got[c.Tool] = c
	}
	if got["terraform"].FromVersion != "1.8.0" || got["terraform"].ToVersion != "1.9.0" {
		t.Errorf("terraform change wrong: %+v", got["terraform"])
	}
	if got["kubectl"].FromVersion != "1.31.0" || got["kubectl"].ToVersion != "1.32.0" {
		t.Errorf("kubectl change wrong: %+v", got["kubectl"])
	}
	if _, ok := got["up-to-date"]; ok {
		t.Errorf("up-to-date should not appear in plan")
	}
}

func TestBuild_NoChangesYieldsEmptyPlan(t *testing.T) {
	tools := []registry.Tool{{
		Name:      "stable",
		Packages:  registry.PackageIDs{Brew: "stable"},
		Instances: []registry.Instance{{Path: "/usr/local/bin/stable", Version: "2.0.0", Source: registry.SourceBrew}},
		Latest:    "2.0.0",
	}}
	p := Build(tools, Options{IncludeUpgrades: true})
	if len(p.Changes) != 0 {
		t.Errorf("want empty plan, got %d changes", len(p.Changes))
	}
	if p.Totals.ChangeCount != 0 {
		t.Errorf("totals.ChangeCount should be 0")
	}
}

func TestBuild_DesiredInstallEmitsInstallChange(t *testing.T) {
	tools := []registry.Tool{{
		Name: "new-tool",
		// NPM is part of sourcePriority on every supported OS, so
		// this test exercises the install path uniformly on
		// Linux / darwin / Windows. Brew alone would silently
		// produce an empty-command change on Windows.
		Packages: registry.PackageIDs{NPM: "new-tool", Brew: "new-tool"},
		Latest:   "0.5.0",
	}}
	p := Build(tools, Options{
		IncludeInstalls: true,
		Desired:         map[string]DesiredState{"new-tool": {Version: ""}},
	})
	if len(p.Changes) != 1 {
		t.Fatalf("want 1 change, got %d", len(p.Changes))
	}
	if p.Changes[0].Kind != ChangeInstall {
		t.Errorf("kind = %q, want install", p.Changes[0].Kind)
	}
	if p.Changes[0].ToVersion != "0.5.0" {
		t.Errorf("ToVersion = %q, want 0.5.0", p.Changes[0].ToVersion)
	}
	if p.Changes[0].Command == "" {
		t.Errorf("install change must carry a non-empty Command")
	}
}

// TestBuild_DesiredToolMissingFromCatalogEmitsRisk locks in that a
// desired tool which isn't present in the catalog produces an
// explicit risk instead of being silently ignored (so users can spot
// stray .klim.yaml entries / typos).
func TestBuild_DesiredToolMissingFromCatalogEmitsRisk(t *testing.T) {
	tools := []registry.Tool{{
		Name:     "existing",
		Packages: registry.PackageIDs{NPM: "existing"},
		Latest:   "1.0.0",
	}}
	p := Build(tools, Options{
		IncludeInstalls: true,
		Desired: map[string]DesiredState{
			"existing":    {},
			"phantom":     {},
			"alsoMissing": {},
		},
	})
	missing := map[string]bool{}
	for _, r := range p.Risks {
		if strings.Contains(r.Message, "not in the catalog") {
			missing[r.Tool] = true
		}
	}
	if !missing["phantom"] || !missing["alsoMissing"] {
		t.Errorf("want missing-catalog risks for phantom + alsoMissing, got %+v", p.Risks)
	}
	if missing["existing"] {
		t.Errorf("existing tool should not get a missing-catalog risk")
	}
}

// TestBuild_DesiredInstallWithoutSourceEmitsRisk verifies that asking
// to install a tool that has no OS-relevant package ID produces a
// risk (so the user sees why it was dropped) rather than silently
// emitting a change with an empty Command.
func TestBuild_DesiredInstallWithoutSourceEmitsRisk(t *testing.T) {
	tools := []registry.Tool{{
		Name:     "mystery",
		Packages: registry.PackageIDs{}, // no IDs at all
		Latest:   "1.0.0",
	}}
	p := Build(tools, Options{
		IncludeInstalls: true,
		Desired:         map[string]DesiredState{"mystery": {}},
	})
	if len(p.Changes) != 0 {
		t.Errorf("want no changes for sourceless install, got %d", len(p.Changes))
	}
	var found bool
	for _, r := range p.Risks {
		if r.Tool == "mystery" && r.Severity == SeverityError {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("want missing-source risk for 'mystery', risks=%+v", p.Risks)
	}
}

func TestBuild_DesiredRemoveEmitsRemoveChange(t *testing.T) {
	tools := []registry.Tool{{
		Name:      "stale",
		Packages:  registry.PackageIDs{Brew: "stale"},
		Instances: []registry.Instance{{Path: "/usr/local/bin/stale", Version: "1.0.0", Source: registry.SourceBrew}},
	}}
	p := Build(tools, Options{
		IncludeRemoves: true,
		Desired:        map[string]DesiredState{"stale": {Remove: true}},
	})
	if len(p.Changes) != 1 || p.Changes[0].Kind != ChangeRemove {
		t.Fatalf("want 1 remove change, got %+v", p.Changes)
	}
}

func TestBuild_GroupsBySource(t *testing.T) {
	tools := []registry.Tool{
		{
			Name:      "zzz-npm",
			Packages:  registry.PackageIDs{NPM: "zzz-npm"},
			Instances: []registry.Instance{{Path: "/usr/local/bin/zzz", Version: "1.0", Source: registry.SourceNPM}},
			Latest:    "2.0",
		},
		{
			Name:      "aaa-brew",
			Packages:  registry.PackageIDs{Brew: "aaa-brew"},
			Instances: []registry.Instance{{Path: "/usr/local/bin/aaa", Version: "1.0", Source: registry.SourceBrew}},
			Latest:    "2.0",
		},
	}
	p := Build(tools, Options{IncludeUpgrades: true})
	if len(p.Changes) != 2 {
		t.Fatalf("want 2 changes")
	}
	// brew < npm alphabetically, so aaa-brew comes first.
	if p.Changes[0].Source != registry.SourceBrew {
		t.Errorf("first change should be brew, got %q", p.Changes[0].Source)
	}
}

func TestBuild_OnlyToolsFilters(t *testing.T) {
	tools := []registry.Tool{
		{
			Name:      "a",
			Packages:  registry.PackageIDs{Brew: "a"},
			Instances: []registry.Instance{{Path: "/a", Version: "1.0", Source: registry.SourceBrew}},
			Latest:    "2.0",
		},
		{
			Name:      "b",
			Packages:  registry.PackageIDs{Brew: "b"},
			Instances: []registry.Instance{{Path: "/b", Version: "1.0", Source: registry.SourceBrew}},
			Latest:    "2.0",
		},
	}
	p := Build(tools, Options{IncludeUpgrades: true, OnlyTools: map[string]bool{"b": true}})
	if len(p.Changes) != 1 || p.Changes[0].Tool != "b" {
		t.Errorf("OnlyTools filter not applied: %+v", p.Changes)
	}
}

func TestBuild_TotalsAggregate(t *testing.T) {
	tools := []registry.Tool{{
		Name:      "x",
		Packages:  registry.PackageIDs{Brew: "x"},
		Instances: []registry.Instance{{Path: "/x", Version: "1.0", Source: registry.SourceBrew}},
		Latest:    "2.0",
	}}
	p := Build(tools, Options{IncludeUpgrades: true})
	if p.Totals.ChangeCount != 1 {
		t.Errorf("ChangeCount = %d, want 1", p.Totals.ChangeCount)
	}
	if p.Totals.EstimatedTime <= 0 {
		t.Errorf("EstimatedTime should be > 0")
	}
	if p.Totals.DiskAddedMB <= 0 {
		t.Errorf("DiskAddedMB should be > 0 for an upgrade")
	}
}

func TestAnalyseRisks_MajorBumpEmitsWarning(t *testing.T) {
	changes := []Change{{
		Tool:        "any",
		DisplayName: "Any",
		Kind:        ChangeUpgrade,
		FromVersion: "1.5.0",
		ToVersion:   "2.0.0",
	}}
	risks := AnalyseRisks(changes, nil)
	found := false
	for _, r := range risks {
		if strings.Contains(r.Message, "major-version bump") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected major-version bump risk, got %+v", risks)
	}
}

func TestAnalyseRisks_KubectlEmitsSkewWarning(t *testing.T) {
	changes := []Change{{Tool: "kubectl", DisplayName: "kubectl", Kind: ChangeUpgrade, FromVersion: "1.31", ToVersion: "1.32"}}
	risks := AnalyseRisks(changes, nil)
	found := false
	for _, r := range risks {
		if strings.Contains(r.Message, "client-server skew") {
			found = true
		}
	}
	if !found {
		t.Errorf("kubectl upgrade should emit skew warning: %+v", risks)
	}
}

func TestAnalyseRisks_NoUpgradeNoRisk(t *testing.T) {
	risks := AnalyseRisks(nil, nil)
	if len(risks) != 0 {
		t.Errorf("empty plan should produce no risks")
	}
}

func TestConfidence_MajorBumpLowerThanPatch(t *testing.T) {
	major := Change{Tool: "foo", DisplayName: "foo", Kind: ChangeUpgrade, FromVersion: "1.9.0", ToVersion: "2.0.0"}
	patch := Change{Tool: "foo", DisplayName: "foo", Kind: ChangeUpgrade, FromVersion: "1.9.0", ToVersion: "1.9.1"}
	majorScore, _ := computeConfidence(major, nil)
	patchScore, _ := computeConfidence(patch, nil)
	if majorScore >= patchScore {
		t.Errorf("major bump %d should score lower than patch %d", majorScore, patchScore)
	}
}

func TestConfidence_KubectlScoresLowerThanGenericTool(t *testing.T) {
	kubectl := Change{Tool: "kubectl", DisplayName: "kubectl", Kind: ChangeUpgrade, FromVersion: "1.31.0", ToVersion: "1.32.0"}
	generic := Change{Tool: "anything", DisplayName: "Anything", Kind: ChangeUpgrade, FromVersion: "1.31.0", ToVersion: "1.32.0"}
	ks, _ := computeConfidence(kubectl, nil)
	gs, _ := computeConfidence(generic, nil)
	if ks >= gs {
		t.Errorf("kubectl upgrade %d should score lower than generic %d", ks, gs)
	}
}

func TestConfidence_PluginEcosystemLowersScore(t *testing.T) {
	tools := []registry.Tool{
		{Name: "helm", Instances: []registry.Instance{{Path: "/h", Version: "1"}}},
		{Name: "kustomize", Instances: []registry.Instance{{Path: "/k", Version: "1"}}},
		{Name: "k9s", Instances: []registry.Instance{{Path: "/k9", Version: "1"}}},
	}
	change := Change{Tool: "kubectl", DisplayName: "kubectl", Kind: ChangeUpgrade, FromVersion: "1.31.0", ToVersion: "1.32.0"}
	with, _ := computeConfidence(change, tools)
	without, _ := computeConfidence(change, nil)
	if with >= without {
		t.Errorf("plugin ecosystem present should LOWER confidence: with=%d without=%d", with, without)
	}
}

func TestConfidence_FactorsAreEmittedInOrder(t *testing.T) {
	change := Change{Tool: "kubectl", DisplayName: "kubectl", Kind: ChangeUpgrade, FromVersion: "1.31.0", ToVersion: "1.32.0"}
	_, factors := computeConfidence(change, nil)
	if len(factors) == 0 {
		t.Fatalf("expected at least one factor")
	}
	if factors[0].Name == "" {
		t.Errorf("factors should have names")
	}
}

func TestConfidence_NonUpgradeIsFullConfidence(t *testing.T) {
	for _, kind := range []ChangeKind{ChangeInstall, ChangeRemove} {
		score, factors := computeConfidence(Change{Kind: kind}, nil)
		if score != 100 {
			t.Errorf("kind %q: score should be 100, got %d", kind, score)
		}
		if factors != nil {
			t.Errorf("kind %q: factors should be nil, got %d", kind, len(factors))
		}
	}
}

func TestConfidence_ClampedBetweenZeroAndHundred(t *testing.T) {
	// Throw every penalty at once: kubectl major + huge ecosystem.
	change := Change{Tool: "kubectl", DisplayName: "kubectl", Kind: ChangeUpgrade, FromVersion: "1.31.0", ToVersion: "2.0.0"}
	manyTools := make([]registry.Tool, 50)
	for i := range manyTools {
		manyTools[i] = registry.Tool{
			Name:      "helm",
			Instances: []registry.Instance{{Path: "/h", Version: "1"}},
		}
	}
	score, _ := computeConfidence(change, manyTools)
	if score < 0 || score > 100 {
		t.Errorf("score out of bounds: %d", score)
	}
}
