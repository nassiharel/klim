package catalog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDiff_NewTools(t *testing.T) {
	local := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
`)
	remote := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
  - name: fzf
    display_name: fzf
    category: CLI
    binary_names: [fzf]
`)

	result := Diff(local, remote)

	if !result.HasChanges() {
		t.Fatal("expected HasChanges=true")
	}
	if len(result.NewTools) != 1 || result.NewTools[0] != "fzf" {
		t.Errorf("NewTools = %v, want [fzf]", result.NewTools)
	}
	if len(result.ChangedTools) != 0 {
		t.Errorf("ChangedTools = %v, want empty", result.ChangedTools)
	}
}

func TestDiff_ChangedFields(t *testing.T) {
	local := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
`)
	remote := []byte(`tools:
  - name: git
    display_name: Git SCM
    category: Version Control
    binary_names: [git]
`)

	result := Diff(local, remote)

	if !result.HasChanges() {
		t.Fatal("expected HasChanges=true")
	}
	changes, ok := result.ChangedTools["git"]
	if !ok {
		t.Fatal("expected git in ChangedTools")
	}
	if len(changes) != 2 {
		t.Errorf("expected 2 changed fields, got %d: %v", len(changes), changes)
	}
}

func TestDiff_NoChanges(t *testing.T) {
	data := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
    packages:
      brew: git
`)

	result := Diff(data, data)

	if result.HasChanges() {
		t.Errorf("expected no changes, got NewTools=%v ChangedTools=%v",
			result.NewTools, result.ChangedTools)
	}
}

func TestDiff_EmptyLocal(t *testing.T) {
	remote := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
`)

	result := Diff(nil, remote)

	if len(result.NewTools) != 1 {
		t.Errorf("expected 1 new tool, got %d", len(result.NewTools))
	}
}

func TestDiff_EmptyRemote(t *testing.T) {
	local := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
`)

	result := Diff(local, nil)

	if result.HasChanges() {
		t.Error("expected no changes when remote is empty (removals are ignored)")
	}
}

func TestDiff_RemovedTools(t *testing.T) {
	local := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
  - name: oldtool
    display_name: Old Tool
    category: CLI
    binary_names: [oldtool]
`)
	remote := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
`)

	result := Diff(local, remote)

	if !result.HasChanges() {
		t.Fatal("expected changes when a tool is removed upstream")
	}
	if len(result.RemovedTools) != 1 || result.RemovedTools[0] != "oldtool" {
		t.Errorf("RemovedTools = %v, want [oldtool]", result.RemovedTools)
	}
	if len(result.NewTools) != 0 {
		t.Errorf("NewTools = %v, want empty", result.NewTools)
	}
	if len(result.ChangedTools) != 0 {
		t.Errorf("ChangedTools = %v, want empty", result.ChangedTools)
	}
}

func TestDiff_MixedAddChangeRemove(t *testing.T) {
	local := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
  - name: oldtool
    display_name: Old Tool
    category: CLI
    binary_names: [oldtool]
`)
	remote := []byte(`tools:
  - name: git
    display_name: Git SCM
    category: VCS
    binary_names: [git]
  - name: fzf
    display_name: fzf
    category: CLI
    binary_names: [fzf]
`)

	result := Diff(local, remote)

	if len(result.NewTools) != 1 || result.NewTools[0] != "fzf" {
		t.Errorf("NewTools = %v, want [fzf]", result.NewTools)
	}
	if len(result.RemovedTools) != 1 || result.RemovedTools[0] != "oldtool" {
		t.Errorf("RemovedTools = %v, want [oldtool]", result.RemovedTools)
	}
	if _, ok := result.ChangedTools["git"]; !ok {
		t.Errorf("expected git in ChangedTools, got %v", result.ChangedTools)
	}
}

func TestDiff_PackagesChanged(t *testing.T) {
	local := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
    packages:
      brew: git
`)
	remote := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
    packages:
      brew: git
      winget: Git.Git
`)

	result := Diff(local, remote)

	if !result.HasChanges() {
		t.Fatal("expected changes when packages differ")
	}
	changes := result.ChangedTools["git"]
	found := false
	for _, c := range changes {
		if c == "packages" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'packages' in changed fields, got %v", changes)
	}
}

func TestDiff_TagsChanged(t *testing.T) {
	local := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
    tags: [vcs]
`)
	remote := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
    tags: [vcs, scm, version-control]
`)

	result := Diff(local, remote)

	if !result.HasChanges() {
		t.Fatal("expected changes when tags differ")
	}
}

func TestIsValidCatalog(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"valid", []byte("tools:\n  - name: git\n"), true},
		{"empty tools", []byte("tools: []\n"), false},
		{"invalid yaml", []byte("{{invalid"), false},
		{"empty", []byte(""), false},
		{"nil", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidCatalog(tt.data)
			if got != tt.want {
				t.Errorf("isValidCatalog() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFetch_Success(t *testing.T) {
	yamlData := "tools:\n  - name: git\n    display_name: Git\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(yamlData))
	}))
	defer srv.Close()

	fetcher := &GitHubFetcher{URL: srv.URL + "/marketplace.yaml"}
	data, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != yamlData {
		t.Errorf("Fetch() = %q, want %q", string(data), yamlData)
	}
}

func TestFetch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	fetcher := &GitHubFetcher{URL: srv.URL + "/marketplace.yaml"}
	_, err := fetcher.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error for 404")
	}
}

func TestFetch_TooLarge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := make([]byte, maxCatalogSize+100)
		w.Write(data)
	}))
	defer srv.Close()

	fetcher := &GitHubFetcher{URL: srv.URL + "/marketplace.yaml"}
	_, err := fetcher.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error for oversized response")
	}
}

func TestFetch_CancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("tools:\n  - name: test\n"))
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	fetcher := &GitHubFetcher{URL: srv.URL}
	_, err := fetcher.Fetch(ctx)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// setConfigDir redirects os.UserConfigDir() to a per-test temp dir across
// Linux/macOS/Windows so cache writes don't leak into the real user home.
func setConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir) // linux
	t.Setenv("HOME", dir)            // macOS fallback ($HOME/Library/Application Support)
	t.Setenv("AppData", dir)         // windows
	return dir
}

type stubFetcher struct {
	data []byte
	err  error
	n    int
}

func (f *stubFetcher) Fetch(context.Context) ([]byte, error) {
	f.n++
	return f.data, f.err
}

func TestLoadOrFetch_FreshCacheServedWithoutRefetch(t *testing.T) {
	setConfigDir(t)

	path, err := CachePath()
	if err != nil {
		t.Fatal(err)
	}
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr != nil {
		t.Fatal(mkErr)
	}
	cached := []byte("tools:\n  - name: git\n    display_name: Git\n")
	if writeErr := os.WriteFile(path, cached, 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}

	fetcher := &stubFetcher{data: []byte("tools:\n  - name: git\n")}
	res, err := LoadOrFetchWithOptions(context.Background(), fetcher, LoadOptions{MaxAge: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourceCache {
		t.Errorf("Source = %q, want cache", res.Source)
	}
	if fetcher.n != 0 {
		t.Errorf("fetcher called %d times, want 0 for a fresh cache", fetcher.n)
	}
	if res.Diff != nil {
		t.Errorf("Diff = %+v, want nil for cache hit", res.Diff)
	}
}

func TestLoadOrFetch_StaleCacheTriggersRefresh(t *testing.T) {
	setConfigDir(t)

	path, err := CachePath()
	if err != nil {
		t.Fatal(err)
	}
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr != nil {
		t.Fatal(mkErr)
	}
	cached := []byte("tools:\n  - name: git\n    display_name: Git\n    category: VCS\n    binary_names: [git]\n  - name: oldtool\n    display_name: Old\n    category: CLI\n    binary_names: [oldtool]\n")
	if writeErr := os.WriteFile(path, cached, 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}
	// Backdate the cache so it's considered stale.
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	remote := []byte("tools:\n  - name: git\n    display_name: Git SCM\n    category: VCS\n    binary_names: [git]\n  - name: fzf\n    display_name: fzf\n    category: CLI\n    binary_names: [fzf]\n")
	fetcher := &stubFetcher{data: remote}

	res, err := LoadOrFetchWithOptions(context.Background(), fetcher, LoadOptions{MaxAge: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if res.Source != SourceRemote {
		t.Errorf("Source = %q, want remote", res.Source)
	}
	if fetcher.n != 1 {
		t.Errorf("fetcher called %d times, want 1", fetcher.n)
	}
	if res.Diff == nil {
		t.Fatal("expected Diff to be populated after auto-refresh")
	}
	if len(res.Diff.NewTools) != 1 || res.Diff.NewTools[0] != "fzf" {
		t.Errorf("Diff.NewTools = %v, want [fzf]", res.Diff.NewTools)
	}
	if len(res.Diff.RemovedTools) != 1 || res.Diff.RemovedTools[0] != "oldtool" {
		t.Errorf("Diff.RemovedTools = %v, want [oldtool]", res.Diff.RemovedTools)
	}
	if _, ok := res.Diff.ChangedTools["git"]; !ok {
		t.Errorf("Diff.ChangedTools missing git: %v", res.Diff.ChangedTools)
	}

	// Cache should have been rewritten with the remote payload.
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(onDisk) != string(remote) {
		t.Errorf("cache not updated after auto-refresh")
	}
}

func TestLoadOrFetch_StaleCacheFallsBackOnFetchFailure(t *testing.T) {
	setConfigDir(t)

	path, err := CachePath()
	if err != nil {
		t.Fatal(err)
	}
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr != nil {
		t.Fatal(mkErr)
	}
	cached := []byte("tools:\n  - name: git\n    display_name: Git\n")
	if writeErr := os.WriteFile(path, cached, 0o644); writeErr != nil {
		t.Fatal(writeErr)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	fetcher := &stubFetcher{err: errors.New("network down")}
	res, err := LoadOrFetchWithOptions(context.Background(), fetcher, LoadOptions{MaxAge: time.Hour})
	if err != nil {
		t.Fatalf("expected stale-cache fallback, got error: %v", err)
	}
	if res.Source != SourceCache {
		t.Errorf("Source = %q, want cache (stale fallback)", res.Source)
	}
	if string(res.Data) != string(cached) {
		t.Errorf("expected stale cache bytes to be returned on fetch failure")
	}
}
