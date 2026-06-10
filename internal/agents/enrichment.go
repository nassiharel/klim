package agents

import (
	"github.com/nassiharel/klim/internal/agents/enrich"
)

// ApplyEnrichment copies the dashboard-friendly fields from a derived
// [enrich.Result] onto a Session. Providers call this after deriving
// the state from their per-event log so every provider populates the
// optional Session fields the same way.
//
// The function preserves any value the caller has already set when
// the corresponding result field is empty / zero — useful for fields
// like Title or ProjectPath which providers may have extracted from
// other sources (e.g. directory naming) before calling the enrich
// pass.
//
// Status is only overwritten when the result carries a terminal kind;
// otherwise it's left to the provider's own heuristic so a session
// without any terminal event still shows as active.
func ApplyEnrichment(s *Session, r enrich.Result) {
	if s == nil {
		return
	}
	if r.State != "" {
		s.LiveState = LiveState(r.State)
	}
	if r.WaitingContext != "" {
		s.WaitingContext = r.WaitingContext
	}
	if r.RecentActivity != "" {
		s.RecentActivity = r.RecentActivity
	}
	if len(r.ToolCounts) > 0 {
		s.ToolCounts = r.ToolCounts
	}
	if len(r.MCPServers) > 0 {
		s.MCPServers = r.MCPServers
	}
	if r.SubagentRuns > 0 {
		s.SubagentRuns = r.SubagentRuns
	}
	if r.BackgroundTasks > 0 {
		s.BackgroundTasks = r.BackgroundTasks
	}
	if r.TurnCount > 0 {
		s.TurnCount = r.TurnCount
	}
	if !r.FirstTimestamp.IsZero() && s.Created.IsZero() {
		s.Created = r.FirstTimestamp
	}
	if !r.LastTimestamp.IsZero() {
		s.LastModified = r.LastTimestamp
	}
	switch r.TerminalKind {
	case enrich.KindSessionEnd:
		s.Status = SessionStatusCompleted
	case enrich.KindSessionStopped:
		s.Status = SessionStatusStopped
	}
}