package vuln

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/nassiharel/clim/internal/registry"
)

// stubLooker returns canned vulnerabilities per coord. Errors only when
// requested — used to test stale-cache fallback.
type stubLooker struct {
	byPackage map[string][]Vulnerability
	err       error
}

func (s *stubLooker) Query(ctx context.Context, coord Coord) ([]Vulnerability, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.byPackage[coord.Package], nil
}

func setEnvDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("AppData", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
}

func mkTool(name, version, npmPkg, brewPkg, slug string) registry.Tool {
	return registry.Tool{
		Name:       name,
		GitHubSlug: slug,
		Packages: registry.PackageIDs{
			NPM:  npmPkg,
			Brew: brewPkg,
		},
		Instances: []registry.Instance{
			{Path: "/usr/local/bin/" + name, Version: version, Source: registry.SourceBrew},
		},
	}
}

func TestLookup_HappyPath(t *testing.T) {
	setEnvDir(t, t.TempDir())

	looker := &stubLooker{byPackage: map[string][]Vulnerability{
		"node": {
			{ID: "GHSA-aaa", Severity: SeverityHigh, FixedIn: "20.0.0", Summary: "RCE"},
		},
		"yarn": nil, // no vulns — clean
	}}

	tools := []registry.Tool{
		mkTool("node", "18.10.0", "node", "", ""),
		mkTool("yarn", "1.22.0", "yarn", "", ""),
	}

	rep, err := Lookup(context.Background(), looker, tools, "https://api.osv.dev", LookupOptions{})
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if rep.ToolsScanned != 2 {
		t.Errorf("ToolsScanned = %d, want 2", rep.ToolsScanned)
	}
	if len(rep.Matches) != 2 {
		t.Fatalf("matches = %d, want 2", len(rep.Matches))
	}
	// Find node match
	var nodeMatch *Match
	for i := range rep.Matches {
		if rep.Matches[i].Tool == "node" {
			nodeMatch = &rep.Matches[i]
		}
	}
	if nodeMatch == nil || len(nodeMatch.Vulnerabilities) != 1 {
		t.Fatalf("expected node match with 1 vuln, got %+v", nodeMatch)
	}
	if nodeMatch.MaxSeverity() != SeverityHigh {
		t.Errorf("node MaxSeverity = %q", nodeMatch.MaxSeverity())
	}
}

func TestLookup_UsesFreshCache(t *testing.T) {
	setEnvDir(t, t.TempDir())

	tools := []registry.Tool{mkTool("node", "18.10.0", "node", "", "")}
	srcKey := "https://api.osv.dev"

	first := &stubLooker{byPackage: map[string][]Vulnerability{
		"node": {{ID: "GHSA-aaa", Severity: SeverityHigh}},
	}}
	rep1, err := Lookup(context.Background(), first, tools, srcKey, LookupOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep1.Matches) != 1 || len(rep1.Matches[0].Vulnerabilities) != 1 {
		t.Fatal("first call didn't populate")
	}

	// Second call: stub returns DIFFERENT data; if cache hits, we get
	// the OLD report.
	second := &stubLooker{byPackage: map[string][]Vulnerability{
		"node": {{ID: "GHSA-bbb", Severity: SeverityCritical}},
	}}
	rep2, err := Lookup(context.Background(), second, tools, srcKey, LookupOptions{MaxAge: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if rep2.Matches[0].Vulnerabilities[0].ID != "GHSA-aaa" {
		t.Errorf("expected cached GHSA-aaa, got %q (stale-cache miss)",
			rep2.Matches[0].Vulnerabilities[0].ID)
	}
}

func TestLookup_StaleCacheRefetches(t *testing.T) {
	setEnvDir(t, t.TempDir())

	tools := []registry.Tool{mkTool("node", "18.10.0", "node", "", "")}
	srcKey := "https://api.osv.dev"

	// Seed cache.
	cachePath, err := cachePathFor(srcKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatal(err)
	}
	old := &Report{
		Matches: []Match{{Tool: "node", Vulnerabilities: []Vulnerability{{ID: "STALE", Severity: SeverityLow}}}},
	}
	if err := writeCache(cachePath, old); err != nil {
		t.Fatal(err)
	}
	// Backdate it.
	pastTime := time.Now().Add(-25 * time.Hour)
	_ = os.Chtimes(cachePath, pastTime, pastTime)

	fresh := &stubLooker{byPackage: map[string][]Vulnerability{
		"node": {{ID: "FRESH", Severity: SeverityHigh}},
	}}
	rep, err := Lookup(context.Background(), fresh, tools, srcKey, LookupOptions{MaxAge: 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Matches[0].Vulnerabilities[0].ID != "FRESH" {
		t.Errorf("stale cache should have been refetched; got %q", rep.Matches[0].Vulnerabilities[0].ID)
	}
}

func TestLookup_FetchFailureFallsBackToStaleCache(t *testing.T) {
	setEnvDir(t, t.TempDir())

	tools := []registry.Tool{mkTool("node", "18.10.0", "node", "", "")}
	srcKey := "https://api.osv.dev"

	// Seed an old cache.
	cachePath, _ := cachePathFor(srcKey)
	_ = os.MkdirAll(filepath.Dir(cachePath), 0o755)
	old := &Report{
		Matches: []Match{{Tool: "node", Vulnerabilities: []Vulnerability{{ID: "STALE", Severity: SeverityLow}}}},
	}
	_ = writeCache(cachePath, old)
	pastTime := time.Now().Add(-48 * time.Hour)
	_ = os.Chtimes(cachePath, pastTime, pastTime)

	failing := &stubLooker{err: errors.New("network down")}
	rep, err := Lookup(context.Background(), failing, tools, srcKey, LookupOptions{MaxAge: 24 * time.Hour})
	if err != nil {
		t.Fatalf("expected stale-cache fallback, got error: %v", err)
	}
	if rep.Matches[0].Vulnerabilities[0].ID != "STALE" {
		t.Errorf("expected stale fallback STALE, got %q", rep.Matches[0].Vulnerabilities[0].ID)
	}
}

func TestLookup_FetchFailureNoCachePropagates(t *testing.T) {
	setEnvDir(t, t.TempDir())

	tools := []registry.Tool{mkTool("node", "18.10.0", "node", "", "")}
	srcKey := "https://api.osv.dev"

	failing := &stubLooker{err: errors.New("network down")}
	_, err := Lookup(context.Background(), failing, tools, srcKey, LookupOptions{})
	if err == nil {
		t.Fatal("expected error when fetch fails and no cache exists")
	}
}

func TestLookup_SkipsUnmappableTool(t *testing.T) {
	setEnvDir(t, t.TempDir())

	tools := []registry.Tool{
		mkTool("node", "18.10.0", "node", "", ""),
		// No NPM, no Brew, no GitHub slug → unmappable.
		{
			Name: "exotic",
			Instances: []registry.Instance{
				{Path: "/usr/bin/exotic", Version: "1.0", Source: registry.SourceApt},
			},
		},
	}

	looker := &stubLooker{byPackage: map[string][]Vulnerability{
		"node": {{ID: "GHSA-aaa", Severity: SeverityHigh}},
	}}

	rep, err := Lookup(context.Background(), looker, tools, "https://api.osv.dev", LookupOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Skipped) != 1 || rep.Skipped[0].Tool != "exotic" {
		t.Errorf("expected exotic in Skipped, got %+v", rep.Skipped)
	}
	if len(rep.Matches) != 1 || rep.Matches[0].Tool != "node" {
		t.Errorf("expected only node in Matches, got %+v", rep.Matches)
	}
}

// guards Windows: setEnvDir works there, but make sure the cache-path
// derivation doesn't accidentally use Unix-style separators.
func TestLookup_CacheUsesPlatformPathSeparator(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only path separator check")
	}
	setEnvDir(t, t.TempDir())
	cachePath, _ := cachePathFor("any-key")
	if !filepath.IsAbs(cachePath) {
		t.Errorf("cache path should be absolute, got %q", cachePath)
	}
}
