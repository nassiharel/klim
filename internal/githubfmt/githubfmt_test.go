package githubfmt

import (
	"strings"
	"testing"
	"time"
)

func TestFormatStars(t *testing.T) {
	// These cases are the contract that both `klim tool info` and the TUI
	// detail view depend on. Drift here is a UX regression visible to
	// users in two places at once.
	tests := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{42, "42"},
		{999, "999"},
		{1_000, "1.0k"},
		{1_234, "1.2k"},
		{9_999, "10.0k"},
		{10_000, "10k"},
		{12_345, "12k"},
		{234_567, "234k"},
		{999_999, "999k"},
		{1_000_000, "1.0M"},
		{1_500_000, "1.5M"},
		{109_000_000, "109.0M"},
	}
	for _, tt := range tests {
		if got := FormatStars(tt.in); got != tt.want {
			t.Errorf("FormatStars(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFormatDate_Empty(t *testing.T) {
	if got := FormatDate(""); got != "" {
		t.Errorf("empty = %q", got)
	}
}

func TestFormatDate_InvalidPassthrough(t *testing.T) {
	if got := FormatDate("not a date"); got != "not a date" {
		t.Errorf("invalid = %q", got)
	}
}

func TestFormatDate_FutureFallsBackToDate(t *testing.T) {
	// Regression for a real bug: time.Since() returns a negative
	// duration for future timestamps; a switch matching `< 24h` would
	// classify those as "today". The correct behavior is to fall back
	// to the literal calendar date so the user can see the timestamp
	// is in the future and decide whether the input was bogus.
	future := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	got := FormatDate(future)
	if strings.Contains(got, "ago") || got == "today" || got == "just now" {
		t.Fatalf("future date misformatted as %q", got)
	}
	if !strings.Contains(got, "-") {
		t.Fatalf("expected calendar date, got %q", got)
	}
}

func TestFormatDate_TenDaysAgo(t *testing.T) {
	ts := time.Now().Add(-10 * 24 * time.Hour).UTC().Format(time.RFC3339)
	got := FormatDate(ts)
	if !strings.Contains(got, "days ago") {
		t.Fatalf("got %q, want 'days ago'", got)
	}
}

func TestFormatDate_HoursAgo(t *testing.T) {
	ts := time.Now().Add(-3 * time.Hour).UTC().Format(time.RFC3339)
	got := FormatDate(ts)
	if !strings.Contains(got, "hours ago") {
		t.Fatalf("got %q", got)
	}
}

func TestRepoURL(t *testing.T) {
	if got := RepoURL(""); got != "" {
		t.Errorf("empty slug = %q", got)
	}
	if got := RepoURL("  cli/cli  "); got != "https://github.com/cli/cli" {
		t.Errorf("trimming: got %q", got)
	}
	if got := RepoURL("kubernetes/kubectl"); got != "https://github.com/kubernetes/kubectl" {
		t.Errorf("got %q", got)
	}
}

// TestFormatDate_OneHourBoundary regression: previously a delta of
// 1.5h would truncate to int=1 and fall into the "just now" branch,
// so a push 90 minutes old looked the same as one 5 minutes old.
// The fix splits the buckets at exactly 1 hour and emits "1 hour ago".
func TestFormatDate_OneHourBoundary(t *testing.T) {
	for _, tt := range []struct {
		offset time.Duration
		want   string
	}{
		{30 * time.Minute, "just now"},
		{59 * time.Minute, "just now"},
		{61 * time.Minute, "1 hour ago"},
		{90 * time.Minute, "1 hour ago"},
		{2 * time.Hour, "2 hours ago"},
		{47 * time.Hour, "47 hours ago"},
	} {
		ts := time.Now().Add(-tt.offset).UTC().Format(time.RFC3339)
		if got := FormatDate(ts); got != tt.want {
			t.Errorf("offset=%v: got %q, want %q", tt.offset, got, tt.want)
		}
	}
}

// TestFormatDate_SingularGrammar guards against "1 days ago" / "1
// months ago" / "1.0 years ago" output. The shared formatter feeds
// both `klim tool info` and the TUI detail page, so awkward grammar
// surfaces in two UIs at once.
func TestFormatDate_SingularGrammar(t *testing.T) {
	cases := []struct {
		offset time.Duration
		want   string
	}{
		{49 * time.Hour, "2 days ago"},              // plural day
		{36 * 24 * time.Hour, "1 month ago"},        // singular month
		{75 * 24 * time.Hour, "2 months ago"},       // plural month
		{370 * 24 * time.Hour, "1 year ago"},        // singular year
		{2 * 365 * 24 * time.Hour, "2.0 years ago"}, // plural with one decimal
	}
	for _, c := range cases {
		ts := time.Now().Add(-c.offset).UTC().Format(time.RFC3339)
		if got := FormatDate(ts); got != c.want {
			t.Errorf("offset=%v: got %q, want %q", c.offset, got, c.want)
		}
	}
}
