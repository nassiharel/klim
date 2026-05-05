package vuln

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultOSVURL is the public OSV.dev query endpoint. Self-hosted
// mirrors override via cfg.Vuln.URL → osvClient.URL.
const DefaultOSVURL = "https://api.osv.dev"

// maxResponseSize caps the payload we'll accept from a /v1/query call.
// OSV response bodies are typically a few KB to ~100 KB; we set the
// ceiling well above that to leave room for tools with many advisories
// (e.g. node) without opening the door to a runaway server eating
// memory.
const maxResponseSize = 16 << 20 // 16 MiB

// OSVClient queries an OSV-compatible HTTP endpoint for vulnerabilities
// affecting a given Coord. Mirrors compliance.HTTPFetcher's design from
// PR #51: scheme-validated URL, redirect cap on the default client,
// context-cancellable.
type OSVClient struct {
	URL        string       // base URL, e.g. https://api.osv.dev
	HTTPClient *http.Client // nil → default with 30s timeout + 3-redirect cap
}

// validateHTTPURL is duplicated from internal/compliance because we
// don't want internal/vuln to depend on internal/compliance for one
// helper. Keep them in sync — both packages reject anything other
// than http/https.
func validateHTTPURL(u string) error {
	parsed, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("vuln URL scheme must be http or https (got %q)", parsed.Scheme)
	}
	if parsed.Host == "" {
		return errors.New("vuln URL missing host")
	}
	return nil
}

const maxRedirects = 3

// Query asks the OSV endpoint for vulnerabilities affecting the
// given coord. Returns an empty slice (not an error) when the package
// is recognised but has no known advisories. Errors only on transport
// or schema problems.
func (c *OSVClient) Query(ctx context.Context, coord Coord) ([]Vulnerability, error) {
	base := strings.TrimSpace(c.URL)
	if base == "" {
		base = DefaultOSVURL
	}
	if err := validateHTTPURL(base); err != nil {
		return nil, err
	}

	body, err := json.Marshal(osvQueryRequest{
		Version: coord.Version,
		Package: osvPackage{
			Name:      coord.Package,
			Ecosystem: string(coord.Ecosystem),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("marshalling query: %w", err)
	}

	endpoint := strings.TrimRight(base, "/") + "/v1/query"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "clim/vuln")

	client := c.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return fmt.Errorf("stopped after %d redirects", maxRedirects)
				}
				return validateHTTPURL(req.URL.String())
			},
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OSV query: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		// Drain at most ~1KB so the error message can include a hint.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("OSV returned %s: %s", resp.Status, strings.TrimSpace(string(snippet)))
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("reading OSV response: %w", err)
	}
	if int64(len(data)) > maxResponseSize {
		return nil, fmt.Errorf("OSV response too large (>%d bytes)", maxResponseSize)
	}

	var raw osvQueryResponse
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("decoding OSV response: %w", err)
	}

	out := make([]Vulnerability, 0, len(raw.Vulns))
	for _, v := range raw.Vulns {
		out = append(out, normalize(v, coord))
	}
	return out, nil
}

// --- OSV wire types ---

type osvPackage struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

type osvQueryRequest struct {
	Version string     `json:"version,omitempty"`
	Package osvPackage `json:"package"`
}

type osvQueryResponse struct {
	Vulns []osvVulnerability `json:"vulns"`
}

type osvVulnerability struct {
	ID               string              `json:"id"`
	Aliases          []string            `json:"aliases,omitempty"`
	Summary          string              `json:"summary,omitempty"`
	Details          string              `json:"details,omitempty"`
	Published        string              `json:"published,omitempty"`
	Modified         string              `json:"modified,omitempty"`
	Severity         []osvSeverityEntry  `json:"severity,omitempty"`
	DatabaseSpecific osvDatabaseSpecific `json:"database_specific,omitempty"`
	Affected         []osvAffected       `json:"affected,omitempty"`
	References       []osvReference      `json:"references,omitempty"`
}

type osvSeverityEntry struct {
	Type  string `json:"type"`  // CVSS_V3, CVSS_V2, …
	Score string `json:"score"` // raw CVSS vector string OR a "5.4" number for some sources
}

type osvDatabaseSpecific struct {
	Severity string `json:"severity,omitempty"` // GHSA-style label: "CRITICAL"/"HIGH"/…
}

type osvAffected struct {
	Package osvPackage `json:"package"`
	Ranges  []osvRange `json:"ranges,omitempty"`
}

type osvRange struct {
	Type   string     `json:"type"` // SEMVER / ECOSYSTEM / GIT
	Events []osvEvent `json:"events,omitempty"`
}

type osvEvent struct {
	Introduced   string `json:"introduced,omitempty"`
	Fixed        string `json:"fixed,omitempty"`
	LastAffected string `json:"last_affected,omitempty"`
}

type osvReference struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// normalize converts the OSV wire format into our public Vulnerability
// type. Severity precedence:
//
//  1. database_specific.severity (GHSA gives a clean label)
//  2. Highest CVSS_V3 score from the severity[] array
//  3. SeverityUnknown
func normalize(v osvVulnerability, coord Coord) Vulnerability {
	out := Vulnerability{
		ID:      v.ID,
		Aliases: append([]string(nil), v.Aliases...),
		Summary: v.Summary,
		URL:     pickAdvisoryURL(v.References),
	}
	if t, err := time.Parse(time.RFC3339, v.Published); err == nil {
		out.Published = t
	}
	out.Severity = pickSeverity(v)
	out.FixedIn = pickFixedIn(v.Affected, coord)
	return out
}

// pickSeverity collapses OSV's per-source severity layout into our
// 4-bucket Severity. database_specific is the GHSA label; falling
// through to CVSS lets us cover non-GHSA sources too.
func pickSeverity(v osvVulnerability) Severity {
	if s := ParseSeverity(v.DatabaseSpecific.Severity); s != SeverityUnknown {
		return s
	}
	best := SeverityUnknown
	for _, sev := range v.Severity {
		if !strings.HasPrefix(strings.ToUpper(sev.Type), "CVSS") {
			continue
		}
		if score, ok := parseCVSSBaseScore(sev.Score); ok {
			if cand := FromCVSSScore(score); cand.Rank() > best.Rank() {
				best = cand
			}
		}
	}
	return best
}

// parseCVSSBaseScore extracts the numeric base score from an OSV
// severity entry. Two formats are common:
//
//	CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H   (vector — no number)
//	7.5                                            (just the number)
//
// For the vector form we don't compute the score ourselves (that
// requires a full CVSS calculator); we rely on database_specific
// when available. Returns (score, true) for plain-number scores only.
func parseCVSSBaseScore(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	if score, err := strconv.ParseFloat(s, 64); err == nil {
		return score, true
	}
	return 0, false
}

// pickFixedIn returns the first "fixed" version from a matching
// affected entry's range events. Only ECOSYSTEM and SEMVER range
// types are considered — GIT ranges contain commit hashes, not
// version strings, and would surface as junk in FixedIn ("fixed in
// abc123..." isn't useful to the user). Empty range type is
// allowed because some OSV records omit it; the value still has to
// be a version-shaped string for the UI to render usefully.
func pickFixedIn(affected []osvAffected, coord Coord) string {
	for _, a := range affected {
		if !strings.EqualFold(a.Package.Name, coord.Package) {
			continue
		}
		if !strings.EqualFold(a.Package.Ecosystem, string(coord.Ecosystem)) {
			continue
		}
		for _, r := range a.Ranges {
			switch strings.ToUpper(strings.TrimSpace(r.Type)) {
			case "", "ECOSYSTEM", "SEMVER":
				// allowed
			default:
				continue
			}
			for _, e := range r.Events {
				if e.Fixed != "" {
					return e.Fixed
				}
			}
		}
	}
	return ""
}

// pickAdvisoryURL picks the most useful canonical URL from the OSV
// references slice. ADVISORY > WEB > anything else.
func pickAdvisoryURL(refs []osvReference) string {
	rank := map[string]int{
		"ADVISORY": 0,
		"WEB":      1,
		"REPORT":   2,
		"FIX":      3,
		"PACKAGE":  4,
	}
	bestIdx := -1
	bestRank := 1 << 30
	for i, r := range refs {
		if rk, ok := rank[strings.ToUpper(r.Type)]; ok && rk < bestRank {
			bestRank = rk
			bestIdx = i
		}
	}
	if bestIdx >= 0 {
		return refs[bestIdx].URL
	}
	if len(refs) > 0 {
		return refs[0].URL
	}
	return ""
}
