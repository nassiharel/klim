package compliance

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setEnvDir(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("AppData", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
}

func TestHTTPFetcher(t *testing.T) {
	policy := []byte("name: test-policy\nblocked_tools:\n  - foo\n")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write(policy)
	}))
	defer srv.Close()

	f := &HTTPFetcher{URL: srv.URL}
	data, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(policy) {
		t.Errorf("got %q, want %q", data, policy)
	}
}

func TestHTTPFetcherNoURL(t *testing.T) {
	f := &HTTPFetcher{}
	_, err := f.Fetch(context.Background())
	if err == nil {
		t.Error("expected error for empty URL")
	}
}

func TestHTTPFetcherServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := &HTTPFetcher{URL: srv.URL}
	_, err := f.Fetch(context.Background())
	if err == nil {
		t.Error("expected error for 500")
	}
}

func TestHTTPFetcher_RejectsNonHTTPScheme(t *testing.T) {
	cases := []string{
		"file:///etc/passwd",
		"ftp://example.com/policy.yaml",
		"javascript:alert(1)",
		"://no-scheme",
		"http://",
	}
	for _, u := range cases {
		t.Run(u, func(t *testing.T) {
			f := &HTTPFetcher{URL: u}
			_, err := f.Fetch(context.Background())
			if err == nil {
				t.Errorf("expected error for %q", u)
			}
		})
	}
}

func TestHTTPFetcher_RejectsRedirectChain(t *testing.T) {
	// Build a chain longer than maxRedirects. Each handler 302's to
	// the next; the last would serve the policy if reached. With a
	// CheckRedirect cap we expect Fetch to fail before reaching it.
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	for i := 0; i < maxRedirects+2; i++ {
		i := i
		mux.HandleFunc(fmt.Sprintf("/r%d", i), func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, fmt.Sprintf("/r%d", i+1), http.StatusFound)
		})
	}
	mux.HandleFunc(fmt.Sprintf("/r%d", maxRedirects+2), func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("name: too-late\n"))
	})
	f := &HTTPFetcher{URL: srv.URL + "/r0"}
	if _, err := f.Fetch(context.Background()); err == nil {
		t.Errorf("expected redirect-chain rejection")
	}
}

func TestHTTPFetcher_RedirectToFileSchemeRejected(t *testing.T) {
	// Server tries to bounce us to file:// — CheckRedirect must refuse.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
	}))
	defer srv.Close()
	f := &HTTPFetcher{URL: srv.URL}
	if _, err := f.Fetch(context.Background()); err == nil {
		t.Error("expected scheme-downgrade redirect to be rejected")
	}
}

type stubFetcher struct {
	data []byte
	err  error
}

func (s *stubFetcher) Fetch(ctx context.Context) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.data, nil
}

func TestLoadOrFetch_NoCacheFetches(t *testing.T) {
	tmp := t.TempDir()
	setEnvDir(t, tmp)

	f := &stubFetcher{data: []byte("name: remote-policy\n")}
	p, path, err := LoadOrFetch(context.Background(), f, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "remote-policy" {
		t.Errorf("name = %q, want remote-policy", p.Name)
	}

	// Cache should now exist.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("cache file not created: %v", err)
	}
}

func TestLoadOrFetch_FreshCacheReturnsCache(t *testing.T) {
	tmp := t.TempDir()
	setEnvDir(t, tmp)

	cachePath, _ := CachePath()
	_ = os.MkdirAll(filepath.Dir(cachePath), 0o755)
	_ = os.WriteFile(cachePath, []byte("name: cached-policy\n"), 0o644)

	f := &stubFetcher{err: errAlwaysFail}
	p, _, err := LoadOrFetch(context.Background(), f, LoadOptions{MaxAge: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "cached-policy" {
		t.Errorf("got %q, want cached-policy (fresh cache should be served without fetch)", p.Name)
	}
}

func TestLoadOrFetch_StaleCacheRefetches(t *testing.T) {
	tmp := t.TempDir()
	setEnvDir(t, tmp)

	cachePath, _ := CachePath()
	_ = os.MkdirAll(filepath.Dir(cachePath), 0o755)
	_ = os.WriteFile(cachePath, []byte("name: old-policy\n"), 0o644)

	// Make cache old.
	old := time.Now().Add(-25 * time.Hour)
	_ = os.Chtimes(cachePath, old, old)

	f := &stubFetcher{data: []byte("name: new-policy\n")}
	p, _, err := LoadOrFetch(context.Background(), f, LoadOptions{MaxAge: 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "new-policy" {
		t.Errorf("got %q, want new-policy (stale cache should refetch)", p.Name)
	}
}

func TestLoadOrFetch_StaleCache_FetchFails_ServesStale(t *testing.T) {
	tmp := t.TempDir()
	setEnvDir(t, tmp)

	cachePath, _ := CachePath()
	_ = os.MkdirAll(filepath.Dir(cachePath), 0o755)
	_ = os.WriteFile(cachePath, []byte("name: stale-policy\n"), 0o644)
	old := time.Now().Add(-25 * time.Hour)
	_ = os.Chtimes(cachePath, old, old)

	f := &stubFetcher{err: errAlwaysFail}
	p, _, err := LoadOrFetch(context.Background(), f, LoadOptions{MaxAge: 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "stale-policy" {
		t.Errorf("got %q, want stale-policy (failed refetch should serve stale)", p.Name)
	}
}

var errAlwaysFail = newSimpleErr("fetch failed")

type simpleErr struct{ s string }

func (e *simpleErr) Error() string { return e.s }
func newSimpleErr(s string) error  { return &simpleErr{s: s} }
