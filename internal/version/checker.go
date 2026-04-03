package version

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/nassiharel/clim/internal/registry"
)

// LatestVersion holds the result of a latest version check.
type LatestVersion struct {
	Version string
	Error   error
}

// Checker can look up the latest version of a tool.
type Checker interface {
	Latest(ctx context.Context, source registry.VersionSource) LatestVersion
}

// HTTPChecker implements Checker using HTTP API calls to various registries.
type HTTPChecker struct {
	client      *http.Client
	githubToken string
	baseURL     string // overridable for testing
}

// NewHTTPChecker creates a Checker that talks to public APIs.
// If githubToken is non-empty, it's used for GitHub API authentication
// (raises rate limit from 60 to 5000 requests/hour).
func NewHTTPChecker(githubToken string) *HTTPChecker {
	return &HTTPChecker{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		githubToken: githubToken,
	}
}

// Latest dispatches to the appropriate API based on the source type.
func (c *HTTPChecker) Latest(ctx context.Context, src registry.VersionSource) LatestVersion {
	switch src.Type {
	case registry.SourceGitHub:
		return c.latestGitHub(ctx, src.Repo)
	case registry.SourcePyPI:
		return c.latestPyPI(ctx, src.Package)
	case registry.SourceNPM:
		return c.latestNPM(ctx, src.Package)
	case registry.SourceCustom:
		return c.latestCustom(ctx, src.URLPattern)
	default:
		return LatestVersion{Error: fmt.Errorf("unknown source type: %s", src.Type)}
	}
}

// CheckAll checks the latest version for all tools concurrently.
// Uses cache to avoid redundant API calls. Returns results in the same
// order as the input slice.
func CheckAll(ctx context.Context, checker Checker, cache *Cache, tools []registry.Tool) []LatestVersion {
	results := make([]LatestVersion, len(tools))
	var wg sync.WaitGroup

	for i, tool := range tools {
		wg.Add(1)
		go func(idx int, t registry.Tool) {
			defer wg.Done()

			// Check cache first.
			key := cacheKey(t)
			if v, ok := cache.Get(key); ok {
				results[idx] = LatestVersion{Version: v}
				return
			}

			result := checker.Latest(ctx, t.LatestSource)
			if result.Error == nil && result.Version != "" {
				cache.Set(key, result.Version)
			}
			results[idx] = result
		}(i, tool)
	}

	wg.Wait()
	return results
}

func cacheKey(tool registry.Tool) string {
	src := tool.LatestSource
	switch src.Type {
	case registry.SourceGitHub:
		return "github:" + src.Repo
	case registry.SourcePyPI:
		return "pypi:" + src.Package
	case registry.SourceNPM:
		return "npm:" + src.Package
	case registry.SourceCustom:
		return "custom:" + src.URLPattern
	default:
		return "unknown:" + tool.Name
	}
}

// TokenFromEnv reads the GitHub token from the environment.
func TokenFromEnv() string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("GH_TOKEN")
}
