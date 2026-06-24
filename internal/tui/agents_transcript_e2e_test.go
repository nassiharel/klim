package tui

// E2E sanity check: read the most recent real Claude transcript from
// ~/.claude/projects and confirm renderTranscriptLine produces a
// non-trivial number of [user] / [assistant] / [tool] lines.
//
// This catches the regression where the renderer was silently
// dropping every line and the viewer ended up empty. Without this
// test the unit-test fix could pass while the real-world experience
// remains broken.
//
// Hermeticity: this test reads the developer's actual home directory,
// which is not OK to do on every `go test ./...` invocation — the
// results vary by machine, by what's currently being edited, and
// implicitly leak whatever conversation text happens to be in the
// transcript. We gate it behind the `KLIM_E2E` env var so the test
// runs only when explicitly opted into (`KLIM_E2E=1 go test ./...`).
// When unset (the common case) the test skips with a clear message.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderTranscriptLine_RealOnDiskTranscript(t *testing.T) {
	if os.Getenv("KLIM_E2E") == "" {
		t.Skip("KLIM_E2E not set; skipping test that reads developer's ~/.claude/projects (non-hermetic)")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir")
	}
	root := filepath.Join(home, ".claude", "projects")
	if _, err := os.Stat(root); err != nil {
		t.Skip("no ~/.claude/projects on this machine")
	}

	// Find ANY .jsonl by walking the projects tree, take the first
	// non-empty one. We don't care which session.
	var picked string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || picked != "" {
			return nil
		}
		if !strings.HasSuffix(p, ".jsonl") {
			return nil
		}
		if info, err := d.Info(); err == nil && info.Size() > 1024 {
			picked = p
		}
		return nil
	})
	if picked == "" {
		t.Skip("no real .jsonl transcripts to test against")
	}

	msgs, err := readSessionTranscript(picked, 60)
	if err != nil {
		t.Fatalf("readSessionTranscript: %v", err)
	}
	// We expect at least a few parsed messages from a real session.
	if len(msgs) < 3 {
		t.Errorf("only %d parsed messages from %s — viewer would look empty.\nmessages: %v", len(msgs), picked, msgs)
	}
	// We expect at least one user OR assistant message — the bug
	// was that strings-as-content dropped every user message, so we
	// can't be too strict here, but completely-empty conversation
	// content is the failure mode.
	conversational := 0
	for _, m := range msgs {
		if m.role == "user" || m.role == "assistant" {
			conversational++
		}
	}
	if conversational == 0 {
		t.Errorf("no user/assistant messages in %d parsed messages — viewer shows tool noise only", len(msgs))
	}
}
