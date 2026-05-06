package compliance

import (
	"bytes"
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
	if !bytes.Equal(data, policy) {
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

func TestHTTPFetcher_TrimsURLWhitespace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("name: trimmed\n"))
	}))
	defer srv.Close()
	f := &HTTPFetcher{URL: "  " + srv.URL + "  "}
	data, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatalf("expected trim to succeed, got %v", err)
	}
	if !bytes.Contains(data, []byte("trimmed")) {
		t.Errorf("unexpected payload: %s", data)
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

func TestLoadOrFetch_RejectsBadFreshFetchAndPreservesCache(t *testing.T) {
	dir := t.TempDir()
	setEnvDir(t, dir)

	// Seed a known-good cached policy.
	cachePath, err := CachePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatal(err)
	}
	good := []byte("name: good-policy\n")
	if err := os.WriteFile(cachePath, good, 0o644); err != nil {
		t.Fatal(err)
	}
	// Make the cache stale so LoadOrFetch will try to refresh.
	old := time.Now().Add(-25 * time.Hour)
	if err := os.Chtimes(cachePath, old, old); err != nil {
		t.Fatal(err)
	}

	// Stub fetcher returns an unparseable HTML/login-page response.
	bad := []byte("<html><body>Please sign in</body></html>")
	fetcher := &stubFetcher{data: bad}

	// Auto-refresh active. Under the old behaviour this would write
	// `bad` over `good`. Under the fixed behaviour, the cache is left
	// alone and the user keeps getting the previous policy.
	_, _, err = LoadOrFetch(context.Background(), fetcher, LoadOptions{MaxAge: 24 * time.Hour})
	if err != nil {
		t.Fatalf("expected fallback to stale cache, got error: %v", err)
	}

	stillThere, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stillThere, good) {
		t.Errorf("cache was poisoned. got %q, want %q", stillThere, good)
	}
}

func TestRefresh_RejectsBadFreshFetchAndPreservesCache(t *testing.T) {
	dir := t.TempDir()
	setEnvDir(t, dir)
	cachePath, err := CachePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatal(err)
	}
	good := []byte("name: good-policy\n")
	if err := os.WriteFile(cachePath, good, 0o644); err != nil {
		t.Fatal(err)
	}

	bad := []byte("<<< not yaml >>>")
	fetcher := &stubFetcher{data: bad}

	_, _, err = Refresh(context.Background(), fetcher)
	if err == nil {
		t.Fatal("expected parse error for bad payload")
	}

	stillThere, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stillThere, good) {
		t.Errorf("cache was poisoned. got %q, want %q", stillThere, good)
	}
}

func TestLoadOrFetch_PerURLCache(t *testing.T) {
	dir := t.TempDir()
	setEnvDir(t, dir)

	// Two fetchers with different keys (simulating different URLs)
	// must NOT share a cache file. Otherwise a config switch from
	// URL-A to URL-B silently reuses A's cached policy.
	a := &stubFetcher{key: "https://a.example.com/policy.yaml", data: []byte("name: from-a\n")}
	b := &stubFetcher{key: "https://b.example.com/policy.yaml", data: []byte("name: from-b\n")}

	// Populate A's cache via fetch.
	pa, _, err := LoadOrFetch(context.Background(), a, LoadOptions{})
	if err != nil || pa.Name != "from-a" {
		t.Fatalf("a load: %v / %+v", err, pa)
	}
	// Now switch to B with no fetch yet — would hit A's cache if shared.
	pb, _, err := LoadOrFetch(context.Background(), b, LoadOptions{})
	if err != nil || pb.Name != "from-b" {
		t.Fatalf("b should fetch its own policy, not reuse A: err=%v p=%+v", err, pb)
	}

	// Verify cache paths actually differ on disk.
	pathA, _ := cachePathFor(a.CacheKey())
	pathB, _ := cachePathFor(b.CacheKey())
	if pathA == pathB {
		t.Errorf("expected distinct cache paths, both = %s", pathA)
	}
	if _, err := os.Stat(pathA); err != nil {
		t.Errorf("A cache file missing at %s: %v", pathA, err)
	}
	if _, err := os.Stat(pathB); err != nil {
		t.Errorf("B cache file missing at %s: %v", pathB, err)
	}
}

type stubFetcher struct {
	data []byte
	err  error
	key  string // optional cache key; "" → unkeyed default
}

func (s *stubFetcher) Fetch(ctx context.Context) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.data, nil
}

func (s *stubFetcher) CacheKey() string { return s.key }

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
