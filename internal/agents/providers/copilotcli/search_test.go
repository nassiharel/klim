package copilotcli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSessionTexts_ExtractsUserAndAssistantText(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "sessions", "abc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	events := `{"type":"session.start","timestamp":"2026-05-15T08:00:00Z","data":{"sessionId":"abc","title":"docs"}}
{"type":"user.message","timestamp":"2026-05-15T08:00:01Z","data":{"sessionId":"abc","text":"hello copilot"}}
{"type":"model.response","timestamp":"2026-05-15T08:00:02Z","data":{"sessionId":"abc","response":"hi back"}}
{"type":"tool.invocation","timestamp":"2026-05-15T08:00:03Z","data":{"sessionId":"abc","text":"running bash"}}
`
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(events), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	p := &Provider{HomeOverride: home}
	texts, err := p.SessionTexts(context.Background())
	if err != nil {
		t.Fatalf("SessionTexts: %v", err)
	}
	if len(texts) != 1 {
		t.Fatalf("expected 1 session, got %d", len(texts))
	}
	st := texts[0]
	if st.SessionID != "copilot:abc" {
		t.Errorf("session id = %q", st.SessionID)
	}
	if st.Title != "docs" {
		t.Errorf("title = %q", st.Title)
	}
	if len(st.Lines) < 2 {
		t.Fatalf("expected ≥2 lines, got %d: %+v", len(st.Lines), st.Lines)
	}
	roles := map[string]string{}
	for _, l := range st.Lines {
		roles[l.Role] = l.Text
	}
	if roles["user"] != "hello copilot" {
		t.Errorf("user text = %q", roles["user"])
	}
	if roles["assistant"] != "hi back" {
		t.Errorf("assistant text = %q", roles["assistant"])
	}
}

func TestSessionTexts_MissingDirIsNotAnError(t *testing.T) {
	p := &Provider{HomeOverride: t.TempDir()}
	if texts, err := p.SessionTexts(context.Background()); err != nil || len(texts) != 0 {
		t.Errorf("got %d texts err=%v, want 0/nil", len(texts), err)
	}
}

// TestSessionTexts_SessionStateLayout pins the 1.x on-disk layout
// (session-state/<uuid>/events.jsonl). The original implementation
// walked sessions/ — a pre-1.0 path that no current install populates —
// so search returned zero results for every Copilot user.
func TestSessionTexts_SessionStateLayout(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "session-state", "abc-1.x")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	events := `{"type":"session.start","timestamp":"2026-06-11T15:00:00Z","data":{"sessionId":"abc-1.x","context":{"repository":"DefaultCollection/Org/Repo"}}}
{"type":"user.message","timestamp":"2026-06-11T15:00:01Z","data":{"sessionId":"abc-1.x","text":"hello from session-state"}}
{"type":"assistant.message","timestamp":"2026-06-11T15:00:02Z","data":{"sessionId":"abc-1.x","content":"hi back"}}
`
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(events), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	p := &Provider{HomeOverride: home}
	texts, err := p.SessionTexts(context.Background())
	if err != nil {
		t.Fatalf("SessionTexts: %v", err)
	}
	if len(texts) != 1 {
		t.Fatalf("expected 1 session under session-state/, got %d", len(texts))
	}
	if texts[0].SessionID != "copilot:abc-1.x" {
		t.Errorf("SessionID = %q", texts[0].SessionID)
	}
	var sawUser bool
	for _, l := range texts[0].Lines {
		if l.Role == "user" && l.Text == "hello from session-state" {
			sawUser = true
		}
	}
	if !sawUser {
		t.Errorf("expected user line, got lines=%+v", texts[0].Lines)
	}
}
