// Package githubfmt formats GitHub project metadata (star counts, push
// dates, repo URLs) for display. Both the TUI's tool detail view and the
// `clim info` CLI command consume these helpers so the two surfaces
// cannot drift out of sync — a regression in either renderer fails the
// shared tests in this package.
package githubfmt

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// FormatStars renders a star count compactly.
//
//	0–999             → "42"
//	1000–9999         → "1.0k" / "9.9k"
//	10_000–999_999    → "12k"  / "234k"
//	1_000_000+        → "1.5M"
//
// Decimal precision intentionally varies by magnitude to keep counts
// short enough to fit in tight TUI columns while staying meaningful at
// the low end where ±50 stars matters more than at the millions level.
func FormatStars(n int) string {
	switch {
	case n < 1000:
		return strconv.Itoa(n)
	case n < 10_000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	case n < 1_000_000:
		return fmt.Sprintf("%dk", n/1000)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
}

// FormatDate converts a GitHub ISO-8601 timestamp into a human-friendly
// "N days ago" / "N months ago" / "on YYYY-MM-DD" string.
//
// Returns the raw input if it cannot be parsed. Future timestamps fall
// back to the calendar date — never "today" or other "ago" phrasing —
// because a future push usually means the input was generated on a
// machine with skewed clock or ahead time zone, and pretending it's
// recent would mislead the user.
func FormatDate(ts string) string {
	if ts == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		// Tolerate legacy "2006-01-02T15:04:05Z" without timezone name.
		if t2, err2 := time.Parse("2006-01-02T15:04:05Z", ts); err2 == nil {
			t = t2
		} else {
			return ts
		}
	}
	delta := time.Since(t)
	switch {
	case delta < 0:
		return t.Format("2006-01-02")
	case delta < 48*time.Hour:
		h := int(delta.Hours())
		if h <= 1 {
			return "just now"
		}
		return fmt.Sprintf("%d hours ago", h)
	case delta < 60*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(delta.Hours()/24))
	case delta < 365*24*time.Hour:
		return fmt.Sprintf("%d months ago", int(delta.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%.1f years ago", delta.Hours()/(24*365))
	}
}

// RepoURL returns the canonical https URL for a "owner/repo" slug, or
// "" if the slug is empty.
func RepoURL(slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ""
	}
	return "https://github.com/" + slug
}
