package tui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/nassiharel/klim/internal/agents"
)

func TestAgentsBulkCapable(t *testing.T) {
	for _, sub := range []int{agentsSubPlugins, agentsSubMCPs, agentsSubSessions} {
		if !agentsBulkCapable(sub) {
			t.Errorf("sub %d should be bulk-capable", sub)
		}
	}
	for _, sub := range []int{agentsSubMarketplaces, agentsSubSkills, agentsSubCosts, agentsSubHealth} {
		if agentsBulkCapable(sub) {
			t.Errorf("sub %d should NOT be bulk-capable", sub)
		}
	}
}

func TestAgentsSelectionToggle_NamespacedPerSubTab(t *testing.T) {
	st := &agentsState{}
	agentsToggleSelection(st, agentsSubPlugins, "p1")
	agentsToggleSelection(st, agentsSubPlugins, "p2")
	agentsToggleSelection(st, agentsSubMCPs, "m1")

	if got := agentsSelectionCount(st, agentsSubPlugins); got != 2 {
		t.Errorf("plugins count = %d, want 2", got)
	}
	if got := agentsSelectionCount(st, agentsSubMCPs); got != 1 {
		t.Errorf("mcps count = %d, want 1", got)
	}
	if got := agentsSelectionCount(st, agentsSubSessions); got != 0 {
		t.Errorf("sessions count should be 0 (not touched), got %d", got)
	}

	// Re-toggling an existing id should remove it.
	agentsToggleSelection(st, agentsSubPlugins, "p1")
	if got := agentsSelectionCount(st, agentsSubPlugins); got != 1 {
		t.Errorf("after re-toggle count = %d, want 1", got)
	}
	if agentsSelected(st, agentsSubPlugins)["p1"] {
		t.Error("p1 should be deselected")
	}
}

func TestAgentsClearSelection(t *testing.T) {
	st := &agentsState{}
	agentsToggleSelection(st, agentsSubPlugins, "a")
	agentsToggleSelection(st, agentsSubMCPs, "b")
	agentsClearSelection(st, agentsSubPlugins)
	if agentsSelectionCount(st, agentsSubPlugins) != 0 {
		t.Error("plugins selection should be empty after clear")
	}
	if agentsSelectionCount(st, agentsSubMCPs) != 1 {
		t.Error("mcps selection should still have 1")
	}
}

func TestBulkHintFor_HasGuidance(t *testing.T) {
	for _, sub := range []int{agentsSubPlugins, agentsSubMCPs, agentsSubSessions} {
		if hint := bulkHintFor(sub); hint == "" {
			t.Errorf("sub %d should have a bulk hint", sub)
		}
	}
	if hint := bulkHintFor(agentsSubMarketplaces); hint != "" {
		t.Errorf("non-bulk sub-tab returned hint: %q", hint)
	}
}

func TestSetSubTab_ClearsSelectionOnChange(t *testing.T) {
	st := &agentsState{subTab: agentsSubPlugins}
	agentsToggleSelection(st, agentsSubPlugins, "x")
	if agentsSelectionCount(st, agentsSubPlugins) != 1 {
		t.Fatal("setup: selection not stored")
	}
	st.setSubTab(agentsSubMCPs)
	if agentsSelectionCount(st, agentsSubPlugins) != 0 {
		t.Error("selection should be cleared on sub-tab change")
	}
}

// TestShiftA_SelectsAllVisibleSessions pins the "delete all
// visible sessions" two-step recipe: Shift+A populates the bulk
// selection with every currently-visible row, then the user can
// fire Shift+X to delete them through the existing bulk pipeline.
// This is the keybinding the user asked for ("option to delete
// all sessions") — we keep it filter-aware so a search-narrowed
// view only deletes what's visible.
func TestShiftA_SelectsAllVisibleSessions(t *testing.T) {
	m := NewModel()
	m.activeTab = tabAgents
	m.width = 120
	m.agents = newAgentsState()
	m.agents.subTab = agentsSubSessions
	m.agents.snapshot = &agents.Snapshot{
		Sessions: []agents.Session{
			{ID: "claude:a", Name: "a", Provider: agents.ProviderClaudeCode},
			{ID: "claude:b", Name: "b", Provider: agents.ProviderClaudeCode},
			{ID: "copilot:c", Name: "c", Provider: agents.ProviderCopilotCLI},
		},
	}
	m.agents.loadedAt = time.Now()

	handled, _ := m.handleAgentsKey(tea.KeyPressMsg(tea.Key{Code: 'A', Text: "A"}))
	if !handled {
		t.Fatal("Shift+A should be handled on the Sessions sub-tab")
	}
	if got := agentsSelectionCount(m.agents, agentsSubSessions); got != 3 {
		t.Errorf("Shift+A should select all 3 visible sessions; got %d", got)
	}

	// Filter to a single provider — Shift+A should pick only the
	// filtered subset, not the full snapshot.
	agentsClearSelection(m.agents, agentsSubSessions)
	m.agents.providerFilter = agents.ProviderClaudeCode
	handled, _ = m.handleAgentsKey(tea.KeyPressMsg(tea.Key{Code: 'A', Text: "A"}))
	if !handled {
		t.Fatal("Shift+A should be handled with a provider filter active")
	}
	if got := agentsSelectionCount(m.agents, agentsSubSessions); got != 2 {
		t.Errorf("Shift+A is filter-aware; expected 2 selected, got %d", got)
	}
}

// TestShiftA_NoOpOnNonBulkSubTab — Shift+A is meaningless outside
// bulk-capable sub-tabs (Marketplaces, Skills, etc) and must not
// silently capture selection state for them.
func TestShiftA_NoOpOnNonBulkSubTab(t *testing.T) {
	m := NewModel()
	m.activeTab = tabAgents
	m.width = 120
	m.agents = newAgentsState()
	m.agents.subTab = agentsSubMarketplaces
	m.agents.snapshot = &agents.Snapshot{
		Marketplaces: []agents.Marketplace{{ID: "x", Name: "x", Provider: agents.ProviderClaudeCode}},
	}
	m.agents.loadedAt = time.Now()
	_, _ = m.handleAgentsKey(tea.KeyPressMsg(tea.Key{Code: 'A', Text: "A"}))
	if got := agentsSelectionCount(m.agents, agentsSubMarketplaces); got != 0 {
		t.Errorf("Shift+A should be a no-op on non-bulk sub-tabs; selected %d", got)
	}
}
