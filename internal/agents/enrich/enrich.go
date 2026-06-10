// Package enrich provides provider-agnostic helpers for deriving
// dashboard-friendly metadata from raw agent session event logs.
//
// The CLI / TUI session dashboards need information that no single
// provider's transcript schema exposes directly: a live "what is this
// session doing right now?" state, a smart project group, a copy-paste
// resume command, a humanized last-modified time. Rather than re-derive
// these in every provider, the providers shape their per-event
// vocabularies into the generic [Event] type and call this package's
// pure functions.
//
// All functions here are deterministic and side-effect-free except for
// optional git probing via [BranchAtCwd] (which only runs `git` if the
// binary is on PATH and the cwd is a worktree).
package enrich

// EventKind classifies an event from a session's append-only log.
// Providers translate their raw event types into one of these
// categories so the state machine can run on a single vocabulary.
type EventKind string

// EventKind values understood by the state machine.
const (
	// KindOther is the catch-all for events that don't influence
	// the live state (telemetry, debug, low-signal messages).
	KindOther EventKind = ""

	// KindToolStarted marks the beginning of a tool call. The Name
	// field carries the tool name (used for ToolCounts).
	KindToolStarted EventKind = "tool.started"

	// KindToolCompleted marks the end of a tool call. The Name field
	// should match the started event so the pending count balances.
	KindToolCompleted EventKind = "tool.completed"

	// KindAskUser is the agent blocking on a user question. The Text
	// field carries the prompt; Choices carries any multiple-choice
	// options.
	KindAskUser EventKind = "ask_user"

	// KindAskPermission is the agent blocking on a permission grant.
	// Same fields as KindAskUser.
	KindAskPermission EventKind = "ask_permission"

	// KindAnswered cancels any pending ask_user / ask_permission.
	// Emitted when the user has supplied the requested input.
	KindAnswered EventKind = "answered"

	// KindSubagentStarted / KindSubagentCompleted track background
	// task counts. The Name field carries the subagent's tag.
	KindSubagentStarted   EventKind = "subagent.started"
	KindSubagentCompleted EventKind = "subagent.completed"

	// KindMCPUsed marks an MCP server interaction. The Name field
	// carries the server name; collected into Result.MCPServers.
	KindMCPUsed EventKind = "mcp.used"

	// KindUserMessage / KindAssistantMessage count toward TurnCount.
	// AssistantMessage's Text field feeds RecentActivity.
	KindUserMessage      EventKind = "user.message"
	KindAssistantMessage EventKind = "assistant.message"

	// KindSessionStart / KindSessionEnd / KindSessionStopped are
	// terminal markers. The state machine treats SessionStart as
	// the canonical "first event" timestamp; the others propagate
	// to the persisted Status (handled by the caller).
	KindSessionStart   EventKind = "session.start"
	KindSessionEnd     EventKind = "session.end"
	KindSessionStopped EventKind = "session.stopped"
)

// Event is a provider-neutral view of one entry in a session log.
// Providers fill the fields they have and leave the rest zero; the
// state machine tolerates missing data.
type Event struct {
	Kind    EventKind
	Name    string   // tool / subagent / MCP server name
	Text    string   // assistant message text, ask_user prompt
	Choices []string // ask_user multiple-choice options
}