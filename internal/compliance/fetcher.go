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
	"strings"
	"time"

	"github.com/nassiharel/klim/internal/fileutil"
	"github.com/nassiharel/klim/internal/paths"
)

// maxRemotePolicySize caps remote downloads to prevent abuse.
const maxRemotePolicySize = 1 << 20 // 1 MB

// maxRedirects bounds the redirect chain so a slow/looping server can't
// burn the whole 30s timeout. http.Client's default is 10, which is more
// than a legitimate policy-host setup ever needs.
const maxRedirects = 3

// Fetcher abstracts policy retrieval from a remote source.
//
// CacheKey returns a stable identifier for the source (typically the
// URL) so the on-disk cache can be keyed per-source. When two
// configurations point at different URLs they get different cache
// files — switching `compliance.url` no longer accidentally serves
// the previous URL's policy. Returning an empty string falls back to
// the default unkeyed path (used by tests + the cli).
type Fetcher interface {
	Fetch(ctx context.Context) ([]byte, error)
	CacheKey() string
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

// Fetch downloads policy YAML from the configured URL. Surrounding
// whitespace on f.URL is tolerated — config.Validate also flags it as
// a warning, but trimming here keeps a config-edit slip from breaking
// the runtime fetch.
func (f *HTTPFetcher) Fetch(ctx context.Context) ([]byte, error) {
	rawURL := strings.TrimSpace(f.URL)
	if rawURL == "" {
		return nil, errors.New("policy URL not configured")
	}
	if err := validateHTTPURL(rawURL); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "klim/compliance")

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

// CacheKey returns the configured URL — used to derive a per-URL
// cache file so different policy hosts don't share a single cache.
func (f *HTTPFetcher) CacheKey() string {
	return strings.TrimSpace(f.URL)
}

// CachePath returns the path to the remote-policy cache file for the
// default (unkeyed) location. Per-URL caching uses cachePathFor below.
func CachePath() (string, error) {
	return paths.ComplianceCachePath()
}

// cachePathFor returns the cache file path for a specific source key
// (typically the URL). When key is empty falls back to CachePath() for
// backwards compatibility.
func cachePathFor(key string) (string, error) {
	if strings.TrimSpace(key) == "" {
		return CachePath()
	}
	return paths.ComplianceCachePathFor(key)
}

// LoadOptions controls LoadOrFetch behavior.
type LoadOptions struct {
	// MaxAge enables cache freshness checks. When > 0 and the cache mtime is
	// older than MaxAge, the policy is refetched.
	MaxAge time.Duration
}

// LoadOrFetch loads a cached policy. If the cache is missing/stale, fetches
// from the remote and updates the cache. On fetch failure or a malformed
// fresh response, the stale cache (if any) is returned and preserved —
// we never overwrite a known-good cache with an unparseable payload.
func LoadOrFetch(ctx context.Context, fetcher Fetcher, opts LoadOptions) (*Policy, string, error) {
	cachePath, err := cachePathFor(fetcher.CacheKey())
	if err != nil {
		return nil, "", fmt.Errorf("resolving cache path: %w", err)
	}

	// Try existing cache.
	if data, err := os.ReadFile(cachePath); err == nil && len(data) > 0 {
		if opts.MaxAge > 0 && cacheIsStale(cachePath, opts.MaxAge) {
			if refreshed, ok := tryRefresh(ctx, fetcher, cachePath); ok {
				slog.Info("compliance policy auto-refreshed", "path", cachePath, "bytes", len(refreshed))
				return parsePolicyBytes(refreshed, cachePath)
			}
			slog.Warn("compliance policy refresh failed, serving stale cache", "path", cachePath)
		}
		return parsePolicyBytes(data, cachePath)
	}

	// No cache — fetch and validate before writing.
	slog.Info("fetching compliance policy from remote")
	data, err := fetcher.Fetch(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("fetching policy (no local cache): %w", err)
	}
	policy, err := parsePolicy(data)
	if err != nil {
		// Don't poison the cache with HTML / login pages / malformed YAML.
		return nil, cachePath, fmt.Errorf("parsing fetched policy: %w", err)
	}

	if err := writeCacheFile(cachePath, data); err != nil {
		slog.Warn("writing policy cache", "path", cachePath, "error", err)
	}
	return policy, cachePath, nil
}

// Refresh forces a fetch and updates the cache. Returns the new policy.
// If the freshly fetched bytes don't parse as a Policy, the previous
// cache is preserved untouched and the parse error is returned —
// matches LoadOrFetch's "never poison the cache" semantics so a flaky
// proxy / login page can't silently break later runs.
func Refresh(ctx context.Context, fetcher Fetcher) (*Policy, string, error) {
	cachePath, err := cachePathFor(fetcher.CacheKey())
	if err != nil {
		return nil, "", err
	}
	data, err := fetcher.Fetch(ctx)
	if err != nil {
		return nil, "", err
	}
	policy, err := parsePolicy(data)
	if err != nil {
		return nil, cachePath, fmt.Errorf("parsing fetched policy: %w", err)
	}
	if err := writeCacheFile(cachePath, data); err != nil {
		slog.Warn("writing policy cache", "path", cachePath, "error", err)
	}
	return policy, cachePath, nil
}

// writeCacheFile creates the cache directory if needed and writes data
// to cachePath atomically (temp file + rename), so other processes
// cannot observe a half-written or empty cache. Both MkdirAll and
// AtomicWrite failures surface so callers can log them.
func writeCacheFile(cachePath string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return fmt.Errorf("creating cache directory: %w", err)
	}
	if err := fileutil.AtomicWrite(cachePath, data, 0o644); err != nil {
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

// tryRefresh fetches a fresh policy and writes it to the cache only if
// it parses successfully. Returns the bytes (and ok=true) on success;
// on fetch error or parse failure it returns ok=false so the caller
// can keep serving the previous cache.
func tryRefresh(ctx context.Context, fetcher Fetcher, cachePath string) ([]byte, bool) {
	data, err := fetcher.Fetch(ctx)
	if err != nil {
		return nil, false
	}
	if len(data) == 0 {
		return nil, false
	}
	if _, err := parsePolicy(data); err != nil {
		slog.Warn("auto-refresh: fetched policy did not parse, keeping previous cache",
			"path", cachePath, "error", err)
		return nil, false
	}
	if err := writeCacheFile(cachePath, data); err != nil {
		// Don't bump mtime on write failure — otherwise the next
		// LoadOrFetch would consider the (still stale) on-disk cache
		// "fresh" and skip retrying for a full refresh interval.
		// Returning ok=false makes LoadOrFetch keep serving the
		// previous bytes it already has and try again next time.
		slog.Warn("auto-refresh: cache write failed, will retry next interval",
			"path", cachePath, "error", err)
		return nil, false
	}
	// Refresh mtime even if contents unchanged.
	now := time.Now()
	_ = os.Chtimes(cachePath, now, now)
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
