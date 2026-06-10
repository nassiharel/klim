package enrich

import (
	"testing"
	"time"
)

// TestDeriveState exercises the state machine across the four
// possible terminal states plus the empty / no-timestamps edge cases.
//
// The table is organised so each row's name describes the scenario
// in plain language; the assertions check the fields that the row
// is meant to demonstrate (other fields are left unverified to keep
// rows focused).
func TestDeriveState(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	at := func(seconds int) time.Time {
		return base.Add(time.Duration(seconds) * time.Second)
	}

	tests := []struct {
		name   string
		events []TimedEvent
		now    time.Time

		wantState           State
		wantWaitingContains string
		wantRecent          string
		wantToolCounts      map[string]int
		wantMCPs            []string
		wantBgTasks         int
		wantSubagentRuns    int
		wantTurns           int
		wantTerminal        EventKind
	}{
		{
			name:      "empty stream yields unknown",
			events:    nil,
			now:       base,
			wantState: StateUnknown,
		},
		{
			name: "fresh tool call with no completion is working",
			events: []TimedEvent{
				{Event: Event{Kind: KindToolStarted, Name: "Bash"}, Timestamp: at(0)},
				{Event: Event{Kind: KindToolStarted, Name: "Read"}, Timestamp: at(1)},
				{Event: Event{Kind: KindToolCompleted, Name: "Bash"}, Timestamp: at(2)},
			},
			now:            at(3),
			wantState:      StateWorking,
			wantToolCounts: map[string]int{"Bash": 1, "Read": 1},
		},
		{
			name: "all tools completed and recent activity is thinking",
			events: []TimedEvent{
				{Event: Event{Kind: KindToolStarted, Name: "Bash"}, Timestamp: at(0)},
				{Event: Event{Kind: KindToolCompleted, Name: "Bash"}, Timestamp: at(1)},
				{Event: Event{Kind: KindAssistantMessage, Text: "  Let me check the\nfile  contents."}, Timestamp: at(2)},
			},
			now:            at(3),
			wantState:      StateThinking,
			wantRecent:     "Let me check the file contents.",
			wantToolCounts: map[string]int{"Bash": 1},
		},
		{
			name: "pending ask_user is waiting, with text in context",
			events: []TimedEvent{
				{Event: Event{Kind: KindAssistantMessage, Text: "scanning"}, Timestamp: at(0)},
				{Event: Event{Kind: KindAskUser, Text: "Proceed?", Choices: []string{"yes", "no"}}, Timestamp: at(1)},
			},
			now:                 at(2),
			wantState:           StateWaiting,
			wantWaitingContains: "Proceed?",
		},
		{
			name: "answered clears the waiting state back to thinking",
			events: []TimedEvent{
				{Event: Event{Kind: KindAskUser, Text: "Proceed?"}, Timestamp: at(0)},
				{Event: Event{Kind: KindAnswered}, Timestamp: at(1)},
			},
			now:       at(2),
			wantState: StateThinking,
		},
		{
			name: "no events for >= 60s is idle even with pending tools",
			events: []TimedEvent{
				{Event: Event{Kind: KindToolStarted, Name: "Bash"}, Timestamp: at(0)},
			},
			now:            at(120), // 2 minutes later
			wantState:      StateIdle,
			wantToolCounts: map[string]int{"Bash": 1},
		},
		{
			name: "subagents running shows background task count",
			events: []TimedEvent{
				{Event: Event{Kind: KindSubagentStarted, Name: "scout"}, Timestamp: at(0)},
				{Event: Event{Kind: KindSubagentStarted, Name: "fixer"}, Timestamp: at(1)},
				{Event: Event{Kind: KindSubagentCompleted, Name: "scout"}, Timestamp: at(2)},
			},
			now:              at(3),
			wantState:        StateThinking,
			wantBgTasks:      1,
			wantSubagentRuns: 2,
		},
		{
			name: "MCP servers deduplicated by name",
			events: []TimedEvent{
				{Event: Event{Kind: KindMCPUsed, Name: "ado"}, Timestamp: at(0)},
				{Event: Event{Kind: KindMCPUsed, Name: "kusto"}, Timestamp: at(1)},
				{Event: Event{Kind: KindMCPUsed, Name: "ado"}, Timestamp: at(2)},
			},
			now:       at(3),
			wantState: StateThinking,
			wantMCPs:  []string{"ado", "kusto"},
		},
		{
			name: "session.end is captured as terminal kind",
			events: []TimedEvent{
				{Event: Event{Kind: KindSessionStart}, Timestamp: at(0)},
				{Event: Event{Kind: KindUserMessage}, Timestamp: at(1)},
				{Event: Event{Kind: KindSessionEnd}, Timestamp: at(2)},
			},
			now:          at(3),
			wantState:    StateThinking,
			wantTurns:    1,
			wantTerminal: KindSessionEnd,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := DeriveState(tt.events, tt.now)
			if got.State != tt.wantState {
				t.Errorf("state: got %q, want %q", got.State, tt.wantState)
			}
			if tt.wantWaitingContains != "" && !contains(got.WaitingContext, tt.wantWaitingContains) {
				t.Errorf("waiting context: got %q, want it to contain %q", got.WaitingContext, tt.wantWaitingContains)
			}
			if tt.wantRecent != "" && got.RecentActivity != tt.wantRecent {
				t.Errorf("recent: got %q, want %q", got.RecentActivity, tt.wantRecent)
			}
			if !sameTools(got.ToolCounts, tt.wantToolCounts) {
				t.Errorf("tool counts: got %v, want %v", got.ToolCounts, tt.wantToolCounts)
			}
			if !sameStrs(got.MCPServers, tt.wantMCPs) {
				t.Errorf("mcps: got %v, want %v", got.MCPServers, tt.wantMCPs)
			}
			if got.BackgroundTasks != tt.wantBgTasks {
				t.Errorf("bg tasks: got %d, want %d", got.BackgroundTasks, tt.wantBgTasks)
			}
			if got.SubagentRuns != tt.wantSubagentRuns {
				t.Errorf("subagent runs: got %d, want %d", got.SubagentRuns, tt.wantSubagentRuns)
			}
			if got.TurnCount != tt.wantTurns {
				t.Errorf("turns: got %d, want %d", got.TurnCount, tt.wantTurns)
			}
			if got.TerminalKind != tt.wantTerminal {
				t.Errorf("terminal kind: got %q, want %q", got.TerminalKind, tt.wantTerminal)
			}
		})
	}
}

// TestTruncateOneLine covers whitespace collapsing and rune-aware
// truncation. Multi-byte rune edge cases matter because terminal
// widths are computed in runes, not bytes.
func TestTruncateOneLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in    string
		limit int
		want  string
	}{
		{"", 10, ""},
		{"hello world", 0, "hello world"},
		{"hello world", 100, "hello world"},
		{"hello world", 5, "hell…"},
		{"  hello\n\tworld  ", 100, "hello world"},
		{"line1\nline2", 100, "line1 line2"},
		{"αβγδε", 3, "αβ…"},
	}
	for _, tt := range tests {
		got := TruncateOneLine(tt.in, tt.limit)
		if got != tt.want {
			t.Errorf("TruncateOneLine(%q, %d) = %q, want %q", tt.in, tt.limit, got, tt.want)
		}
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func sameTools(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func sameStrs(a, b []string) bool {
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