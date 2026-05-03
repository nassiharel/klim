package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/service"
)

// fixtureLoader serves deterministic tool data so handler tests don't
// depend on the host machine's PATH or installed package managers.
type fixtureLoader struct {
	tools []registry.Tool
	favs  map[string]bool
}

func (l *fixtureLoader) LoadInstalled(_ context.Context) ([]registry.Tool, catalogSummary, error) {
	out := make([]registry.Tool, len(l.tools))
	copy(out, l.tools)
	return out, catalogSummary{Source: "fixture", Count: len(out)}, nil
}

func (l *fixtureLoader) LoadTool(_ context.Context, name string) (registry.Tool, error) {
	for i := range l.tools {
		if strings.EqualFold(l.tools[i].Name, name) {
			return l.tools[i], nil
		}
	}
	return registry.Tool{}, notFoundError{Name: name}
}

func (l *fixtureLoader) Favorites() (map[string]bool, error) {
	if l.favs == nil {
		return map[string]bool{}, nil
	}
	out := make(map[string]bool, len(l.favs))
	for k, v := range l.favs {
		out[k] = v
	}
	return out, nil
}

func (l *fixtureLoader) ToggleFavorite(name string) (bool, error) {
	if l.favs == nil {
		l.favs = map[string]bool{}
	}
	if l.favs[name] {
		delete(l.favs, name)
		return false, nil
	}
	l.favs[name] = true
	return true, nil
}

func fixtureTools() []registry.Tool {
	return []registry.Tool{
		{
			Name:        "git",
			DisplayName: "Git",
			Category:    "Version Control",
			Tags:        []string{"vcs"},
			Latest:      "2.53.0",
			LatestFrom:  "winget",
			Instances: []registry.Instance{
				{Path: "/usr/bin/git", Version: "2.53.0", Source: registry.SourceWinget},
			},
		},
		{
			Name:        "kubectl",
			DisplayName: "kubectl",
			Category:    "Containers",
			Tags:        []string{"kubernetes", "cli"},
			Latest:      "1.31.0",
			LatestFrom:  "brew",
			Instances: []registry.Instance{
				{Path: "/usr/local/bin/kubectl", Version: "1.28.4", Source: registry.SourceBrew},
			},
		},
		{
			// Catalog-only tool — not installed on this fixture host.
			Name:        "terraform",
			DisplayName: "Terraform",
			Category:    "IaC",
			Tags:        []string{"hashicorp", "infrastructure"},
		},
	}
}

// startTestServer wires a Server to an httptest.NewServer with the
// fixture loader so tests can hit real HTTP without flakey port races.
func startTestServer(t *testing.T) (*httptest.Server, *fixtureLoader) {
	t.Helper()
	srv, err := New(Options{Service: service.New(), Bind: "127.0.0.1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	loader := &fixtureLoader{tools: fixtureTools(), favs: map[string]bool{}}
	srv.loader = loader
	t.Cleanup(func() { _ = srv.Close() })
	ts := httptest.NewServer(srv.httpsrv.Handler)
	t.Cleanup(ts.Close)
	return ts, loader
}

func get(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, string(body)
}

func TestServer_Healthz(t *testing.T) {
	ts, _ := startTestServer(t)
	resp, body := get(t, ts.URL+"/healthz")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, "ok") {
		t.Fatalf("body: %q", body)
	}
}

func TestServer_InstalledPage(t *testing.T) {
	ts, _ := startTestServer(t)
	resp, body := get(t, ts.URL+"/")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d (body=%s)", resp.StatusCode, body)
	}
	if !strings.Contains(body, "Installed tools") {
		t.Fatalf("missing page header: %q", body)
	}
	if !strings.Contains(body, ">git<") || !strings.Contains(body, ">kubectl<") {
		t.Fatalf("expected git and kubectl rows, got:\n%s", body)
	}
	// kubectl is outdated in the fixture (1.28.4 < 1.31.0).
	if !strings.Contains(body, "update") {
		t.Fatalf("expected update badge for kubectl, got:\n%s", body)
	}
	// git is up to date.
	if !strings.Contains(body, "up to date") {
		t.Fatalf("expected ok badge for git, got:\n%s", body)
	}
}

func TestServer_InstalledPage_FilterByQuery(t *testing.T) {
	ts, _ := startTestServer(t)
	resp, body := get(t, ts.URL+"/?q=git")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, ">git<") {
		t.Fatalf("query=git should keep git row")
	}
	if strings.Contains(body, ">kubectl<") {
		t.Fatalf("query=git should drop kubectl row, got:\n%s", body)
	}
}

func TestServer_ToolPage(t *testing.T) {
	ts, _ := startTestServer(t)
	resp, body := get(t, ts.URL+"/tools/git")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d (body=%s)", resp.StatusCode, body)
	}
	if !strings.Contains(body, "git") || !strings.Contains(body, "/usr/bin/git") {
		t.Fatalf("tool page missing detail: %s", body)
	}
}

func TestServer_ToolPage_NotFound(t *testing.T) {
	ts, _ := startTestServer(t)
	resp, _ := get(t, ts.URL+"/tools/this-tool-does-not-exist")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestServer_DashboardPage(t *testing.T) {
	ts, _ := startTestServer(t)
	resp, body := get(t, ts.URL+"/dashboard")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, "Dashboard") {
		t.Fatalf("missing dashboard header: %s", body)
	}
}

func TestServer_StubPages(t *testing.T) {
	ts, _ := startTestServer(t)
	for _, path := range []string{"/backup", "/config"} {
		resp, body := get(t, ts.URL+path)
		if resp.StatusCode != 200 {
			t.Errorf("%s: status %d", path, resp.StatusCode)
		}
		if !strings.Contains(body, "Coming soon") {
			t.Errorf("%s: expected stub copy, got:\n%s", path, body)
		}
	}
}

func TestServer_StaticAsset(t *testing.T) {
	ts, _ := startTestServer(t)
	resp, body := get(t, ts.URL+"/static/styles.css")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, "--bg") {
		t.Fatalf("expected CSS variables in body")
	}
}

func TestAPI_Tools(t *testing.T) {
	ts, _ := startTestServer(t)
	resp, body := get(t, ts.URL+"/api/tools")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d (%s)", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content-type: %q", ct)
	}
	var payload struct {
		Count int `json:"count"`
		Tools []struct {
			Name            string `json:"name"`
			Installed       bool   `json:"installed"`
			UpdateAvailable bool   `json:"update_available"`
		} `json:"tools"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, body)
	}
	if payload.Count != 3 {
		t.Fatalf("count=%d, want 3", payload.Count)
	}
	byName := map[string]bool{}
	for _, t := range payload.Tools {
		byName[t.Name] = t.UpdateAvailable
	}
	if byName["kubectl"] != true {
		t.Errorf("kubectl should have update_available=true (1.28.4 < 1.31.0)")
	}
	if byName["git"] != false {
		t.Errorf("git should have update_available=false (2.53.0 == 2.53.0)")
	}
}

func TestAPI_Tool_NotFound(t *testing.T) {
	ts, _ := startTestServer(t)
	resp, body := get(t, ts.URL+"/api/tools/nope")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: %d (%s)", resp.StatusCode, body)
	}
	if !strings.Contains(body, "error") {
		t.Fatalf("expected error payload: %s", body)
	}
}

func TestAPI_Dashboard(t *testing.T) {
	ts, _ := startTestServer(t)
	resp, body := get(t, ts.URL+"/api/dashboard")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var d dashboardView
	if err := json.Unmarshal([]byte(body), &d); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, body)
	}
	if d.InstalledTools != 2 {
		t.Errorf("installed_tools=%d, want 2", d.InstalledTools)
	}
	if d.UpdatesAvail != 1 {
		t.Errorf("updates_available=%d, want 1 (kubectl)", d.UpdatesAvail)
	}
}

func TestServer_RejectsNonLoopbackBindWithoutInsecureFlag(t *testing.T) {
	_, err := New(Options{Service: service.New(), Bind: "0.0.0.0"})
	if err == nil {
		t.Fatal("expected refusal for non-loopback bind without --insecure-bind")
	}
	if !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("error should mention loopback: %v", err)
	}
}

func TestServer_AcceptsInsecureBindOptIn(t *testing.T) {
	srv, err := New(Options{Service: service.New(), Bind: "127.0.0.1", InsecureBind: true, Port: 0})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = srv.Close()
}

func TestIsLoopback(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"127.0.0.1", true},
		{"localhost", true},
		{"::1", true},
		{"0.0.0.0", false},
		{"192.168.1.1", false},
		{"example.com", false},
	}
	for _, tc := range cases {
		if got := isLoopback(tc.host); got != tc.want {
			t.Errorf("isLoopback(%q)=%v, want %v", tc.host, got, tc.want)
		}
	}
}
