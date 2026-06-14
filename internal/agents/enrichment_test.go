package agents

import (
	"testing"

	"github.com/nassiharel/klim/internal/agents/enrich"
)

// TestApplyEnrichment_CopiesInvocations pins that the bridge between
// enrich.Result and Session correctly transfers each per-kind
// invocation map. Same shape, same keys, same counts — the bridge
// is a field-for-field copy, not a re-derivation.
func TestApplyEnrichment_CopiesInvocations(t *testing.T) {
	t.Parallel()
	r := enrich.Result{
		Invocations: enrich.Invocations{
			Skills:        map[string]int{"superpowers:tdd": 2},
			Subagents:     map[string]int{"Explore": 3},
			Hooks:         map[string]int{"SessionStart:startup": 1},
			SlashCommands: map[string]int{"/exit": 1},
			MCPTools:      map[string]int{"ado-tools::repo_pull_request": 4},
		},
	}
	var s Session
	ApplyEnrichment(&s, r)

	if s.Invocations.Skills["superpowers:tdd"] != 2 {
		t.Errorf("Skills not copied: %v", s.Invocations.Skills)
	}
	if s.Invocations.Subagents["Explore"] != 3 {
		t.Errorf("Subagents not copied: %v", s.Invocations.Subagents)
	}
	if s.Invocations.Hooks["SessionStart:startup"] != 1 {
		t.Errorf("Hooks not copied: %v", s.Invocations.Hooks)
	}
	if s.Invocations.SlashCommands["/exit"] != 1 {
		t.Errorf("SlashCommands not copied: %v", s.Invocations.SlashCommands)
	}
	if s.Invocations.MCPTools["ado-tools::repo_pull_request"] != 4 {
		t.Errorf("MCPTools not copied: %v", s.Invocations.MCPTools)
	}
}

// TestApplyEnrichment_EmptyInvocationsLeavesNil pins that an empty
// enrich.Result leaves Session.Invocations zero (every sub-map nil),
// so [Invocations.IsEmpty] reports true and Session.MarshalJSON
// drops the wrapper.
func TestApplyEnrichment_EmptyInvocationsLeavesNil(t *testing.T) {
	t.Parallel()
	var s Session
	ApplyEnrichment(&s, enrich.Result{})

	if !s.Invocations.IsEmpty() {
		t.Errorf("Invocations should be empty after copy of zero Result; got %+v", s.Invocations)
	}
}

// TestApplyEnrichment_PreservesExistingInvocations pins that a
// non-empty Session.Invocations is NOT clobbered when the enrich
// result has nothing to add. Same defensive pattern as TurnCount /
// ToolCounts handling above — providers may have populated
// Invocations from a side channel before calling enrich.
func TestApplyEnrichment_PreservesExistingInvocations(t *testing.T) {
	t.Parallel()
	s := Session{
		Invocations: Invocations{
			Skills: map[string]int{"superpowers:tdd": 7},
		},
	}
	ApplyEnrichment(&s, enrich.Result{})

	if s.Invocations.Skills["superpowers:tdd"] != 7 {
		t.Errorf("pre-populated Skills clobbered; got %v", s.Invocations.Skills)
	}
}
