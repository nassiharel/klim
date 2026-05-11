// Package service provides the ToolService — a high-level composition root
// that wires together tool catalog loading, PATH scanning, and version
// resolution into reusable pipelines. CLI commands and the TUI call
// ToolService methods instead of directly coupling to finder, pkgmgr,
// and detector packages.
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nassiharel/klim/internal/catalog"
	"github.com/nassiharel/klim/internal/config"
	"github.com/nassiharel/klim/internal/fileutil"
	"github.com/nassiharel/klim/internal/finder"
	"github.com/nassiharel/klim/internal/paths"
	"github.com/nassiharel/klim/internal/pkgmgr"
	"github.com/nassiharel/klim/internal/registry"
	"github.com/nassiharel/klim/internal/scancache"
)

// ScanSource describes where fully-resolved tool data came from.
type ScanSource string

const (
	// ScanSourceCache means the scan result was loaded from the on-disk
	// scan cache (fast path).
	ScanSourceCache ScanSource = "cache"
	// ScanSourceFresh means a fresh PATH scan + version resolution ran.
	ScanSourceFresh ScanSource = "fresh"
)

// ScanInfo describes how a resolved tool list was produced.
type ScanInfo struct {
	Source  ScanSource
	CacheAt time.Time // when the cache was written (zero if not from cache)
}

// CatalogInfo describes how the catalog was loaded.
type CatalogInfo struct {
	Source catalog.LoadSource // "cache" or "remote"
	Tools  int                // number of tool definitions
	// Diff is non-nil when the catalog was auto-refreshed from the remote as
	// part of this load (i.e. the cache was stale). Callers can use it to
	// surface tool additions, modifications, and removals.
	Diff *catalog.DiffResult
}

// ToolCatalog abstracts loading the tool definitions.
type ToolCatalog interface {
	LoadTools(ctx context.Context) ([]registry.Tool, *CatalogInfo, error)
}

// ToolService orchestrates tool discovery and version resolution.
// It composes a catalog, a finder, and a resolver behind clean interfaces.
type ToolService struct {
	Catalog     ToolCatalog
	Finder      finder.ToolFinder
	Resolver    pkgmgr.VersionResolver
	Concurrency int // 0 = auto (runtime.NumCPU)
}

// New returns a ToolService wired with the default implementations.
func New() *ToolService {
	return NewWithConfig(config.Default())
}

// NewWithConfig returns a ToolService configured from the given Config.
func NewWithConfig(cfg *config.Config) *ToolService {
	fetcher := &catalog.GitHubFetcher{
		URL: cfg.Marketplace.URL,
	}

	var extraFetchers []catalog.MarketplaceFetcher
	seen := make(map[string]bool)
	for _, rawURL := range cfg.Marketplace.ExtraURLs {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" || seen[rawURL] {
			continue
		}
		// Validate URL scheme before adding.
		parsed, parseErr := url.Parse(rawURL)
		if parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			slog.Warn("skipping invalid extra marketplace URL", "url", rawURL)
			continue
		}
		seen[rawURL] = true
		extraFetchers = append(extraFetchers, &catalog.GitHubFetcher{URL: rawURL})
	}

	var maxAge time.Duration
	if cfg.Marketplace.AutoRefresh {
		maxAge = cfg.Marketplace.RefreshInterval.Duration
	}

	return &ToolService{
		Catalog:     &DefaultCatalog{Fetcher: fetcher, ExtraFetchers: extraFetchers, MaxAge: maxAge},
		Finder:      finder.NewFinder(),
		Resolver:    pkgmgr.NewResolverWithTimeout(cfg.Performance.CommandTimeout.Duration),
		Concurrency: cfg.Performance.Concurrency,
	}
}

// LoadAndResolve loads the tool catalog, scans PATH, resolves versions,
// and returns the tools sorted by name. This is the full pipeline used
// by `klim list` and `klim export`.
func (s *ToolService) LoadAndResolve(ctx context.Context) ([]registry.Tool, *CatalogInfo, error) {
	tools, info, err := s.Catalog.LoadTools(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("loading catalog: %w", err)
	}

	if err := s.Finder.FindAll(ctx, tools); err != nil {
		return nil, info, fmt.Errorf("scanning PATH: %w", err)
	}

	s.Resolver.ResolveVersions(ctx, tools, s.Concurrency)
	sortToolsByName(tools)
	_ = scancache.Save(tools) // best-effort: cache is a pure optimisation
	return tools, info, nil
}

// LoadAndResolveCached returns fully resolved tools, preferring the on-disk
// scan cache to avoid the cost of running package-manager subprocesses on
// every invocation. When force is true, or the cache is missing/invalid,
// it falls back to the full LoadAndResolve pipeline and rewrites the cache.
// The returned ScanInfo reports which path was taken.
func (s *ToolService) LoadAndResolveCached(ctx context.Context, force bool) ([]registry.Tool, *CatalogInfo, *ScanInfo, error) {
	if force {
		// Skip cache, do fresh scan. Old cache stays on disk until overwritten
		// by scancache.Save() after scan completes — avoids losing the cache
		// if user quits mid-scan.
		tools, info, err := s.LoadAndResolve(ctx)
		return tools, info, &ScanInfo{Source: ScanSourceFresh}, err
	}

	entries, savedAt, err := scancache.Load()
	switch {
	case err == nil:
		tools, info, catErr := s.Catalog.LoadTools(ctx)
		if catErr != nil {
			return nil, nil, nil, fmt.Errorf("loading catalog: %w", catErr)
		}
		tools = scancache.Apply(tools, entries)
		sortToolsByName(tools)
		return tools, info, &ScanInfo{Source: ScanSourceCache, CacheAt: savedAt}, nil
	case errors.Is(err, os.ErrNotExist):
		// Cold start — no cache yet.
	default:
		// Cache unreadable/incompatible — ignore it, fresh scan will overwrite.
		slog.Warn("scan cache unreadable, will rescan", "error", err)
	}

	tools, info, resolveErr := s.LoadAndResolve(ctx)
	return tools, info, &ScanInfo{Source: ScanSourceFresh}, resolveErr
}

// RewalkPath re-runs only the PATH-walk phase on an already-loaded
// tool list, mutating tools[i].Instances in place. Skips catalog
// loading and version resolution, so it's measured in milliseconds —
// the right thing to call after the user applies a PATH-only fix
// (duplicate-removal, reorder, etc.) instead of a multi-second full
// rescan that re-queries every package manager.
func (s *ToolService) RewalkPath(ctx context.Context, tools []registry.Tool) error {
	if s.Finder == nil {
		return errors.New("service: no finder configured")
	}
	return s.Finder.FindAll(ctx, tools)
}

// ScanOnly loads the catalog and scans PATH without resolving versions.
// Used by `klim import`, `klim open`, and the TUI import plan builder
// where only installed/not-installed status is needed.
func (s *ToolService) ScanOnly(ctx context.Context) ([]registry.Tool, *CatalogInfo, error) {
	tools, info, err := s.Catalog.LoadTools(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("loading catalog: %w", err)
	}

	if err := s.Finder.FindAll(ctx, tools); err != nil {
		return nil, info, fmt.Errorf("scanning PATH: %w", err)
	}

	return tools, info, nil
}

// LoadAndScan loads the catalog and scans PATH (no version resolution).
// Returns tools sorted by name. Used by the TUI's initial scan phase.
func (s *ToolService) LoadAndScan(ctx context.Context) ([]registry.Tool, *CatalogInfo, error) {
	tools, info, err := s.Catalog.LoadTools(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("loading catalog: %w", err)
	}

	if err := s.Finder.FindAll(ctx, tools); err != nil {
		return tools, info, err // return partial results with error
	}

	sortToolsByName(tools)
	return tools, info, nil
}

// LoadCached tries to produce a fully resolved tool list from the on-disk
// scan cache. It is the TUI fast path: on a cache hit, both PATH scanning
// and version resolution are skipped. Returns os.ErrNotExist (wrapped) when
// no cache is available so the caller can fall back to a fresh scan.
func (s *ToolService) LoadCached(ctx context.Context) ([]registry.Tool, *CatalogInfo, *ScanInfo, error) {
	entries, savedAt, err := scancache.Load()
	if err != nil {
		return nil, nil, nil, err
	}
	tools, info, catErr := s.Catalog.LoadTools(ctx)
	if catErr != nil {
		return nil, nil, nil, fmt.Errorf("loading catalog: %w", catErr)
	}
	tools = scancache.Apply(tools, entries)
	sortToolsByName(tools)
	return tools, info, &ScanInfo{Source: ScanSourceCache, CacheAt: savedAt}, nil
}

// SaveScanCache persists the given fully-resolved tools to the scan cache.
// Errors are swallowed by callers that treat the cache as an optimisation.
func (s *ToolService) SaveScanCache(tools []registry.Tool) error {
	return scancache.Save(tools)
}

// InvalidateScanCache removes the on-disk scan cache so the next load
// performs a fresh scan. Called before mutating actions (install, remove,
// upgrade) to avoid serving stale install state.
func (s *ToolService) InvalidateScanCache() error {
	return scancache.Delete()
}

// ResolveOne resolves versions for a single tool.
func (s *ToolService) ResolveOne(ctx context.Context, tool *registry.Tool) {
	s.Resolver.ResolveOne(ctx, tool)
}

// PackLoader is an optional interface that catalogs can implement to provide packs.
type PackLoader interface {
	LoadPacks(ctx context.Context) ([]registry.Pack, error)
}

// LoadPacks returns the curated packs from the catalog.
// Returns an empty slice if the catalog doesn't support packs.
func (s *ToolService) LoadPacks(ctx context.Context) ([]registry.Pack, error) {
	if pl, ok := s.Catalog.(PackLoader); ok {
		return pl.LoadPacks(ctx)
	}
	return []registry.Pack{}, nil
}

// RefreshTool re-scans a single tool's PATH presence and resolves its versions.
func (s *ToolService) RefreshTool(ctx context.Context, tool registry.Tool) registry.Tool {
	singleTool := []registry.Tool{tool}
	_ = s.Finder.FindAll(ctx, singleTool) // best-effort
	tool = singleTool[0]
	if tool.IsInstalled() {
		s.Resolver.ResolveOne(ctx, &tool)
	}
	return tool
}

func sortToolsByName(tools []registry.Tool) {
	registry.SortByName(tools)
}

// DefaultCatalog loads tools by fetching/caching the marketplace from GitHub.
type DefaultCatalog struct {
	Fetcher catalog.MarketplaceFetcher
	// ExtraFetchers holds additional marketplace fetchers. Tools from extra
	// sources are merged into the primary catalog; extra tools take priority
	// over primary ones with the same name.
	ExtraFetchers []catalog.MarketplaceFetcher
	// MaxAge, if > 0, enables cache freshness checks: when the cache mtime
	// exceeds MaxAge the catalog is refetched so new/updated/deleted tools
	// land in the local cache. Zero disables auto-refresh.
	MaxAge time.Duration
}

// LoadTools implements ToolCatalog.
func (c *DefaultCatalog) LoadTools(ctx context.Context) ([]registry.Tool, *CatalogInfo, error) {
	result, err := catalog.LoadOrFetchWithOptions(ctx, c.Fetcher, catalog.LoadOptions{MaxAge: c.MaxAge})
	if err != nil {
		return nil, nil, err
	}
	tools := registry.ToolsFromBytes(result.Data)
	if tools == nil {
		return nil, nil, errors.New("failed to parse marketplace catalog")
	}
	info := &CatalogInfo{
		Source: result.Source,
		Tools:  result.Tools,
		Diff:   result.Diff,
	}

	// Merge tools from extra marketplaces.
	if len(c.ExtraFetchers) > 0 {
		tools = c.mergeExtraTools(ctx, tools)
		info.Tools = len(tools)
	}

	return tools, info, nil
}

// mergeExtraTools fetches tools from extra marketplace URLs and merges them
// into the primary tool list. Extra tools with the same name as primary
// tools take priority (override). Uses per-URL caching for performance.
func (c *DefaultCatalog) mergeExtraTools(ctx context.Context, primary []registry.Tool) []registry.Tool {
	toolMap := make(map[string]registry.Tool, len(primary))
	for _, t := range primary {
		toolMap[t.Name] = t
	}

	for i, fetcher := range c.ExtraFetchers {
		data, err := fetchExtraCached(ctx, fetcher, i, c.MaxAge)
		if err != nil {
			slog.Warn("extra marketplace fetch failed", "index", i, "error", err)
			continue
		}
		extraTools, parseErr := registry.ToolsFromBytesWithError(data)
		if parseErr != nil {
			slog.Warn("extra marketplace YAML invalid", "index", i, "error", parseErr)
			continue
		}
		if len(extraTools) == 0 {
			slog.Debug("extra marketplace has no tools section", "index", i)
			continue
		}
		slog.Info("extra marketplace loaded", "index", i, "tools", len(extraTools))
		for _, t := range extraTools {
			toolMap[t.Name] = t
		}
	}

	merged := make([]registry.Tool, 0, len(toolMap))
	for _, t := range toolMap {
		merged = append(merged, t)
	}
	registry.SortByName(merged)
	return merged
}

// LoadPacks returns the curated packs from the catalog.
func (c *DefaultCatalog) LoadPacks(ctx context.Context) ([]registry.Pack, error) {
	result, err := catalog.LoadOrFetchWithOptions(ctx, c.Fetcher, catalog.LoadOptions{MaxAge: c.MaxAge})
	if err != nil {
		return nil, err
	}
	packs, err := registry.ParsePacksFromBytes(result.Data)
	if err != nil {
		return nil, err
	}

	// Merge packs from extra marketplaces — build map once.
	if len(c.ExtraFetchers) > 0 {
		packMap := make(map[string]registry.Pack, len(packs))
		for _, p := range packs {
			packMap[p.Name] = p
		}
		for i, fetcher := range c.ExtraFetchers {
			data, fetchErr := fetchExtraCached(ctx, fetcher, i, c.MaxAge)
			if fetchErr != nil {
				slog.Warn("extra marketplace packs fetch failed", "index", i, "error", fetchErr)
				continue
			}
			extraPacks, parseErr := registry.ParsePacksFromBytes(data)
			if parseErr != nil {
				slog.Warn("extra marketplace packs parse failed", "index", i, "error", parseErr)
				continue
			}
			for _, p := range extraPacks {
				packMap[p.Name] = p
			}
		}
		packs = make([]registry.Pack, 0, len(packMap))
		for _, p := range packMap {
			packs = append(packs, p)
		}
	}

	sort.Slice(packs, func(i, j int) bool { return packs[i].Name < packs[j].Name })
	return packs, nil
}

// fetchExtraCached fetches an extra marketplace with local file caching.
// Cache files are stored under ~/.klim/marketplace/extra-<hash>.yaml.
func fetchExtraCached(ctx context.Context, fetcher catalog.MarketplaceFetcher, index int, maxAge time.Duration) ([]byte, error) {
	// Determine the URL for cache key.
	url := ""
	if gf, ok := fetcher.(*catalog.GitHubFetcher); ok {
		url = gf.URL
	}
	if url == "" {
		url = fmt.Sprintf("extra-%d", index)
	}

	cachePath, err := extraCachePath(url)
	if err != nil {
		// Fall back to direct fetch if cache path fails.
		return fetcher.Fetch(ctx)
	}

	// Try cache.
	if data, err := os.ReadFile(cachePath); err == nil && len(data) > 0 {
		if maxAge <= 0 || !cacheFileStale(cachePath, maxAge) {
			return data, nil
		}
	}

	// Fetch fresh.
	data, err := fetcher.Fetch(ctx)
	if err != nil {
		// Try stale cache on fetch failure.
		if cached, readErr := os.ReadFile(cachePath); readErr == nil && len(cached) > 0 {
			slog.Warn("extra marketplace fetch failed, using stale cache", "index", index, "error", err)
			return cached, nil
		}
		return nil, err
	}

	// Validate before caching — don't poison cache with HTML/garbage.
	// Must have at least one tool or pack to be considered valid.
	tools, toolErr := registry.ToolsFromBytesWithError(data)
	packs, packErr := registry.ParsePacksFromBytes(data)
	if toolErr != nil && packErr != nil {
		slog.Warn("extra marketplace fetched invalid data, not caching", "index", index, "error", toolErr)
		return nil, toolErr
	}
	if len(tools) == 0 && len(packs) == 0 {
		slog.Warn("extra marketplace has no tools or packs, not caching", "index", index)
		return nil, errors.New("extra marketplace contains no tools or packs")
	}

	// Write cache.
	if mkErr := os.MkdirAll(filepath.Dir(cachePath), 0o755); mkErr != nil {
		slog.Warn("failed to create extra marketplace cache dir", "path", cachePath, "error", mkErr)
	} else if writeErr := fileutil.AtomicWrite(cachePath, data, 0o644); writeErr != nil {
		slog.Warn("failed to write extra marketplace cache", "path", cachePath, "error", writeErr)
	}

	return data, nil
}

// extraCachePath returns a cache file path for an extra marketplace URL.
func extraCachePath(url string) (string, error) {
	hash := sha256.Sum256([]byte(url))
	name := "extra-" + hex.EncodeToString(hash[:8]) + ".yaml"
	return paths.Join("marketplace", name)
}

// cacheFileStale reports whether a file's mtime exceeds maxAge.
func cacheFileStale(path string, maxAge time.Duration) bool {
	info, err := os.Stat(path)
	if err != nil {
		return true
	}
	return time.Since(info.ModTime()) > maxAge
}
