package envid

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/nassiharel/clim/internal/audit"
	"github.com/nassiharel/clim/internal/build"
	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/custompacks"
	"github.com/nassiharel/clim/internal/favorites"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/security"
	"github.com/nassiharel/clim/internal/service"
	"github.com/nassiharel/clim/internal/vuln"
)

// BuildOptions controls Build. Zero value is valid for normal use.
type BuildOptions struct {
	// Now overrides the timestamp baked into Profile.GeneratedAt.
	// Tests set this; production callers leave it zero.
	Now time.Time
}

// Build assembles a Profile from the live system. It performs a tool
// scan via the supplied service and reads favorites/custom packs
// from disk. Vuln data is read from the cache only — Build never
// hits the network.
func Build(ctx context.Context, svc *service.ToolService, cfg *config.Config, opts BuildOptions) (*Profile, error) {
	if svc == nil {
		return nil, errors.New("envid.Build: service is required")
	}

	tools, _, _, err := svc.LoadAndResolveCached(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("envid.Build: scanning tools: %w", err)
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	p := &Profile{
		SchemaVersion: SchemaVersion,
		Clim: ClimInfo{
			Version: build.VersionOnly(),
			Commit:  build.Commit,
		},
		GeneratedAt:     now.UTC(),
		OS:              osInfo(),
		PackageManagers: pmStatusMap(),
		Tools:           collectTools(tools),
		Favorites:       collectFavorites(),
		Packs:           collectCustomPacks(),
		Security:        collectSecurity(tools, cfg),
	}

	canonicalize(p)
	p.Hash = ComputeHash(p)
	return p, nil
}

func osInfo() OSInfo {
	return OSInfo{
		GOOS:   runtime.GOOS,
		Arch:   runtime.GOARCH,
		Distro: detectDistro(),
	}
}

func pmStatusMap() map[string]bool {
	out := make(map[string]bool)
	for _, st := range registry.AllPMStatusForOS() {
		out[strings.ToLower(string(st.Source))] = st.Available
	}
	return out
}

func collectTools(tools []registry.Tool) []Tool {
	out := make([]Tool, 0, len(tools))
	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		entry := Tool{
			Name:     t.Name,
			Category: t.Category,
		}
		if primary := t.PrimaryInstance(); primary != nil {
			entry.Version = primary.Version
			entry.Source = string(primary.Source)
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func collectFavorites() []string {
	names, err := favorites.Load()
	if err != nil || len(names) == 0 {
		return nil
	}
	// dedupSorted (canonicalize helper) trims, dedupes, and sorts —
	// yields a stable favorite list that doesn't perturb the hash
	// from manual file edits.
	return dedupSorted(names)
}

func collectCustomPacks() []Pack {
	packs, err := custompacks.Load()
	if err != nil || len(packs) == 0 {
		return nil
	}
	out := make([]Pack, 0, len(packs))
	for _, p := range packs {
		out = append(out, Pack{
			Name:        p.Name,
			DisplayName: p.DisplayName,
			Tools:       dedupSorted(p.ToolNames),
		})
	}
	sortPacksByName(out)
	return out
}

func collectSecurity(tools []registry.Tool, cfg *config.Config) Security {
	findings, _ := audit.Analyze(tools)
	warn, info := audit.CountBySeverity(findings)

	url := vuln.DefaultOSVURL
	if cfg != nil {
		if u := strings.TrimSpace(cfg.Vuln.URL); u != "" {
			url = u
		}
	}
	var report *vuln.Report
	if rep, ok := vuln.ReadCache(url); ok {
		report = rep
	}
	idx := security.BuildIndex(tools, findings, report)
	clean, watch, risk, unknown := idx.Counts()

	return Security{
		AuditWarnings: warn,
		AuditInfos:    info,
		Verdicts: VerdictsCounts{
			Clean:   clean,
			Watch:   watch,
			Risk:    risk,
			Unknown: unknown,
		},
	}
}
