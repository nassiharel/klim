package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/service"
)

func TestLifecycle_ShutsDownWhenLastTabClosesAfterGrace(t *testing.T) {
	var fired atomic.Int32
	cancel := func() { fired.Add(1) }
	lc := newLifecycle(20*time.Millisecond, cancel)

	lc.register()
	lc.register()
	lc.unregister()
	// Still 1 connection — grace timer should not have started.
	time.Sleep(40 * time.Millisecond)
	if fired.Load() != 0 {
		t.Fatalf("cancel fired with one tab still open")
	}

	lc.unregister()
	// Now 0 connections — grace timer running. Wait for it.
	time.Sleep(60 * time.Millisecond)
	if fired.Load() != 1 {
		t.Fatalf("cancel should have fired exactly once, got %d", fired.Load())
	}
}

func TestLifecycle_NewConnectionCancelsPendingShutdown(t *testing.T) {
	var fired atomic.Int32
	cancel := func() { fired.Add(1) }
	lc := newLifecycle(50*time.Millisecond, cancel)

	lc.register()
	lc.unregister() // schedules shutdown after 50ms
	// Reconnect before the grace timer fires.
	time.Sleep(20 * time.Millisecond)
	lc.register()
	time.Sleep(60 * time.Millisecond)
	if fired.Load() != 0 {
		t.Fatalf("cancel should not fire when a new connection arrived during grace, got %d", fired.Load())
	}
}

func TestApiLifecycleStream_TracksConnections(t *testing.T) {
	srv, err := New(Options{
		Service:            service.New(),
		Bind:               "127.0.0.1",
		AutoShutdownOnIdle: true,
		AutoShutdownGrace:  50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	srv.loader = &fixtureLoader{tools: fixtureTools(), favs: map[string]bool{}}
	t.Cleanup(func() { _ = srv.Close() })

	var fired atomic.Int32
	srv.EnableAutoShutdown(func() { fired.Add(1) })

	ts := httptest.NewServer(srv.httpsrv.Handler)
	defer ts.Close()

	ctx, cancelReq := context.WithCancel(context.Background())
	defer cancelReq()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/lifecycle", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}

	// Read the first event so we know the server has registered the
	// connection.
	buf := make([]byte, 64)
	if _, err := resp.Body.Read(buf); err != nil {
		t.Fatalf("read: %v", err)
	}

	// Drop the client. The grace timer should fire shortly after.
	cancelReq()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && fired.Load() == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if fired.Load() != 1 {
		t.Fatalf("expected shutdown cancel to fire exactly once after the last tab closed, got %d", fired.Load())
	}
}

func TestServer_JobContextCancelsOnShutdown(t *testing.T) {
	// Regression for the PR #48 review: jobs used to run under
	// context.Background() and would not be cancelled when the
	// server shut down, orphaning long-running install subprocesses
	// past clim's own exit. Fixed by deriving a server-lifetime
	// jobCtx from Serve's parent ctx; this test confirms that
	// cancelling Serve's ctx propagates to jobCtx.
	srv, err := New(Options{
		Service: service.New(),
		Bind:    "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	srv.loader = &fixtureLoader{tools: fixtureToolWithPackages(), favs: map[string]bool{}}
	t.Cleanup(func() { _ = srv.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	// Run Serve in the background; it'll exit cleanly when we cancel ctx.
	done := make(chan struct{})
	go func() {
		_ = srv.Serve(ctx)
		close(done)
	}()
	// Wait until Serve has installed its jobCtx (replacement happens
	// at the top of Serve, before its goroutine spins up).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		jctx := srv.jobContext()
		if jctx != nil && jctx != context.Background() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	// The job context must be alive at this point.
	jctx := srv.jobContext()
	select {
	case <-jctx.Done():
		t.Fatal("job context was already cancelled before server shutdown")
	default:
	}

	// Trigger shutdown.
	cancel()
	// jobCtx should fire promptly — Serve's parent cancellation
	// propagates through context.WithCancel.
	select {
	case <-jctx.Done():
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("job context did not cancel after server shutdown")
	}
	<-done
}

func TestPageTool_RendersPMRowsAndRelated(t *testing.T) {
	allOSPkgs := registry.PackageIDs{Brew: "kubectl", Winget: "Kubernetes.kubectl", Apt: "kubectl"}
	tools := []registry.Tool{
		{
			Name:      "kubectl",
			Tags:      []string{"kubernetes"},
			Latest:    "1.31.0",
			Instances: []registry.Instance{{Path: "/k", Version: "1.31.0", Source: registry.SourceBrew}},
			Packages:  allOSPkgs,
		},
		{
			Name:     "stern",
			Tags:     []string{"kubernetes"},
			Packages: allOSPkgs,
		},
	}
	srv, err := New(Options{Service: service.New(), Bind: "127.0.0.1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	srv.loader = &fixtureLoader{tools: tools, favs: map[string]bool{"kubectl": true}}
	t.Cleanup(func() { _ = srv.Close() })
	ts := httptest.NewServer(srv.httpsrv.Handler)
	defer ts.Close()

	resp, body := get(t, ts.URL+"/tools/kubectl")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, "Package managers") {
		t.Fatalf("expected 'Package managers' section, got:\n%s", body)
	}
	if !strings.Contains(body, "kubectl") || !strings.Contains(body, "install") {
		t.Fatalf("expected install command in PM table, got:\n%s", body)
	}
	if !strings.Contains(body, "You might also like") {
		t.Fatalf("expected related section, got:\n%s", body)
	}
	if !strings.Contains(body, "stern") {
		t.Fatalf("expected stern in related, got:\n%s", body)
	}
	if !strings.Contains(body, "Unfavorite") {
		t.Fatalf("expected 'Unfavorite' button when tool is favorited, got:\n%s", body)
	}
}

func TestBuildDashboard_PopulatesAllSections(t *testing.T) {
	tools := []registry.Tool{
		{
			Name:       "kubectl",
			Category:   "Containers",
			Tags:       []string{"kubernetes", "cli"},
			Latest:     "1.31.0",
			Instances:  []registry.Instance{{Path: "/k", Version: "1.28.4", Source: registry.SourceBrew}},
			GitHubInfo: &registry.GitHubInfo{Stars: 1234, PushedAt: "2026-04-01T00:00:00Z"},
		},
		{
			Name:     "git",
			Category: "Version Control",
			Tags:     []string{"vcs"},
			Latest:   "2.53.0",
			Instances: []registry.Instance{
				{Path: "/git", Version: "2.53.0", Source: registry.SourceBrew},
			},
		},
	}
	favs := map[string]bool{"git": true}
	packs := []registry.Pack{
		{Name: "fully", ToolNames: []string{"git"}},
		{Name: "partial", ToolNames: []string{"git", "missing"}},
	}
	view := buildDashboard(tools, favs, packs, nil, 3)

	if view.InstalledTools != 2 {
		t.Errorf("InstalledTools=%d, want 2", view.InstalledTools)
	}
	if view.UpdatesAvail != 1 {
		t.Errorf("UpdatesAvail=%d, want 1 (kubectl)", view.UpdatesAvail)
	}
	if view.PctInstalled != 100 {
		t.Errorf("PctInstalled=%d, want 100 (everything in fixture is installed)", view.PctInstalled)
	}
	if len(view.StarredHighlights) != 1 || view.StarredHighlights[0].Name != "kubectl" {
		t.Errorf("StarredHighlights=%+v", view.StarredHighlights)
	}
	if view.PacksMarketplaceFull != 1 || view.PacksMarketplacePartial != 1 {
		t.Errorf("packs full=%d partial=%d, want 1/1", view.PacksMarketplaceFull, view.PacksMarketplacePartial)
	}
	if view.BackupCount != 3 {
		t.Errorf("BackupCount=%d", view.BackupCount)
	}
	if len(view.BySource) == 0 {
		t.Errorf("BySource should not be empty")
	}
}
