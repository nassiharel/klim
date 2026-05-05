package vuln

import (
	"testing"
	"time"
)

func TestSeverity_Rank(t *testing.T) {
	cases := []struct {
		s    Severity
		rank int
	}{
		{SeverityCritical, 4},
		{SeverityHigh, 3},
		{SeverityMedium, 2},
		{SeverityLow, 1},
		{SeverityUnknown, 0},
		{Severity("garbage"), 0},
	}
	for _, c := range cases {
		if got := c.s.Rank(); got != c.rank {
			t.Errorf("Rank(%q) = %d, want %d", c.s, got, c.rank)
		}
	}
}

func TestSeverity_AtLeast(t *testing.T) {
	if !SeverityCritical.AtLeast(SeverityHigh) {
		t.Error("CRITICAL ≥ HIGH should be true")
	}
	if SeverityLow.AtLeast(SeverityMedium) {
		t.Error("LOW ≥ MEDIUM should be false")
	}
	if !SeverityHigh.AtLeast(SeverityHigh) {
		t.Error("HIGH ≥ HIGH should be true (inclusive)")
	}
}

func TestParseSeverity(t *testing.T) {
	cases := map[string]Severity{
		"CRITICAL":      SeverityCritical,
		"critical":      SeverityCritical,
		"  HIGH  ":      SeverityHigh,
		"Moderate":      SeverityMedium,
		"medium":        SeverityMedium,
		"low":           SeverityLow,
		"":              SeverityUnknown,
		"informational": SeverityUnknown,
	}
	for in, want := range cases {
		if got := ParseSeverity(in); got != want {
			t.Errorf("ParseSeverity(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFromCVSSScore(t *testing.T) {
	cases := []struct {
		score float64
		want  Severity
	}{
		{9.8, SeverityCritical},
		{9.0, SeverityCritical},
		{8.9, SeverityHigh},
		{7.0, SeverityHigh},
		{6.5, SeverityMedium},
		{4.0, SeverityMedium},
		{3.9, SeverityLow},
		{0.1, SeverityLow},
		{0, SeverityUnknown},
		{-1, SeverityUnknown},
	}
	for _, c := range cases {
		if got := FromCVSSScore(c.score); got != c.want {
			t.Errorf("FromCVSSScore(%v) = %q, want %q", c.score, got, c.want)
		}
	}
}

func TestMatch_MaxSeverity(t *testing.T) {
	m := Match{
		Vulnerabilities: []Vulnerability{
			{Severity: SeverityLow},
			{Severity: SeverityHigh},
			{Severity: SeverityMedium},
		},
	}
	if got := m.MaxSeverity(); got != SeverityHigh {
		t.Errorf("MaxSeverity = %q, want HIGH", got)
	}
	empty := Match{}
	if got := empty.MaxSeverity(); got != SeverityUnknown {
		t.Errorf("empty MaxSeverity = %q, want UNKNOWN", got)
	}
}

func TestReport_HasFindings_MaxSeverity(t *testing.T) {
	r := Report{
		ScannedAt: time.Now(),
		Matches: []Match{
			{Tool: "a", Vulnerabilities: []Vulnerability{{Severity: SeverityMedium}}},
			{Tool: "b"},
			{Tool: "c", Vulnerabilities: []Vulnerability{{Severity: SeverityCritical}}},
		},
	}
	if !r.HasFindings() {
		t.Error("HasFindings should be true")
	}
	if got := r.MaxSeverity(); got != SeverityCritical {
		t.Errorf("Report.MaxSeverity = %q, want CRITICAL", got)
	}

	clean := Report{Matches: []Match{{Tool: "x"}, {Tool: "y"}}}
	if clean.HasFindings() {
		t.Error("HasFindings should be false when no vulns")
	}
	if got := clean.MaxSeverity(); got != SeverityUnknown {
		t.Errorf("clean MaxSeverity = %q, want UNKNOWN", got)
	}
}
