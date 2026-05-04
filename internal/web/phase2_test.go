package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// postSameOrigin sends a POST whose Origin matches the test server's
// URL so the CSRF middleware lets it through. The default
// http.Client follows redirects from POSTs, so we install a no-op
// CheckRedirect to capture the 303 our toggle handler emits.
func postSameOrigin(t *testing.T, ts string, path string) (*http.Response, string) {
	t.Helper()
	c := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequest(http.MethodPost, ts+path, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Origin", ts)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, string(body)
}

func TestServer_UpdatesPage_ListsOutdatedTools(t *testing.T) {
	ts, _ := startTestServer(t)
	resp, body := get(t, ts.URL+"/updates")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, "Updates available") {
		t.Fatalf("missing header: %s", body)
	}
	// kubectl is the only outdated tool in the fixture (1.28.4 < 1.31.0).
	if !strings.Contains(body, ">kubectl<") {
		t.Fatalf("expected kubectl row, got:\n%s", body)
	}
	if strings.Contains(body, ">git<") {
		t.Fatalf("git is up to date, should not appear in updates list:\n%s", body)
	}
}

func TestServer_DiscoverPage_ShowsCatalog(t *testing.T) {
	ts, _ := startTestServer(t)
	resp, body := get(t, ts.URL+"/discover")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	// All three fixture tools — including non-installed terraform —
	// must appear here.
	for _, name := range []string{"git", "kubectl", "terraform"} {
		if !strings.Contains(body, ">"+name+"<") {
			t.Errorf("expected %q row in /discover, got:\n%s", name, body)
		}
	}
	if !strings.Contains(body, "not installed") {
		t.Fatalf("expected not-installed badge for terraform")
	}
}

func TestServer_DiscoverPage_FilterByTag(t *testing.T) {
	ts, _ := startTestServer(t)
	resp, body := get(t, ts.URL+"/discover?tag=hashicorp")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, ">terraform<") {
		t.Fatalf("tag=hashicorp should keep terraform")
	}
	if strings.Contains(body, ">git<") || strings.Contains(body, ">kubectl<") {
		t.Fatalf("tag=hashicorp should drop git/kubectl, got:\n%s", body)
	}
}

func TestServer_FavoritesPage_Empty(t *testing.T) {
	ts, _ := startTestServer(t)
	resp, body := get(t, ts.URL+"/favorites")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, "No favorites yet") {
		t.Fatalf("expected empty-state copy:\n%s", body)
	}
}

func TestServer_FavoritesPage_AfterToggle(t *testing.T) {
	ts, loader := startTestServer(t)
	loader.favs["kubectl"] = true
	resp, body := get(t, ts.URL+"/favorites")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, ">kubectl<") {
		t.Fatalf("expected kubectl row in favorites list:\n%s", body)
	}
}

func TestAPI_FavoritesList(t *testing.T) {
	ts, loader := startTestServer(t)
	loader.favs["git"] = true
	resp, body := get(t, ts.URL+"/api/favorites")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d (%s)", resp.StatusCode, body)
	}
	var p struct {
		Favorites []string `json:"favorites"`
		Count     int      `json:"count"`
	}
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, body)
	}
	if p.Count != 1 || p.Favorites[0] != "git" {
		t.Fatalf("got %+v", p)
	}
}

func TestAPI_FavoritesToggle_Toggles(t *testing.T) {
	ts, loader := startTestServer(t)
	resp, body := postSameOrigin(t, ts.URL, "/api/favorites/git/toggle")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d (%s)", resp.StatusCode, body)
	}
	var p struct {
		Name     string `json:"name"`
		Favorite bool   `json:"favorite"`
	}
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, body)
	}
	if p.Name != "git" || !p.Favorite {
		t.Fatalf("first toggle should mark git as favorite, got %+v", p)
	}
	if !loader.favs["git"] {
		t.Fatalf("loader state should reflect git favorited")
	}
	// Toggle again — should remove.
	resp, body = postSameOrigin(t, ts.URL, "/api/favorites/git/toggle")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d (%s)", resp.StatusCode, body)
	}
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Favorite {
		t.Fatalf("second toggle should clear favorite, got %+v", p)
	}
	if loader.favs["git"] {
		t.Fatalf("loader state should reflect git un-favorited")
	}
}

func TestAPI_FavoritesToggle_RejectedWithoutOrigin(t *testing.T) {
	ts, _ := startTestServer(t)
	c := &http.Client{}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/favorites/git/toggle", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately omit Origin and Referer.
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestAPI_FavoritesToggle_RejectedFromForeignOrigin(t *testing.T) {
	ts, _ := startTestServer(t)
	c := &http.Client{}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/favorites/git/toggle", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "https://attacker.example")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.StatusCode)
	}
}

func TestPageFavoritesToggle_RedirectsBack(t *testing.T) {
	ts, loader := startTestServer(t)
	c := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/favorites/git/toggle", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", ts.URL)
	req.Header.Set("Referer", ts.URL+"/discover?q=git")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if want := ts.URL + "/discover?q=git"; loc != want {
		t.Fatalf("redirect target: got %q, want %q", loc, want)
	}
	if !loader.favs["git"] {
		t.Fatalf("loader state should reflect git favorited")
	}
}

func TestCSRF_RejectsDNSRebindingHost(t *testing.T) {
	// Regression for the PR #48 review: CSRF used to pass when the
	// browser was tricked via DNS rebinding into addressing the
	// request to attacker.com (resolved to 127.0.0.1) rather than a
	// loopback hostname — both Origin and r.Host would say
	// "attacker.com" and the same-origin check trivially passed.
	// The fix enforces a loopback-hostname allowlist on the Host
	// header when the server is bound to loopback.
	ts, _ := startTestServer(t)
	c := &http.Client{}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/favorites/git/toggle", nil)
	// Pretend the browser was tricked into addressing attacker.com.
	// Origin matches the bogus host so step 2 of csrfProtect would
	// pass; the Host allowlist is what saves us.
	req.Host = "attacker.example"
	req.Header.Set("Origin", "http://attacker.example")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d (DNS rebinding scenario should be blocked)", resp.StatusCode)
	}
}

func TestCSRF_AllowsLoopbackHostSpellings(t *testing.T) {
	// 127.0.0.1, localhost, and ::1 must all be accepted as Host
	// header values (after passing the same-origin check) when the
	// server is bound to loopback.
	ts, _ := startTestServer(t)
	cases := []struct {
		host   string
		origin string
	}{
		{"127.0.0.1:7777", "http://127.0.0.1:7777"},
		{"localhost:7777", "http://localhost:7777"},
		{"[::1]:7777", "http://[::1]:7777"},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			c := &http.Client{}
			req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/favorites/git/toggle", nil)
			req.Host = tc.host
			req.Header.Set("Origin", tc.origin)
			resp, err := c.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			// We don't assert 200 — the toggle handler may not match
			// these synthetic Host values for the same-origin check
			// (httptest binds 127.0.0.1 so the Origin must match what
			// the server actually saw). What matters is that the
			// REJECTION is not from the Host allowlist (which would
			// be 403 with body "host not allowed"). 200 / 403 cross-
			// origin rejection / 404 are all fine; "host not allowed"
			// is not.
			if resp.StatusCode == http.StatusForbidden {
				body := readAllString(t, resp)
				if strings.Contains(body, "host not allowed") {
					t.Errorf("loopback host %q rejected by allowlist", tc.host)
				}
			}
		})
	}
}

func TestIsLoopbackHostHeader(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"127.0.0.1:7777", true},
		{"localhost:7777", true},
		{"[::1]:7777", true},
		{"127.0.0.1", true},
		{"localhost", true},
		{"::1", true},
		{"attacker.example:7777", false},
		{"192.168.1.5:7777", false},
		{"example.com", false},
		{"", true}, // empty Host is unusual but not a rebinding signal
	}
	for _, tc := range cases {
		if got := isLoopbackHostHeader(tc.host); got != tc.want {
			t.Errorf("isLoopbackHostHeader(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestSameOriginRequest_LoopbackEquivalence(t *testing.T) {
	cases := []struct {
		name string
		want bool
		// expected is the server URL; origin is the request's Origin.
		expected, origin string
	}{
		{"exact match", true, "http://127.0.0.1:7777", "http://127.0.0.1:7777"},
		{"localhost↔127.0.0.1", true, "http://127.0.0.1:7777", "http://localhost:7777"},
		{"different port", false, "http://127.0.0.1:7777", "http://127.0.0.1:9999"},
		{"different scheme", false, "http://127.0.0.1:7777", "https://127.0.0.1:7777"},
		{"foreign host", false, "http://127.0.0.1:7777", "https://attacker.example"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &http.Request{Header: http.Header{}}
			req.Header.Set("Origin", tc.origin)
			if got := sameOriginRequest(tc.expected, req); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// TestSameOriginRequest_FallsBackToReferer covers the case where the
// browser omits Origin (older browsers, some same-origin requests) but
// includes Referer. The check should still pass as long as Referer
// resolves to the same origin.
func TestSameOriginRequest_FallsBackToReferer(t *testing.T) {
	req := &http.Request{Header: http.Header{}}
	req.Header.Set("Referer", "http://127.0.0.1:7777/discover")
	if !sameOriginRequest("http://127.0.0.1:7777", req) {
		t.Fatal("expected referer-based same-origin to pass")
	}
}

// Sanity: url.Parse round-trips the test server URL the way our CSRF
// helper expects, so the equivalence above isn't an artifact of the
// test bypassing the production code path.
func TestServerURL_ParsesCleanly(t *testing.T) {
	ts, _ := startTestServer(t)
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Hostname() == "" || u.Port() == "" {
		t.Fatalf("server URL missing host/port: %q", ts.URL)
	}
}
