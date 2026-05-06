// Package githubfmt formats GitHub project metadata (star counts, push
// dates, repo URLs) for display. Both the TUI's tool detail view and the
// `klim info` CLI command consume these helpers so the two surfaces
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
//	0–999             → "42"      / "999"
//	1000–9999         → "1.0k"    / "1.2k"  (rounded to one decimal,
//	                                          so 9999 prints as "10.0k")
//	10_000–999_999    → "12k"     / "234k"
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
// "N days ago" / "N months ago" / bare "YYYY-MM-DD" string.
//
// Returns the raw input if it cannot be parsed. Future timestamps fall
// back to the calendar date (no "ago" phrasing) — a future push usually
// means the input was generated on a machine with a skewed clock, and
// pretending it's recent would mislead the user. Singular buckets
// ("1 hour ago", "1 day ago", …) use the singular noun explicitly so
// the output reads naturally in both `klim info` and the TUI detail
// page that share this helper.
func FormatDate(ts string) string {
	if ts == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	delta := time.Since(t)
	switch {
	case delta < 0:
		return t.Format("2006-01-02")
	case delta < time.Hour:
		// Anything under an hour is "just now" — the user doesn't care
		// whether it was 5 or 55 minutes ago for a push timestamp.
		return "just now"
	case delta < 48*time.Hour:
		return pluralAgo(int(delta.Hours()), "hour")
	case delta < 30*24*time.Hour:
		// 2–29 days. The month bucket below would otherwise never be
		// able to emit "1 month ago" because at 30 days int(d/30)==1
		// can be reached.
		return pluralAgo(int(delta.Hours()/24), "day")
	case delta < 365*24*time.Hour:
		return pluralAgo(int(delta.Hours()/(24*30)), "month")
	default:
		// Years use one decimal so 1.0/1.5/2.0 read naturally; we
		// special-case exact 1.0 to "1 year ago" for grammar.
		years := delta.Hours() / (24 * 365)
		if years < 1.05 {
			return "1 year ago"
		}
		return fmt.Sprintf("%.1f years ago", years)
	}
}

// pluralAgo formats counts with the right singular/plural noun so we
// never emit "1 hours ago" / "1 days ago".
func pluralAgo(n int, unit string) string {
	if n == 1 {
		return "1 " + unit + " ago"
	}
	return fmt.Sprintf("%d %ss ago", n, unit)
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
