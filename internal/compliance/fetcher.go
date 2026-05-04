// Fetcher loads compliance policies from remote URLs with local caching.
package compliance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/nassiharel/clim/internal/paths"
)

// maxRemotePolicySize caps remote downloads to prevent abuse.
const maxRemotePolicySize = 1 << 20 // 1 MB

// Fetcher abstracts policy retrieval from a remote source.
type Fetcher interface {
	Fetch(ctx context.Context) ([]byte, error)
}

// HTTPFetcher fetches a policy from an HTTP/HTTPS URL.
type HTTPFetcher struct {
	URL        string
	HTTPClient *http.Client // nil = default with 30s timeout
}

// Fetch downloads policy YAML from the configured URL.
func (f *HTTPFetcher) Fetch(ctx context.Context) ([]byte, error) {
	if f.URL == "" {
		return nil, errors.New("policy URL not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "clim/compliance")

	client := f.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching policy: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %s", resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRemotePolicySize+1))
	if err != nil {
		return nil, fmt.Errorf("reading policy: %w", err)
	}
	if int64(len(data)) > maxRemotePolicySize {
		return nil, fmt.Errorf("policy too large (max %d bytes)", maxRemotePolicySize)
	}
	return data, nil
}

// CachePath returns the path to the remote-policy cache file.
func CachePath() (string, error) {
	return paths.ComplianceCachePath()
}

// LoadOptions controls LoadOrFetch behavior.
type LoadOptions struct {
	// MaxAge enables cache freshness checks. When > 0 and the cache mtime is
	// older than MaxAge, the policy is refetched.
	MaxAge time.Duration
}

// LoadOrFetch loads a cached policy. If the cache is missing/stale, fetches
// from the remote and updates the cache. On fetch failure, the stale cache
// (if any) is returned.
func LoadOrFetch(ctx context.Context, fetcher Fetcher, opts LoadOptions) (*Policy, string, error) {
	cachePath, err := CachePath()
	if err != nil {
		return nil, "", fmt.Errorf("resolving cache path: %w", err)
	}

	// Try existing cache.
	if data, err := os.ReadFile(cachePath); err == nil && len(data) > 0 {
		if opts.MaxAge > 0 && cacheIsStale(cachePath, opts.MaxAge) {
			if refreshed, ok := tryRefresh(ctx, fetcher, cachePath, data); ok {
				slog.Info("compliance policy auto-refreshed", "path", cachePath, "bytes", len(refreshed))
				return parsePolicyBytes(refreshed, cachePath)
			}
			slog.Warn("compliance policy refresh failed, serving stale cache", "path", cachePath)
		}
		return parsePolicyBytes(data, cachePath)
	}

	// No cache — fetch.
	slog.Info("fetching compliance policy from remote")
	data, err := fetcher.Fetch(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("fetching policy (no local cache): %w", err)
	}

	if mkErr := os.MkdirAll(filepath.Dir(cachePath), 0o755); mkErr == nil {
		if writeErr := os.WriteFile(cachePath, data, 0o644); writeErr != nil {
			slog.Warn("writing policy cache", "path", cachePath, "error", writeErr)
		}
	}
	return parsePolicyBytes(data, cachePath)
}

// Refresh forces a fetch and updates the cache. Returns the new policy.
func Refresh(ctx context.Context, fetcher Fetcher) (*Policy, string, error) {
	cachePath, err := CachePath()
	if err != nil {
		return nil, "", err
	}
	data, err := fetcher.Fetch(ctx)
	if err != nil {
		return nil, "", err
	}
	if mkErr := os.MkdirAll(filepath.Dir(cachePath), 0o755); mkErr == nil {
		if writeErr := os.WriteFile(cachePath, data, 0o644); writeErr != nil {
			slog.Warn("writing policy cache", "path", cachePath, "error", writeErr)
		}
	}
	return parsePolicyBytes(data, cachePath)
}

func cacheIsStale(path string, maxAge time.Duration) bool {
	info, err := os.Stat(path)
	if err != nil {
		return true
	}
	return time.Since(info.ModTime()) > maxAge
}

func tryRefresh(ctx context.Context, fetcher Fetcher, cachePath string, prev []byte) ([]byte, bool) {
	data, err := fetcher.Fetch(ctx)
	if err != nil {
		return nil, false
	}
	if len(data) == 0 {
		return nil, false
	}
	if mkErr := os.MkdirAll(filepath.Dir(cachePath), 0o755); mkErr == nil {
		if writeErr := os.WriteFile(cachePath, data, 0o644); writeErr != nil {
			slog.Warn("writing policy cache", "path", cachePath, "error", writeErr)
		}
	}
	// Refresh mtime even if contents unchanged.
	now := time.Now()
	_ = os.Chtimes(cachePath, now, now)
	_ = prev // keep var for future diff support
	return data, true
}

// parsePolicyBytes parses YAML bytes into a Policy and returns it with its path.
func parsePolicyBytes(data []byte, path string) (*Policy, string, error) {
	p, err := parsePolicy(data)
	if err != nil {
		return nil, path, err
	}
	return p, path, nil
}
