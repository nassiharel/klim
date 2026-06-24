package tui

import (
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/agents/costs"
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
	out := renderSessionBody(&Model{agents: newAgentsState()}, agentRow{id: s.ID, session: s})
	if strings.Contains(out, "Invocations") {
		t.Errorf("renderSessionBody must omit Invocations block when all sub-maps empty; got:\n%s", out)
	}
}

// TestRenderSessionBody_TokenLine pins the per-session cost line: it
// shows a placeholder while loading, the total + in/out once known,
// and an error message when the load failed.
func TestRenderSessionBody_TokenLine(t *testing.T) {
	t.Parallel()
	s := &agents.Session{ID: "claude:proj", Provider: agents.ProviderClaudeCode, Title: "x"}
	row := agentRow{id: s.ID, session: s, provider: s.Provider}

	loading := &Model{agents: newAgentsState()}
	loading.agents.sessionCostLoading = map[string]bool{"claude:proj": true}
	if out := stripANSIForTest(renderSessionBody(loading, row)); !strings.Contains(out, "tokens: computing") {
		t.Errorf("loading state should show a computing placeholder; got:\n%s", out)
	}

	loaded := &Model{agents: newAgentsState()}
	loaded.agents.sessionCost = map[string]costs.Totals{"claude:proj": {Input: 982345, Output: 251456}}
	out := stripANSIForTest(renderSessionBody(loaded, row))
	for _, want := range []string{"tokens:", "1.2M", "982.3K in", "251.5K out"} {
		if !strings.Contains(out, want) {
			t.Errorf("loaded token line missing %q; got:\n%s", want, out)
		}
	}

	failed := &Model{agents: newAgentsState()}
	failed.agents.sessionCostErr = map[string]string{"claude:proj": "boom"}
	if out := stripANSIForTest(renderSessionBody(failed, row)); !strings.Contains(out, "unavailable") {
		t.Errorf("error state should show 'unavailable'; got:\n%s", out)
	}
}

// TestSessionCostMsg_RoutesThroughUpdate is the regression for the
// infinite "computing…" hang: agentSessionCostMsg must be in Update's
// agent-message allowlist, otherwise the result is silently dropped and
// sessionCostLoading never clears. Drives the REAL runtime path (Update)
// — testing handleAgentsMsg directly would mask the bug.
func TestSessionCostMsg_RoutesThroughUpdate(t *testing.T) {
	m := NewModel()
	m.activeTab = tabAgents
	m.agents = newAgentsState()
	m.agents.sessionCostLoading = map[string]bool{"claude:proj": true}

	updated, _ := m.Update(agentSessionCostMsg{id: "claude:proj", totals: costs.Totals{Input: 100, Output: 20}})
	mm := updated.(Model)
	if mm.agents.sessionCostLoading["claude:proj"] {
		t.Error("loading not cleared — agentSessionCostMsg was dropped by Update's allowlist")
	}
	if got := mm.agents.sessionCost["claude:proj"]; got.Input != 100 || got.Output != 20 {
		t.Errorf("totals not applied via Update: %+v", got)
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
	out := renderSessionBody(&Model{agents: newAgentsState()}, agentRow{id: s.ID, session: s})
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
	out := renderSessionBody(&Model{agents: newAgentsState()}, agentRow{id: s.ID, session: s})
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
