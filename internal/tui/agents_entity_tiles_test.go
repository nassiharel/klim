package tui

// Tests for the per-entity tile renderers used by the non-session
// Agents sub-tabs (Marketplaces, Plugins, Skills, MCPs). Verifies
// dispatch, the neutral-border / selected-cyan-border rule, and
// per-entity state classification.

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/agents"
)

// TestRenderAgentsTiles_DispatchesByEntity walks each payload
// pointer kind and confirms the right entity-tile renderer ran by
// checking for a marker substring unique to each tile.
func TestRenderAgentsTiles_DispatchesByEntity(t *testing.T) {
	t.Parallel()
	rows := []agentRow{
		{id: "1", marketplace: &agents.Marketplace{Name: "MKMARK", Provider: agents.ProviderClaudeCode, Installed: true}},
		{id: "2", plugin: &agents.Plugin{Name: "PLMARK", Provider: agents.ProviderClaudeCode, Installed: true, Enabled: true}},
		{id: "3", skill: &agents.Skill{Name: "SKMARK", Provider: agents.ProviderClaudeCode, Enabled: true}},
		{id: "4", mcp: &agents.MCP{Name: "MCMARK", Provider: agents.ProviderClaudeCode, Transport: "stdio", Enabled: true}},
	}
	out := stripANSIForTest(renderAgentsTiles(rows, -1, 200))
	for _, marker := range []string{"MKMARK", "PLMARK", "SKMARK", "MCMARK"} {
		if !strings.Contains(out, marker) {
			t.Errorf("expected %q in rendered tiles output", marker)
		}
	}
}

// TestRenderAgentsTiles_SkipsSessionRows confirms session rows are
// quietly skipped — session tile rendering goes through a different
// renderer, but a mixed slice shouldn't crash this one.
func TestRenderAgentsTiles_SkipsSessionRows(t *testing.T) {
	t.Parallel()
	rows := []agentRow{
		{id: "s1", session: &agents.Session{ID: "claude:session-marker-XYZ"}},
	}
	out := renderAgentsTiles(rows, -1, 200)
	if strings.Contains(out, "session-marker-XYZ") {
		t.Errorf("session row leaked into entity-tile output")
	}
}

// TestEntityBorder_NeutralUnselectedColoredSelected pins the
// border-color rule the user specifically asked for: borders MUST
// be neutral except on the cursor-selected tile (which gets cyan).
func TestEntityBorder_NeutralUnselectedColoredSelected(t *testing.T) {
	t.Parallel()
	idle := entityBorder(40, false).Render("body")
	sel := entityBorder(40, true).Render("body")
	if idle == sel {
		t.Error("selected border must differ from idle border")
	}
	// The dim color (cyberFGDim = #90a4b0) must appear in the idle
	// render; the bright cyan (cyberPrimary = #00d9ff) must appear
	// in the selected render.
	if !strings.Contains(idle, "90;164;176") && !strings.Contains(idle, "144;164;176") {
		t.Errorf("idle border should use cyberFGDim, got:\n%s", idle)
	}
	if !strings.Contains(sel, "0;217;255") {
		t.Errorf("selected border should use cyberPrimary, got:\n%s", sel)
	}
}

// TestPluginState_Mapping covers the enabled / disabled / catalog
// classifier that drives the dot glyph and badge text.
func TestPluginState_Mapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		plugin agents.Plugin
		want   string
	}{
		{"enabled when installed+enabled", agents.Plugin{Installed: true, Enabled: true}, "enabled"},
		{"disabled when installed but not enabled", agents.Plugin{Installed: true, Enabled: false}, "disabled"},
		{"catalog when not installed", agents.Plugin{Installed: false, Enabled: false}, "catalog"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pluginState(&c.plugin); got != c.want {
				t.Errorf("pluginState = %q, want %q", got, c.want)
			}
		})
	}
}

// TestRenderEachEntityTile_StaysWithinHeight makes sure every entity
// tile renders to exactly tileHeight rows, which keeps the grid
// aligned when sub-tabs mix sizes.
func TestRenderEachEntityTile_StaysWithinHeight(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		out  string
	}{
		{"marketplace", renderMarketplaceTile(agents.Marketplace{Name: "x", Provider: agents.ProviderClaudeCode}, 50, false)},
		{"plugin", renderPluginTile(agents.Plugin{Name: "x", Provider: agents.ProviderClaudeCode, Installed: true, Enabled: true}, 50, false)},
		{"skill", renderSkillTile(agents.Skill{Name: "x", Provider: agents.ProviderClaudeCode, Enabled: true}, false, 50, false)},
		{"mcp", renderMCPTile(agents.MCP{Name: "x", Provider: agents.ProviderClaudeCode, Transport: "stdio", Enabled: true}, 50, false)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lines := strings.Split(c.out, "\n")
			if len(lines) != tileHeight {
				t.Errorf("%s tile: got %d lines, want %d", c.name, len(lines), tileHeight)
			}
			w0 := lipgloss.Width(lines[0])
			for i, ln := range lines {
				if w := lipgloss.Width(ln); w != w0 {
					t.Errorf("%s tile line %d width %d != line 0 width %d", c.name, i, w, w0)
				}
			}
		})
	}
}
