package tui

import (
	"testing"
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
