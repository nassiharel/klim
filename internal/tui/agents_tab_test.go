package tui

import (
	"testing"
	"time"

	"github.com/nassiharel/klim/internal/agents"
)

func TestSortAgentRows_Sessions(t *testing.T) {
	now := time.Now()
	mk := func(name string, modified time.Time, turns int) agentRow {
		s := &agents.Session{LastModified: modified, TurnCount: turns}
		return agentRow{name: name, session: s}
	}
	rows := []agentRow{
		mk("c", now.Add(-1*time.Hour), 5),
		mk("a", now.Add(-2*time.Hour), 50),
		mk("b", now.Add(-30*time.Minute), 1),
	}

	// Sort by name (alphabetical, ascending).
	got := sortAgentRows(append([]agentRow(nil), rows...), agentsSubSessions, agentsSortName)
	if got[0].name != "a" || got[2].name != "c" {
		t.Errorf("name sort wrong: %v", names(got))
	}

	// Sort by modified (most recent first).
	got = sortAgentRows(append([]agentRow(nil), rows...), agentsSubSessions, agentsSortModified)
	if got[0].name != "b" || got[2].name != "a" {
		t.Errorf("modified sort wrong: %v", names(got))
	}

	// Sort by turns (highest first).
	got = sortAgentRows(append([]agentRow(nil), rows...), agentsSubSessions, agentsSortTurns)
	if got[0].name != "a" || got[1].name != "c" || got[2].name != "b" {
		t.Errorf("turns sort wrong: %v", names(got))
	}

	// Default leaves the input order alone.
	got = sortAgentRows(append([]agentRow(nil), rows...), agentsSubSessions, agentsSortDefault)
	if got[0].name != "c" {
		t.Errorf("default should preserve input order; got %v", names(got))
	}
}

func TestNextSortMode_CyclesThroughList(t *testing.T) {
	modes := []agentsSortMode{agentsSortDefault, agentsSortName, agentsSortModified}
	seen := map[agentsSortMode]bool{}
	cur := modes[0]
	for i := 0; i < len(modes)+1; i++ {
		seen[cur] = true
		cur = nextSortMode(modes, cur)
	}
	if len(seen) != len(modes) {
		t.Errorf("expected to cycle through %d modes, saw %d", len(modes), len(seen))
	}
}

func TestAgentsSnapshotCounts(t *testing.T) {
	snap := &agents.Snapshot{
		Marketplaces: []agents.Marketplace{{}},
		Plugins:      []agents.Plugin{{}, {}},
		Skills:       []agents.Skill{{}, {}, {}},
		MCPs:         []agents.MCP{{}, {}, {}, {}},
		Sessions:     []agents.Session{{}, {}, {}, {}, {}},
	}
	got := agentsSnapshotCounts(snap)
	want := [5]int{1, 2, 3, 4, 5}
	if got != want {
		t.Errorf("counts = %v, want %v", got, want)
	}
	if agentsSnapshotCounts(nil) != [5]int{} {
		t.Error("nil snapshot should return zeros")
	}
}

func names(rows []agentRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.name
	}
	return out
}
