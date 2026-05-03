package web

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/service"
)

// scriptedExecutor emits a fixed set of lines and a chosen final
// error when Run is called. Lines are gated on a release channel so
// tests can pause execution between lines and observe streaming
// behavior deterministically.
type scriptedExecutor struct {
	lines  []string
	exit   error
	gate   chan struct{} // unbuffered; receiver blocks before each line
	gateMu sync.Mutex
}

func newScriptedExecutor(lines []string, exit error, gate chan struct{}) *scriptedExecutor {
	return &scriptedExecutor{lines: lines, exit: exit, gate: gate}
}

func (e *scriptedExecutor) Run(_ context.Context, _ []string) (<-chan string, <-chan error) {
	out := make(chan string, len(e.lines))
	done := make(chan error, 1)
	go func() {
		for _, line := range e.lines {
			if e.gate != nil {
				<-e.gate
			}
			out <- line
		}
		close(out)
		done <- e.exit
		close(done)
	}()
	return out, done
}

// fixtureToolWithPackages returns the fixture with package definitions
// so resolveAction can build a real command (we never actually run the
// command; the scriptedExecutor short-circuits that).
func fixtureToolWithPackages() []registry.Tool {
	out := fixtureTools()
	for i := range out {
		switch out[i].Name {
		case "git":
			out[i].Packages = registry.PackageIDs{Brew: "git", Winget: "Git.Git", Apt: "git"}
		case "kubectl":
			out[i].Packages = registry.PackageIDs{Brew: "kubectl", Winget: "Kubernetes.kubectl"}
		case "terraform":
			out[i].Packages = registry.PackageIDs{Brew: "terraform", Winget: "Hashicorp.Terraform"}
		}
	}
	return out
}

// startTestServerWithExecutor wires a Server with a scripted executor
// so tests can drive job lifecycle deterministically without shelling
// out for real.
func startTestServerWithExecutor(t *testing.T, exec Executor) (*httptest.Server, *fixtureLoader, *Server) {
	t.Helper()
	srv, err := New(Options{Service: service.New(), Bind: "127.0.0.1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	loader := &fixtureLoader{tools: fixtureToolWithPackages(), favs: map[string]bool{}}
	srv.loader = loader
	srv.jobs = newJobManager(exec)
	t.Cleanup(func() { _ = srv.Close() })
	ts := httptest.NewServer(srv.httpsrv.Handler)
	t.Cleanup(ts.Close)
	return ts, loader, srv
}

func TestJobs_StartAndComplete(t *testing.T) {
	exec := newScriptedExecutor([]string{"==> Downloading", "==> Installing", "Done."}, nil, nil)
	ts, _, _ := startTestServerWithExecutor(t, exec)

	resp, body := postJSONSameOrigin(t, ts.URL, "/api/jobs", map[string]any{
		"action": "install", "tool": "terraform",
	})
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("create: status=%d body=%s", resp.StatusCode, body)
	}
	var created Job
	if err := json.Unmarshal([]byte(body), &created); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, body)
	}
	if created.Status != JobStatusRunning {
		t.Fatalf("expected running on create, got %s", created.Status)
	}
	if created.Source == "" || len(created.Cmd) == 0 {
		t.Fatalf("expected resolved cmd: %+v", created)
	}

	// Poll for completion. The scripted executor writes all 3 lines
	// without gating, so this resolves within a few ms in practice.
	deadline := time.Now().Add(2 * time.Second)
	var snap Job
	for time.Now().Before(deadline) {
		r2, b2 := get(t, ts.URL+"/api/jobs/"+created.ID)
		if r2.StatusCode != 200 {
			t.Fatalf("snapshot: %d", r2.StatusCode)
		}
		if err := json.Unmarshal([]byte(b2), &snap); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if snap.Status != JobStatusRunning {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if snap.Status != JobStatusSuccess {
		t.Fatalf("expected success, got status=%s err=%s", snap.Status, snap.Err)
	}
	if len(snap.Output) != 3 {
		t.Fatalf("expected 3 output lines, got %d (%v)", len(snap.Output), snap.Output)
	}
}

func TestJobs_StreamReplaysHistoryAndFinishes(t *testing.T) {
	gate := make(chan struct{}, 3)
	exec := newScriptedExecutor([]string{"first", "second", "third"}, nil, gate)
	ts, _, srv := startTestServerWithExecutor(t, exec)

	job, err := srv.jobs.Start(context.Background(), ActionInstall, "terraform", "brew", []string{"brew", "install", "terraform"})
	if err != nil {
		t.Fatal(err)
	}

	// Release first line, give it time to land in the buffer.
	gate <- struct{}{}
	time.Sleep(20 * time.Millisecond)

	// Subscribe — the SSE stream must replay the line we already wrote.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/jobs/"+job.ID+"/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("stream status: %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type: %q", ct)
	}

	// Release the rest so the runner finishes and the stream closes.
	go func() {
		gate <- struct{}{}
		gate <- struct{}{}
	}()

	// Read events until we see "done" or hit the deadline.
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var events []string
	var lines []string
	for scanner.Scan() {
		text := scanner.Text()
		if strings.HasPrefix(text, "event: ") {
			events = append(events, strings.TrimPrefix(text, "event: "))
		}
		if strings.HasPrefix(text, "data: ") {
			lines = append(lines, strings.TrimPrefix(text, "data: "))
		}
		if text == "" && len(events) > 0 && events[len(events)-1] == "done" {
			break
		}
	}

	// We expect 3 line events + 1 done event, and the data lines must
	// be in order.
	if got := countString(events, "line"); got != 3 {
		t.Errorf("line events: got %d, want 3 (events=%v)", got, events)
	}
	if got := countString(events, "done"); got != 1 {
		t.Errorf("done events: got %d, want 1", got)
	}
	if len(lines) < 3 {
		t.Fatalf("data lines: got %d, want at least 3 (%v)", len(lines), lines)
	}
	if lines[0] != "first" || lines[1] != "second" || lines[2] != "third" {
		t.Errorf("lines out of order: %v", lines)
	}
}

func TestJobs_StreamRejectsUnknownID(t *testing.T) {
	exec := newScriptedExecutor(nil, nil, nil)
	ts, _, _ := startTestServerWithExecutor(t, exec)
	resp, _ := get(t, ts.URL+"/api/jobs/notarealid/stream")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestJobs_RejectsUnknownAction(t *testing.T) {
	exec := newScriptedExecutor(nil, nil, nil)
	ts, _, _ := startTestServerWithExecutor(t, exec)
	resp, body := postJSONSameOrigin(t, ts.URL, "/api/jobs", map[string]any{
		"action": "nuke", "tool": "terraform",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
}

func TestJobs_RejectsUnknownTool(t *testing.T) {
	exec := newScriptedExecutor(nil, nil, nil)
	ts, _, _ := startTestServerWithExecutor(t, exec)
	resp, body := postJSONSameOrigin(t, ts.URL, "/api/jobs", map[string]any{
		"action": "install", "tool": "this-tool-does-not-exist",
	})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", resp.StatusCode, body)
	}
}

func TestJobs_FormSubmitRedirectsToJobPage(t *testing.T) {
	exec := newScriptedExecutor([]string{"installed"}, nil, nil)
	ts, _, _ := startTestServerWithExecutor(t, exec)
	c := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/tools/terraform/install", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", ts.URL)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.HasPrefix(loc, "/jobs/") {
		t.Fatalf("expected redirect to /jobs/<id>, got %q", loc)
	}
}

func TestJobs_RejectsCrossOriginPOST(t *testing.T) {
	exec := newScriptedExecutor(nil, nil, nil)
	ts, _, _ := startTestServerWithExecutor(t, exec)
	c := &http.Client{}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/jobs", strings.NewReader(`{"action":"install","tool":"terraform"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
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

// --- helpers ---

func postJSONSameOrigin(t *testing.T, ts string, path string, body map[string]any) (*http.Response, string) {
	t.Helper()
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, ts+path, strings.NewReader(string(buf)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", ts)
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp, string(respBody)
}

func countString(xs []string, want string) int {
	n := 0
	for _, x := range xs {
		if x == want {
			n++
		}
	}
	return n
}
