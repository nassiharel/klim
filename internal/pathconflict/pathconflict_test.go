package pathconflict

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/registry"
)

func TestAnalyzeByTool_SingleInstanceIsIgnored(t *testing.T) {
	tools := []registry.Tool{{
		Name:      "git",
		Instances: []registry.Instance{{Path: "/usr/bin/git", Version: "2.40.0"}},
	}}
	got := analyzeByTool(tools)
	if len(got) != 0 {
		t.Fatalf("single-instance tool should not appear in ByTool, got %d", len(got))
	}
}

func TestAnalyzeByTool_ShadowedListIsPATHOrdered(t *testing.T) {
	tools := []registry.Tool{{
		Name:        "node",
		DisplayName: "Node.js",
		Instances: []registry.Instance{
			{Path: "/home/u/.nvm/bin/node", Version: "20.0.0", Source: registry.SourceManual},
			{Path: "/usr/local/bin/node", Version: "18.0.0", Source: registry.SourceBrew},
			{Path: "/usr/bin/node", Version: "16.0.0", Source: registry.SourceApt},
		},
		Packages: registry.PackageIDs{Brew: "node", Apt: "nodejs"},
	}}
	rep := analyzeByTool(tools)
	if len(rep) != 1 {
		t.Fatalf("want 1 tool view, got %d", len(rep))
	}
	v := rep[0]
	if v.Active.Path != "/home/u/.nvm/bin/node" {
		t.Errorf("Active should be the first PATH instance, got %q", v.Active.Path)
	}
	if len(v.Shadowed) != 2 {
		t.Fatalf("want 2 shadowed, got %d", len(v.Shadowed))
	}
	if v.Shadowed[0].Path != "/usr/local/bin/node" || v.Shadowed[1].Path != "/usr/bin/node" {
		t.Errorf("Shadowed order wrong: %+v", v.Shadowed)
	}
	if !v.VersionConflict {
		t.Errorf("VersionConflict should be true (3 different versions)")
	}
	// Manual-source active has no uninstall command.
	if v.Active.UninstallCmd != "" {
		t.Errorf("manual-source instance should have empty UninstallCmd, got %q", v.Active.UninstallCmd)
	}
	// PM-source shadowed copy gets a remove command.
	if !strings.Contains(v.Shadowed[0].UninstallCmd, "brew uninstall") {
		t.Errorf("brew shadowed copy should have a brew uninstall command, got %q", v.Shadowed[0].UninstallCmd)
	}
}

func TestAnalyzeByTool_NoVersionConflictWhenAllMatch(t *testing.T) {
	tools := []registry.Tool{{
		Name: "kubectl",
		Instances: []registry.Instance{
			{Path: "/usr/local/bin/kubectl", Version: "1.29.0", Source: registry.SourceBrew},
			{Path: "/opt/homebrew/bin/kubectl", Version: "1.29.0", Source: registry.SourceBrew},
		},
	}}
	rep := analyzeByTool(tools)
	if len(rep) != 1 {
		t.Fatalf("want 1 view, got %d", len(rep))
	}
	if rep[0].VersionConflict {
		t.Errorf("identical versions should not be flagged as conflict")
	}
}

func TestAnalyzeByTool_DisplayNameFallback(t *testing.T) {
	tools := []registry.Tool{{
		Name: "rg", // no DisplayName
		Instances: []registry.Instance{
			{Path: "/a/rg"}, {Path: "/b/rg"},
		},
	}}
	rep := analyzeByTool(tools)
	if rep[0].DisplayName != "rg" {
		t.Errorf("want Name fallback %q, got %q", "rg", rep[0].DisplayName)
	}
}

func TestAnalyze_SortPutsConflictsFirst(t *testing.T) {
	tools := []registry.Tool{
		{
			Name: "zzz-quiet",
			Instances: []registry.Instance{
				{Path: "/a/zzz", Version: "1.0"}, {Path: "/b/zzz", Version: "1.0"},
			},
		},
		{
			Name: "aaa-conflict",
			Instances: []registry.Instance{
				{Path: "/a/aaa", Version: "1.0"}, {Path: "/b/aaa", Version: "2.0"},
			},
		},
	}
	rep := analyzeByTool(tools)
	if len(rep) != 2 {
		t.Fatalf("want 2 views, got %d", len(rep))
	}
	if rep[0].Name != "aaa-conflict" {
		t.Errorf("version-conflict tool should sort first, got %q first", rep[0].Name)
	}
}

func TestHasConflictsAndCountShadowed(t *testing.T) {
	rep := Report{ByTool: []ToolView{
		{
			Name:            "x",
			VersionConflict: true,
			Shadowed:        []InstanceView{{}, {}},
		},
		{
			Name:     "y",
			Shadowed: []InstanceView{{}},
		},
	}}
	if !rep.HasConflicts() {
		t.Errorf("HasConflicts should be true")
	}
	if rep.CountShadowed() != 3 {
		t.Errorf("CountShadowed want 3, got %d", rep.CountShadowed())
	}
}

func TestAnalyzeByDir_OrdersPATHAndFlagsDuplicates(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	sep := ":"
	if runtime.GOOS == "windows" {
		sep = ";"
	}
	// dir1 appears twice → second entry is a duplicate.
	t.Setenv("PATH", strings.Join([]string{dir1, dir2, dir1}, sep))

	tools := []registry.Tool{{
		Name: "demo",
		Instances: []registry.Instance{
			{Path: filepath.Join(dir1, "demo"), Source: registry.SourceManual},
			{Path: filepath.Join(dir2, "demo"), Source: registry.SourceManual},
		},
	}}
	dirs := analyzeByDir(tools)
	if len(dirs) != 3 {
		t.Fatalf("want 3 dir views, got %d", len(dirs))
	}
	if dirs[0].Order != 1 || dirs[1].Order != 2 || dirs[2].Order != 3 {
		t.Errorf("Order should be 1-based PATH index, got %d/%d/%d", dirs[0].Order, dirs[1].Order, dirs[2].Order)
	}
	if dirs[2].Duplicate != true || dirs[0].Duplicate {
		t.Errorf("third entry should be Duplicate=true (got %v); first should not (got %v)",
			dirs[2].Duplicate, dirs[0].Duplicate)
	}
	if len(dirs[0].Tools) != 1 || !dirs[0].Tools[0].Active {
		t.Errorf("first dir should provide demo and mark it Active")
	}
	if len(dirs[1].Tools) != 1 || dirs[1].Tools[0].Active {
		t.Errorf("second dir should provide demo but mark it shadowed (Active=false)")
	}
}
