// Package vuln looks up known vulnerabilities (CVEs / GHSAs) for the
// installed tools. Data source is OSV.dev, which mirrors GitHub
// Security Advisories plus the major language ecosystems and is free
// without authentication.
package vuln

import (
	"strings"
	"time"
)

// Severity is the normalized severity bucket. OSV reports severity in
// CVSS v3.1 score form per ecosystem; we collapse that into the four
// buckets every UI surface (CLI/TUI/web) renders consistently.
type Severity string

// Severity values, ordered low → high.
const (
	SeverityUnknown  Severity = "UNKNOWN"
	SeverityLow      Severity = "LOW"
	SeverityMedium   Severity = "MEDIUM"
	SeverityHigh     Severity = "HIGH"
	SeverityCritical Severity = "CRITICAL"
)

// Rank returns an ascending integer suitable for comparing severities.
// Higher means worse. SeverityUnknown sorts at the bottom.
func (s Severity) Rank() int {
	switch s {
	case SeverityCritical:
		return 4
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 1
	}
	return 0
}

// AtLeast reports whether s is at least as severe as threshold.
func (s Severity) AtLeast(threshold Severity) bool {
	return s.Rank() >= threshold.Rank()
}

// ParseSeverity normalizes a free-form severity label into a Severity.
// Accepts CVSS-style strings ("CRITICAL", "HIGH", …), case-insensitive,
// with whitespace tolerated. Unknown / empty inputs return
// SeverityUnknown — never an error.
func ParseSeverity(s string) Severity {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "CRITICAL":
		return SeverityCritical
	case "HIGH":
		return SeverityHigh
	case "MEDIUM", "MODERATE":
		return SeverityMedium
	case "LOW":
		return SeverityLow
	}
	return SeverityUnknown
}

// FromCVSSScore converts a CVSS v3.x base score (0.0–10.0) into a
// Severity bucket using the standard CVSS bands.
func FromCVSSScore(score float64) Severity {
	switch {
	case score >= 9.0:
		return SeverityCritical
	case score >= 7.0:
		return SeverityHigh
	case score >= 4.0:
		return SeverityMedium
	case score > 0:
		return SeverityLow
	}
	return SeverityUnknown
}

// Ecosystem identifies an OSV ecosystem.
type Ecosystem string

// Ecosystem values supported by the mapper.
const (
	EcosystemNPM      Ecosystem = "npm"
	EcosystemPyPI     Ecosystem = "PyPI"
	EcosystemGo       Ecosystem = "Go"
	EcosystemCargo    Ecosystem = "crates.io"
	EcosystemHomebrew Ecosystem = "Homebrew"
	// EcosystemGitHub is a synthetic marker for "match by repo slug".
	// Translates into a query that scopes by the tool's GitHub slug.
	EcosystemGitHub Ecosystem = "GitHub"
)

// Coord identifies a package version inside a single ecosystem.
type Coord struct {
	Ecosystem Ecosystem `json:"ecosystem"`
	Package   string    `json:"package"`
	Version   string    `json:"version"`
}

// Vulnerability is a single advisory entry.
type Vulnerability struct {
	ID        string    `json:"id"`
	Aliases   []string  `json:"aliases,omitempty"`
	Summary   string    `json:"summary"`
	Severity  Severity  `json:"severity"`
	FixedIn   string    `json:"fixed_in,omitempty"`
	Published time.Time `json:"published,omitempty"`
	URL       string    `json:"url,omitempty"`
}

// Match pairs a tool with the vulnerabilities affecting its installed
// version, plus the coordinate that produced the match.
type Match struct {
	Tool            string          `json:"tool"`
	DisplayName     string          `json:"display_name,omitempty"`
	InstalledVer    string          `json:"installed_version"`
	Coord           Coord           `json:"coord"`
	Vulnerabilities []Vulnerability `json:"vulnerabilities"`
}

// MaxSeverity returns the worst severity across the vulnerabilities
// in the match. Returns SeverityUnknown when the match has no vulns.
func (m Match) MaxSeverity() Severity {
	out := SeverityUnknown
	for _, v := range m.Vulnerabilities {
		if v.Severity.Rank() > out.Rank() {
			out = v.Severity
		}
	}
	return out
}

// Skip records a tool that was excluded from the vulnerability scan
// and the human-readable reason. Surfaced in CLI output so users can
// see coverage gaps.
type Skip struct {
	Tool   string `json:"tool"`
	Reason string `json:"reason"`
}

// Report is the aggregate result of a Lookup call.
type Report struct {
	ScannedAt    time.Time `json:"scanned_at"`
	ToolsScanned int       `json:"tools_scanned"`
	Source       string    `json:"source"`
	Matches      []Match   `json:"matches"`
	Skipped      []Skip    `json:"skipped,omitempty"`
}

// MaxSeverity returns the worst severity across all matches.
func (r Report) MaxSeverity() Severity {
	out := SeverityUnknown
	for _, m := range r.Matches {
		if s := m.MaxSeverity(); s.Rank() > out.Rank() {
			out = s
		}
	}
	return out
}

// HasFindings reports whether any tool has a recorded vulnerability.
func (r Report) HasFindings() bool {
	for _, m := range r.Matches {
		if len(m.Vulnerabilities) > 0 {
			return true
		}
	}
	return false
}
