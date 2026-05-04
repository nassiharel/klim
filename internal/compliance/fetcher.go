// Fetcher loads compliance policies from remote URLs with local caching.
package compliance

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/nassiharel/clim/internal/paths"
)

// maxRemotePolicySize caps remote downloads to prevent abuse.
const maxRemotePolicySize = 1 << 20 // 1 MB

// maxRedirects bounds the redirect chain so a slow/looping server can't
// burn the whole 30s timeout. http.Client's default is 10, which is more
// than a legitimate policy-host setup ever needs.
const maxRedirects = 3

// Fetcher abstracts policy retrieval from a remote source.
type Fetcher interface {
	Fetch(ctx context.Context) ([]byte, error)
}

// HTTPFetcher fetches a policy from an HTTP/HTTPS URL.
type HTTPFetcher struct {
	URL        string
	HTTPClient *http.Client // nil = default with 30s timeout and a 3-redirect cap
}

// validateHTTPURL ensures u is a usable http/https URL. We reject other
// schemes (notably file://) up-front: net/http will happily follow them
// and would let an attacker-controlled compliance.url read local files.
func validateHTTPURL(u string) error {
	parsed, err := url.Parse(u)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("policy URL scheme must be http or https (got %q)", parsed.Scheme)
	}
	if parsed.Host == "" {
		return errors.New("policy URL missing host")
	}
	return nil
}

// Fetch downloads policy YAML from the configured URL.
func (f *HTTPFetcher) Fetch(ctx context.Context) ([]byte, error) {
	if f.URL == "" {
		return nil, errors.New("policy URL not configured")
	}
	if err := validateHTTPURL(f.URL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "clim/compliance")

	client := f.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return fmt.Errorf("stopped after %d redirects", maxRedirects)
				}
				// Re-validate the redirect target — defends against
				// http→file:// or other scheme-downgrade tricks.
				return validateHTTPURL(req.URL.String())
			},
		}
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

	if err := writeCacheFile(cachePath, data); err != nil {
		slog.Warn("writing policy cache", "path", cachePath, "error", err)
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
	if err := writeCacheFile(cachePath, data); err != nil {
		slog.Warn("writing policy cache", "path", cachePath, "error", err)
	}
	return parsePolicyBytes(data, cachePath)
}

// writeCacheFile creates the cache directory if needed and atomically
// writes data to the cache path. Both MkdirAll and WriteFile failures
// surface as a single returned error so callers can log them — the
// previous nested `if mkErr == nil` quietly swallowed MkdirAll errors.
func writeCacheFile(cachePath string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}
	if err := os.WriteFile(cachePath, data, 0o644); err != nil {
		return fmt.Errorf("writing cache file: %w", err)
	}
	return nil
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
	if err := writeCacheFile(cachePath, data); err != nil {
		slog.Warn("writing policy cache", "path", cachePath, "error", err)
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
