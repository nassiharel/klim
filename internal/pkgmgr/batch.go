package pkgmgr

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/nassiharel/klim/internal/registry"
)

// pmBatch is the per-PM bulk-query result. installed[id] is the
// currently-installed version (empty string when the package is not
// listed). latest[id] is the target version when the PM reports an
// upgrade is available; if missing the resolver falls back to the
// per-tool query so cache misses degrade gracefully.
type pmBatch struct {
	installed map[string]string
	latest    map[string]string
	// hasLatest is true after the latest map has been populated.
	// Distinguishes "not in upgrades list — already up-to-date"
	// from "we never queried — please fall back to per-tool".
	hasLatest bool
}

// batchCache aggregates pmBatch results keyed by source. Lives on
// PackageManagerResolver so each ResolveVersions invocation starts
// fresh — the bulk data is intentionally not persisted across calls
// (versions change after install/upgrade/remove, and a stale cache
// would surface wrong versions in the TUI).
type batchCache struct {
	mu      sync.RWMutex
	sources map[registry.InstallSource]*pmBatch
}

func newBatchCache() *batchCache {
	return &batchCache{sources: map[registry.InstallSource]*pmBatch{}}
}

func (c *batchCache) get(s registry.InstallSource) (*pmBatch, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	b, ok := c.sources[s]
	return b, ok
}

func (c *batchCache) set(s registry.InstallSource, b *pmBatch) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sources[s] = b
}

// prewarm runs every bulk fetcher in parallel for the package
// managers that actually own at least one installed tool. Cheap PM
// probes (e.g. apt is two file reads on Linux; brew list --versions
// is ~50 ms on a warm SSD) fan out to all cores so the total wall-
// clock cost is the slowest individual PM, not the sum.
//
// Each bulk fetcher writes to its own pmBatch entry in the cache;
// failures land as nil-but-present sentinels so callers can tell
// "queried and PM had no entry" from "never queried".
func (c *batchCache) prewarm(ctx context.Context, tools []registry.Tool, timeout time.Duration) {
	needed := map[registry.InstallSource]bool{}
	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		for _, inst := range t.Instances {
			if inst.Source != "" && inst.Source != registry.SourceManual {
				needed[inst.Source] = true
			}
		}
	}
	if len(needed) == 0 {
		return
	}

	// Bulk fetches are bound by their own context — twice the
	// per-tool timeout because each bulk command is a single
	// subprocess that does the work N per-tool calls would have.
	bulkTimeout := timeout * 2
	if bulkTimeout < 30*time.Second {
		bulkTimeout = 30 * time.Second
	}

	var wg sync.WaitGroup
	for src := range needed {
		// Skip PMs whose binary isn't on PATH — falling back to
		// per-tool queries is what the rest of the resolver does
		// today, so we get the same result without spawning a
		// guaranteed-to-fail bulk subprocess.
		if !pmBinaryAvailable(src) {
			continue
		}
		wg.Add(1)
		go func(s registry.InstallSource) {
			defer wg.Done()
			bctx, cancel := context.WithTimeout(ctx, bulkTimeout)
			defer cancel()
			b := &pmBatch{installed: map[string]string{}, latest: map[string]string{}}
			switch s {
			case registry.SourceWinget:
				wingetBulkFetch(bctx, b)
			case registry.SourceBrew:
				brewBulkFetch(bctx, b)
			case registry.SourceScoop:
				scoopBulkFetch(bctx, b)
			case registry.SourceChoco:
				chocoBulkFetch(bctx, b)
			}
			c.set(s, b)
		}(src)
	}
	wg.Wait()
}

func pmBinaryAvailable(src registry.InstallSource) bool {
	bin := ""
	switch src {
	case registry.SourceWinget:
		bin = "winget"
	case registry.SourceBrew:
		bin = "brew"
	case registry.SourceScoop:
		bin = "scoop"
	case registry.SourceChoco:
		bin = "choco"
	case registry.SourceApt:
		bin = "apt"
	case registry.SourceSnap:
		bin = "snap"
	case registry.SourceNPM:
		bin = "npm"
	}
	if bin == "" {
		return false
	}
	_, err := exec.LookPath(bin)
	return err == nil
}

// --- winget bulk -----------------------------------------------------

// wingetBulkFetch runs `winget list` once to populate installed
// versions for every package, and `winget upgrade` once to learn
// which packages have a newer version. Both commands accept source
// agreements non-interactively so they don't block on the prompt.
//
// hasLatest flips to true ONLY when the upgrade command succeeded
// (non-empty output OR the parser populated at least one entry).
// If only the list call worked, latestVersion() must fall back to
// per-tool queries rather than silently treating every missing entry
// as "up-to-date" — that would mask real upgrades.
func wingetBulkFetch(ctx context.Context, b *pmBatch) {
	if out := runCmd(ctx, "winget", "list", "--accept-source-agreements"); out != "" {
		parseWingetTable(cleanWingetOutput(out), b.installed)
	}
	if out := runCmd(ctx, "winget", "upgrade", "--include-unknown", "--accept-source-agreements"); out != "" {
		parseWingetUpgradeTable(cleanWingetOutput(out), b.latest)
		// Even an empty parse result is acceptable here as long as
		// the upgrade command itself produced output — that means
		// the bulk query succeeded and "missing from upgrades" can
		// safely be interpreted as "up-to-date".
		b.hasLatest = true
	}
}

// parseWingetTable parses `winget list` output. The table is
// whitespace-aligned with columns: Name, Id, Version, [Available,]
// Source. Header rows have separator lines under them; we recognise
// the separator (a row of dashes) and treat every line after it as
// data until a blank line or another header.
func parseWingetTable(out string, into map[string]string) {
	cols := wingetColumnRanges(out, "Id", "Version")
	if cols == nil {
		return
	}
	idStart, idEnd := cols[0][0], cols[0][1]
	verStart, verEnd := cols[1][0], cols[1][1]
	inBody := false
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "---") {
			inBody = true
			continue
		}
		if !inBody {
			continue
		}
		if strings.TrimSpace(line) == "" {
			inBody = false
			continue
		}
		id := strings.TrimSpace(safeSlice(line, idStart, idEnd))
		ver := strings.TrimSpace(safeSlice(line, verStart, verEnd))
		if id == "" || ver == "" || strings.HasPrefix(ver, "-") {
			continue
		}
		into[id] = ver
	}
}

// parseWingetUpgradeTable parses `winget upgrade` output. Same
// table format as `winget list` but with an additional "Available"
// column that we want.
func parseWingetUpgradeTable(out string, into map[string]string) {
	cols := wingetColumnRanges(out, "Id", "Available")
	if cols == nil {
		return
	}
	idStart, idEnd := cols[0][0], cols[0][1]
	avStart, avEnd := cols[1][0], cols[1][1]
	inBody := false
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "---") {
			inBody = true
			continue
		}
		if !inBody {
			continue
		}
		if strings.TrimSpace(line) == "" {
			inBody = false
			continue
		}
		id := strings.TrimSpace(safeSlice(line, idStart, idEnd))
		av := strings.TrimSpace(safeSlice(line, avStart, avEnd))
		if id == "" || av == "" || strings.HasPrefix(av, "-") {
			continue
		}
		into[id] = av
	}
}

// wingetKnownColumns is the full set of column headers winget may
// emit. wingetColumnRanges scans for ALL of them when bounding any
// requested column so that e.g. asking for "Version" never returns
// a slice that overruns into the trailing "Available" or "Source"
// column. The previous implementation only bounded by columns the
// caller explicitly requested, which made `winget list` return
// versions like `"0.71.0  winget"`.
var wingetKnownColumns = []string{"Name", "Id", "Version", "Available", "Source", "Match"}

// wingetColumnRanges scans `out` for the header row containing every
// column name in want; returns one [start,end] byte range per
// requested column, in the order requested. Returns nil if any
// requested column is missing from the header. Bounds use the full
// set of known winget column names — not just the requested ones —
// so the returned ranges never swallow adjacent columns we don't
// care about.
func wingetColumnRanges(out string, want ...string) [][2]int {
	for _, line := range strings.Split(out, "\n") {
		// Collect start offsets for every known column that appears
		// on this line. We require every requested column to be
		// present; otherwise we keep scanning for a better header.
		allStarts := make(map[string]int, len(wingetKnownColumns))
		for _, name := range wingetKnownColumns {
			if idx := strings.Index(line, name); idx >= 0 {
				allStarts[name] = idx
			}
		}
		ranges := make([][2]int, len(want))
		ok := true
		for i, w := range want {
			start, present := allStarts[w]
			if !present {
				ok = false
				break
			}
			// Bound by the nearest later column from the FULL known
			// set, not just from `want`. That keeps Version from
			// running into Available/Source on `winget list`.
			end := len(line)
			for _, otherStart := range allStarts {
				if otherStart > start && otherStart < end {
					end = otherStart
				}
			}
			ranges[i] = [2]int{start, end}
		}
		if ok {
			return ranges
		}
	}
	return nil
}

func safeSlice(s string, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(s) {
		end = len(s)
	}
	if start >= end {
		return ""
	}
	return s[start:end]
}

// --- brew bulk -------------------------------------------------------

// brewBulkFetch runs `brew list --versions` and `brew outdated`
// once each. `brew list --versions` prints "name version [version
// ...]" lines for every installed formula; we take the first
// version. `brew outdated --json=v2` gives us reliable structured
// data about available upgrades.
//
// hasLatest flips to true ONLY when the outdated query produced
// output; an empty/failed outdated result leaves it false so
// latestVersion() falls back to per-tool queries.
func brewBulkFetch(ctx context.Context, b *pmBatch) {
	if out := runCmd(ctx, "brew", "list", "--versions"); out != "" {
		for _, line := range strings.Split(out, "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				b.installed[fields[0]] = fields[1]
			}
		}
	}
	if out := runCmd(ctx, "brew", "outdated", "--json=v2"); out != "" {
		parseBrewOutdated(out, b.latest)
		b.hasLatest = true
	}
}

// parseBrewOutdated extracts {name -> current_version} pairs from
// brew's JSON v2 output. Uses string scanning to keep the implementation
// dependency-free; brew's schema is stable enough that this is fine
// in practice, and a failure here just falls through to per-tool.
func parseBrewOutdated(out string, into map[string]string) {
	// Match: "name":"foo","installed_versions":["1.0.0"],"current_version":"1.0.1"
	type span struct {
		name string
		cur  string
	}
	scan := func(jsonField, target string) string {
		key := `"` + jsonField + `":"`
		idx := strings.Index(target, key)
		if idx < 0 {
			return ""
		}
		idx += len(key)
		end := strings.Index(target[idx:], `"`)
		if end < 0 {
			return ""
		}
		return target[idx : idx+end]
	}
	// Split on object boundary heuristic: each formula entry starts
	// with `"name":"`.
	chunks := strings.Split(out, `"name":"`)
	for i, chunk := range chunks {
		if i == 0 {
			continue
		}
		end := strings.Index(chunk, `"`)
		if end < 0 {
			continue
		}
		s := span{name: chunk[:end]}
		s.cur = scan("current_version", chunk)
		if s.name != "" && s.cur != "" {
			into[s.name] = s.cur
		}
	}
}

// --- scoop bulk ------------------------------------------------------

// scoopBulkFetch runs `scoop list` once. Scoop has no JSON output;
// we reuse parseScoopList's table semantics across every line. For
// latest we run `scoop status` which lists outdated packages.
//
// hasLatest flips to true ONLY when the scoop status query produced
// output; an empty/failed result leaves it false so latestVersion()
// falls back to per-tool queries.
func scoopBulkFetch(ctx context.Context, b *pmBatch) {
	if out := runCmd(ctx, "scoop", "list"); out != "" {
		for _, line := range strings.Split(stripANSI(out), "\n") {
			fields := strings.Fields(strings.TrimSpace(line))
			if len(fields) < 2 {
				continue
			}
			if strings.EqualFold(fields[0], "Name") || strings.HasPrefix(fields[0], "---") {
				continue
			}
			b.installed[fields[0]] = fields[1]
		}
	}
	if out := runCmd(ctx, "scoop", "status"); out != "" {
		for _, line := range strings.Split(stripANSI(out), "\n") {
			fields := strings.Fields(strings.TrimSpace(line))
			// Output columns: Name InstalledVersion LatestVersion Notes
			if len(fields) >= 3 && !strings.EqualFold(fields[0], "Name") {
				b.latest[fields[0]] = fields[2]
			}
		}
		b.hasLatest = true
	}
}

// --- choco bulk ------------------------------------------------------

// chocoBulkFetch runs `choco list --local-only --limit-output` and
// `choco outdated --limit-output`. The --limit-output flag emits one
// pipe-separated record per line; both commands have stable output
// formats so we can parse without JSON.
//
// hasLatest flips to true ONLY when the choco outdated query
// produced output; an empty/failed result leaves it false so
// latestVersion() falls back to per-tool queries.
func chocoBulkFetch(ctx context.Context, b *pmBatch) {
	if out := runCmd(ctx, "choco", "list", "--local-only", "--limit-output"); out != "" {
		for _, line := range strings.Split(out, "\n") {
			parts := strings.SplitN(strings.TrimSpace(line), "|", 2)
			if len(parts) == 2 {
				b.installed[parts[0]] = parts[1]
			}
		}
	}
	if out := runCmd(ctx, "choco", "outdated", "--limit-output"); out != "" {
		// Format: pkg|currentVersion|latestVersion|pinned
		for _, line := range strings.Split(out, "\n") {
			parts := strings.Split(strings.TrimSpace(line), "|")
			if len(parts) >= 3 && parts[0] != "" {
				b.latest[parts[0]] = parts[2]
			}
		}
		b.hasLatest = true
	}
}
