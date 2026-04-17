// Package service provides the ToolService — a high-level composition root
// that wires together tool catalog loading, PATH scanning, and version
// resolution into reusable pipelines. CLI commands and the TUI call
// ToolService methods instead of directly coupling to finder, pkgmgr,
// and detector packages.
package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/nassiharel/clim/internal/catalog"
	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/finder"
	"github.com/nassiharel/clim/internal/pkgmgr"
	"github.com/nassiharel/clim/internal/registry"
)

// CatalogInfo describes how the catalog was loaded.
type CatalogInfo struct {
	Source catalog.LoadSource // "cache" or "remote"
	Tools  int                // number of tool definitions
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

	return &ToolService{
		Catalog:     &DefaultCatalog{Fetcher: fetcher},
		Finder:      finder.NewFinder(),
		Resolver:    pkgmgr.NewResolverWithTimeout(cfg.Performance.CommandTimeout.Duration),
		Concurrency: cfg.Performance.Concurrency,
	}
}

// LoadAndResolve loads the tool catalog, scans PATH, resolves versions,
// and returns the tools sorted by name. This is the full pipeline used
// by `clim list` and `clim export`.
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
	return tools, info, nil
}

// ScanOnly loads the catalog and scans PATH without resolving versions.
// Used by `clim import`, `clim open`, and the TUI import plan builder
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
	sort.Slice(tools, func(i, j int) bool {
		return strings.ToLower(tools[i].Name) < strings.ToLower(tools[j].Name)
	})
}

// DefaultCatalog loads tools by fetching/caching the marketplace from GitHub.
type DefaultCatalog struct {
	Fetcher catalog.MarketplaceFetcher
}

// LoadTools implements ToolCatalog.
func (c *DefaultCatalog) LoadTools(ctx context.Context) ([]registry.Tool, *CatalogInfo, error) {
	result, err := catalog.LoadOrFetch(ctx, c.Fetcher)
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
	}
	return tools, info, nil
}

// LoadPacks returns the curated packs from the catalog.
func (c *DefaultCatalog) LoadPacks(ctx context.Context) ([]registry.Pack, error) {
	result, err := catalog.LoadOrFetch(ctx, c.Fetcher)
	if err != nil {
		return nil, err
	}
	return registry.ParsePacksFromBytes(result.Data)
}
