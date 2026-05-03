package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/service"
)

// startServerWithOpts is the most flexible test helper — callers fully
// control Options. Use this for tests that exercise non-default
// behaviors like AuthToken or Config.
func startServerWithOpts(t *testing.T, opts Options) (*httptest.Server, *fixtureLoader, *Server) {
	t.Helper()
	if opts.Service == nil {
		opts.Service = service.New()
	}
	if opts.Bind == "" {
		opts.Bind = "127.0.0.1"
	}
	srv, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	loader := &fixtureLoader{tools: fixtureToolWithPackages(), favs: map[string]bool{}}
	srv.loader = loader
	t.Cleanup(func() { _ = srv.Close() })
	ts := httptest.NewServer(srv.httpsrv.Handler)
	t.Cleanup(ts.Close)
	return ts, loader, srv
}

// --- Auth ---

func TestAuth_HealthzAlwaysOpen(t *testing.T) {
	ts, _, _ := startServerWithOpts(t, Options{AuthToken: "secret123"})
	resp, body := get(t, ts.URL+"/healthz")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d body=%s", resp.StatusCode, body)
	}
}

func TestAuth_RejectsHTMLWithoutToken(t *testing.T) {
	ts, _, _ := startServerWithOpts(t, Options{AuthToken: "secret123"})
	resp, body := get(t, ts.URL+"/")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, "Authentication required") {
		t.Fatalf("expected paste-the-token page, got:\n%s", body)
	}
}

func TestAuth_RejectsAPIWithoutToken(t *testing.T) {
	ts, _, _ := startServerWithOpts(t, Options{AuthToken: "secret123"})
	resp, body := get(t, ts.URL+"/api/tools")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, "missing or invalid token") {
		t.Fatalf("expected JSON error: %s", body)
	}
}

func TestAuth_AcceptsBearerHeader(t *testing.T) {
	ts, _, _ := startServerWithOpts(t, Options{AuthToken: "secret123"})
	c := &http.Client{}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/tools", nil)
	req.Header.Set("Authorization", "Bearer secret123")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestAuth_RejectsBearerWithWrongToken(t *testing.T) {
	ts, _, _ := startServerWithOpts(t, Options{AuthToken: "secret123"})
	c := &http.Client{}
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/tools", nil)
	req.Header.Set("Authorization", "Bearer wrongthing")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestAuth_QueryTokenSetsCookieAndRedirects(t *testing.T) {
	ts, _, _ := startServerWithOpts(t, Options{AuthToken: "secret123"})
	c := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := c.Get(ts.URL + "/?token=secret123")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc != "/" {
		t.Fatalf("redirect target: %q, want /", loc)
	}
	var found bool
	for _, ck := range resp.Cookies() {
		if ck.Name == authCookieName && ck.Value == "secret123" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected auth cookie to be set, got %+v", resp.Cookies())
	}
}

func TestAuth_CookieAuthorizesSubsequentRequests(t *testing.T) {
	ts, _, _ := startServerWithOpts(t, Options{AuthToken: "secret123"})
	jar, _ := cookieJar()
	c := &http.Client{Jar: jar, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	// First visit primes the cookie.
	if _, err := c.Get(ts.URL + "/?token=secret123"); err != nil {
		t.Fatal(err)
	}
	// Now a plain GET should pass.
	resp, err := c.Get(ts.URL + "/api/tools")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 with cookie, got %d", resp.StatusCode)
	}
}

func TestAuth_RejectsWrongQueryToken(t *testing.T) {
	ts, _, _ := startServerWithOpts(t, Options{AuthToken: "secret123"})
	resp, _ := get(t, ts.URL+"/?token=wrong")
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status: %d", resp.StatusCode)
	}
}

func TestAuth_NoTokenMeansOpenAccess(t *testing.T) {
	ts, _, _ := startServerWithOpts(t, Options{})
	resp, _ := get(t, ts.URL+"/api/tools")
	if resp.StatusCode != 200 {
		t.Fatalf("default config should be open, got %d", resp.StatusCode)
	}
}

func TestAuthedURL_AppendsToken(t *testing.T) {
	cases := []struct {
		base, token, want string
	}{
		{"http://127.0.0.1:7777", "abc", "http://127.0.0.1:7777?token=abc"},
		{"http://127.0.0.1:7777/", "abc", "http://127.0.0.1:7777/?token=abc"},
		{"http://127.0.0.1:7777", "", "http://127.0.0.1:7777"},
	}
	for _, tc := range cases {
		if got := authedURL(tc.base, tc.token); got != tc.want {
			t.Errorf("authedURL(%q,%q)=%q, want %q", tc.base, tc.token, got, tc.want)
		}
	}
}

// --- Per-tool job concurrency lock ---

func TestJobs_RejectsSecondJobForSameTool(t *testing.T) {
	gate := make(chan struct{})
	exec := newScriptedExecutor([]string{"working"}, nil, gate)
	ts, _, srv := startTestServerWithExecutor(t, exec)

	// First job — never released, so it stays running.
	first, err := srv.jobs.Start(context.Background(), ActionInstall, "terraform", "brew",
		[]string{"brew", "install", "terraform"})
	if err != nil {
		t.Fatal(err)
	}

	// Second attempt via API should 409 with a redirect to the first job.
	resp, body := postJSONSameOrigin(t, ts.URL, "/api/jobs", map[string]any{
		"action": "install", "tool": "terraform",
	})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status: %d body=%s", resp.StatusCode, body)
	}
	var p struct {
		ExistingJobID string `json:"existing_job_id"`
		RedirectTo    string `json:"redirect_to"`
	}
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatal(err)
	}
	if p.ExistingJobID != first.ID {
		t.Errorf("existing_job_id=%q, want %q", p.ExistingJobID, first.ID)
	}
	if p.RedirectTo != "/jobs/"+first.ID {
		t.Errorf("redirect_to=%q", p.RedirectTo)
	}
	// Release the first job so the test can finish cleanly.
	close(gate)
}

func TestJobs_AllowsSameToolAfterPreviousFinishes(t *testing.T) {
	exec := newScriptedExecutor([]string{"installed"}, nil, nil)
	_, _, srv := startTestServerWithExecutor(t, exec)
	first, err := srv.jobs.Start(context.Background(), ActionInstall, "terraform", "brew",
		[]string{"brew", "install", "terraform"})
	if err != nil {
		t.Fatal(err)
	}
	// Wait for the first job to finish.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		snap := srv.jobs.Snapshot(first.ID)
		if snap != nil && snap.Status != JobStatusRunning {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if snap := srv.jobs.Snapshot(first.ID); snap == nil || snap.Status == JobStatusRunning {
		t.Fatalf("first job did not finish in time")
	}
	// Now a second job for the same tool should be accepted —
	// running entry was cleared when the first finished.
	if _, err := srv.jobs.Start(context.Background(), ActionUpgrade, "terraform", "brew",
		[]string{"brew", "upgrade", "terraform"}); err != nil {
		t.Fatalf("second job for same tool should succeed once first is done: %v", err)
	}
}

// --- Backup / Config ---

func TestPageBackup_RendersExportLinkAndShareToken(t *testing.T) {
	ts, _, _ := startServerWithOpts(t, Options{})
	resp, body := get(t, ts.URL+"/backup")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, "/backup/export.yaml") {
		t.Fatalf("missing export link: %s", body)
	}
	if !strings.Contains(body, "clim:v1:") {
		t.Fatalf("missing share token: %s", body)
	}
}

func TestDownloadExport_ReturnsYAMLAttachment(t *testing.T) {
	ts, _, _ := startServerWithOpts(t, Options{})
	resp, body := get(t, ts.URL+"/backup/export.yaml")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/yaml") {
		t.Fatalf("content-type: %q", ct)
	}
	if !strings.Contains(resp.Header.Get("Content-Disposition"), "clim-export.yaml") {
		t.Fatalf("missing attachment header: %q", resp.Header.Get("Content-Disposition"))
	}
	if !strings.Contains(body, "tools:") {
		t.Fatalf("expected tools key in YAML, got:\n%s", body)
	}
}

func TestPageConfig_RendersYAMLDump(t *testing.T) {
	cfg := config.Default()
	ts, _, _ := startServerWithOpts(t, Options{Config: cfg})
	resp, body := get(t, ts.URL+"/config")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, "marketplace:") {
		t.Fatalf("expected marketplace section in rendered config, got:\n%s", body)
	}
}

func TestPageConfig_HandlesMissingConfig(t *testing.T) {
	ts, _, _ := startServerWithOpts(t, Options{})
	resp, body := get(t, ts.URL+"/config")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, "no config loaded") {
		t.Fatalf("expected no-config copy, got:\n%s", body)
	}
}

// --- helpers ---

// cookieJar wraps net/http/cookiejar so the test file doesn't need to
// import cookiejar from every test.
func cookieJar() (http.CookieJar, error) {
	return newCookieJar()
}
