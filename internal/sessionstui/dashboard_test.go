package sessionstui

import (
	"testing"
	"time"

	"github.com/nassiharel/klim/internal/agents"
)

func TestRebuildView(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	m := &Model{
		svc:     nil,
		now:     func() time.Time { return now },
		tab:     tabActive,
		groupBy: groupByProject,
		snapshot: &agents.Snapshot{Sessions: []agents.Session{
			{ID: "a", Status: agents.SessionStatusActive, LiveState: agents.StateWorking, LastModified: now.Add(-1 * time.Minute), Title: "alpha", ProjectPath: "/dev/klim"},
			{ID: "b", Status: agents.SessionStatusCompleted, LiveState: agents.StateIdle, LastModified: now.Add(-1 * time.Hour), Title: "beta"},
			{ID: "c", Status: agents.SessionStatusActive, LiveState: agents.StateWaiting, LastModified: now.Add(-2 * time.Minute), Title: "gamma", ProjectPath: "/dev/other", Starred: true},
		}},
	}

	m.rebuildView()
	// Active tab excludes the completed one and orders by:
	// starred-first, then state-rank, then mtime.
	if len(m.flat) != 2 {
		t.Fatalf("active count = %d, want 2", len(m.flat))
	}
	if m.flat[0].ID != "c" {
		t.Errorf("expected starred 'c' first, got %q", m.flat[0].ID)
	}

	m.tab = tabPrevious
	m.rebuildView()
	if len(m.flat) != 1 || m.flat[0].ID != "b" {
		t.Errorf("previous tab: got %d sessions, want [b]", len(m.flat))
	}

	m.tab = tabActive
	m.search = "alpha"
	m.rebuildView()
	if len(m.flat) != 1 || m.flat[0].ID != "a" {
		t.Errorf("search alpha: got %v, want [a]", idsOf(m.flat))
	}
}

func TestNextGroupBy(t *testing.T) {
	t.Parallel()
	if next := nextGroupBy(groupByProject); next != groupByProvider {
		t.Errorf("project → %s, want %s", next, groupByProvider)
	}
	if next := nextGroupBy(groupByProvider); next != groupByNone {
		t.Errorf("provider → %s, want %s", next, groupByNone)
	}
	if next := nextGroupBy(groupByNone); next != groupByProject {
		t.Errorf("none → %s, want %s", next, groupByProject)
	}
}

func TestProviderForSessionID(t *testing.T) {
	t.Parallel()
	tests := map[string]agents.ProviderID{
		"claude:abc":  agents.ProviderClaudeCode,
		"copilot:abc": agents.ProviderCopilotCLI,
		"bare":        "",
	}
	for in, want := range tests {
		if got := providerForSessionID(in); got != want {
			t.Errorf("providerForSessionID(%q) = %q, want %q", in, got, want)
		}
	}
}

func idsOf(in []agents.Session) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = s.ID
	}
	return out
}
