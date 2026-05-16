package tui

import (
	"testing"

	"github.com/nassiharel/klim/internal/agents"
)

func TestPinBookmarkedFirst(t *testing.T) {
	rows := []agentRow{
		{name: "a"},
		{name: "b", bookmarked: true},
		{name: "c"},
		{name: "d", bookmarked: true},
	}
	got := pinBookmarkedFirst(rows)
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
	if got[0].name != "b" || got[1].name != "d" {
		t.Errorf("bookmarked rows not pinned first: %+v", got)
	}
	if got[2].name != "a" || got[3].name != "c" {
		t.Errorf("non-bookmarked relative order not preserved: %+v", got)
	}
}

func TestApplyBookmarkedFilter(t *testing.T) {
	rows := []agentRow{
		{name: "a", bookmarked: true},
		{name: "b"},
	}
	// Only fires when statusFilterValue == "bookmarked" AND subtab is Sessions.
	if out := applyBookmarkedFilter(rows, agentsSubSessions, "bookmarked"); len(out) != 1 || out[0].name != "a" {
		t.Errorf("filter result = %+v", out)
	}
	if out := applyBookmarkedFilter(rows, agentsSubSessions, "active"); len(out) != 2 {
		t.Error("other status values must not filter")
	}
	if out := applyBookmarkedFilter(rows, agentsSubPlugins, "bookmarked"); len(out) != 2 {
		t.Error("other sub-tabs must not filter on bookmarked")
	}
}

func TestFormatSessionTitle_PrefixesStar(t *testing.T) {
	plain := formatSessionTitle(agentRow{}, "untitled")
	if plain != "untitled" {
		t.Errorf("plain title = %q", plain)
	}
	starred := formatSessionTitle(agentRow{bookmarked: true}, "title")
	if !contains(starred, "★") {
		t.Errorf("bookmarked title missing star: %q", starred)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

var _ = agents.SessionStatusActive // keep import alive for future tests
