package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nassiharel/klim/internal/agents"
)

const sampleMarketplaceJSON = `{
  "name": "test-marketplace",
  "description": "Test",
  "owner": {"name": "ExampleOrg"},
  "plugins": [
    {
      "name": "react-helper",
      "description": "React helper",
      "version": "1.0.0",
      "author": {"name": "Alice"},
      "license": "MIT",
      "tags": ["frontend", "react"]
    },
    {
      "name": "api-toolkit",
      "description": "API toolkit",
      "version": "2.1.0",
      "keywords": ["api"]
    }
  ]
}`

func TestFetcher_FetchAll_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sampleMarketplaceJSON))
	}))
	defer srv.Close()

	f := &Fetcher{
		HTTPClient: srv.Client(),
		TTL:        DefaultTTL,
		Sources: []Source{
			{
				Name:     "test-marketplace",
				URL:      srv.URL,
				Provider: agents.ProviderClaudeCode,
				Source:   agents.SourceCatalogClaude,
			},
		},
	}

	results := f.FetchAll(context.Background())
	// FetchAll always prepends a synthetic discoverable-marketplaces
	// result before the per-source fetches. In test environments the
	// catalog cache may not exist, so the discoverable list can be
	// empty — we only verify the entry is present with the right name.
	if len(results) != 2 {
		t.Fatalf("expected 2 results (discoverable + source), got %d", len(results))
	}
	if results[0].Source.Name != "discoverable-marketplaces" {
		t.Errorf("first result = %q, want discoverable-marketplaces", results[0].Source.Name)
	}
	r := results[1]
	if r.Err != nil {
		t.Fatalf("unexpected err: %v", r.Err)
	}
	if len(r.Plugins) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(r.Plugins))
	}

	// Verify enrichment.
	react := r.Plugins[0]
	if react.Name != "react-helper" {
		t.Errorf("first plugin name = %q", react.Name)
	}
	if react.Provider != agents.ProviderClaudeCode {
		t.Errorf("provider = %q", react.Provider)
	}
	if react.Source != agents.SourceCatalogClaude {
		t.Errorf("source = %q", react.Source)
	}
	if react.Scope != agents.ScopeRemote {
		t.Errorf("scope = %q, want remote", react.Scope)
	}
	if react.Installed {
		t.Error("catalog plugin should not be marked installed")
	}
	if react.Author != "Alice" {
		t.Errorf("author = %q", react.Author)
	}
	if react.License != "MIT" {
		t.Errorf("license = %q", react.License)
	}
}

func TestFetcher_FetchAll_HTTPError_NoCacheFallback(t *testing.T) {
	// Server that always 500s.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := &Fetcher{
		HTTPClient: srv.Client(),
		TTL:        DefaultTTL,
		Sources: []Source{{
			// Use a uniquely-named source so we won't hit a real cache.
			Name:     "test-no-cache-source-zzz",
			URL:      srv.URL,
			Provider: agents.ProviderClaudeCode,
			Source:   agents.SourceCatalogClaude,
		}},
	}

	results := f.FetchAll(context.Background())
	if len(results) != 2 {
		t.Fatalf("expected 2 results (discoverable + source), got %d", len(results))
	}
	r := results[1]
	if r.Err == nil {
		t.Error("expected an error from a 500")
	}
	// Plugins might be empty or come from a leftover cache; just don't crash.
	_ = r.Plugins
}

func TestParsePlugins_MergesKeywordsAndTags(t *testing.T) {
	body := []byte(`{
		"plugins": [{
			"name": "p",
			"keywords": ["a", "b"],
			"tags": ["b", "c"]
		}]
	}`)
	plugins := parsePlugins(body, Source{
		Name: "x", Provider: agents.ProviderClaudeCode, Source: agents.SourceCatalogClaude,
	})
	if len(plugins) != 1 {
		t.Fatalf("got %d plugins", len(plugins))
	}
	got := plugins[0].Keywords
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("keywords len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("keyword[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"copilot-plugins":         "copilot-plugins",
		"awesome.copilot":         "awesome-copilot",
		"a/b/c":                   "a-b-c",
		"claude-plugins-official": "claude-plugins-official",
	}
	for in, want := range cases {
		if got := sanitize(in); got != want {
			t.Errorf("sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

// Sanity that DefaultTTL is non-zero and DefaultSources is non-empty so
// the production wiring doesn't accidentally ship as a no-op.
func TestDefaultsAreWired(t *testing.T) {
	if DefaultTTL <= 0 {
		t.Error("DefaultTTL should be > 0")
	}
	if len(DefaultSources) == 0 {
		t.Error("DefaultSources should not be empty")
	}
	for i, src := range DefaultSources {
		if src.Name == "" || src.URL == "" || src.Provider == "" {
			t.Errorf("source[%d] missing fields: %+v", i, src)
		}
	}
	// Make sure the test-only time package doesn't get tree-shaken.
	_ = time.Now
}
