package agents

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nassiharel/klim/internal/agents/bookmarks"
	"github.com/nassiharel/klim/internal/agents/enrich"
)

// Service is the composition root for the agents subsystem. It owns the
// Registry of Providers, the on-disk cache, and coordinates concurrent
// scans with a bounded worker pool.
type Service struct {
	registry *Registry
	sem      chan struct{}

	// RemoteCatalog, when set, contributes catalog (available-to-install)
	// plugins to every scan's snapshot. Nil means "local only".
	RemoteCatalog RemoteCatalog

	mu       sync.RWMutex
	snapshot *Snapshot
}

// RemoteCatalog fetches available plugins from online marketplaces.
// Defined here as an interface so the agents package doesn't import
// the catalog package directly (avoids a cycle).
type RemoteCatalog interface {
	FetchAll(ctx context.Context) []RemoteCatalogResult
}

// RemoteCatalogResult is one source's contribution to the snapshot.
// Errors are non-fatal — a partial result still flows through.
type RemoteCatalogResult struct {
	SourceName   string
	Plugins      []Plugin
	Marketplaces []Marketplace
	Err          error
}

// LoadOpts configures a Service.LoadAll call.
type LoadOpts struct {
	// Refresh, when true, ignores the cache and rescans.
	Refresh bool
	// MaxAge controls cache acceptance; older caches are refreshed.
	// Zero means accept any cache.
	MaxAge time.Duration
}

// NewService constructs a Service with the given Providers registered.
// Concurrency 0 means default (4 — matches the version-resolver pool).
func NewService(concurrency int, providers ...Provider) *Service {
	if concurrency <= 0 {
		concurrency = 4
	}
	reg := NewRegistry()
	for _, p := range providers {
		reg.Register(p)
	}
	return &Service{
		registry: reg,
		sem:      make(chan struct{}, concurrency),
	}
}

// Registry exposes the underlying Registry for introspection.
func (s *Service) Registry() *Registry { return s.registry }

// LoadAll returns the merged Snapshot across every registered provider.
// Uses the cache when present and acceptable; otherwise scans fresh and
// writes a new cache.
//
// hydrateSessionExtras runs on every return path — even cache hits —
// because star toggles and grouping mapping edits happen outside the
// scan and must be reflected on the next list / TUI render. The
// helper is idempotent and fast (one bookmarks load + one mappings
// load + a linear pass over Sessions).
func (s *Service) LoadAll(ctx context.Context, opts LoadOpts) (*Snapshot, error) {
	if !opts.Refresh {
		if c, ok, err := LoadCache(); err == nil && ok {
			if opts.MaxAge == 0 || time.Since(c.WrittenAt) <= opts.MaxAge {
				snap := c.Snapshot
				// Clear any stale Starred flags so a removed bookmark
				// doesn't linger after `unstar`. Group is left alone
				// since it's content-derived and persists naturally.
				for i := range snap.Sessions {
					snap.Sessions[i].Starred = false
				}
				hydrateSessionExtras(&snap)
				s.mu.Lock()
				s.snapshot = &snap
				s.mu.Unlock()
				return &snap, nil
			}
		}
	}

	snap, err := s.scan(ctx)
	if err != nil {
		return nil, err
	}

	// Cache write is best-effort — a write failure doesn't fail the call.
	_ = SaveCache(*snap)

	s.mu.Lock()
	s.snapshot = snap
	s.mu.Unlock()
	return snap, nil
}

// Snapshot returns the last loaded Snapshot (or nil if LoadAll hasn't run).
func (s *Service) Snapshot() *Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.snapshot == nil {
		return nil
	}
	cp := *s.snapshot
	return &cp
}

// Invalidate forces the next LoadAll to skip the cache.
func (s *Service) Invalidate() {
	_ = DeleteCache()
	s.mu.Lock()
	s.snapshot = nil
	s.mu.Unlock()
}

// scan runs every provider's read methods concurrently and merges results.
// Errors from individual providers/methods are collected onto the Snapshot
// via ProviderStatus rather than failing the whole scan — a missing
// `claude` binary should not hide a working `copilot`.
func (s *Service) scan(ctx context.Context) (*Snapshot, error) {
	providers := s.registry.Providers()
	snap := &Snapshot{
		ProviderStatus: make(map[ProviderID]Status, len(providers)),
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, p := range providers {
		wg.Add(1)
		go func(p Provider) {
			defer wg.Done()
			s.sem <- struct{}{}
			defer func() { <-s.sem }()

			st := p.Detect(ctx)
			results := scanProvider(ctx, p)

			// PR #77 review #9: surface real errors (non-NotSupported)
			// via ProviderStatus so the doctor command and provider-
			// health pill can tell the user something is wrong. We
			// merge results.scanErr into the Status.Error field
			// without clobbering an existing Detect-time error.
			if results.scanErr != nil && st.Error == "" {
				st.Error = results.scanErr.Error()
			}

			mu.Lock()
			defer mu.Unlock()
			snap.ProviderStatus[p.ID()] = st
			snap.Marketplaces = append(snap.Marketplaces, results.marketplaces...)
			snap.Plugins = append(snap.Plugins, results.plugins...)
			snap.Skills = append(snap.Skills, results.skills...)
			snap.MCPs = append(snap.MCPs, results.mcps...)
			snap.Sessions = append(snap.Sessions, results.sessions...)
		}(p)
	}
	wg.Wait()

	// Merge in the remote catalog (online marketplaces). De-duplicate
	// against the locally-installed plugin list — when a plugin already
	// shows up as installed, we never want a phantom "available" copy
	// of the same plugin to push it down the list. Marketplaces are
	// deduped case-insensitively by Provider+Name; catalog entries
	// already present in the snapshot (registered with the provider)
	// are skipped so the canonical "installed" copy wins.
	if s.RemoteCatalog != nil {
		installed := make(map[string]bool, len(snap.Plugins))
		for _, p := range snap.Plugins {
			installed[p.Provider.String()+"/"+p.Name] = true
		}
		mpKey := func(provider ProviderID, name string) string {
			return string(provider) + "/" + strings.ToLower(name)
		}
		seenMP := make(map[string]bool, len(snap.Marketplaces))
		for _, m := range snap.Marketplaces {
			seenMP[mpKey(m.Provider, m.Name)] = true
		}
		for _, rc := range s.RemoteCatalog.FetchAll(ctx) {
			for _, p := range rc.Plugins {
				key := p.Provider.String() + "/" + p.Name
				if installed[key] {
					continue
				}
				snap.Plugins = append(snap.Plugins, p)
			}
			for _, m := range rc.Marketplaces {
				k := mpKey(m.Provider, m.Name)
				if seenMP[k] {
					continue
				}
				seenMP[k] = true
				m.Installed = false
				snap.Marketplaces = append(snap.Marketplaces, m)
			}
		}
	}

	sortSnapshot(snap)
	hydrateSessionExtras(snap)
	return snap, nil
}

type providerResults struct {
	marketplaces []Marketplace
	plugins      []Plugin
	skills       []Skill
	mcps         []MCP
	sessions     []Session
	// scanErr captures the first non-ErrNotSupported error returned by
	// any of the read methods (PR #77 review #9). Lets the scan
	// surface real configuration problems via ProviderStatus.Error.
	scanErr error
}

func scanProvider(ctx context.Context, p Provider) providerResults {
	var r providerResults
	record := func(err error) {
		if err == nil || errors.Is(err, ErrNotSupported) {
			return
		}
		if r.scanErr == nil {
			r.scanErr = err
		}
	}
	if v, err := p.Marketplaces(ctx); err == nil {
		r.marketplaces = v
	} else {
		record(err)
	}
	if v, err := p.Plugins(ctx); err == nil {
		r.plugins = v
	} else {
		record(err)
	}
	if v, err := p.Skills(ctx); err == nil {
		r.skills = v
	} else {
		record(err)
	}
	// PR #77 review: collapsed the MCP branch — providers return
	// (earlier-pages, err) on per-page failures, so assigning the
	// returned slice unconditionally preserves whatever the provider
	// gave us (nil is harmless, equivalent to "no MCPs from this
	// provider"). A partially-decoded page within a failing page is
	// NOT preserved; json.Decode is all-or-nothing per page.
	v, err := p.MCPs(ctx)
	r.mcps = v
	if err != nil {
		record(err)
	}
	if v, err := p.Sessions(ctx); err == nil {
		r.sessions = v
	} else {
		record(err)
	}
	return r
}

func sortSnapshot(s *Snapshot) {
	sort.SliceStable(s.Marketplaces, func(i, j int) bool { return s.Marketplaces[i].Name < s.Marketplaces[j].Name })
	sort.SliceStable(s.Plugins, func(i, j int) bool { return s.Plugins[i].Name < s.Plugins[j].Name })
	sort.SliceStable(s.Skills, func(i, j int) bool { return s.Skills[i].Name < s.Skills[j].Name })
	sort.SliceStable(s.MCPs, func(i, j int) bool { return s.MCPs[i].Name < s.MCPs[j].Name })
	sort.SliceStable(s.Sessions, func(i, j int) bool {
		// recent first
		return s.Sessions[i].LastModified.After(s.Sessions[j].LastModified)
	})
}

// Search runs a fuzzy search across the current Snapshot.
// scope == "" means all entity types. A `<type>:<query>` prefix in
// query overrides scope.
func (s *Service) Search(query string, scope EntityType) []SearchResult {
	snap := s.Snapshot()
	if snap == nil {
		return nil
	}

	// Honour scoped-query prefix.
	if t, rest := ParseScopedQuery(query); t != "" {
		scope = t
		query = rest
	}
	query = strings.TrimSpace(query)

	var results []SearchResult

	collect := func(typ EntityType, id, name, subtitle string, provider ProviderID) {
		if scope != "" && scope != typ {
			return
		}
		score, matches := FuzzyMatch(query, name)
		if score == 0 && query != "" {
			// Try description / subtitle as a secondary match target.
			altScore, _ := FuzzyMatch(query, subtitle)
			if altScore == 0 {
				return
			}
			score = altScore / 2
		}
		results = append(results, SearchResult{
			Score:    score,
			Type:     typ,
			ID:       id,
			Name:     name,
			Subtitle: subtitle,
			Provider: provider,
			Matches:  matches,
		})
	}

	for _, m := range snap.Marketplaces {
		collect(EntityMarketplace, m.ID, m.Name, m.Description, m.Provider)
	}
	for _, p := range snap.Plugins {
		collect(EntityPlugin, p.ID, p.Name, p.Description, p.Provider)
	}
	for _, k := range snap.Skills {
		collect(EntitySkill, k.ID, k.Name, k.Description, k.Provider)
	}
	for _, m := range snap.MCPs {
		collect(EntityMCP, m.ID, m.Name, mcpSubtitle(m), m.Provider)
	}
	for _, s := range snap.Sessions {
		collect(EntitySession, s.ID, sessionDisplayName(s), s.ProjectPath, s.Provider)
	}

	rankResults(results)
	return results
}

func mcpSubtitle(m MCP) string {
	switch m.Transport {
	case "http", "sse":
		return m.URL
	default:
		if len(m.Args) > 0 {
			return m.Command + " " + strings.Join(m.Args, " ")
		}
		return m.Command
	}
}

func sessionDisplayName(s Session) string {
	if s.Name != "" {
		return s.Name
	}
	if s.Title != "" {
		return s.Title
	}
	return s.ID
}

// Launch builds the ExecPlan for the spec. Caller (TUI or CLI) is
// responsible for actually executing it.
func (s *Service) Launch(spec LaunchSpec) (ExecPlan, error) {
	if spec.Provider == "" {
		return ExecPlan{}, errors.New("launch: provider is required")
	}
	p := s.registry.Get(spec.Provider)
	if p == nil {
		return ExecPlan{}, errors.New("launch: provider not registered: " + string(spec.Provider))
	}
	return p.BuildLaunch(spec)
}

// ProviderFor returns the registered provider with the given ID, or nil.
func (s *Service) ProviderFor(id ProviderID) Provider { return s.registry.Get(id) }

// hydrateSessionExtras fills in dashboard-only Session fields that are
// not derivable from a single provider's view: the Starred flag (from
// the bookmarks store) and the Group label (from the smart grouping
// resolver, with user-defined cwd→group overrides).
//
// Failures are silent: a missing bookmarks store or grouping file is
// not an error condition — the snapshot's Group and Starred fields
// just stay at their zero values, and `klim agents sessions list`
// renders without grouping headers.
//
// $HOME (or %USERPROFILE% on Windows) is resolved once here and
// passed into Resolve so the "🏠 Home" special-case fires correctly.
func hydrateSessionExtras(snap *Snapshot) {
	if snap == nil || len(snap.Sessions) == 0 {
		return
	}

	var starred map[string]bool
	if st, err := bookmarks.Load(); err == nil && st != nil {
		starred = make(map[string]bool, st.Count())
		for _, e := range st.All() {
			starred[e.SessionID] = true
		}
	} else if err != nil {
		// Bookmarks file is corrupt or unreadable — surface a hint
		// to stderr so a "my stars aren't showing" issue is visible
		// at the source rather than silently ignored. We still
		// proceed with an empty starred set so the rest of the
		// snapshot stays usable.
		fmt.Fprintf(os.Stderr, "agents: bookmarks load failed (stars not hydrated): %v\n", err)
	}

	mappings := map[string]string{}
	if gm, err := enrich.LoadGroupingMappings(); err == nil && gm != nil {
		mappings = gm.All()
	} else if err != nil {
		// Grouping file is corrupt or unreadable — same approach as
		// bookmarks: warn loudly to stderr so a "my custom groups
		// stopped working" issue is debuggable, then fall back to
		// the empty mapping set (resolver still produces a group
		// via repo / cwd / keyword fallback).
		fmt.Fprintf(os.Stderr, "agents: grouping mappings load failed (custom groups not applied): %v\n", err)
	}

	home := homeDir()

	for i := range snap.Sessions {
		s := &snap.Sessions[i]
		if starred[s.ID] {
			s.Starred = true
		}
		// Always recompute Group so user mapping edits (saved via
		// `klim agents sessions group set …`) take effect on the
		// next list / TUI render even when the snapshot comes from
		// cache.
		s.Group = enrich.Resolve(s.ProjectPath, s.Repository, s.Title, home, mappings)
	}
}

// homeDir returns the user's home directory across POSIX and Windows,
// or the empty string when neither HOME nor USERPROFILE is set.
func homeDir() string {
	if env := os.Getenv("HOME"); env != "" {
		return env
	}
	return os.Getenv("USERPROFILE")
}
