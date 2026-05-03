package githubfmt

import (
	"strings"
	"testing"
	"time"
)

func TestFormatStars(t *testing.T) {
	// These cases are the contract that both `clim info` and the TUI
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

func TestFormatDate_LegacyFormat(t *testing.T) {
	// Older catalog entries used "2006-01-02T15:04:05Z" without the
	// trailing timezone-name notation. Make sure we still parse them.
	ts := "2024-01-15T12:00:00Z"
	got := FormatDate(ts)
	if got == ts {
		t.Fatalf("legacy format not parsed: got %q", got)
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
