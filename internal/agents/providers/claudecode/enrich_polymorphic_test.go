package claudecode

// Regression test for the polymorphic message.content bug in
// enrichSessionFromJSONL. Real Claude transcripts emit user
// messages as a plain string (`"content":"hi there"`) AND as an
// array of typed blocks (`"content":[{"type":"text",...}]`). The
// original code declared Content as `[]struct{...}`, so any user
// line with string content failed json.Unmarshal and was silently
// dropped — taking TurnCount, first-user-message Title, and other
// enrichment with it. Same bug shape as the TUI's
// renderTranscriptLine, but here it affects the persisted
// session record, not just the viewer.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnrichSessionFromJSONL_HandlesPolymorphicUserContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "polymorphic.jsonl")

	// Two user messages: first uses the string form (the dominant
	// real-world case for user-typed prompts), second uses the
	// array form (Claude's "block" content for IDE-attached or
	// programmatic payloads). Both must be visible in the result.
	lines := []string{
		`{"type":"user","sessionId":"abc","cwd":"/dev/klim","gitBranch":"main","timestamp":"2026-06-10T08:54:49.851Z","message":{"role":"user","content":"please refactor the tile renderer"}}`,
		`{"type":"assistant","sessionId":"abc","timestamp":"2026-06-10T08:54:55.000Z","message":{"role":"assistant","content":[{"type":"text","text":"on it"}]}}`,
		`{"type":"user","sessionId":"abc","timestamp":"2026-06-10T08:55:00.000Z","message":{"role":"user","content":[{"type":"text","text":"second turn"}]}}`,
	}
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	got := enrichSessionFromJSONL(path, now)

	if got.SessionID != "abc" {
		t.Errorf("SessionID = %q, want %q", got.SessionID, "abc")
	}
	if got.Cwd != "/dev/klim" {
		t.Errorf("Cwd = %q, want %q", got.Cwd, "/dev/klim")
	}
	if got.Branch != "main" {
		t.Errorf("Branch = %q, want %q", got.Branch, "main")
	}
	// FirstUserMsg must come from the STRING-form first user
	// message — that's the regression: pre-fix it was empty
	// because the line was dropped.
	if got.FirstUserMsg != "please refactor the tile renderer" {
		t.Errorf("FirstUserMsg = %q, want %q — string-form user content was dropped",
			got.FirstUserMsg, "please refactor the tile renderer")
	}
	if got.Created.IsZero() {
		t.Error("Created not set — first event timestamp should populate it")
	}
}

func TestEnrichSessionFromJSONL_AssistantArrayContentStillWorks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "assistant.jsonl")
	body := `{"type":"assistant","sessionId":"x","timestamp":"2026-06-10T08:00:00.000Z","message":{"role":"assistant","content":[{"type":"text","text":"hi"},{"type":"tool_use","name":"Bash"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got := enrichSessionFromJSONL(path, time.Now())
	if got.SessionID != "x" {
		t.Errorf("SessionID = %q, want x", got.SessionID)
	}
}
