package scancache

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nassiharel/clim/internal/registry"
)

// withTempCache redirects UserConfigDir to a temp dir for the duration of
// the test so Save/Load operate on a fresh cache.
func withTempCache(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	switch {
	case isWindows():
		t.Setenv("APPDATA", tmp)
	default:
		t.Setenv("XDG_CONFIG_HOME", tmp)
		t.Setenv("HOME", tmp)
	}
}

func isWindows() bool {
	return os.PathSeparator == '\\'
}

func sampleTool() registry.Tool {
	return registry.Tool{
		Name:        "kubectl",
		DisplayName: "kubectl",
		Category:    "Kubernetes",
		Instances: []registry.Instance{
			{Path: "/usr/local/bin/kubectl", Version: "1.30.0", Source: registry.SourceBrew},
		},
		Latest:     "1.30.1",
		LatestFrom: "brew",
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	withTempCache(t)

	tools := []registry.Tool{sampleTool()}
	if err := Save(tools); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !Exists() {
		t.Fatalf("Exists returned false after Save")
	}

	entries, savedAt, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if savedAt.IsZero() {
		t.Errorf("expected non-zero savedAt")
	}
	entry, ok := entries["kubectl"]
	if !ok {
		t.Fatalf("kubectl missing from cache")
	}
	if entry.Latest != "1.30.1" || entry.LatestFrom != "brew" {
		t.Errorf("unexpected latest fields: %+v", entry)
	}
	if len(entry.Instances) != 1 || entry.Instances[0].Version != "1.30.0" {
		t.Errorf("unexpected instances: %+v", entry.Instances)
	}
}

func TestApplyOverlaysFields(t *testing.T) {
	withTempCache(t)

	// Apply now drops cached instances whose path no longer exists,
	// so the fixture has to point at a real file. Use a temp file
	// the test owns and tears down.
	dir := t.TempDir()
	binPath := filepath.Join(dir, "kubectl")
	if err := os.WriteFile(binPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}

	catalog := []registry.Tool{
		{Name: "kubectl", DisplayName: "kubectl", Category: "Kubernetes"},
		{Name: "helm", DisplayName: "helm", Category: "Kubernetes"},
	}
	entries := map[string]Entry{
		"kubectl": {
			Instances:  []instanceYAML{{Path: binPath, Version: "1.30.0", Source: "brew"}},
			Latest:     "1.30.1",
			LatestFrom: "brew",
		},
	}

	got := Apply(catalog, entries)

	if len(got[0].Instances) != 1 || got[0].Instances[0].Path != binPath {
		t.Errorf("kubectl instances not applied: %+v", got[0].Instances)
	}
	if got[0].Latest != "1.30.1" {
		t.Errorf("kubectl latest not applied: %q", got[0].Latest)
	}
	if len(got[1].Instances) != 0 {
		t.Errorf("helm should remain uninstalled: %+v", got[1].Instances)
	}
}

// TestApplyDropsStaleInstances locks in the fix for the
// 'jq shows installed via winget but where jq finds nothing'
// bug: a cached instance whose path no longer exists must be
// dropped on Apply, otherwise the TUI will offer a remove plan
// that ultimately fails (winget rejects with NO_APPLICATIONS_FOUND
// or the user gets a confusing 'no such file' error).
func TestApplyDropsStaleInstances(t *testing.T) {
	withTempCache(t)

	dir := t.TempDir()
	missing := filepath.Join(dir, "ghost")

	catalog := []registry.Tool{{Name: "ghost", DisplayName: "ghost"}}
	entries := map[string]Entry{
		"ghost": {
			Instances: []instanceYAML{{Path: missing, Version: "1.0", Source: "winget"}},
		},
	}
	got := Apply(catalog, entries)
	if len(got[0].Instances) != 0 {
		t.Errorf("stale instance should be dropped: %+v", got[0].Instances)
	}
}

func TestDeleteRemovesFile(t *testing.T) {
	withTempCache(t)

	if err := Save([]registry.Tool{sampleTool()}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := Delete(); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if Exists() {
		t.Fatalf("cache still exists after Delete")
	}
	// Second delete should be a no-op.
	if err := Delete(); err != nil {
		t.Fatalf("Delete on missing file returned error: %v", err)
	}
}

func TestSaveSkipsEmptyEntries(t *testing.T) {
	withTempCache(t)

	// Tool with latest known but not installed — should also be skipped.
	notInstalledWithLatest := registry.Tool{
		Name: "terraform", DisplayName: "Terraform",
		Latest: "1.9.0", LatestFrom: "brew",
	}

	tools := []registry.Tool{
		sampleTool(),
		{Name: "ansible", DisplayName: "Ansible"}, // not installed, no latest
		{Name: "argocd", DisplayName: "ArgoCD"},   // not installed, no latest
		notInstalledWithLatest,
	}
	if err := Save(tools); err != nil {
		t.Fatalf("Save: %v", err)
	}
	entries, _, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := entries["kubectl"]; !ok {
		t.Errorf("kubectl should be cached")
	}
	for _, name := range []string{"ansible", "argocd", "terraform"} {
		if _, ok := entries[name]; ok {
			t.Errorf("%s is not installed — should not be cached", name)
		}
	}
}

func TestLoadMissingReturnsError(t *testing.T) {
	withTempCache(t)

	_, _, err := Load()
	if err == nil {
		t.Fatalf("expected error loading missing cache")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected os.IsNotExist-compatible error, got %T: %v", err, err)
	}
}

func TestIncompatibleVersionRejected(t *testing.T) {
	withTempCache(t)

	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("version: 99\ntools: {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, _, err := Load(); err == nil {
		t.Fatalf("expected error for incompatible schema version")
	}
}

func TestApplyRecoversViaPATHWhenCachedPathStale(t *testing.T) {
	// Cached path is gone, but the binary is still on PATH (e.g.
	// brew rotated its versioned dir). Apply should keep the tool
	// installed by re-resolving via exec.LookPath.
	withTempCache(t)

	dir := t.TempDir()
	binName := "fake-tool-on-path"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	realPath := filepath.Join(dir, binName)
	if err := os.WriteFile(realPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	stalePath := filepath.Join(dir, "no-longer-here")
	catalog := []registry.Tool{{Name: "fake-tool-on-path", BinaryNames: []string{strings.TrimSuffix(binName, ".exe")}}}
	entries := map[string]Entry{
		"fake-tool-on-path": {
			Instances: []instanceYAML{{Path: stalePath, Version: "1.0", Source: "brew"}},
		},
	}

	got := Apply(catalog, entries)
	if len(got[0].Instances) != 1 {
		t.Fatalf("expected recovery via PATH, got %d instances", len(got[0].Instances))
	}
	// Path should be re-resolved, not the stale one.
	if got[0].Instances[0].Path == stalePath {
		t.Errorf("expected path to be re-resolved, still got stale %q", got[0].Instances[0].Path)
	}
}

func TestApplyRecoveryDedupes(t *testing.T) {
	// Two stale cached instances must not both append the same
	// recovered binary — the recovery should run once per tool and
	// the recovered path is deduped in the per-tool seen-set.
	withTempCache(t)

	dir := t.TempDir()
	binName := "rec-tool"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	realPath := filepath.Join(dir, binName)
	if err := os.WriteFile(realPath, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	stale1 := filepath.Join(dir, "old-1")
	stale2 := filepath.Join(dir, "old-2")
	catalog := []registry.Tool{{
		Name:        "rec-tool",
		BinaryNames: []string{strings.TrimSuffix(binName, ".exe")},
	}}
	entries := map[string]Entry{
		"rec-tool": {
			Instances: []instanceYAML{
				{Path: stale1, Version: "1.0", Source: "brew"},
				{Path: stale2, Version: "1.0", Source: "winget"},
			},
		},
	}

	got := Apply(catalog, entries)
	if len(got[0].Instances) != 1 {
		t.Fatalf("expected single recovered instance, got %d: %+v", len(got[0].Instances), got[0].Instances)
	}
}

func TestApplyRecoveryRespectsPathOrder(t *testing.T) {
	// Tools with multiple binary names (e.g. python: python3, python)
	// must recover the binary in the earliest PATH dir, not the
	// first alias's first match. Mirrors finder.scanDir's
	// 'for dir { for name }' nesting.
	withTempCache(t)
	dirA := t.TempDir()
	dirB := t.TempDir()

	// dirA holds 'python' (the second alias).
	pyBin := "python"
	if runtime.GOOS == "windows" {
		pyBin += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dirA, pyBin), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// dirB holds 'python3' (the first alias).
	py3Bin := "python3"
	if runtime.GOOS == "windows" {
		py3Bin += ".exe"
	}
	if err := os.WriteFile(filepath.Join(dirB, py3Bin), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// PATH order: dirA, dirB. The earlier dir's 'python' should
	// win even though 'python3' is the first alias the finder
	// would normally try.
	t.Setenv("PATH", dirA+string(os.PathListSeparator)+dirB+string(os.PathListSeparator)+os.Getenv("PATH"))

	stale := filepath.Join(dirA, "stale-no-such-path")
	catalog := []registry.Tool{{
		Name:        "python",
		BinaryNames: []string{strings.TrimSuffix(py3Bin, ".exe"), strings.TrimSuffix(pyBin, ".exe")},
	}}
	entries := map[string]Entry{
		"python": {Instances: []instanceYAML{{Path: stale, Source: "winget"}}},
	}
	got := Apply(catalog, entries)
	if len(got[0].Instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(got[0].Instances))
	}
	wantPath := filepath.Join(dirA, pyBin)
	resolvedWant, _ := filepath.EvalSymlinks(wantPath)
	if resolvedWant == "" {
		resolvedWant = wantPath
	}
	if got[0].Instances[0].Path != resolvedWant {
		t.Errorf("got %q, want %q (earliest PATH dir should win regardless of alias order)",
			got[0].Instances[0].Path, resolvedWant)
	}
}
