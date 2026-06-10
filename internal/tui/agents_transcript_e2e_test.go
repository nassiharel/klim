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
// Skipped when no transcript is available (CI on a fresh machine).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderTranscriptLine_RealOnDiskTranscript(t *testing.T) {
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

	lines, err := readSessionTranscript(picked, 60)
	if err != nil {
		t.Fatalf("readSessionTranscript: %v", err)
	}
	// We expect at least a few rendered lines from a real session.
	if len(lines) < 3 {
		t.Errorf("only %d rendered lines from %s — viewer would look empty.\nlines: %v", len(lines), picked, lines)
	}
	// We expect at least one [user] OR [assistant] line — the bug
	// was that strings-as-content dropped every [user] line, so we
	// can't be too strict here, but completely-empty conversation
	// content is the failure mode.
	conversational := 0
	for _, ln := range lines {
		if strings.HasPrefix(ln, "[user]") || strings.HasPrefix(ln, "[assistant]") {
			conversational++
		}
	}
	if conversational == 0 {
		t.Errorf("no [user]/[assistant] lines in %d rendered lines — viewer shows tool noise only", len(lines))
	}
}
