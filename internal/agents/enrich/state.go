package enrich

import (
	"strings"
	"time"
)

// StaleThreshold is the inactivity window after which a session is
// considered idle (rather than thinking or working). Mirrors the value
// used by ghcpCliDashboard.
const StaleThreshold = 60 * time.Second

// State is the run-state classification. It mirrors agents.LiveState
// but lives in this package to keep enrich import-free of the agents
// types (avoids a cycle when providers package both).
type State string

// State values.
const (
	StateUnknown  State = ""
	StateWorking  State = "working"
	StateThinking State = "thinking"
	StateWaiting  State = "waiting"
	StateIdle     State = "idle"
)

// Result is the output of [DeriveState] — everything a provider needs
// to populate the enrichment fields on an agents.Session.
type Result struct {
	State           State
	WaitingContext  string
	RecentActivity  string
	ToolCounts      map[string]int
	MCPServers      []string
	SubagentRuns    int // total subagent.started events seen
	BackgroundTasks int // started minus completed (>= 0)
	TurnCount       int // user.message events
	LastTimestamp   time.Time
	FirstTimestamp  time.Time

	// TerminalKind is the kind of the last event when it is a
	// terminal marker (session.end / session.stopped). Empty
	// otherwise. Callers use it to set the persisted Status.
	TerminalKind EventKind

	// Invocations groups the per-kind name → count maps populated
	// from KindSkillInvoked / KindSlashCommand / KindHookFired /
	// KindMCPToolCall / KindSubagentStarted events. Each sub-map
	// stays nil unless the corresponding event was seen — the
	// downstream marshaller treats `nil` and "empty map" identically,
	// but keeping the slot nil keeps allocations minimal for the
	// common no-skill-no-hook case.
	//
	// Parallel to agents.Invocations; the agents package's
	// ApplyEnrichment copies field-by-field to avoid an import cycle.
	Invocations Invocations
}

// Invocations is the enrich-package mirror of agents.Invocations.
// Defined here so DeriveState has somewhere to put per-kind maps
// without importing the agents package (which would create a cycle).
// See agents.Invocations for the per-field documentation; this
// struct's field set is intentionally identical so the bridge in
// agents.ApplyEnrichment is a simple field-for-field copy.
type Invocations struct {
	Skills        map[string]int
	Subagents     map[string]int
	Hooks         map[string]int
	SlashCommands map[string]int
	MCPTools      map[string]int
}

// TimedEvent pairs an Event with its timestamp. Providers stream
// these through [DeriveState] in order; the function does not sort.
type TimedEvent struct {
	Event
	Timestamp time.Time
}

// DeriveState runs the state machine over an ordered event stream
// and returns the dashboard-friendly Result. The `now` reference is
// used to compute the idle window; pass time.Now() in production and
// a fixed value in tests.
//
// The algorithm:
//
//  1. Walk every event, maintaining running counts of pending tool
//     executions, pending ask_user prompts, subagent runs, turn
//     count, tool histogram, and MCP server set.
//  2. Track first / last event timestamps (for Created / LastModified).
//  3. Remember the last assistant message text as RecentActivity.
//  4. Resolve the final state:
//     • now - lastTS >= StaleThreshold     → StateIdle
//     • pending ask_user > 0               → StateWaiting
//     • pending tool calls > 0             → StateWorking
//     • saw at least one event             → StateThinking
//     • no events at all                   → StateUnknown
//
// The function is safe to call with a nil / empty slice and never panics.
func DeriveState(events []TimedEvent, now time.Time) Result {
	r := Result{
		ToolCounts: map[string]int{},
	}
	pendingTools := 0
	pendingAsk := 0
	var askText string
	var askChoices []string
	mcpSeen := map[string]bool{}
	subagentsRunning := 0

	for _, ev := range events {
		if !ev.Timestamp.IsZero() {
			if r.FirstTimestamp.IsZero() {
				r.FirstTimestamp = ev.Timestamp
			}
			r.LastTimestamp = ev.Timestamp
		}

		switch ev.Kind {
		case KindToolStarted:
			pendingTools++
			if ev.Name != "" {
				r.ToolCounts[ev.Name]++
			}
		case KindToolCompleted:
			if pendingTools > 0 {
				pendingTools--
			}
		case KindAskUser, KindAskPermission:
			pendingAsk++
			askText = ev.Text
			askChoices = ev.Choices
		case KindAnswered:
			if pendingAsk > 0 {
				pendingAsk--
			}
			if pendingAsk == 0 {
				askText = ""
				askChoices = nil
			}
		case KindSubagentStarted:
			r.SubagentRuns++
			subagentsRunning++
			if ev.Name != "" {
				if r.Invocations.Subagents == nil {
					r.Invocations.Subagents = map[string]int{}
				}
				r.Invocations.Subagents[ev.Name]++
			}
		case KindSubagentCompleted:
			if subagentsRunning > 0 {
				subagentsRunning--
			}
		case KindMCPUsed:
			if ev.Name != "" && !mcpSeen[ev.Name] {
				mcpSeen[ev.Name] = true
				r.MCPServers = append(r.MCPServers, ev.Name)
			}
		case KindUserMessage:
			r.TurnCount++
		case KindAssistantMessage:
			if t := strings.TrimSpace(ev.Text); t != "" {
				r.RecentActivity = TruncateOneLine(t, 120)
			}
		case KindSessionEnd:
			r.TerminalKind = KindSessionEnd
		case KindSessionStopped:
			r.TerminalKind = KindSessionStopped
		case KindSkillInvoked:
			if ev.Name != "" {
				if r.Invocations.Skills == nil {
					r.Invocations.Skills = map[string]int{}
				}
				r.Invocations.Skills[ev.Name]++
			}
		case KindSlashCommand:
			if ev.Name != "" {
				if r.Invocations.SlashCommands == nil {
					r.Invocations.SlashCommands = map[string]int{}
				}
				r.Invocations.SlashCommands[ev.Name]++
			}
		case KindHookFired:
			if ev.Name != "" {
				if r.Invocations.Hooks == nil {
					r.Invocations.Hooks = map[string]int{}
				}
				r.Invocations.Hooks[ev.Name]++
			}
		case KindMCPToolCall:
			if ev.Name != "" {
				if r.Invocations.MCPTools == nil {
					r.Invocations.MCPTools = map[string]int{}
				}
				r.Invocations.MCPTools[ev.Name]++
			}
		}
	}

	r.BackgroundTasks = subagentsRunning

	// Resolve state.
	switch {
	case len(events) == 0:
		r.State = StateUnknown
	case !r.LastTimestamp.IsZero() && !now.IsZero() && now.Sub(r.LastTimestamp) >= StaleThreshold:
		r.State = StateIdle
	case pendingAsk > 0:
		r.State = StateWaiting
		r.WaitingContext = formatWaitingContext(askText, askChoices)
	case pendingTools > 0:
		r.State = StateWorking
	default:
		r.State = StateThinking
	}

	// Drop the empty map so JSON omitempty kicks in.
	if len(r.ToolCounts) == 0 {
		r.ToolCounts = nil
	}
	return r
}

// formatWaitingContext renders the ask_user prompt + choices into a
// single short string suitable for the WaitingContext field.
func formatWaitingContext(text string, choices []string) string {
	t := TruncateOneLine(strings.TrimSpace(text), 200)
	if len(choices) == 0 {
		return t
	}
	cs := strings.Join(choices, " / ")
	if t == "" {
		return TruncateOneLine(cs, 200)
	}
	return TruncateOneLine(t+" — "+cs, 200)
}

// TruncateOneLine collapses internal whitespace and trims to n runes
// (adding a horizontal ellipsis when truncation happens). Returns
// the empty string for empty input.
//
// Used by the state machine to keep RecentActivity / WaitingContext
// dashboard-friendly: no embedded newlines, no runaway lengths.
func TruncateOneLine(s string, n int) string {
	if s == "" {
		return ""
	}
	// Replace any run of whitespace (including newlines / tabs) with
	// a single space, then trim.
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || r == ' ' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	out := strings.TrimSpace(b.String())
	if n <= 0 || len([]rune(out)) <= n {
		return out
	}
	if n < 2 {
		return string([]rune(out)[:n])
	}
	rs := []rune(out)
	return string(rs[:n-1]) + "…"
}
