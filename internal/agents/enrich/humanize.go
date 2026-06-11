package enrich

import (
	"fmt"
	"time"
)

// RelativeTime renders a time.Time as a short, human-friendly
// relative string suitable for dashboard tables. The granularity is
// intentionally coarse — "5m ago", "3h ago", "yesterday" — because
// the dashboard's job is "did this happen recently or not?" rather
// than "exactly when did this happen?".
//
// `now` is the reference time so tests can pass a fixed value.
//
// Special cases:
//   - zero time            → "—" (the placeholder used elsewhere in
//     klim for missing values, kept consistent here).
//   - in the future        → "in <duration>" (rare; can happen when
//     the session file's mtime races with the clock).
//   - more than 7 days ago → ISO date YYYY-MM-DD.
func RelativeTime(t, now time.Time) string {
	if t.IsZero() {
		return "—"
	}

	d := now.Sub(t)
	if d < 0 {
		return "in " + shortDuration(-d)
	}

	switch {
	case d < 5*time.Second:
		return "just now"
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "yesterday"
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("2006-01-02")
	}
}

// shortDuration renders a duration with one unit, used by the "in N"
// branch of RelativeTime.
func shortDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// ParseSince converts a CLI-friendly "--since" value into the
// corresponding time. Accepted forms:
//
//   - Go duration string ("2h", "30m", "7d" — `d` is interpreted as
//     24h units since time.ParseDuration doesn't accept it).
//   - ISO date "YYYY-MM-DD".
//   - ISO datetime RFC3339.
//
// Returns the absolute time, evaluated against `now` for relative
// durations. An empty input returns the zero time and a nil error
// so callers can pass user input through directly.
func ParseSince(s string, now time.Time) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}

	// Try duration-with-days first.
	if d, ok := parseDurationWithDays(s); ok {
		return now.Add(-d), nil
	}

	// Try ISO date.
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}

	// Try full RFC3339.
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}

	return time.Time{}, fmt.Errorf("invalid time value %q: expected duration (2h, 30m, 7d), date (YYYY-MM-DD), or RFC3339 timestamp", s)
}

// parseDurationWithDays extends time.ParseDuration with `d` and `w`
// suffixes for whole-day and week units. Returns ok=false on failure
// so callers can fall through to other formats.
func parseDurationWithDays(s string) (time.Duration, bool) {
	if len(s) < 2 {
		return 0, false
	}
	// Look at the trailing unit.
	last := s[len(s)-1]
	switch last {
	case 'd', 'w':
		var n int
		_, err := fmt.Sscanf(s[:len(s)-1], "%d", &n)
		if err != nil || n < 0 {
			return 0, false
		}
		if last == 'd' {
			return time.Duration(n) * 24 * time.Hour, true
		}
		return time.Duration(n) * 7 * 24 * time.Hour, true
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, false
	}
	return d, true
}
