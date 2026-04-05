package latest

import (
	"runtime"
	"strings"
	"sync"

	"github.com/nassiharel/clim/internal/registry"
)

// CheckAll looks up the latest available version for each tool that has a
// known LatestSource. Populates tool.LatestVersion in place. Uses a cache
// to avoid redundant API calls.
func CheckAll(tools []registry.Tool, cache *Cache, concurrency int) {
	if concurrency <= 0 {
		concurrency = runtime.NumCPU()
	}

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i := range tools {
		kt, ok := registry.LookupKnown(strings.ToLower(tools[i].Name))
		if !ok || kt.LatestSource.Type == "" {
			continue
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(t *registry.Tool, src registry.LatestSource) {
			defer wg.Done()
			defer func() { <-sem }()
			t.LatestVersion = checkOne(src, cache)
		}(&tools[i], kt.LatestSource)
	}

	wg.Wait()
}

// FetchOne fetches the latest version for a single source (no caching).
// Exported for use by the TUI's per-tool commands.
func FetchOne(src registry.LatestSource) string {
	switch src.Type {
	case registry.SourceGitHub:
		return fetchGitHub(src.Repo)
	case registry.SourcePyPI:
		return fetchPyPI(src.Package)
	case registry.SourceNPM:
		return fetchNPM(src.Package)
	}
	return ""
}

// checkOne fetches the latest version for a single source, using cache.
func checkOne(src registry.LatestSource, cache *Cache) string {
	key := cacheKey(src)

	// Check cache first.
	if v, ok := cache.Get(key); ok {
		return v
	}

	var ver string
	switch src.Type {
	case registry.SourceGitHub:
		ver = fetchGitHub(src.Repo)
	case registry.SourcePyPI:
		ver = fetchPyPI(src.Package)
	case registry.SourceNPM:
		ver = fetchNPM(src.Package)
	}

	if ver != "" {
		cache.Set(key, ver)
	}
	return ver
}

func cacheKey(src registry.LatestSource) string {
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
