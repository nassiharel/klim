package doctor

import (
	"runtime"
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/registry"
)

func TestActions_DuplicatePATHHasCopyCommand(t *testing.T) {
	dup := "/tmp/klim-doctor-dup"
	sep := ":"
	if runtime.GOOS == "windows" {
		sep = ";"
	}
	t.Setenv("PATH", strings.Join([]string{dup, "/usr/bin", dup}, sep))

	issues := checkDuplicatePATH()
	if len(issues) == 0 {
		t.Fatalf("expected at least one duplicate-PATH issue")
	}
	got := issues[0]
	if got.Action.Kind != ActionCopyCommand {
		t.Errorf("Action.Kind = %q, want %q", got.Action.Kind, ActionCopyCommand)
	}
	if got.Action.Command == "" {
		t.Errorf("Action.Command must be populated")
	}
	if got.Action.Target != dup {
		t.Errorf("Action.Target = %q, want %q", got.Action.Target, dup)
	}
}

func TestActions_ShadowingJumpsToPATHView(t *testing.T) {
	tools := []registry.Tool{{
		Name:        "node",
		DisplayName: "Node.js",
		Instances: []registry.Instance{
			{Path: "/home/u/.nvm/bin/node", Version: "20", Source: "manual"},
			{Path: "/usr/local/bin/node", Version: "20", Source: "brew"},
		},
	}}
	issues := checkPATHShadowing(tools)
	if len(issues) != 1 {
		t.Fatalf("want 1 shadowing issue, got %d", len(issues))
	}
	got := issues[0]
	if got.Action.Kind != ActionJumpPathView {
		t.Errorf("Action.Kind = %q, want %q", got.Action.Kind, ActionJumpPathView)
	}
	if got.Action.Target != "node" {
		t.Errorf("Action.Target = %q, want %q (tool name)", got.Action.Target, "node")
	}
}

func TestActions_MultipleInstallationsJumpsToPATHView(t *testing.T) {
	tools := []registry.Tool{{
		Name:        "node",
		DisplayName: "Node.js",
		Instances: []registry.Instance{
			{Path: "/a/node", Version: "20", Source: "manual"},
			{Path: "/b/node", Version: "18", Source: "manual"},
		},
	}}
	issues := checkMultipleInstallations(tools)
	if len(issues) != 1 {
		t.Fatalf("want 1 issue, got %d", len(issues))
	}
	if issues[0].Action.Kind != ActionJumpPathView {
		t.Errorf("Action.Kind = %q, want %q", issues[0].Action.Kind, ActionJumpPathView)
	}
}

func TestRemovePathEntryCommand_includesEntry(t *testing.T) {
	cmd := removePathEntryCommand("/tmp/bad")
	if cmd == "" {
		t.Fatalf("expected a non-empty command")
	}
	if !strings.Contains(cmd, "/tmp/bad") {
		t.Errorf("command should reference the bad entry: %q", cmd)
	}
}
