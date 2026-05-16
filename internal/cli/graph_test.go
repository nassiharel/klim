package cli

import (
	"errors"
	"testing"

	"github.com/nassiharel/klim/internal/registry"
)

// TestGraphCmd_NoArgs verifies stray positional args are a usage
// error (CLI-CONVENTIONS.md:48).
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
// guard. Important: cmd.Flags().Set permanently flips pflag's Changed
// bit, so we reset by re-creating the command instead of just resetting
// the value (which would leave Changed=true and leak into later tests).
func TestRunGraph_RejectsWidthWithTUI(t *testing.T) {
	origTUI := graphTUI
	origBy := graphBy
	// Snapshot the original flag object so we can restore it
	// completely — including the Changed bit — after the test.
	origFlags := graphCmd.Flags()
	_ = origFlags // placate the linter if we don't use it directly

	t.Cleanup(func() {
		graphTUI = origTUI
		graphBy = origBy
		// Defensive: explicitly reset both the value and the
		// Changed bit. pflag exposes neither directly, so the
		// only safe option is to look up the flag and clear it.
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
