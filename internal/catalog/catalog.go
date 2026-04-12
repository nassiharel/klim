// Package catalog manages fetching, caching, and diffing the tool marketplace
// catalog from GitHub. The marketplace.yaml is treated as an external service
// rather than an embedded resource — it's fetched on first run and cached
// locally for offline use.
package catalog

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// --- Fetcher ---

// MarketplaceFetcher abstracts fetching the catalog YAML from a remote source.
type MarketplaceFetcher interface {
	Fetch(ctx context.Context) ([]byte, error)
}

// GitHubFetcher fetches marketplace.yaml from GitHub raw content.
type GitHubFetcher struct {
	HTTPClient *http.Client // defaults to http.DefaultClient
	Owner      string       // defaults to "nassiharel"
	Repo       string       // defaults to "clim"
	Branch     string       // defaults to "main"
	BaseURL    string       // defaults to "https://raw.githubusercontent.com"
}

// maxCatalogSize caps the downloaded catalog to prevent memory exhaustion.
const maxCatalogSize = 2 << 20 // 2 MB — marketplace.yaml is ~20 KB

// Fetch downloads the marketplace.yaml from GitHub raw content.
func (f *GitHubFetcher) Fetch(ctx context.Context) ([]byte, error) {
	url := fmt.Sprintf("%s/%s/%s/%s/marketplace.yaml",
		f.baseURL(), f.owner(), f.repo(), f.branch())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := f.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching marketplace: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github returned %s", resp.Status)
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
	return http.DefaultClient
}

func (f *GitHubFetcher) owner() string {
	if f.Owner != "" {
		return f.Owner
	}
	return "nassiharel"
}

func (f *GitHubFetcher) repo() string {
	if f.Repo != "" {
		return f.Repo
	}
	return "clim"
}

func (f *GitHubFetcher) branch() string {
	if f.Branch != "" {
		return f.Branch
	}
	return "main"
}

func (f *GitHubFetcher) baseURL() string {
	if f.BaseURL != "" {
		return f.BaseURL
	}
	return "https://raw.githubusercontent.com"
}

// --- Cache ---

// CachePath returns the path to the locally cached marketplace.yaml.
func CachePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "clim", "marketplace.yaml"), nil
}

// LoadOrFetch loads the cached marketplace YAML. If the cache doesn't exist,
// it fetches from GitHub and writes the cache. Returns the raw YAML bytes.
func LoadOrFetch(ctx context.Context, fetcher MarketplaceFetcher) ([]byte, error) {
	path, err := CachePath()
	if err != nil {
		return nil, fmt.Errorf("resolving cache path: %w", err)
	}

	// Try reading the local cache first.
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		return data, nil
	}

	// No cache — fetch from remote.
	data, err := fetcher.Fetch(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching marketplace (no local cache): %w", err)
	}

	// Write cache.
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr == nil {
		_ = os.WriteFile(path, data, 0o644)
	}

	return data, nil
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
	Enabled     bool       `yaml:"enabled"`
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
// local cache, updates the cache if changed, and returns the result.
func Refresh(ctx context.Context, fetcher MarketplaceFetcher) (*RefreshResult, error) {
	path, err := CachePath()
	if err != nil {
		return nil, fmt.Errorf("resolving cache path: %w", err)
	}

	// Read current local cache.
	local, _ := os.ReadFile(path) // may be empty/missing — that's fine

	// Fetch latest from remote.
	remote, err := fetcher.Fetch(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetching latest marketplace: %w", err)
	}

	// Diff.
	diff := Diff(local, remote)

	// Update local cache with the remote version.
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr == nil {
		_ = os.WriteFile(path, remote, 0o644)
	}

	return &RefreshResult{
		Diff:    diff,
		Updated: diff.HasChanges(),
	}, nil
}
