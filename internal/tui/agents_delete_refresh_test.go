package tui

// Regression test for the user-reported "after delete its not got
// refreshed" bug in the agents Sessions sub-tab. This test simulates
// the actual TUI dispatch path:
//
//  1. Build a fixture with N Copilot sessions on disk.
//  2. Load the initial snapshot.
//  3. Fire the delete dispatch (same code path the `d` keybinding
//     triggers).
//  4. Verify the agentsDeletedMsg lands and the subsequent
//     loadAgentsCmd reload removes the session from the visible
//     snapshot.
//
// Built after the user reported delete worked end-to-end on the
// CLI but the TUI didn't refresh — so the bug must be in either the
// dispatch wiring (Cmd not executed?) or the snapshot replacement.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/agents/providers/claudecode"
	"github.com/nassiharel/klim/internal/agents/providers/copilotcli"
)

// TestDeleteFlow_RefreshesSnapshot verifies the TUI's delete dispatch
// removes the session from the next render. Pure simulation — no
// bubbletea runtime needed; we drive Update directly.
func TestDeleteFlow_RefreshesSnapshot(t *testing.T) {
	// 1. Fixture: two Copilot sessions in a fake ~/.copilot.
	fakeHome := t.TempDir()
	keepID := "keep-uuid-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	doomedID := "doomed-uuid-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	for _, sid := range []string{keepID, doomedID} {
		dir := filepath.Join(fakeHome, "session-state", sid)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		events := `{"type":"session.start","timestamp":"2026-06-10T08:00:00.000Z","data":{"sessionId":"` + sid +
			`","startTime":"2026-06-10T08:00:00.000Z","context":{"cwd":"/tmp"}}}` + "\n"
		if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(events), 0o644); err != nil {
			t.Fatalf("write events: %v", err)
		}
	}

	// 2. Swap the agentsService factory to point at our fixture, and
	//    redirect the on-disk cache path to a temp file so we don't
	//    poison the real one.
	prev := agentsService
	agentsService = func() *agents.Service {
		svc := agents.NewService(4,
			&copilotcli.Provider{HomeOverride: fakeHome},
			&claudecode.Provider{HomeOverride: t.TempDir()}, // empty claude home
		)
		return svc
	}
	defer func() { agentsService = prev }()

	// 3. Initial load.
	m := NewModel()
	m.width = 140
	m.height = 40
	m.phase = phaseDone
	m.bootStart = time.Now().Add(-time.Hour)
	m.activeTab = tabAgents
	m.agents = newAgentsState()
	m.agents.subTab = agentsSubSessions

	loadCmd := loadAgentsCmd(true)
	msg := loadCmd()
	if _, ok := msg.(agentsLoadedMsg); !ok {
		t.Fatalf("expected agentsLoadedMsg, got %T", msg)
	}
	_, _ = m.handleAgentsMsg(msg)

	gotIDs := sessionIDs(m.agents.snapshot)
	if !containsStr(gotIDs, "copilot:"+keepID) || !containsStr(gotIDs, "copilot:"+doomedID) {
		t.Fatalf("initial snapshot missing fixtures: %v", gotIDs)
	}

	// 4. Dispatch the delete the same way the `d` confirm handler does.
	deleteCmd := deleteAgentEntityCmd(agents.ProviderCopilotCLI, agents.EntitySession, "copilot:"+doomedID)
	deletedMsg := deleteCmd()
	dm, ok := deletedMsg.(agentsDeletedMsg)
	if !ok {
		t.Fatalf("expected agentsDeletedMsg, got %T", deletedMsg)
	}
	if dm.err != nil {
		t.Fatalf("delete failed: %v", dm.err)
	}

	// 5. handleAgentsMsg should return a follow-up Cmd that triggers
	//    a refresh. Execute it and verify the session is gone.
	_, followup := m.handleAgentsMsg(dm)
	if followup == nil {
		t.Fatal("agentsDeletedMsg handler returned no follow-up Cmd — refresh won't fire!")
	}
	reloadedMsg := followup()
	if _, ok := reloadedMsg.(agentsLoadedMsg); !ok {
		t.Fatalf("follow-up Cmd produced %T, want agentsLoadedMsg", reloadedMsg)
	}
	_, _ = m.handleAgentsMsg(reloadedMsg)

	gotIDs = sessionIDs(m.agents.snapshot)
	if containsStr(gotIDs, "copilot:"+doomedID) {
		t.Errorf("BUG: deleted session still in snapshot after refresh: %v", gotIDs)
	}
	if !containsStr(gotIDs, "copilot:"+keepID) {
		t.Errorf("BUG: kept session disappeared from snapshot after refresh: %v", gotIDs)
	}
}

func sessionIDs(snap *agents.Snapshot) []string {
	if snap == nil {
		return nil
	}
	ids := make([]string, 0, len(snap.Sessions))
	for _, s := range snap.Sessions {
		ids = append(ids, s.ID)
	}
	return ids
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

var _ = strings.Contains // keep imports stable if/when we extend

// TestDeleteFlow_RefreshesEvenOnError verifies the "not found"
// recovery path: when DeleteSession returns an error (e.g. the
// session dir was already gone because of a stale snapshot, a
// concurrent delete, or a manual rm), the TUI must still refresh
// the snapshot so the stale row disappears from the screen.
//
// This is the user-reported "after delete its not got refreshed"
// bug — the dispatch was returning (true, nil) on error which left
// the deleted-but-erroring session visibly on screen forever.
func TestDeleteFlow_RefreshesEvenOnError(t *testing.T) {
	// 1. Fixture: one Copilot session on disk PLUS a snapshot entry
	//    for a session whose dir doesn't exist (simulates a stale
	//    cache row).
	fakeHome := t.TempDir()
	realID := "real-uuid-aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	dir := filepath.Join(fakeHome, "session-state", realID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	prev := agentsService
	agentsService = func() *agents.Service {
		svc := agents.NewService(4,
			&copilotcli.Provider{HomeOverride: fakeHome},
			&claudecode.Provider{HomeOverride: t.TempDir()},
		)
		return svc
	}
	defer func() { agentsService = prev }()

	m := NewModel()
	m.width = 140
	m.height = 40
	m.phase = phaseDone
	m.bootStart = time.Now().Add(-time.Hour)
	m.activeTab = tabAgents
	m.agents = newAgentsState()
	m.agents.subTab = agentsSubSessions

	// Attempt to delete a session that doesn't exist on disk.
	missingID := "ghost-uuid-zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz"
	deleteCmd := deleteAgentEntityCmd(agents.ProviderCopilotCLI, agents.EntitySession, "copilot:"+missingID)
	deletedMsg := deleteCmd()
	dm, ok := deletedMsg.(agentsDeletedMsg)
	if !ok {
		t.Fatalf("expected agentsDeletedMsg, got %T", deletedMsg)
	}
	// Provider correctly returns an error for a missing session.
	if dm.err == nil {
		t.Fatal("expected error for missing session, got nil")
	}

	// Now the critical check: the handler MUST still trigger a
	// refresh. Without it the stale row stays on screen.
	_, followup := m.handleAgentsMsg(dm)
	if followup == nil {
		t.Fatal("BUG: agentsDeletedMsg with err returned no follow-up Cmd — TUI won't refresh after a failed delete!")
	}
}

// TestDeleteFromDetailPage_AutoPopsBackToList covers the user-facing
// follow-up bug: after a successful delete launched from the
// full-screen detail view, the refreshed snapshot no longer contains
// the deleted entity, so the detail view used to render the unhelpful
// "entity no longer present — press Esc to return" line until the user
// dismissed it. The fix prunes detail-stack frames whose entityID
// disappears on the next snapshot load, so the detail page auto-closes
// and the user lands back on the list.
func TestDeleteFromDetailPage_AutoPopsBackToList(t *testing.T) {
	fakeHome := t.TempDir()
	doomedID := "doomed-uuid-cccccccc-cccc-cccc-cccc-cccccccccccc"
	dir := filepath.Join(fakeHome, "session-state", doomedID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	events := `{"type":"session.start","timestamp":"2026-06-10T08:00:00.000Z","data":{"sessionId":"` + doomedID +
		`","startTime":"2026-06-10T08:00:00.000Z","context":{"cwd":"/tmp"}}}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(events), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	prev := agentsService
	agentsService = func() *agents.Service {
		return agents.NewService(4,
			&copilotcli.Provider{HomeOverride: fakeHome},
			&claudecode.Provider{HomeOverride: t.TempDir()},
		)
	}
	defer func() { agentsService = prev }()

	m := NewModel()
	m.width = 140
	m.height = 40
	m.phase = phaseDone
	m.bootStart = time.Now().Add(-time.Hour)
	m.activeTab = tabAgents
	m.agents = newAgentsState()
	m.agents.subTab = agentsSubSessions

	// Initial load + open detail page on the doomed session.
	_, _ = m.handleAgentsMsg(loadAgentsCmd(true)())
	m.agents.detailPage = true
	m.agents.detailStack = []agentDetailFrame{{
		subTab:   agentsSubSessions,
		entityID: "copilot:" + doomedID,
	}}

	// Delete the session and run the refresh follow-up.
	deletedMsg := deleteAgentEntityCmd(agents.ProviderCopilotCLI, agents.EntitySession, "copilot:"+doomedID)()
	_, followup := m.handleAgentsMsg(deletedMsg)
	if followup == nil {
		t.Fatal("delete handler returned no follow-up cmd")
	}
	_, _ = m.handleAgentsMsg(followup())

	// The detail page should have auto-closed because the entity is
	// no longer in the snapshot.
	if m.agents.detailPage {
		t.Errorf("detail page still open after deleted entity was removed")
	}
	if len(m.agents.detailStack) != 0 {
		t.Errorf("detail stack should be empty after auto-prune, got %d frames", len(m.agents.detailStack))
	}

	// And rendering the view must NOT contain the no-longer-present message.
	out := m.renderView()
	if strings.Contains(out, "entity no longer present") {
		t.Errorf("render still shows 'entity no longer present' after auto-pop")
	}
}
