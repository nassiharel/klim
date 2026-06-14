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

// TestDeriveState_InvocationsByKind pins that the per-kind
// invocation maps populate from the new event kinds:
// KindSkillInvoked, KindSlashCommand, KindHookFired, KindMCPToolCall,
// and per-name KindSubagentStarted. These maps are independent of
// the scalar counters (TurnCount, SubagentRuns) so existing callers
// keep their semantics.
func TestDeriveState_InvocationsByKind(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	at := func(s int) time.Time { return base.Add(time.Duration(s) * time.Second) }

	events := []TimedEvent{
		{Event: Event{Kind: KindSkillInvoked, Name: "superpowers:tdd"}, Timestamp: at(0)},
		{Event: Event{Kind: KindSkillInvoked, Name: "superpowers:tdd"}, Timestamp: at(1)},
		{Event: Event{Kind: KindSkillInvoked, Name: "superpowers:brainstorming"}, Timestamp: at(2)},
		{Event: Event{Kind: KindSlashCommand, Name: "/exit"}, Timestamp: at(3)},
		{Event: Event{Kind: KindHookFired, Name: "SessionStart:startup"}, Timestamp: at(4)},
		{Event: Event{Kind: KindHookFired, Name: "SessionStart:startup"}, Timestamp: at(5)},
		{Event: Event{Kind: KindMCPToolCall, Name: "ado-tools::repo_pull_request"}, Timestamp: at(6)},
		{Event: Event{Kind: KindSubagentStarted, Name: "Explore"}, Timestamp: at(7)},
		{Event: Event{Kind: KindSubagentStarted, Name: "Explore"}, Timestamp: at(8)},
		{Event: Event{Kind: KindSubagentStarted, Name: "general-purpose"}, Timestamp: at(9)},
	}
	got := DeriveState(events, at(10))

	if got.Invocations.Skills["superpowers:tdd"] != 2 {
		t.Errorf("Skills[tdd]=%d, want 2", got.Invocations.Skills["superpowers:tdd"])
	}
	if got.Invocations.Skills["superpowers:brainstorming"] != 1 {
		t.Errorf("Skills[brainstorming]=%d, want 1", got.Invocations.Skills["superpowers:brainstorming"])
	}
	if got.Invocations.SlashCommands["/exit"] != 1 {
		t.Errorf("SlashCommands[/exit]=%d, want 1", got.Invocations.SlashCommands["/exit"])
	}
	if got.Invocations.Hooks["SessionStart:startup"] != 2 {
		t.Errorf("Hooks[SessionStart:startup]=%d, want 2", got.Invocations.Hooks["SessionStart:startup"])
	}
	if got.Invocations.MCPTools["ado-tools::repo_pull_request"] != 1 {
		t.Errorf("MCPTools[ado-tools::repo_pull_request]=%d, want 1", got.Invocations.MCPTools["ado-tools::repo_pull_request"])
	}
	if got.Invocations.Subagents["Explore"] != 2 {
		t.Errorf("Subagents[Explore]=%d, want 2", got.Invocations.Subagents["Explore"])
	}
	if got.Invocations.Subagents["general-purpose"] != 1 {
		t.Errorf("Subagents[general-purpose]=%d, want 1", got.Invocations.Subagents["general-purpose"])
	}

	// The legacy SubagentRuns scalar must keep its existing semantic
	// (total dispatches) so downstream code that reads it doesn't
	// regress.
	if got.SubagentRuns != 3 {
		t.Errorf("SubagentRuns=%d, want 3 (sum of all dispatches)", got.SubagentRuns)
	}
}

// TestDeriveState_InvocationsEmptyByDefault pins that an event
// stream with no new-kind events leaves all Invocations sub-maps
// nil — never an empty allocated map. The Session marshaller relies
// on `len(m) == 0` to drop unpopulated sub-maps from JSON; an empty
// allocated map would still get marshalled as `{}`.
func TestDeriveState_InvocationsEmptyByDefault(t *testing.T) {
	t.Parallel()
	got := DeriveState([]TimedEvent{
		{Event: Event{Kind: KindUserMessage}, Timestamp: time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)},
	}, time.Date(2026, 6, 11, 12, 0, 1, 0, time.UTC))

	if got.Invocations.Skills != nil {
		t.Errorf("Skills should be nil with no skill events, got %v", got.Invocations.Skills)
	}
	if got.Invocations.Subagents != nil {
		t.Errorf("Subagents should be nil with no subagent events, got %v", got.Invocations.Subagents)
	}
	if got.Invocations.Hooks != nil {
		t.Errorf("Hooks should be nil with no hook events, got %v", got.Invocations.Hooks)
	}
	if got.Invocations.SlashCommands != nil {
		t.Errorf("SlashCommands should be nil with no slash events, got %v", got.Invocations.SlashCommands)
	}
	if got.Invocations.MCPTools != nil {
		t.Errorf("MCPTools should be nil with no MCP tool events, got %v", got.Invocations.MCPTools)
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
