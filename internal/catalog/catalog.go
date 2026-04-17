// Package catalog manages fetching, caching, and diffing the tool marketplace
// catalog from GitHub. The marketplace.yaml is treated as an external service
// rather than an embedded resource — it's fetched on first run and cached
// locally for offline use.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/registry"
)

// --- Fetcher ---

// MarketplaceFetcher abstracts fetching the catalog YAML from a remote source.
type MarketplaceFetcher interface {
	Fetch(ctx context.Context) ([]byte, error)
}

// GitHubFetcher fetches marketplace.yaml from a configured URL,
// defaulting to GitHub raw content if no URL is set.
type GitHubFetcher struct {
	HTTPClient *http.Client // nil = default client with 30s timeout
	URL        string       // marketplace URL; empty = config.DefaultMarketplaceURL
}

// maxCatalogSize caps the downloaded catalog to prevent memory exhaustion.
const maxCatalogSize = 2 << 20 // 2 MB — marketplace.yaml is ~20 KB

// Fetch downloads the marketplace.yaml from the configured URL.
func (f *GitHubFetcher) Fetch(ctx context.Context) ([]byte, error) {
	url := f.URL
	if url == "" {
		url = config.DefaultMarketplaceURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "clim/catalog")

	resp, err := f.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching marketplace: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %s", resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCatalogSize+1))
	if err != nil {
		return nil, fmt.Errorf("reading marketplace: %w", err)
	}
	if int64(len(data)) > maxCatalogSize {
		return nil, fmt.Errorf("marketplace too large (max %d bytes)", maxCatalogSize)
	}

	return data, nil
}

func (f *GitHubFetcher) httpClient() *http.Client {
	if f.HTTPClient != nil {
		return f.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// --- Cache ---

// CachePath returns the path to the marketplace cache file.
func CachePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "clim", "marketplace-cache.yaml"), nil
}

// LoadSource indicates where the catalog was loaded from.
type LoadSource string

const (
	// SourceCache means the catalog was loaded from the local cache.
	SourceCache LoadSource = "cache"
	// SourceRemote means the catalog was fetched from the remote URL.
	SourceRemote LoadSource = "remote"
)

// LoadResult contains the loaded catalog data and metadata about the load.
type LoadResult struct {
	Data   []byte
	Source LoadSource
	Tools  int // number of tools parsed
	// Diff describes what changed relative to the previous cache when the
	// catalog was refreshed from remote as part of this load (e.g. the cache
	// was stale and auto-refresh was enabled). Nil when no refresh happened
	// or the cache was empty before the fetch.
	Diff *DiffResult
}

// LoadOptions controls LoadOrFetch behaviour.
type LoadOptions struct {
	// MaxAge enables cache freshness checks. When > 0 and the cache mtime is
	// older than MaxAge, LoadOrFetch will attempt to refetch from the remote
	// so new/updated/deleted tools make it into the local cache. On fetch
	// failure the existing (stale) cache is still returned.
	MaxAge time.Duration
}

// LoadOrFetch loads the cached marketplace YAML. If the cache doesn't exist
// or contains invalid YAML, it fetches from GitHub and rewrites the cache.
// With a zero LoadOptions this preserves the historical behaviour.
func LoadOrFetch(ctx context.Context, fetcher MarketplaceFetcher) (*LoadResult, error) {
	return LoadOrFetchWithOptions(ctx, fetcher, LoadOptions{})
}

// LoadOrFetchWithOptions is LoadOrFetch with control over cache freshness.
// When opts.MaxAge > 0 and the cached file is older than MaxAge, the remote
// is refetched so tool additions, updates, and removals propagate into the
// local cache. If the refetch fails the stale cache is still returned and
// LoadResult.Diff is left nil.
func LoadOrFetchWithOptions(ctx context.Context, fetcher MarketplaceFetcher, opts LoadOptions) (*LoadResult, error) {
	path, err := CachePath()
	if err != nil {
		return nil, fmt.Errorf("resolving cache path: %w", err)
	}

	// Try reading and validating the local cache.
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		if n := countTools(data); n > 0 {
			if opts.MaxAge > 0 && cacheIsStale(path, opts.MaxAge) {
				if refreshed, diff, ok := tryRefreshCache(ctx, fetcher, path, data); ok {
					rn := countTools(refreshed)
					slog.Info("catalog auto-refreshed",
						"path", path,
						"tools", rn,
						"new", len(diff.NewTools),
						"changed", len(diff.ChangedTools),
						"removed", len(diff.RemovedTools),
					)
					return &LoadResult{Data: refreshed, Source: SourceRemote, Tools: rn, Diff: &diff}, nil
				}
				slog.Warn("catalog auto-refresh failed, serving stale cache", "path", path)
			}
			slog.Info("catalog loaded from cache", "path", path, "tools", n)
			return &LoadResult{Data: data, Source: SourceCache, Tools: n}, nil
		}
		slog.Warn("catalog cache invalid, refetching", "path", path)
	}

	// No valid cache — fetch from remote.
	slog.Info("fetching catalog from remote")
	data, err := fetcher.Fetch(ctx)
	if err != nil {
		slog.Warn("catalog fetch failed", "error", err)
		return nil, fmt.Errorf("unable to fetch remote marketplace (no local cache available): %w", err)
	}

	// Validate before caching — don't poison the cache with HTML/garbage.
	n := countTools(data)
	if n == 0 {
		slog.Warn("fetched catalog is invalid", "bytes", len(data))
		return nil, errors.New("fetched catalog is invalid (not parseable YAML with tools)")
	}

	slog.Info("catalog fetched and cached", "path", path, "tools", n, "bytes", len(data))

	// Write cache atomically: write to temp file, then rename.
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr == nil {
		if writeErr := atomicWriteFile(path, data, 0o644); writeErr != nil {
			slog.Warn("failed to write catalog cache", "path", path, "error", writeErr)
		}
	}

	return &LoadResult{Data: data, Source: SourceRemote, Tools: n}, nil
}

// cacheIsStale reports whether the cache file's mtime is older than maxAge.
// Missing/unreadable mtime is treated as stale so the caller retries remote.
func cacheIsStale(path string, maxAge time.Duration) bool {
	info, err := os.Stat(path)
	if err != nil {
		return true
	}
	return time.Since(info.ModTime()) > maxAge
}

// tryRefreshCache fetches the remote catalog, validates it, writes it to the
// cache, and returns the new bytes plus a diff against the previous cache.
// Returns ok=false on any failure so callers can fall back to the stale cache.
func tryRefreshCache(ctx context.Context, fetcher MarketplaceFetcher, path string, prev []byte) ([]byte, DiffResult, bool) {
	remote, err := fetcher.Fetch(ctx)
	if err != nil {
		slog.Warn("catalog refetch failed", "error", err)
		return nil, DiffResult{}, false
	}
	if !isValidCatalog(remote) {
		slog.Warn("refetched catalog is invalid", "bytes", len(remote))
		return nil, DiffResult{}, false
	}
	diff := Diff(prev, remote)
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr == nil {
		if writeErr := atomicWriteFile(path, remote, 0o644); writeErr != nil {
			slog.Warn("failed to write catalog cache", "path", path, "error", writeErr)
		}
	} else {
		slog.Warn("failed to create cache dir", "path", path, "error", mkErr)
	}
	// Refresh mtime even if contents were identical so we don't retry the
	// remote on every call until something actually changes.
	_ = os.Chtimes(path, time.Now(), time.Now())
	return remote, diff, true
}

// countTools returns the number of tools in the YAML data, or 0 if invalid.
func countTools(data []byte) int {
	var f struct {
		Tools []registry.ToolDef `yaml:"tools"`
	}
	if err := yaml.Unmarshal(data, &f); err != nil {
		return 0
	}
	return len(f.Tools)
}

// isValidCatalog checks whether data is parseable YAML with at least one tool.
func isValidCatalog(data []byte) bool {
	return countTools(data) > 0
}

// --- Diff ---

// DiffResult describes what changed between two catalog versions.
type DiffResult struct {
	NewTools     []string            // tool names present in remote but absent locally
	ChangedTools map[string][]string // tool name → list of changed field descriptions
	RemovedTools []string            // tool names present locally but absent in remote
}

// HasChanges reports whether any tools were added, modified, or removed.
func (d DiffResult) HasChanges() bool {
	return len(d.NewTools) > 0 || len(d.ChangedTools) > 0 || len(d.RemovedTools) > 0
}

func parseToolDefs(data []byte) []registry.ToolDef {
	var f struct {
		Tools []registry.ToolDef `yaml:"tools"`
	}
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil
	}
	return f.Tools
}

// Diff compares local and remote catalog YAML and returns the differences.
// Detects additions, modifications, and removals. The local cache is a
// mirror of the last-fetched remote (see LoadOrFetch), so removals mean a
// tool that used to exist upstream is no longer published.
func Diff(local, remote []byte) DiffResult {
	localDefs := parseToolDefs(local)
	remoteDefs := parseToolDefs(remote)

	localMap := make(map[string]*registry.ToolDef, len(localDefs))
	for i := range localDefs {
		localMap[localDefs[i].Name] = &localDefs[i]
	}
	remoteMap := make(map[string]struct{}, len(remoteDefs))
	for i := range remoteDefs {
		remoteMap[remoteDefs[i].Name] = struct{}{}
	}

	result := DiffResult{
		ChangedTools: make(map[string][]string),
	}

	for _, rd := range remoteDefs {
		ld, exists := localMap[rd.Name]
		if !exists {
			result.NewTools = append(result.NewTools, rd.Name)
			continue
		}

		// Compare fields.
		var changes []string
		if ld.DisplayName != rd.DisplayName {
			changes = append(changes, "display_name")
		}
		if ld.Category != rd.Category {
			changes = append(changes, "category")
		}
		if !slicesEqual(ld.BinaryNames, rd.BinaryNames) {
			changes = append(changes, "binary_names")
		}
		if !slicesEqual(ld.Tags, rd.Tags) {
			changes = append(changes, "tags")
		}
		if ld.Packages != rd.Packages {
			changes = append(changes, "packages")
		}

		if len(changes) > 0 {
			result.ChangedTools[rd.Name] = changes
		}
	}

	// Anything in local but not in remote has been removed upstream. Only
	// report removals when remote is a non-empty, valid catalog — an empty
	// or unparseable remote would otherwise look like everything was deleted.
	if len(remoteDefs) > 0 {
		for _, ld := range localDefs {
			if _, ok := remoteMap[ld.Name]; !ok {
				result.RemovedTools = append(result.RemovedTools, ld.Name)
			}
		}
	}

	sort.Strings(result.NewTools)
	sort.Strings(result.RemovedTools)
	return result
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- Refresh ---

// RefreshResult is returned after a marketplace refresh.
type RefreshResult struct {
	Diff    DiffResult
	Updated bool // true if the local cache was updated
}

// Refresh fetches the latest catalog from the remote, diffs it against the
// local cache, updates the cache, and returns the result.
func Refresh(ctx context.Context, fetcher MarketplaceFetcher) (*RefreshResult, error) {
	cachePath, err := CachePath()
	if err != nil {
		return nil, fmt.Errorf("resolving cache path: %w", err)
	}

	// Read the current cache for diffing.
	local, _ := os.ReadFile(cachePath) // may be empty/missing — that's fine

	// Fetch latest from remote.
	remote, err := fetcher.Fetch(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching latest marketplace: %w", err)
	}

	// Validate before caching — don't poison the cache with HTML/garbage.
	if !isValidCatalog(remote) {
		return nil, errors.New("fetched catalog is invalid (not parseable YAML with tools)")
	}

	// Diff against what we had cached.
	diff := Diff(local, remote)

	// Update the cache.
	if mkErr := os.MkdirAll(filepath.Dir(cachePath), 0o755); mkErr == nil {
		if writeErr := atomicWriteFile(cachePath, remote, 0o644); writeErr != nil {
			slog.Warn("failed to write catalog cache", "path", cachePath, "error", writeErr)
		}
	}

	return &RefreshResult{
		Diff:    diff,
		Updated: diff.HasChanges(),
	}, nil
}

// atomicWriteFile writes data to a temp file in the same directory, then
// renames it to the target path. This prevents partial/corrupt files if the
// process is interrupted mid-write. Uses os.CreateTemp for safe temp names.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	if err := os.Rename(tmpPath, path); err != nil {
		// On Windows os.Rename fails if the destination exists.
		// Remove the destination and retry once; on non-Windows this
		// path is only reached for unexpected errors.
		if removeErr := os.Remove(path); removeErr == nil {
			if retryErr := os.Rename(tmpPath, path); retryErr == nil {
				return nil
			}
		}
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}
