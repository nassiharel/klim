package vuln

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeOSV stands up an httptest.Server that responds to /v1/query with
// the supplied vuln JSON. Any other path returns 404. Returns the
// server URL ready to plug into OSVClient.URL.
func fakeOSV(t *testing.T, payload osvQueryResponse) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/query" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestOSVClient_Query_HappyPath(t *testing.T) {
	url := fakeOSV(t, osvQueryResponse{
		Vulns: []osvVulnerability{
			{
				ID:               "GHSA-aaaa-bbbb-cccc",
				Aliases:          []string{"CVE-2024-12345"},
				Summary:          "Bad stuff in node",
				DatabaseSpecific: osvDatabaseSpecific{Severity: "HIGH"},
				Affected: []osvAffected{{
					Package: osvPackage{Name: "node", Ecosystem: "npm"},
					Ranges: []osvRange{{
						Type: "ECOSYSTEM",
						Events: []osvEvent{
							{Introduced: "0"},
							{Fixed: "18.19.1"},
						},
					}},
				}},
				References: []osvReference{
					{Type: "PACKAGE", URL: "https://nodejs.org"},
					{Type: "ADVISORY", URL: "https://github.com/advisories/GHSA-aaaa-bbbb-cccc"},
				},
			},
		},
	})

	c := &OSVClient{URL: url}
	vulns, err := c.Query(context.Background(), Coord{
		Ecosystem: EcosystemNPM,
		Package:   "node",
		Version:   "18.10.0",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(vulns) != 1 {
		t.Fatalf("vulns = %d, want 1", len(vulns))
	}
	v := vulns[0]
	if v.ID != "GHSA-aaaa-bbbb-cccc" {
		t.Errorf("ID = %q", v.ID)
	}
	if v.Severity != SeverityHigh {
		t.Errorf("Severity = %q, want HIGH", v.Severity)
	}
	if v.FixedIn != "18.19.1" {
		t.Errorf("FixedIn = %q, want 18.19.1", v.FixedIn)
	}
	if v.URL != "https://github.com/advisories/GHSA-aaaa-bbbb-cccc" {
		t.Errorf("URL = %q, want ADVISORY pick", v.URL)
	}
	if len(v.Aliases) != 1 || v.Aliases[0] != "CVE-2024-12345" {
		t.Errorf("Aliases = %v", v.Aliases)
	}
}

func TestOSVClient_Query_EmptyResponseIsNotError(t *testing.T) {
	url := fakeOSV(t, osvQueryResponse{})
	c := &OSVClient{URL: url}
	vulns, err := c.Query(context.Background(), Coord{
		Ecosystem: EcosystemNPM,
		Package:   "left-pad",
		Version:   "1.3.0",
	})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(vulns) != 0 {
		t.Errorf("expected 0 vulns, got %d", len(vulns))
	}
}

func TestOSVClient_Query_RejectsNonHTTPScheme(t *testing.T) {
	c := &OSVClient{URL: "file:///etc/passwd"}
	_, err := c.Query(context.Background(), Coord{Ecosystem: EcosystemNPM, Package: "x", Version: "1"})
	if err == nil {
		t.Fatal("expected error for file:// URL")
	}
}

func TestOSVClient_Query_PropagatesServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	c := &OSVClient{URL: srv.URL}
	_, err := c.Query(context.Background(), Coord{Ecosystem: EcosystemNPM, Package: "x", Version: "1"})
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

func TestPickSeverity_DatabaseSpecificWins(t *testing.T) {
	v := osvVulnerability{
		DatabaseSpecific: osvDatabaseSpecific{Severity: "MODERATE"},
		Severity: []osvSeverityEntry{
			{Type: "CVSS_V3", Score: "9.8"},
		},
	}
	if got := pickSeverity(v); got != SeverityMedium {
		t.Errorf("got %q, want MEDIUM (db-specific wins over CVSS)", got)
	}
}

func TestPickSeverity_FallsBackToCVSSNumber(t *testing.T) {
	v := osvVulnerability{
		Severity: []osvSeverityEntry{
			{Type: "CVSS_V3", Score: "7.5"},
			{Type: "CVSS_V3", Score: "5.0"},
		},
	}
	if got := pickSeverity(v); got != SeverityHigh {
		t.Errorf("got %q, want HIGH", got)
	}
}

func TestPickSeverity_Unknown(t *testing.T) {
	v := osvVulnerability{
		Severity: []osvSeverityEntry{
			// Vector form — we don't compute the score; should fall through.
			{Type: "CVSS_V3", Score: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"},
		},
	}
	if got := pickSeverity(v); got != SeverityUnknown {
		t.Errorf("got %q, want UNKNOWN for vector-only score", got)
	}
}

func TestPickFixedIn(t *testing.T) {
	affected := []osvAffected{
		{Package: osvPackage{Name: "other", Ecosystem: "npm"}, Ranges: []osvRange{
			{Events: []osvEvent{{Fixed: "0.0.1"}}},
		}},
		{Package: osvPackage{Name: "node", Ecosystem: "npm"}, Ranges: []osvRange{
			{Type: "ECOSYSTEM", Events: []osvEvent{
				{Introduced: "0"},
				{Fixed: "18.19.1"},
			}},
		}},
	}
	got := pickFixedIn(affected, Coord{Package: "node", Ecosystem: EcosystemNPM})
	if got != "18.19.1" {
		t.Errorf("got %q, want 18.19.1", got)
	}
	got = pickFixedIn(affected, Coord{Package: "absent", Ecosystem: EcosystemNPM})
	if got != "" {
		t.Errorf("missing package should give empty FixedIn, got %q", got)
	}
}

func TestPickFixedIn_RejectsGitRanges(t *testing.T) {
// OSV GIT ranges contain commit hashes, not version strings —
// surfacing them as 'FixedIn' would render junk in the CLI/web
// (e.g. 'fixed in abc123...'). Filter them out.
affected := []osvAffected{
{Package: osvPackage{Name: "node", Ecosystem: "npm"}, Ranges: []osvRange{
{Type: "GIT", Events: []osvEvent{{Fixed: "abc123def456"}}},
{Type: "ECOSYSTEM", Events: []osvEvent{{Fixed: "18.19.1"}}},
}},
}
got := pickFixedIn(affected, Coord{Package: "node", Ecosystem: EcosystemNPM})
if got != "18.19.1" {
t.Errorf("got %q, want 18.19.1 (GIT range commit hash should be skipped)", got)
}
}

func TestPickFixedIn_AllowsEmptyType(t *testing.T) {
// Some OSV records omit the range type. Be permissive there —
// the value still has to be version-shaped, but we don't have a
// way to enforce that without parsing. Common case in the wild.
affected := []osvAffected{
{Package: osvPackage{Name: "node", Ecosystem: "npm"}, Ranges: []osvRange{
{Type: "", Events: []osvEvent{{Fixed: "1.2.3"}}},
}},
}
got := pickFixedIn(affected, Coord{Package: "node", Ecosystem: EcosystemNPM})
if got != "1.2.3" {
t.Errorf("got %q, want 1.2.3 (empty type is allowed)", got)
}
}
