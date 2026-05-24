// Package catalog fetches and caches the published `marketplace.json`
// of known online plugin marketplaces (Anthropic official, GitHub
// copilot-plugins, awesome-copilot) so klim's Agents tab can surface
// available-to-install plugins without the user having to run the
// underlying agent CLI first.
//
// Both Claude Code and GitHub Copilot CLI use the same `marketplace.json`
// schema (see superpowers/specs/2026-05-14-agents-tab-design.md), so a
// single parser covers every source. The fetcher does best-effort
// remote loads with on-disk caching — a transient HTTP failure leaves
// the user with the previous cached snapshot, never a hard error.
package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/fileutil"
	"github.com/nassiharel/klim/internal/paths"
)

// DefaultTTL is how long a cached marketplace.json is considered fresh
// before the fetcher tries to re-download it. The on-disk cache is
// always used as a fallback if the network fails.
const DefaultTTL = 24 * time.Hour

// DefaultSources are the marketplaces klim fetches at scan time.
// Claude marketplaces are tagged so the rendered Plugin's Provider is
// set correctly even when the JSON itself is provider-neutral.
var DefaultSources = []Source{
	{
		Name:     "claude-plugins-official",
		URL:      "https://raw.githubusercontent.com/anthropics/claude-plugins-official/main/.claude-plugin/marketplace.json",
		Provider: agents.ProviderClaudeCode,
		Source:   agents.SourceCatalogClaude,
	},
	{
		Name:     "copilot-plugins",
		URL:      "https://raw.githubusercontent.com/github/copilot-plugins/main/.github/plugin/marketplace.json",
		Provider: agents.ProviderCopilotCLI,
		Source:   agents.SourceCatalogCopilot,
	},
	{
		Name:     "awesome-copilot",
		URL:      "https://raw.githubusercontent.com/github/awesome-copilot/main/.github/plugin/marketplace.json",
		Provider: agents.ProviderCopilotCLI,
		Source:   agents.SourceCatalogCopilot,
	},
}

// Source describes one remote marketplace to fetch.
type Source struct {
	Name     string
	URL      string
	Provider agents.ProviderID
	Source   agents.Source
}

// Fetcher fetches marketplace.json from remote sources. Both the HTTP
// client and the cache are swappable for testing.
type Fetcher struct {
	HTTPClient *http.Client
	TTL        time.Duration
	Sources    []Source
}

// New returns a Fetcher wired with the default sources and a 10s HTTP timeout.
func New() *Fetcher {
	return &Fetcher{
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		TTL:        DefaultTTL,
		Sources:    DefaultSources,
	}
}

// Result is what one source contributes to the merged snapshot.
type Result struct {
	Source       Source
	Plugins      []agents.Plugin
	Marketplaces []agents.Marketplace
	Err          error
}

// FetchAll returns every source's plugin listings in registration order.
// Cached responses are used when fresh; expired caches are refreshed
// against the network with fallback to the last good cache on failure.
//
// The first entry in the returned slice is a synthetic "discoverable"
// result that carries the curated marketplace list loaded from the
// embedded marketplace/marketplaces/*.yaml files via
// DiscoverableMarketplaces() (no network I/O). This lets the merged
// snapshot surface well-known community marketplaces (e.g.
// openai/codex-plugin-cc) as installable-but-not-yet-installed
// entries, so users can browse and add them from the Marketplaces
// sub-tab the same way they discover new tools.
func (f *Fetcher) FetchAll(ctx context.Context) []Result {
	out := make([]Result, 0, len(f.Sources)+1)
	out = append(out, Result{
		Source:       Source{Name: "discoverable-marketplaces"},
		Marketplaces: DiscoverableMarketplaces(),
	})
	for _, src := range f.Sources {
		out = append(out, f.fetchOne(ctx, src))
	}
	return out
}

// fetchOne resolves a single source. The on-disk cache key is derived
// from the source name (kebab-cased) so different sources never share
// a cache file.
func (f *Fetcher) fetchOne(ctx context.Context, src Source) Result {
	cachePath, _ := cacheFilePath(src.Name)

	// 1. Cache fresh — return it.
	if data, ok := readCacheIfFresh(cachePath, f.TTL); ok {
		return Result{Source: src, Plugins: parsePlugins(data, src)}
	}

	// 2. Try network.
	body, err := f.httpGet(ctx, src.URL)
	if err == nil {
		_ = writeCache(cachePath, body)
		return Result{Source: src, Plugins: parsePlugins(body, src)}
	}

	// 3. Network failed — fall back to any cached body.
	if data, ok := readCache(cachePath); ok {
		return Result{Source: src, Plugins: parsePlugins(data, src), Err: err}
	}

	// 4. Nothing.
	return Result{Source: src, Err: err}
}

func (f *Fetcher) httpGet(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("catalog: %s returned HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// rawMarketplace mirrors the shared marketplace.json schema.
type rawMarketplace struct {
	Name        string      `json:"name,omitempty"`
	Description string      `json:"description,omitempty"`
	Metadata    rawMeta     `json:"metadata,omitempty"`
	Owner       rawOwner    `json:"owner,omitempty"`
	Plugins     []rawPlugin `json:"plugins,omitempty"`
}

type rawMeta struct {
	PluginRoot  string `json:"pluginRoot,omitempty"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`
}

type rawOwner struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email,omitempty"`
}

type rawPlugin struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Version     string   `json:"version,omitempty"`
	Source      any      `json:"source,omitempty"` // string or {source, repo, path}
	Category    string   `json:"category,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
	Repository  string   `json:"repository,omitempty"`
	License     string   `json:"license,omitempty"`
	Author      rawOwner `json:"author,omitempty"`
}

func parsePlugins(body []byte, src Source) []agents.Plugin {
	var raw rawMarketplace
	if json.Unmarshal(body, &raw) != nil {
		return nil
	}
	out := make([]agents.Plugin, 0, len(raw.Plugins))
	for _, p := range raw.Plugins {
		if p.Name == "" {
			continue
		}
		out = append(out, agents.Plugin{
			ID:          src.Name + "/" + p.Name,
			Name:        p.Name,
			Description: p.Description,
			Version:     p.Version,
			Author:      p.Author.Name,
			Homepage:    p.Homepage,
			Repository:  p.Repository,
			License:     p.License,
			Keywords:    mergeKeywords(p.Keywords, p.Tags),
			Provider:    src.Provider,
			Marketplace: src.Name,
			Installed:   false,
			Enabled:     false,
			Scope:       agents.ScopeRemote,
			Source:      src.Source,
		})
	}
	return out
}

func mergeKeywords(a, b []string) []string {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range append(a, b...) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// ---- cache ----

// cachedFile wraps a cached marketplace.json with its fetch timestamp.
// Stored under ~/.klim/agents/catalog/<source>.yaml so each source is
// independent.
type cachedFile struct {
	WrittenAt time.Time `yaml:"written_at"`
	Body      []byte    `yaml:"body"`
}

func cacheFilePath(source string) (string, error) {
	return paths.Join("agents", "catalog", sanitize(source)+".yaml")
}

func sanitize(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			out = append(out, r)
		} else {
			out = append(out, '-')
		}
	}
	return string(out)
}

func readCache(path string) ([]byte, bool) {
	if path == "" {
		return nil, false
	}
	var c cachedFile
	found, err := fileutil.ReadYAML(path, &c)
	if err != nil || !found {
		return nil, false
	}
	return c.Body, len(c.Body) > 0
}

func readCacheIfFresh(path string, ttl time.Duration) ([]byte, bool) {
	if path == "" {
		return nil, false
	}
	var c cachedFile
	found, err := fileutil.ReadYAML(path, &c)
	if err != nil || !found {
		return nil, false
	}
	if time.Since(c.WrittenAt) > ttl {
		return nil, false
	}
	return c.Body, len(c.Body) > 0
}

func writeCache(path string, body []byte) error {
	if path == "" {
		return errors.New("catalog: empty cache path")
	}
	c := cachedFile{WrittenAt: time.Now().UTC(), Body: body}
	return fileutil.WriteYAML(path, &c, "# klim agents catalog cache — auto-generated\n")
}
