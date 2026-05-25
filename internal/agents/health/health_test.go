package health

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMCPReach_StdioCommandResolution(t *testing.T) {
	// Use go (always on PATH for CI) as a known-good command.
	resolvable := "go"
	if _, err := os.Stat("/bin/sh"); err == nil {
		resolvable = "/bin/sh"
	}
	c := &checkMCPReach{HTTPProbe: func(context.Context, string) error { return nil }}
	snap := Snapshot{MCPs: []MCPRef{
		{Name: "ok", Scope: "user", Transport: "stdio", Command: resolvable},
		{Name: "missing", Scope: "user", Transport: "stdio", Command: "definitely-not-a-real-binary-xyz123"},
	}}
	issues := c.Run(context.Background(), snap)
	if len(issues) != 1 || issues[0].Subject != "missing" {
		t.Errorf("expected 1 issue for 'missing', got %+v", issues)
	}
	if issues[0].Severity != SeverityError {
		t.Errorf("missing-cmd severity = %v, want ERROR", issues[0].Severity)
	}
}

func TestMCPReach_HTTPProbe(t *testing.T) {
	probeErr := errors.New("connection refused")
	probe := func(context.Context, string) error { return probeErr }
	c := &checkMCPReach{HTTPProbe: probe}
	snap := Snapshot{MCPs: []MCPRef{
		{Name: "bad", Scope: "user", Transport: "http", URL: "https://nope.example"},
		{Name: "no-url", Scope: "user", Transport: "http"},
		{Name: "remote", Scope: "remote", Transport: "http", URL: "https://anything"},
	}}
	issues := c.Run(context.Background(), snap)
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues (bad URL + no URL), got %d: %+v", len(issues), issues)
	}
	subjects := map[string]bool{}
	for _, i := range issues {
		subjects[i.Subject] = true
	}
	for _, want := range []string{"bad", "no-url"} {
		if !subjects[want] {
			t.Errorf("missing issue for %q", want)
		}
	}
}

func TestDuplicateMCP(t *testing.T) {
	c := &checkDuplicateMCP{}
	snap := Snapshot{MCPs: []MCPRef{
		{Name: "shared", Provider: "claude-code", Scope: "user"},
		{Name: "shared", Provider: "claude-code", Scope: "project"},
		{Name: "shared", Provider: "copilot-cli", Scope: "user"}, // different provider, not a duplicate
		{Name: "unique", Provider: "claude-code", Scope: "user"},
	}}
	issues := c.Run(context.Background(), snap)
	if len(issues) != 1 || issues[0].Subject != "shared" {
		t.Errorf("expected one 'shared' duplicate, got %+v", issues)
	}
}

func TestShadowedSkill(t *testing.T) {
	c := &checkShadowedSkill{}
	snap := Snapshot{Skills: []SkillRef{
		{Name: "summary", Provider: "claude-code", Scope: "user"},
		{Name: "summary", Provider: "claude-code", Scope: "plugin", SourcePlugin: "my-pack"},
		{Name: "ship-it", Provider: "claude-code", Scope: "plugin", SourcePlugin: "my-pack"}, // not shadowed
	}}
	issues := c.Run(context.Background(), snap)
	if len(issues) != 1 || issues[0].Subject != "summary" {
		t.Errorf("expected one 'summary' shadow, got %+v", issues)
	}
}

func TestPluginManifest_MissingPath(t *testing.T) {
	c := &checkPluginManifest{}
	snap := Snapshot{Plugins: []PluginRef{
		{Name: "p1", Installed: true, InstallPath: "/definitely/not/a/path", Version: "1.0"},
	}}
	issues := c.Run(context.Background(), snap)
	if len(issues) != 1 || issues[0].Severity != SeverityError {
		t.Errorf("expected 1 ERROR issue, got %+v", issues)
	}
}

func TestBrokenJSON(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.json")
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(good, []byte(`{"ok": true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte(`{not-json`), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &checkBrokenJSON{}
	snap := Snapshot{ConfigFiles: []ConfigFileRef{
		{Path: good, Provider: "x"},
		{Path: bad, Provider: "x"},
		{Path: filepath.Join(dir, "missing.json"), Provider: "x"},
	}}
	issues := c.Run(context.Background(), snap)
	if len(issues) != 1 || issues[0].Subject != "bad.json" {
		t.Errorf("expected one 'bad.json' issue, got %+v", issues)
	}
}

func TestStaleCatalog(t *testing.T) {
	now := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	c := &checkStaleCatalog{ThresholdDays: 14, NowFunc: func() time.Time { return now }}
	snap := Snapshot{Marketplaces: []MarketplaceRef{
		{Name: "fresh", LastSynced: now.Add(-2 * 24 * time.Hour)},
		{Name: "stale", LastSynced: now.Add(-30 * 24 * time.Hour)},
		{Name: "never", LastSynced: time.Time{}}, // zero — skipped
	}}
	issues := c.Run(context.Background(), snap)
	if len(issues) != 1 || issues[0].Subject != "stale" {
		t.Errorf("expected one 'stale' issue, got %+v", issues)
	}
	if issues[0].Severity != SeverityInfo {
		t.Errorf("stale severity = %v, want INFO", issues[0].Severity)
	}
}

func TestProviderInstalled(t *testing.T) {
	c := &checkProviderInstalled{}
	snap := Snapshot{Providers: []ProviderRef{
		{ID: "claude-code", Installed: true},
		{ID: "copilot-cli", Installed: false},
	}}
	issues := c.Run(context.Background(), snap)
	if len(issues) != 1 || issues[0].Subject != "copilot-cli" {
		t.Errorf("expected only copilot-cli missing, got %+v", issues)
	}
}

func TestRun_SortsErrorsFirst(t *testing.T) {
	snap := Snapshot{
		MCPs:        []MCPRef{{Name: "broken", Scope: "user", Transport: "http"}},
		ConfigFiles: []ConfigFileRef{},
		Providers:   []ProviderRef{{ID: "claude-code", Installed: false}},
	}
	checks := []Check{&checkMCPReach{HTTPProbe: func(context.Context, string) error { return nil }}, &checkProviderInstalled{}}
	issues := Run(context.Background(), snap, checks)
	if len(issues) == 0 {
		t.Fatal("expected issues")
	}
	// First issue should be the ERROR (no URL), not the INFO (provider missing).
	if issues[0].Severity != SeverityError {
		t.Errorf("first severity = %v, want ERROR", issues[0].Severity)
	}
}

func TestCountIssues(t *testing.T) {
	issues := []Issue{
		{Severity: SeverityError},
		{Severity: SeverityError},
		{Severity: SeverityWarn},
		{Severity: SeverityInfo},
	}
	c := CountIssues(issues)
	if c.Error != 2 || c.Warn != 1 || c.Info != 1 {
		t.Errorf("counts = %+v", c)
	}
}
