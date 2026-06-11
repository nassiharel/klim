package cli

import (
	"testing"
	"time"

	"github.com/nassiharel/klim/internal/agents"
)

// TestFilterSessions exercises the pure filter pipeline used by
// `klim agents sessions list` (and reused by stats / files via the
// same package-level entry point).
//
// Not t.Parallel — filterSessions writes to lastSessionsFilterErr.
func TestFilterSessions(t *testing.T) {
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	sessions := []agents.Session{
		{
			ID:           "claude:a",
			Provider:     agents.ProviderClaudeCode,
			ProjectPath:  "/dev/klim",
			Repository:   "klim",
			Title:        "fix bug in dashboard",
			LiveState:    agents.StateWorking,
			Status:       agents.SessionStatusActive,
			LastModified: now.Add(-1 * time.Hour),
			Starred:      true,
		},
		{
			ID:           "claude:b",
			Provider:     agents.ProviderClaudeCode,
			ProjectPath:  "/dev/other",
			Repository:   "other",
			LiveState:    agents.StateIdle,
			Status:       agents.SessionStatusActive,
			LastModified: now.Add(-48 * time.Hour),
		},
		{
			ID:           "copilot:c",
			Provider:     agents.ProviderCopilotCLI,
			ProjectPath:  "/dev/klim",
			LiveState:    agents.StateWaiting,
			Status:       agents.SessionStatusActive,
			LastModified: now.Add(-10 * time.Minute),
		},
	}

	tests := []struct {
		name     string
		status   string
		project  string
		starred  bool
		since    string
		until    string
		provider string
		want     []string
	}{
		{
			name: "no filters returns all",
			want: []string{"claude:a", "claude:b", "copilot:c"},
		},
		{
			name:   "status filter on live state",
			status: "waiting",
			want:   []string{"copilot:c"},
		},
		{
			name:   "status filter on persisted status",
			status: "active",
			want:   []string{"claude:a", "claude:b", "copilot:c"},
		},
		{
			name:    "project substring matches multiple",
			project: "klim",
			want:    []string{"claude:a", "copilot:c"},
		},
		{
			name:    "starred only",
			starred: true,
			want:    []string{"claude:a"},
		},
		{
			name:  "since 2h excludes day-old sessions",
			since: "2h",
			want:  []string{"claude:a", "copilot:c"},
		},
		{
			name:     "provider filter",
			provider: "claude-code",
			want:     []string{"claude:a", "claude:b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// NOT t.Parallel — subtests share lastSessionsFilterErr.
			got := filterSessions(sessions, tt.status, tt.project, tt.starred, tt.since, tt.until, tt.provider, now)
			if lastSessionsFilterErr != nil {
				t.Fatalf("unexpected filter err: %v", lastSessionsFilterErr)
			}
			ids := make([]string, 0, len(got))
			for _, s := range got {
				ids = append(ids, s.ID)
			}
			if !sameStrSlice(ids, tt.want) {
				t.Errorf("got %v, want %v", ids, tt.want)
			}
		})
	}
}

func TestFilterSessionsBadSince(t *testing.T) {
	// Not t.Parallel — mutates package-level lastSessionsFilterErr.
	defer func() { lastSessionsFilterErr = nil }()
	in := []agents.Session{{ID: "x"}}
	_ = filterSessions(in, "", "", false, "yesterday", "", "", time.Now())
	if lastSessionsFilterErr == nil {
		t.Errorf("expected lastSessionsFilterErr to be set for invalid --since")
	}
}

// TestSortSessions covers the supported sort keys and the reverse
// modifier. Each scenario is intentionally small so the expected
// ordering is obvious from the input.
func TestSortSessions(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	mk := func(id string, ls agents.LiveState, turns int, modOff, createdOff time.Duration, title string) agents.Session {
		return agents.Session{
			ID:           id,
			LiveState:    ls,
			TurnCount:    turns,
			LastModified: now.Add(-modOff),
			Created:      now.Add(-createdOff),
			Title:        title,
		}
	}
	in := []agents.Session{
		mk("a", agents.StateIdle, 5, 1*time.Hour, 5*time.Hour, "alpha"),
		mk("b", agents.StateWorking, 20, 5*time.Hour, 1*time.Hour, "beta"),
		mk("c", agents.StateWaiting, 1, 30*time.Minute, 30*time.Minute, "gamma"),
	}

	tests := []struct {
		name    string
		key     string
		reverse bool
		want    []string
	}{
		{"modified default", "modified", false, []string{"c", "a", "b"}},
		{"modified reversed", "modified", true, []string{"b", "a", "c"}},
		{"created descending", "created", false, []string{"c", "b", "a"}},
		{"turns descending", "turns", false, []string{"b", "a", "c"}},
		{"state ordering (working > waiting > idle)", "state", false, []string{"b", "c", "a"}},
		{"title ascending", "title", false, []string{"a", "b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cp := make([]agents.Session, len(in))
			copy(cp, in)
			sortSessions(cp, tt.key, tt.reverse)
			ids := make([]string, len(cp))
			for i, s := range cp {
				ids[i] = s.ID
			}
			if !sameStrSlice(ids, tt.want) {
				t.Errorf("sort %q reverse=%v: got %v, want %v", tt.key, tt.reverse, ids, tt.want)
			}
		})
	}
}

func TestFindSession(t *testing.T) {
	t.Parallel()
	in := []agents.Session{
		{ID: "claude:abcdef", Title: "first"},
		{ID: "copilot:fedcba", ProjectPath: "/dev/klim"},
		{ID: "claude:bbccdd", Title: "second"},
	}
	tests := []struct {
		name string
		q    string
		want string
		ok   bool
	}{
		{"exact id", "claude:abcdef", "claude:abcdef", true},
		{"bare uuid", "abcdef", "claude:abcdef", true},
		{"unique substring on project", "klim", "copilot:fedcba", true},
		{"unique substring on title", "second", "claude:bbccdd", true},
		{"ambiguous substring fails", "c", "", false},
		{"missing", "zzz", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := findSession(in, tt.q)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v (got id %q)", ok, tt.ok, got.ID)
			}
			if ok && got.ID != tt.want {
				t.Errorf("id = %q, want %q", got.ID, tt.want)
			}
		})
	}
}

func TestComputeStats(t *testing.T) {
	t.Parallel()
	in := []agents.Session{
		{Provider: agents.ProviderClaudeCode, LiveState: agents.StateWorking, Status: agents.SessionStatusActive, TurnCount: 10, ToolCounts: map[string]int{"Bash": 3}, MCPServers: []string{"ado"}, Group: "klim"},
		{Provider: agents.ProviderClaudeCode, LiveState: agents.StateIdle, Status: agents.SessionStatusActive, TurnCount: 4, ToolCounts: map[string]int{"Read": 2}, Group: "klim"},
		{Provider: agents.ProviderCopilotCLI, LiveState: agents.StateThinking, Status: agents.SessionStatusActive, TurnCount: 1, MCPServers: []string{"kusto"}, Group: "other"},
	}
	stats := computeStats(in)
	if stats.Total != 3 {
		t.Errorf("total = %d, want 3", stats.Total)
	}
	if stats.TotalTurns != 15 {
		t.Errorf("total turns = %d, want 15", stats.TotalTurns)
	}
	if stats.TotalToolCalls != 5 {
		t.Errorf("total tool calls = %d, want 5", stats.TotalToolCalls)
	}
	if stats.ByLiveState["working"] != 1 || stats.ByLiveState["idle"] != 1 {
		t.Errorf("by live state = %v", stats.ByLiveState)
	}
	if stats.ByProvider["claude-code"] != 2 {
		t.Errorf("by provider claude-code = %d, want 2", stats.ByProvider["claude-code"])
	}
	if len(stats.TopProjects) == 0 || stats.TopProjects[0].Name != "klim" {
		t.Errorf("top projects[0] = %+v, want klim", stats.TopProjects[0])
	}
}

func sameStrSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
