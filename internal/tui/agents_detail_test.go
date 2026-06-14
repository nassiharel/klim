package tui

import (
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/agents"
)

func TestWindowDetailList(t *testing.T) {
	cases := []struct {
		name                      string
		termHeight, total, cursor int
		wantStart, wantEnd        int
	}{
		{"empty list", 30, 0, 0, 0, 0},
		{"fits entirely", 30, 8, 3, 0, 8},
		{"cursor near top, windowed", 24, 100, 2, 0, 10},
		{"cursor mid, centered", 24, 100, 20, 15, 25},
		{"cursor at end, clamps", 24, 100, 99, 90, 100},
		{"tiny terminal floors at 5, centered", 5, 100, 50, 48, 53},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end := windowDetailList(tc.termHeight, tc.total, tc.cursor)
			if start != tc.wantStart || end != tc.wantEnd {
				t.Errorf("windowDetailList(%d, %d, %d) = (%d, %d); want (%d, %d)",
					tc.termHeight, tc.total, tc.cursor, start, end, tc.wantStart, tc.wantEnd)
			}
		})
	}
}

// TestRenderSessionBody_OmitsInvocationsBlockWhenEmpty pins that
// a Session whose Invocations is fully empty does NOT render an
// "Invocations" header. The block is opt-in based on signal —
// today's Copilot sessions without hooks shouldn't show an empty
// box.
func TestRenderSessionBody_OmitsInvocationsBlockWhenEmpty(t *testing.T) {
	t.Parallel()
	s := &agents.Session{
		ID:        "claude:abc",
		Provider:  agents.ProviderClaudeCode,
		Title:     "fix tile renderer",
		TurnCount: 5,
	}
	out := renderSessionBody(s)
	if strings.Contains(out, "Invocations") {
		t.Errorf("renderSessionBody must omit Invocations block when all sub-maps empty; got:\n%s", out)
	}
}

// TestRenderSessionBody_RendersAllPopulatedInvocations pins the
// five labelled rows that appear when each kind has entries. Each
// row uses the kind's lowercase label and lists `name ×count`
// (the ×N suffix is omitted when count == 1) joined with `, `.
func TestRenderSessionBody_RendersAllPopulatedInvocations(t *testing.T) {
	t.Parallel()
	s := &agents.Session{
		ID:       "claude:abc",
		Provider: agents.ProviderClaudeCode,
		Invocations: agents.Invocations{
			Skills:        map[string]int{"superpowers:tdd": 2, "superpowers:brainstorming": 1},
			Subagents:     map[string]int{"Explore": 3},
			Hooks:         map[string]int{"SessionStart:startup": 1},
			SlashCommands: map[string]int{"/exit": 1},
			MCPTools:      map[string]int{"ado-tools::repo_pull_request": 4},
		},
	}
	out := renderSessionBody(s)
	if !strings.Contains(out, "Invocations") {
		t.Fatalf("expected Invocations header; got:\n%s", out)
	}
	// Each row's label.
	for _, want := range []string{"skills", "sub-agents", "hooks", "slash cmds", "mcp tools"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing row label %q; got:\n%s", want, out)
		}
	}
	// Counts > 1 render with ×N suffix; count == 1 renders bare.
	for _, want := range []string{"superpowers:tdd ×2", "Explore ×3", "ado-tools::repo_pull_request ×4"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing populated entry %q; got:\n%s", want, out)
		}
	}
	for _, want := range []string{"superpowers:brainstorming", "/exit", "SessionStart:startup"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing single-count entry %q; got:\n%s", want, out)
		}
	}
}

// TestRenderSessionBody_PartialInvocationsOnlyRendersPopulatedRows
// pins that absent sub-maps produce NO rows (not an empty row, no
// "skills: —" placeholder). Keeps the detail pane compact when a
// session only used hooks.
func TestRenderSessionBody_PartialInvocationsOnlyRendersPopulatedRows(t *testing.T) {
	t.Parallel()
	s := &agents.Session{
		ID:       "copilot:xyz",
		Provider: agents.ProviderCopilotCLI,
		Invocations: agents.Invocations{
			Hooks: map[string]int{"postToolUse": 5},
		},
	}
	out := renderSessionBody(s)
	if !strings.Contains(out, "Invocations") {
		t.Fatalf("expected Invocations header for hooks-only session; got:\n%s", out)
	}
	if !strings.Contains(out, "hooks") || !strings.Contains(out, "postToolUse") {
		t.Errorf("expected hooks row populated; got:\n%s", out)
	}
	for _, banned := range []string{"skills", "sub-agents", "slash cmds", "mcp tools"} {
		if strings.Contains(out, banned) {
			t.Errorf("unpopulated row label %q leaked; got:\n%s", banned, out)
		}
	}
}
