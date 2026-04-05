package tui

import (
	"runtime"
	"strings"

	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/detector"
	"github.com/nassiharel/clim/internal/latest"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/scanner"
)

// scanResultMsg is sent when PATH scanning completes (Phase 1).
type scanResultMsg struct {
	tools []registry.Tool
	err   error
}

// versionResultMsg is sent when version detection completes for one tool.
type versionResultMsg struct {
	index   int
	version string
}

// latestResultMsg is sent when latest-version check completes for one tool.
type latestResultMsg struct {
	index   int
	version string
}

// scanPATHCmd scans PATH for executables.
func scanPATHCmd(cfg config.Config) func() scanResultMsg {
	return func() scanResultMsg {
		tools, err := scanner.ScanPATH(cfg)
		return scanResultMsg{tools: tools, err: err}
	}
}

// detectVersionCmd detects the installed version for a single tool.
func detectVersionCmd(index int, path string) func() versionResultMsg {
	return func() versionResultMsg {
		ver := detector.Detect(path)
		return versionResultMsg{index: index, version: ver}
	}
}

// checkLatestCmd checks the latest version for a single known tool.
func checkLatestCmd(index int, name string, cache *latest.Cache) func() latestResultMsg {
	return func() latestResultMsg {
		kt, ok := registry.LookupKnown(strings.ToLower(name))
		if !ok || kt.LatestSource.Type == "" {
			return latestResultMsg{index: index}
		}

		_ = runtime.NumCPU() // satisfy import if needed
		ver := checkOneLatest(kt.LatestSource, cache)
		return latestResultMsg{index: index, version: ver}
	}
}

// checkOneLatest fetches the latest version for a single source.
func checkOneLatest(src registry.LatestSource, cache *latest.Cache) string {
	key := cacheKeyFor(src)
	if v, ok := cache.Get(key); ok {
		return v
	}

	// Reuse the latest package's fetch functions via a direct call.
	// Since we can't import the unexported functions, we duplicate the
	// lookup logic here. The latest.CheckAll is used for CLI; this is for TUI.
	ver := latest.FetchOne(src)
	if ver != "" {
		cache.Set(key, ver)
	}
	return ver
}

func cacheKeyFor(src registry.LatestSource) string {
	switch src.Type {
	case registry.SourceGitHub:
		return "github:" + src.Repo
	case registry.SourcePyPI:
		return "pypi:" + src.Package
	case registry.SourceNPM:
		return "npm:" + src.Package
	default:
		return "unknown"
	}
}
