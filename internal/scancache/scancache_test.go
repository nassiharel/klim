package scancache

import (
	"os"
	"path/filepath"
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

	catalog := []registry.Tool{
		{Name: "kubectl", DisplayName: "kubectl", Category: "Kubernetes"},
		{Name: "helm", DisplayName: "helm", Category: "Kubernetes"},
	}
	entries := map[string]Entry{
		"kubectl": {
			Instances:  []instanceYAML{{Path: "/bin/kubectl", Version: "1.30.0", Source: "brew"}},
			Latest:     "1.30.1",
			LatestFrom: "brew",
		},
	}

	got := Apply(catalog, entries)

	if len(got[0].Instances) != 1 || got[0].Instances[0].Path != "/bin/kubectl" {
		t.Errorf("kubectl instances not applied: %+v", got[0].Instances)
	}
	if got[0].Latest != "1.30.1" {
		t.Errorf("kubectl latest not applied: %q", got[0].Latest)
	}
	if len(got[1].Instances) != 0 {
		t.Errorf("helm should remain uninstalled: %+v", got[1].Instances)
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
