package cli

import (
	"errors"
	"testing"

	"github.com/nassiharel/klim/internal/registry"
)

// TestGraphCmd_NoArgs verifies stray positional args are a usage
// error (see CLI-CONVENTIONS.md "Exit codes" — extra args are usage errors, exit 2).
func TestGraphCmd_NoArgs(t *testing.T) {
	if err := graphCmd.Args(graphCmd, []string{"extra"}); err == nil {
		t.Error("graphCmd.Args(['extra']) returned nil; want error")
	}
	if err := graphCmd.Args(graphCmd, nil); err != nil {
		t.Errorf("graphCmd.Args(nil) returned %v; want nil", err)
	}
}

// TestRunGraph_RejectsInvalidBy exercises the --by validator.
func TestRunGraph_RejectsInvalidBy(t *testing.T) {
	orig := graphBy
	t.Cleanup(func() { graphBy = orig })

	graphBy = "not-a-real-grouping"
	err := runGraph(graphCmd, nil)
	if err == nil {
		t.Fatal("runGraph with bogus --by returned nil; want UsageError")
	}
	var ue *UsageError
	if !errors.As(err, &ue) {
		t.Errorf("runGraph err = %v (%T); want *UsageError", err, err)
	}
}

// TestRunGraph_RejectsWidthWithTUI exercises the --tui + --width/--height
// guard. cmd.Flags().Set permanently flips pflag's Changed bit, so the
// cleanup must explicitly reset both Value and Changed on each affected
// flag — otherwise the next test that calls runGraph with --tui would
// fail as if --width was still explicitly provided.
func TestRunGraph_RejectsWidthWithTUI(t *testing.T) {
	origTUI := graphTUI
	origBy := graphBy

	t.Cleanup(func() {
		graphTUI = origTUI
		graphBy = origBy
		// pflag exposes neither default-restoration nor a way to
		// clear Changed cleanly, so we hand-reset both fields.
		if f := graphCmd.Flags().Lookup("width"); f != nil {
			_ = f.Value.Set("0")
			f.Changed = false
		}
		if f := graphCmd.Flags().Lookup("height"); f != nil {
			_ = f.Value.Set("0")
			f.Changed = false
		}
	})

	graphBy = "category"
	graphTUI = true
	if err := graphCmd.Flags().Set("width", "80"); err != nil {
		t.Fatalf("flag set: %v", err)
	}
	err := runGraph(graphCmd, nil)
	if err == nil {
		t.Fatal("runGraph with --tui and --width returned nil; want UsageError")
	}
	var ue *UsageError
	if !errors.As(err, &ue) {
		t.Errorf("err = %v (%T); want *UsageError", err, err)
	}
}

// TestGroupByEdges_DeterministicOrder protects the map-iteration fix
// (PR-78 review): with the same input, edge order must be identical
// across runs.
func TestGroupByEdges_DeterministicOrder(t *testing.T) {
	mk := func() []registry.Tool {
		return []registry.Tool{
			{Name: "a", Category: "lang"},
			{Name: "b", Category: "lang"},
			{Name: "c", Category: "build"},
			{Name: "d", Category: "build"},
			{Name: "e", Category: "shell"},
			{Name: "f", Category: "shell"},
		}
	}
	first := buildToolGraph(mk(), "category")
	second := buildToolGraph(mk(), "category")
	if len(first.Edges) != len(second.Edges) {
		t.Fatalf("edge counts diverge: %d vs %d", len(first.Edges), len(second.Edges))
	}
	for i := range first.Edges {
		if first.Edges[i] != second.Edges[i] {
			t.Errorf("edge order differs at %d: %+v vs %+v", i, first.Edges[i], second.Edges[i])
		}
	}
}

func TestGroupByEdges_NoKeyCollisionOnPipeCharacter(t *testing.T) {
	// Regression: under the old "a|b" key scheme, the unordered
	// pairs (a, b|c) and (a|b, c) both produced key "a|b|c", so
	// only one edge would be drawn. With length-prefixed keys both
	// pairs must produce distinct entries.
	tools := []registry.Tool{
		{Name: "a", Category: "shared"},
		{Name: "b|c", Category: "shared"},
		{Name: "a|b", Category: "shared"},
		{Name: "c", Category: "shared"},
	}
	g := buildToolGraph(tools, "category")
	// 4 nodes in one bucket → C(4,2) = 6 distinct edges.
	if got := len(g.Edges); got != 6 {
		t.Errorf("len(edges) = %d; want 6 (one per unique pair, including pipe-containing names)", got)
	}
}
