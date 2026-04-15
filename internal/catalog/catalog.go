// Package catalog manages fetching, caching, and diffing the tool marketplace
// catalog from GitHub. The marketplace.yaml is treated as an external service
// rather than an embedded resource — it's fetched on first run and cached
// locally for offline use.
package catalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/config"
)

// --- Fetcher ---

// MarketplaceFetcher abstracts fetching the catalog YAML from a remote source.
type MarketplaceFetcher interface {
	Fetch(ctx context.Context) ([]byte, error)
}

// GitHubFetcher fetches marketplace.yaml from a configured URL,
// defaulting to GitHub raw content if no URL is set.
type GitHubFetcher struct {
	HTTPClient *http.Client // defaults to http.DefaultClient
	URL        string       // marketplace URL; empty = config.DefaultMarketplaceURL
}

// maxCatalogSize caps the downloaded catalog to prevent memory exhaustion.
const maxCatalogSize = 2 << 20 // 2 MB — marketplace.yaml is ~20 KB

// Fetch downloads the marketplace.yaml from the configured URL.
func (f *GitHubFetcher) Fetch(ctx context.Context) ([]byte, error) {
	url := f.URL
	if url == "" {
		url = config.DefaultMarketplaceURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("User-Agent", "clim/catalog")

	resp, err := f.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching marketplace: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %s", resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxCatalogSize+1))
	if err != nil {
		return nil, fmt.Errorf("reading marketplace: %w", err)
	}
	if int64(len(data)) > maxCatalogSize {
		return nil, fmt.Errorf("marketplace too large (max %d bytes)", maxCatalogSize)
	}

	return data, nil
}

func (f *GitHubFetcher) httpClient() *http.Client {
	if f.HTTPClient != nil {
		return f.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// --- Cache ---

// CachePath returns the path to the remote marketplace cache.
// This is separate from the user's marketplace.yaml (which holds
// customizations like user-added tools and package ID overrides).
func CachePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "clim", "marketplace-cache.yaml"), nil
}

// LoadOrFetch loads the cached marketplace YAML. If the cache doesn't exist
// or contains invalid YAML, it fetches from GitHub and rewrites the cache.
// Returns the raw YAML bytes.
func LoadOrFetch(ctx context.Context, fetcher MarketplaceFetcher) ([]byte, error) {
	path, err := CachePath()
	if err != nil {
		return nil, fmt.Errorf("resolving cache path: %w", err)
	}

	// Try reading and validating the local cache.
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		if isValidCatalog(data) {
			slog.Debug("catalog cache hit", "path", path, "bytes", len(data))
			return data, nil
		}
		slog.Warn("catalog cache invalid, refetching", "path", path)
	}

	// No valid cache — fetch from remote.
	slog.Debug("catalog cache miss, fetching from remote")
	data, err := fetcher.Fetch(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching marketplace (no valid local cache): %w", err)
	}

	// Validate before caching — don't poison the cache with HTML/garbage.
	if !isValidCatalog(data) {
		slog.Warn("fetched catalog is invalid", "bytes", len(data))
		return nil, errors.New("fetched catalog is invalid (not parseable YAML with tools)")
	}

	slog.Debug("catalog fetched and cached", "path", path, "bytes", len(data))

	// Write cache atomically: write to temp file, then rename.
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr == nil {
		_ = atomicWriteFile(path, data, 0o644)
	}

	return data, nil
}

// isValidCatalog checks whether data is parseable YAML with at least one tool.
func isValidCatalog(data []byte) bool {
	defs := parseToolDefs(data)
	return len(defs) > 0
}

// --- Diff ---

// DiffResult describes what changed between two catalog versions.
type DiffResult struct {
	NewTools     []string            // tool names present in remote but absent locally
	ChangedTools map[string][]string // tool name → list of changed field descriptions
}

// HasChanges reports whether any tools were added or modified.
func (d DiffResult) HasChanges() bool {
	return len(d.NewTools) > 0 || len(d.ChangedTools) > 0
}

// toolDef mirrors the YAML structure for diffing purposes.
type toolDef struct {
	Name        string     `yaml:"name"`
	DisplayName string     `yaml:"display_name"`
	Category    string     `yaml:"category"`
	Tags        []string   `yaml:"tags,omitempty"`
	BinaryNames []string   `yaml:"binary_names"`
	Packages    packageDef `yaml:"packages"`
}

type packageDef struct {
	Winget string `yaml:"winget,omitempty"`
	Choco  string `yaml:"choco,omitempty"`
	Brew   string `yaml:"brew,omitempty"`
	Apt    string `yaml:"apt,omitempty"`
	Snap   string `yaml:"snap,omitempty"`
	NPM    string `yaml:"npm,omitempty"`
}

type toolsFile struct {
	Tools []toolDef `yaml:"tools"`
}

func parseToolDefs(data []byte) []toolDef {
	var f toolsFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil
	}
	return f.Tools
}

// Diff compares local and remote catalog YAML and returns the differences.
// Only detects additions and modifications — removals are ignored (user-added
// custom tools in the local file are preserved).
func Diff(local, remote []byte) DiffResult {
	localDefs := parseToolDefs(local)
	remoteDefs := parseToolDefs(remote)

	localMap := make(map[string]*toolDef, len(localDefs))
	for i := range localDefs {
		localMap[localDefs[i].Name] = &localDefs[i]
	}

	result := DiffResult{
		ChangedTools: make(map[string][]string),
	}

	for _, rd := range remoteDefs {
		ld, exists := localMap[rd.Name]
		if !exists {
			result.NewTools = append(result.NewTools, rd.Name)
			continue
		}

		// Compare fields.
		var changes []string
		if ld.DisplayName != rd.DisplayName {
			changes = append(changes, "display_name")
		}
		if ld.Category != rd.Category {
			changes = append(changes, "category")
		}
		if !slicesEqual(ld.BinaryNames, rd.BinaryNames) {
			changes = append(changes, "binary_names")
		}
		if !slicesEqual(ld.Tags, rd.Tags) {
			changes = append(changes, "tags")
		}
		if ld.Packages != rd.Packages {
			changes = append(changes, "packages")
		}

		if len(changes) > 0 {
			result.ChangedTools[rd.Name] = changes
		}
	}

	sort.Strings(result.NewTools)
	return result
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- Refresh ---

// RefreshResult is returned after a marketplace refresh.
type RefreshResult struct {
	Diff    DiffResult
	Updated bool // true if the local cache was updated
}

// Refresh fetches the latest catalog from the remote, diffs it against the
// user's marketplace.yaml (not the cache), updates the cache, and returns
// the result. The diff reflects what the user will see as new or changed
// relative to their current tool list.
func Refresh(ctx context.Context, fetcher MarketplaceFetcher) (*RefreshResult, error) {
	cachePath, err := CachePath()
	if err != nil {
		return nil, fmt.Errorf("resolving cache path: %w", err)
	}

	// Read the user's current marketplace file for diffing.
	// This is what they actually see — not the remote cache.
	userPath, _ := userMarketplacePath()
	local, _ := os.ReadFile(userPath) // may be empty/missing — that's fine

	// Fetch latest from remote.
	remote, err := fetcher.Fetch(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching latest marketplace: %w", err)
	}

	// Validate before caching — don't poison the cache with HTML/garbage.
	if !isValidCatalog(remote) {
		return nil, errors.New("fetched catalog is invalid (not parseable YAML with tools)")
	}

	// Diff against what the user currently has.
	diff := Diff(local, remote)

	// Update the remote cache (not the user file — the merge in registry
	// will incorporate new tools on next load).
	if mkErr := os.MkdirAll(filepath.Dir(cachePath), 0o755); mkErr == nil {
		_ = atomicWriteFile(cachePath, remote, 0o644)
	}

	return &RefreshResult{
		Diff:    diff,
		Updated: diff.HasChanges(),
	}, nil
}

// userMarketplacePath returns the path to the user's marketplace.yaml.
func userMarketplacePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "clim", "marketplace.yaml"), nil
}

// atomicWriteFile writes data to a temp file in the same directory, then
// renames it to the target path. This prevents partial/corrupt files if the
// process is interrupted mid-write.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
