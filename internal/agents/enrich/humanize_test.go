package enrich

import (
	"testing"
	"time"
)

func TestRelativeTime(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		when time.Time
		want string
	}{
		{"zero", time.Time{}, "—"},
		{"just now (1s)", now.Add(-time.Second), "just now"},
		{"30s ago", now.Add(-30 * time.Second), "30s ago"},
		{"5m ago", now.Add(-5 * time.Minute), "5m ago"},
		{"3h ago", now.Add(-3 * time.Hour), "3h ago"},
		{"yesterday (30h)", now.Add(-30 * time.Hour), "yesterday"},
		{"5d ago", now.Add(-5 * 24 * time.Hour), "5d ago"},
		{"long ago is iso date", now.Add(-30 * 24 * time.Hour), "2026-05-11"},
		{"future is prefixed", now.Add(30 * time.Minute), "in 30m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := RelativeTime(tt.when, now)
			if got != tt.want {
				t.Errorf("RelativeTime(%v) = %q, want %q", tt.when, got, tt.want)
			}
		})
	}
}

func TestParseSince(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		in      string
		want    time.Time
		wantErr bool
	}{
		{"", time.Time{}, false},
		{"2h", now.Add(-2 * time.Hour), false},
		{"30m", now.Add(-30 * time.Minute), false},
		{"7d", now.Add(-7 * 24 * time.Hour), false},
		{"1w", now.Add(-7 * 24 * time.Hour), false},
		{"2026-06-01", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC), false},
		{"2026-06-10T08:00:00Z", time.Date(2026, 6, 10, 8, 0, 0, 0, time.UTC), false},
		{"yesterday", time.Time{}, true},
		{"bogus", time.Time{}, true},
	}
	for _, tt := range tests {
		got, err := ParseSince(tt.in, now)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseSince(%q) err = %v, wantErr = %v", tt.in, err, tt.wantErr)
			continue
		}
		if err == nil && !got.Equal(tt.want) {
			t.Errorf("ParseSince(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}