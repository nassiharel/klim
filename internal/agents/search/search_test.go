package search

import (
	"testing"
	"time"
)

func TestQuery_FindsCaseInsensitiveMatches(t *testing.T) {
	idx := NewIndex()
	idx.Sessions["s1"] = SessionText{
		SessionID: "s1",
		Provider:  "claude-code",
		Title:     "Refactor klim",
		Lines: []SessionLine{
			{Role: "user", Text: "How do I lay out a sidebar in the TUI?", LineNo: 1},
			{Role: "assistant", Text: "Use lipgloss.Width to keep columns aligned.", LineNo: 2},
		},
	}
	idx.Sessions["s2"] = SessionText{
		SessionID: "s2",
		Provider:  "copilot-cli",
		Title:     "Docs review",
		Lines: []SessionLine{
			{Role: "user", Text: "What is the SIDEBAR width default?", LineNo: 1},
		},
	}
	hits := idx.Query("sidebar", 10)
	if len(hits) < 2 {
		t.Fatalf("expected at least 2 hits, got %d", len(hits))
	}
	// Title match on s1 should give it a boost.
	if hits[0].SessionID != "s1" {
		t.Errorf("top hit = %q, want s1 (title match boost)", hits[0].SessionID)
	}
	// Snippet should contain the query (case-insensitive).
	for _, h := range hits {
		if !contains(h.Snippet, "idebar") && !contains(h.Snippet, "IDEBAR") {
			t.Errorf("snippet %q missing query text", h.Snippet)
		}
	}
}

func TestQuery_LimitsResults(t *testing.T) {
	idx := NewIndex()
	idx.Sessions["s"] = SessionText{
		SessionID: "s", Provider: "claude-code",
		Lines: []SessionLine{
			{Role: "user", Text: "test test test", LineNo: 1},
			{Role: "user", Text: "test test test", LineNo: 2},
			{Role: "user", Text: "test test test", LineNo: 3},
			{Role: "user", Text: "test test test", LineNo: 4},
			{Role: "user", Text: "test test test", LineNo: 5},
		},
	}
	if hits := idx.Query("test", 3); len(hits) != 3 {
		t.Errorf("got %d, want 3", len(hits))
	}
	if hits := idx.Query("test", 0); len(hits) != 5 {
		t.Errorf("got %d (limit 0 = unlimited), want 5", len(hits))
	}
}

func TestQuery_EmptyReturnsNoHits(t *testing.T) {
	idx := NewIndex()
	idx.Sessions["s"] = SessionText{
		Lines: []SessionLine{{Text: "hello"}},
	}
	if hits := idx.Query("", 10); len(hits) != 0 {
		t.Errorf("empty query should yield no hits, got %d", len(hits))
	}
	if hits := idx.Query("   ", 10); len(hits) != 0 {
		t.Errorf("whitespace-only query should yield no hits, got %d", len(hits))
	}
}

func TestPruneMissingDropsAbsentSessions(t *testing.T) {
	idx := NewIndex()
	idx.Sessions["keep"] = SessionText{SessionID: "keep"}
	idx.Sessions["drop"] = SessionText{SessionID: "drop"}
	idx.PruneMissing(map[string]bool{"keep": true})
	if _, ok := idx.Sessions["drop"]; ok {
		t.Error("drop should have been pruned")
	}
	if _, ok := idx.Sessions["keep"]; !ok {
		t.Error("keep should remain")
	}
}

func TestMergeReplacesEntries(t *testing.T) {
	idx := NewIndex()
	idx.Sessions["s1"] = SessionText{SessionID: "s1", Title: "old"}
	idx.Merge([]SessionText{{SessionID: "s1", Title: "new"}})
	if got := idx.Sessions["s1"].Title; got != "new" {
		t.Errorf("title = %q, want new", got)
	}
}

func TestSnippetCenteredOnMatch(t *testing.T) {
	long := "abcdefghijklmnopqrstuvwxyz0123456789"
	got := snippetAround(long, 20, 1, 10)
	// Should include ellipses on both sides since we trimmed.
	if got == long {
		t.Errorf("snippet should be shorter than input")
	}
	if !contains(got, "…") {
		t.Errorf("expected ellipsis: %q", got)
	}
}

func TestScoreHit_AssistantOutranksUser(t *testing.T) {
	user := SessionLine{Role: "user", Text: "hello world"}
	asst := SessionLine{Role: "assistant", Text: "hello world"}
	if scoreHit(asst, 0, "hello", false) <= scoreHit(user, 0, "hello", false) {
		t.Error("assistant should outrank user for the same match")
	}
}

func contains(s, sub string) bool { return indexOf(s, sub) >= 0 }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

var _ = time.Now // keep time import alive if we add timestamp-based tests later
