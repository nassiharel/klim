package claudecode

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSessionTexts_ParsesUserAndAssistantContent(t *testing.T) {
	home := t.TempDir()
	proj := filepath.Join(home, ".claude", "projects", "home%2Fuser%2Frepo")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	transcript := `{"type":"user","timestamp":"2026-05-15T08:00:00Z","sessionId":"abc","message":{"role":"user","content":"hello agent"}}
{"type":"assistant","timestamp":"2026-05-15T08:00:01Z","sessionId":"abc","message":{"role":"assistant","content":[{"type":"text","text":"hello human"}]}}
{"type":"tool_use","timestamp":"2026-05-15T08:00:02Z","sessionId":"abc"}
`
	if err := os.WriteFile(filepath.Join(proj, "session.jsonl"), []byte(transcript), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	p := &Provider{HomeOverride: home}
	texts, err := p.SessionTexts(context.Background())
	if err != nil {
		t.Fatalf("SessionTexts: %v", err)
	}
	if len(texts) != 1 {
		t.Fatalf("expected 1 session text, got %d", len(texts))
	}
	st := texts[0]
	if st.SessionID != "claude:abc" {
		t.Errorf("session id = %q", st.SessionID)
	}
	if len(st.Lines) != 2 {
		t.Fatalf("expected 2 lines (user+assistant, tool_use skipped), got %d: %+v", len(st.Lines), st.Lines)
	}
	if st.Lines[0].Role != "user" || st.Lines[0].Text != "hello agent" {
		t.Errorf("line[0] = %+v", st.Lines[0])
	}
	if st.Lines[1].Role != "assistant" || st.Lines[1].Text != "hello human" {
		t.Errorf("line[1] = %+v", st.Lines[1])
	}
}

func TestSessionTexts_MissingDirIsNotAnError(t *testing.T) {
	p := &Provider{HomeOverride: t.TempDir()}
	if texts, err := p.SessionTexts(context.Background()); err != nil || len(texts) != 0 {
		t.Errorf("got %d texts err=%v, want 0/nil", len(texts), err)
	}
}
