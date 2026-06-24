package costs

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/nassiharel/klim/internal/paths"
)

func TestBuild_AggregatesAndSortsTopSessions(t *testing.T) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterday := today.Add(-24 * time.Hour)
	samples := []TokenSample{
		{Provider: "claude", SessionID: "s1", Model: "sonnet", Day: today, Input: 1000, Output: 100},
		{Provider: "claude", SessionID: "s1", Model: "sonnet", Day: yesterday, Input: 500, Output: 50},
		{Provider: "copilot", SessionID: "s2", Model: "gpt5", Day: today, Input: 200, Output: 20},
	}

	rep := Build(samples, Range7d)
	if rep.Totals.Input != 1700 || rep.Totals.Output != 170 {
		t.Errorf("totals = %+v", rep.Totals)
	}
	if len(rep.TopSessions) != 2 {
		t.Fatalf("top sessions: %d", len(rep.TopSessions))
	}
	if rep.TopSessions[0].SessionID != "s1" {
		t.Errorf("top session = %q, want s1", rep.TopSessions[0].SessionID)
	}
	if rep.ByProvider["claude"].Total() != 1650 {
		t.Errorf("claude total = %d, want 1650", rep.ByProvider["claude"].Total())
	}
	if rep.ByModel["claude/sonnet"].Total() != 1650 {
		t.Errorf("model claude/sonnet total = %d", rep.ByModel["claude/sonnet"].Total())
	}
}

func TestBuild_RespectsRangeCutoff(t *testing.T) {
	now := time.Now()
	old := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -40)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	samples := []TokenSample{
		{Provider: "x", SessionID: "old", Day: old, Input: 9999, Output: 1},
		{Provider: "x", SessionID: "new", Day: today, Input: 10, Output: 1},
	}
	if rep := Build(samples, RangeToday); rep.Totals.Total() != 11 {
		t.Errorf("today should only include 'new' session: total=%d", rep.Totals.Total())
	}
	if rep := Build(samples, RangeAllTime); rep.Totals.Total() != 10011 {
		t.Errorf("all-time should include everything: total=%d", rep.Totals.Total())
	}
}

func TestSparklineBucketsByDay(t *testing.T) {
	now := time.Now()
	d := func(daysAgo int) time.Time {
		t := now.AddDate(0, 0, -daysAgo)
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	}
	samples := []TokenSample{
		{Day: d(0), Input: 5},  // today
		{Day: d(1), Input: 10}, // yesterday
		{Day: d(6), Input: 20}, // 6 days ago
	}
	rep := Build(samples, Range7d)
	if len(rep.DailySparkline) != 7 {
		t.Fatalf("sparkline len = %d", len(rep.DailySparkline))
	}
	// Last bucket = today.
	if rep.DailySparkline[6] != 5 {
		t.Errorf("today bucket = %d, want 5", rep.DailySparkline[6])
	}
	if rep.DailySparkline[5] != 10 {
		t.Errorf("yesterday bucket = %d, want 10", rep.DailySparkline[5])
	}
	if rep.DailySparkline[0] != 20 {
		t.Errorf("6-days-ago bucket = %d, want 20", rep.DailySparkline[0])
	}
}

func TestCacheRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("KLIM_HOME", tmp)
	// paths.AgentCostsCache builds on paths.Join which honours KLIM_HOME
	// when set; force the package to recompute by writing to that path.
	p, err := paths.AgentCostsCache()
	if err != nil {
		t.Fatalf("AgentCostsCache: %v", err)
	}
	t.Logf("cache path: %s", filepath.Dir(p))

	c := NewCache()
	now := time.Now()
	c.Sessions["claude:abc"] = CachedEntry{
		Provider:        "claude-code",
		Title:           "demo",
		Model:           "sonnet",
		TranscriptMtime: now,
		Days: map[string]Totals{
			now.Format("2006-01-02"): {Input: 100, Output: 10},
		},
	}
	if err := c.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := LoadCache()
	if err != nil {
		t.Fatalf("LoadCache: %v", err)
	}
	got, ok := loaded.Sessions["claude:abc"]
	if !ok {
		t.Fatalf("session missing in loaded cache: %+v", loaded)
	}
	if got.Title != "demo" || got.Model != "sonnet" {
		t.Errorf("loaded entry mismatch: %+v", got)
	}
	if total := got.Days[now.Format("2006-01-02")].Total(); total != 110 {
		t.Errorf("day total = %d, want 110", total)
	}
}

func TestAggregateSession_HandlesEmpty(t *testing.T) {
	now := time.Now()
	e := AggregateSession(nil, now)
	if !e.TranscriptMtime.Equal(now) {
		t.Errorf("empty entry mtime = %v, want %v", e.TranscriptMtime, now)
	}
	if len(e.Days) != 0 {
		t.Errorf("expected empty Days map")
	}
}

func TestCachePruneMissing(t *testing.T) {
	c := &Cache{
		Sessions: map[string]CachedEntry{
			"keep":   {Title: "k"},
			"remove": {Title: "r"},
		},
	}
	c.PruneMissing(map[string]bool{"keep": true})
	if _, ok := c.Sessions["remove"]; ok {
		t.Errorf("remove should be gone")
	}
	if _, ok := c.Sessions["keep"]; !ok {
		t.Errorf("keep should remain")
	}
}

// TestCacheSessionTotal covers the per-session total lookup used by the
// session detail page for an instant (cache-first) cost read.
func TestCacheSessionTotal(t *testing.T) {
	c := &Cache{
		Sessions: map[string]CachedEntry{
			"claude:proj": {Days: map[string]Totals{
				"2026-05-15": {Input: 100, Output: 10},
				"2026-05-16": {Input: 200, Output: 20},
			}},
		},
	}
	got, ok := c.SessionTotal("claude:proj")
	if !ok {
		t.Fatal("expected the session to be present")
	}
	if got.Input != 300 || got.Output != 30 {
		t.Errorf("total in/out = %d/%d, want 300/30", got.Input, got.Output)
	}
	if _, ok := c.SessionTotal("claude:absent"); ok {
		t.Errorf("absent session should report ok=false")
	}
	var nilCache *Cache
	if _, ok := nilCache.SessionTotal("x"); ok {
		t.Errorf("nil cache should report ok=false")
	}
}
