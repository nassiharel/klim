package cli

import (
	"runtime"
	"strings"
	"testing"

	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/registry"
)

func TestValidateSource(t *testing.T) {
	cases := []struct {
		in      string
		wantErr bool
	}{
		{"", false},
		{"  ", false},
		{"brew", false},
		{"winget", false},
		{"choco", false},
		{"scoop", false},
		{"apt", false},
		{"snap", false},
		{"npm", false},
		{"go", true},
		{"cargo", true},
		{"manual", true},
		{"foobar", true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			err := validateSource(tc.in)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateSource(%q) err=%v, wantErr=%v", tc.in, err, tc.wantErr)
			}
		})
	}
}

func TestResolveSource(t *testing.T) {
	cfg := &config.Config{}
	cfg.Defaults.PreferredSource = "brew"

	if got := resolveSource("scoop", cfg); got != "scoop" {
		t.Errorf("flag should win: got %q", got)
	}
	if got := resolveSource("", cfg); got != "brew" {
		t.Errorf("config default should win when no flag: got %q", got)
	}
	if got := resolveSource("  ", cfg); got != "brew" {
		t.Errorf("whitespace flag treated as empty: got %q", got)
	}
	empty := &config.Config{}
	if got := resolveSource("", empty); got != "" {
		t.Errorf("no flag, empty config → empty: got %q", got)
	}
	if got := resolveSource("", nil); got != "" {
		t.Errorf("nil config tolerated: got %q", got)
	}
}

func TestExpandTargets_Dedup(t *testing.T) {
	targets, unknown := expandTargets([]string{"jq", "fzf", "jq", "  ", ""}, nil, nil)
	if len(unknown) != 0 {
		t.Errorf("no packs → no unknown: %v", unknown)
	}
	if len(targets) != 2 || targets[0] != "jq" || targets[1] != "fzf" {
		t.Errorf("expected dedup [jq, fzf], got %v", targets)
	}
}

func TestExpandTargets_PackExpansion(t *testing.T) {
	packs := []registry.Pack{
		{Name: "k8s", ToolNames: []string{"kubectl", "helm"}},
		{Name: "data", ToolNames: []string{"jq", "yq"}},
	}
	targets, unknown := expandTargets([]string{"jq"}, []string{"k8s", "data"}, packs)
	if len(unknown) != 0 {
		t.Errorf("expected no unknown packs: %v", unknown)
	}
	want := map[string]bool{"jq": true, "kubectl": true, "helm": true, "yq": true}
	if len(targets) != len(want) {
		t.Fatalf("expected %d targets, got %v", len(want), targets)
	}
	for _, n := range targets {
		if !want[n] {
			t.Errorf("unexpected target %q", n)
		}
	}
}

func TestExpandTargets_UnknownPack(t *testing.T) {
	_, unknown := expandTargets(nil, []string{"k8s", "ghost"}, []registry.Pack{
		{Name: "k8s", ToolNames: []string{"kubectl"}},
	})
	if len(unknown) != 1 || unknown[0] != "ghost" {
		t.Errorf("expected [ghost], got %v", unknown)
	}
}

// makeTool builds a minimal registry.Tool for plan tests. installed
// controls whether an Instance is attached; latest sets the upgradable
// state when combined with a different installed version.
func makeTool(name string, installed bool, installedVer, latest string) registry.Tool {
	t := registry.Tool{
		Name:        name,
		DisplayName: strings.Title(name), //nolint:staticcheck // ASCII test fixture
		Latest:      latest,
		Packages: registry.PackageIDs{
			Brew:   name,
			Winget: "Vendor." + name,
			Apt:    name,
			Scoop:  name,
		},
	}
	if installed {
		t.Instances = []registry.Instance{{
			Path:    "/usr/local/bin/" + name,
			Version: installedVer,
			Source:  registry.SourceBrew,
		}}
	}
	return t
}

func toolMap(tools ...registry.Tool) map[string]*registry.Tool {
	m := make(map[string]*registry.Tool, len(tools))
	for i := range tools {
		m[tools[i].Name] = &tools[i]
	}
	return m
}

func TestBuildActionPlan_Install(t *testing.T) {
	tools := []registry.Tool{
		makeTool("jq", false, "", "1.7"),
		makeTool("fzf", true, "0.42", "0.42"),
	}
	plan := buildActionPlan(ActionInstall, []string{"jq", "fzf", "ghost"}, toolMap(tools...), "")

	if len(plan.toExec) != 1 || plan.toExec[0].name != "jq" {
		t.Errorf("expected jq in toExec: %+v", plan.toExec)
	}
	if len(plan.alreadyInstalled) != 1 {
		t.Errorf("expected fzf in alreadyInstalled: %v", plan.alreadyInstalled)
	}
	if len(plan.unknown) != 1 || plan.unknown[0] != "ghost" {
		t.Errorf("expected ghost in unknown: %v", plan.unknown)
	}
}

func TestBuildActionPlan_Upgrade(t *testing.T) {
	tools := []registry.Tool{
		makeTool("jq", true, "1.6", "1.7"),    // upgradable
		makeTool("fzf", true, "0.42", "0.42"), // up to date
		makeTool("rg", false, "", "14.0"),     // not installed
	}
	plan := buildActionPlan(ActionUpgrade, []string{"jq", "fzf", "rg"}, toolMap(tools...), "")

	if len(plan.toExec) != 1 || plan.toExec[0].name != "jq" {
		t.Errorf("expected jq in toExec: %+v", plan.toExec)
	}
	if len(plan.upToDate) != 1 {
		t.Errorf("expected fzf in upToDate: %v", plan.upToDate)
	}
	if len(plan.notInstalled) != 1 {
		t.Errorf("expected rg in notInstalled: %v", plan.notInstalled)
	}
}

func TestBuildActionPlan_Remove_SelfProtected(t *testing.T) {
	tools := []registry.Tool{
		makeTool("clim", true, "1.0", "1.0"),
		makeTool("jq", true, "1.7", "1.7"),
	}
	plan := buildActionPlan(ActionRemove, []string{"clim", "jq"}, toolMap(tools...), "")

	if len(plan.selfProtected) != 1 {
		t.Errorf("expected clim in selfProtected: %v", plan.selfProtected)
	}
	if len(plan.toExec) != 1 || plan.toExec[0].name != "jq" {
		t.Errorf("expected jq in toExec: %+v", plan.toExec)
	}
}

func TestBuildActionPlan_Remove_NotInstalled(t *testing.T) {
	tools := []registry.Tool{makeTool("jq", false, "", "1.7")}
	plan := buildActionPlan(ActionRemove, []string{"jq"}, toolMap(tools...), "")
	if len(plan.toExec) != 0 {
		t.Errorf("nothing to remove when not installed: %+v", plan.toExec)
	}
	if len(plan.notInstalled) != 1 {
		t.Errorf("expected jq in notInstalled: %v", plan.notInstalled)
	}
}

func TestBuildActionPlan_SourceHintHonored(t *testing.T) {
	// On linux brew is the second-priority source; on macOS it's first.
	// Either way --source=brew should resolve to brew when we have a brew id.
	tools := []registry.Tool{makeTool("jq", false, "", "1.7")}
	plan := buildActionPlan(ActionInstall, []string{"jq"}, toolMap(tools...), "brew")
	if len(plan.toExec) != 1 {
		t.Fatalf("expected one plan: %+v", plan)
	}
	if plan.toExec[0].source != "brew" {
		t.Errorf("expected source brew, got %q", plan.toExec[0].source)
	}
	if got := plan.toExec[0].cmdArgs; len(got) == 0 || got[0] != "brew" {
		t.Errorf("expected cmd to start with brew, got %v", got)
	}
}

func TestBuildActionPlan_FallbackWhenSourceUnavailable(t *testing.T) {
	// --source hint that does not match any package id should fall back
	// to PackageIDs.BestInstallSource. We populate every per-OS
	// candidate so the test is portable across linux/macOS/windows.
	pkgs := registry.PackageIDs{
		Brew:   "jq",
		Apt:    "jq",
		Snap:   "jq",
		Winget: "Stedolan.jq",
		Choco:  "jq",
		Scoop:  "jq",
		NPM:    "jq",
	}
	tools := []registry.Tool{{Name: "jq", Packages: pkgs}}
	plan := buildActionPlan(ActionInstall, []string{"jq"}, toolMap(tools...), "ghost-source")
	if len(plan.toExec) != 1 {
		t.Fatalf("expected one plan with fallback source, got %+v", plan)
	}
	got := plan.toExec[0].source
	switch runtime.GOOS {
	case "windows":
		if got != "winget" {
			t.Errorf("expected winget on windows, got %q", got)
		}
	case "darwin":
		if got != "brew" {
			t.Errorf("expected brew on darwin, got %q", got)
		}
	case "linux":
		if got != "apt" {
			t.Errorf("expected apt on linux, got %q", got)
		}
	}
}

func TestResolveUpgradePlan_NoPackageForOS(t *testing.T) {
	// PackageIDs with no field set → nothing to do.
	plan, hasAny := resolveUpgradePlan("ghost", "Ghost", registry.PackageIDs{}, "")
	if plan != nil {
		t.Errorf("expected nil plan, got %+v", plan)
	}
	if hasAny {
		t.Errorf("expected hasAny=false")
	}
}

func TestResolveRemovePlan_BasicShape(t *testing.T) {
	pkgs := registry.PackageIDs{Brew: "jq", Apt: "jq", Winget: "Stedolan.jq", Scoop: "jq"}
	plan, hasAny := resolveRemovePlan("jq", "jq", pkgs, "brew")
	if !hasAny || plan == nil {
		t.Fatalf("expected a plan: hasAny=%v plan=%v", hasAny, plan)
	}
	if plan.source != "brew" {
		t.Errorf("expected brew, got %q", plan.source)
	}
	if len(plan.cmdArgs) < 2 || plan.cmdArgs[0] != "brew" {
		t.Errorf("unexpected cmd args: %v", plan.cmdArgs)
	}
}

func TestPastTense(t *testing.T) {
	cases := map[Action]string{
		ActionInstall: "installed",
		ActionUpgrade: "upgraded",
		ActionRemove:  "removed",
	}
	for a, want := range cases {
		if got := pastTense(a); got != want {
			t.Errorf("pastTense(%v)=%q, want %q", a, got, want)
		}
	}
}

func TestTitleCase(t *testing.T) {
	if got := titleCase("install"); got != "Install" {
		t.Errorf("titleCase(install)=%q", got)
	}
	if got := titleCase(""); got != "" {
		t.Errorf("titleCase(empty) should stay empty: %q", got)
	}
	if got := titleCase("Already"); got != "Already" {
		t.Errorf("already-cap stays: %q", got)
	}
}
