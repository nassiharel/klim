package vuln

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/klim/internal/fileutil"
	"github.com/nassiharel/klim/internal/paths"
	"github.com/nassiharel/klim/internal/registry"
)

// Looker queries one Coord at a time. OSVClient implements it; tests
// stub it out with a fake.
type Looker interface {
	Query(ctx context.Context, coord Coord) ([]Vulnerability, error)
}

// LookupOptions controls Lookup behaviour.
type LookupOptions struct {
	// MaxAge enables cache freshness checks. When > 0 and the cache
	// mtime is older than MaxAge, the lookup refetches.
	MaxAge time.Duration

	// ForceRefresh ignores the cache and always queries the Looker.
	ForceRefresh bool

	// Concurrency caps how many in-flight queries to run against the
	// Looker. 0 → 8.
	Concurrency int
}

// ReadCache loads the most recent persisted Report for sourceKey
// without ever hitting the network. Returns (nil, false) when no
// cache exists or it can't be parsed. Used by surfaces (klim tool info,
// TUI tool detail, web tool page) that want to show vuln data when
// it's already there but not pay the latency of a fresh fetch.
func ReadCache(sourceKey string) (*Report, bool) {
	cachePath, err := cachePathFor(sourceKey)
	if err != nil {
		return nil, false
	}
	return readCache(cachePath, 0)
}

// Lookup builds a Report for the given tools using a cached Looker.
//
// Cache: results are persisted to paths.VulnCachePathFor(sourceKey).
// On a fresh, non-stale cache we deserialize and return without
// touching the network. On stale cache or first run we query the
// Looker, write the cache, and return.
//
// On Looker failure with a stale cache, the stale cache is returned
// rather than an error — same "graceful degradation" pattern as
// internal/compliance.LoadOrFetch.
//
// sourceKey is the URL / identifier of the data source; pass the
// OSV.dev URL or a self-hosted mirror so the cache is keyed per-source
// (PR #51's URL-keyed pattern).
func Lookup(ctx context.Context, looker Looker, tools []registry.Tool, sourceKey string, opts LookupOptions) (*Report, error) {
	cachePath, err := cachePathFor(sourceKey)
	if err != nil {
		return nil, fmt.Errorf("resolving cache path: %w", err)
	}

	if !opts.ForceRefresh {
		if cached, ok := readCache(cachePath, opts.MaxAge); ok {
			return cached, nil
		}
	}

	report, fetchErr := fetch(ctx, looker, tools, sourceKey, opts.Concurrency)
	if fetchErr != nil {
		// Fall back to stale cache when available — this is what makes
		// `klim security vuln` work on a plane / disconnected. But not
		// when the caller explicitly asked for fresh data: returning a
		// silent stale value to a `--force-refresh-vulns` user makes
		// CI gating unsafe.
		if !opts.ForceRefresh {
			if cached, ok := readCache(cachePath, 0); ok {
				slog.Warn("vuln fetch failed, serving stale cache", "path", cachePath, "error", fetchErr)
				return cached, nil
			}
		}
		return nil, fetchErr
	}

	if err := writeCache(cachePath, report); err != nil {
		slog.Warn("vuln cache write failed", "path", cachePath, "error", err)
	}
	return report, nil
}

// cachePathFor mirrors compliance.cachePathFor: per-source cache file
// so switching `vuln.url` doesn't reuse another source's data.
func cachePathFor(key string) (string, error) {
	return paths.VulnCachePathFor(key)
}

// readCache loads a previously-written report. Returns ok=false on
// any read/parse failure or when maxAge>0 and the cache mtime is
// older than maxAge.
func readCache(path string, maxAge time.Duration) (*Report, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	if maxAge > 0 && time.Since(info.ModTime()) > maxAge {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var r Report
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, false
	}
	return &r, true
}

// writeCache serializes the report through fileutil.AtomicWrite —
// concurrent readers (TUI + CLI in two terminals) get a consistent
// view, never a half-written file. Caller decides whether to log or
// surface a write failure (Lookup just slog.Warn's it).
func writeCache(path string, r *Report) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating vuln cache dir: %w", err)
	}
	data, err := yaml.Marshal(r)
	if err != nil {
		return fmt.Errorf("marshalling vuln cache: %w", err)
	}
	return fileutil.AtomicWrite(path, data, 0o644)
}

// fetch queries the Looker for every mappable tool. Tools with no
// ecosystem coords land in Skipped instead of erroring the whole
// run.
func fetch(ctx context.Context, looker Looker, tools []registry.Tool, sourceKey string, concurrency int) (*Report, error) {
	if concurrency <= 0 {
		concurrency = 8
	}

	type plan struct {
		tool   registry.Tool
		coords []Coord
	}
	plans := make([]plan, 0, len(tools))
	skipped := make([]Skip, 0)
	scanned := 0
	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		scanned++
		coords, reason := Map(t)
		if reason != "" {
			skipped = append(skipped, Skip{Tool: t.Name, Reason: reason})
			continue
		}
		plans = append(plans, plan{tool: t, coords: coords})
	}

	// Fan out: one goroutine per plan, capped by `concurrency`. We
	// collect results in a deterministic order (input order) by
	// pre-allocating the result slice and indexing into it.
	results := make([]Match, len(plans))
	resultErrs := make([]error, len(plans))

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i := range plans {
		i := i
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			p := plans[i]
			merged := make(map[string]Vulnerability)
			var queryErr error
			for _, c := range p.coords {
				vulns, err := looker.Query(ctx, c)
				if err != nil {
					queryErr = err
					break
				}
				for _, v := range vulns {
					if _, exists := merged[v.ID]; !exists {
						merged[v.ID] = v
					}
				}
			}
			if queryErr != nil {
				resultErrs[i] = fmt.Errorf("%s: %w", p.tool.Name, queryErr)
				return
			}
			vlist := make([]Vulnerability, 0, len(merged))
			for _, v := range merged {
				vlist = append(vlist, v)
			}
			// Map iteration is nondeterministic — sort by severity
			// (high first), then by ID, so CLI/TUI/web output is
			// stable across runs and tests can assert ordering.
			sort.SliceStable(vlist, func(a, b int) bool {
				ra, rb := vlist[a].Severity.Rank(), vlist[b].Severity.Rank()
				if ra != rb {
					return ra > rb
				}
				return vlist[a].ID < vlist[b].ID
			})
			primary := p.tool.PrimaryInstance()
			ver := ""
			if primary != nil {
				ver = primary.Version
			}
			coordPick := p.coords[0]
			results[i] = Match{
				Tool:            p.tool.Name,
				DisplayName:     p.tool.DisplayName,
				InstalledVer:    ver,
				Coord:           coordPick,
				Vulnerabilities: vlist,
			}
		}()
	}
	wg.Wait()

	// If everything errored, surface that — but a single tool's
	// transport failure shouldn't kill the whole report. Treat
	// per-tool errors as Skips and only return an error when no
	// tool succeeded.
	matches := make([]Match, 0, len(results))
	successCount := 0
	for i, r := range results {
		if resultErrs[i] != nil {
			skipped = append(skipped, Skip{Tool: plans[i].tool.Name, Reason: "query failed: " + resultErrs[i].Error()})
			continue
		}
		matches = append(matches, r)
		successCount++
	}

	if successCount == 0 && len(plans) > 0 {
		return nil, fmt.Errorf("vuln lookup failed for all %d candidate tools: %w", len(plans), resultErrs[0])
	}

	return &Report{
		ScannedAt:    time.Now().UTC(),
		ToolsScanned: scanned,
		Source:       sourceKey,
		Matches:      matches,
		Skipped:      skipped,
	}, nil
}
