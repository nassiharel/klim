package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const (
	defaultOwner   = "nassiharel"
	defaultRepo    = "clim"
	defaultBaseURL = "https://api.github.com"
)

// Release represents the subset of a GitHub release we care about.
type Release struct {
	TagName string  `json:"tag_name"` // e.g. "v2.1.92"
	Assets  []Asset `json:"assets"`
}

// Asset represents a single downloadable file attached to a release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// Version returns the tag with any leading "v" stripped.
func (r Release) Version() string {
	return strings.TrimPrefix(r.TagName, "v")
}

// GitHubClient handles communication with the GitHub Releases API.
// Fields are exported to allow test injection.
type GitHubClient struct {
	HTTPClient *http.Client // nil = default client with 60s timeout (via Options.httpClient)
	Owner      string       // defaults to "nassiharel"
	Repo       string       // defaults to "clim"
	BaseURL    string       // defaults to "https://api.github.com"
}

// FetchLatestRelease calls GET /repos/{owner}/{repo}/releases/latest.
func (g *GitHubClient) FetchLatestRelease(ctx context.Context) (*Release, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest",
		g.baseURL(), g.owner(), g.repo())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "clim/selfupdate")

	resp, err := g.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching latest release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github API returned %s", resp.Status)
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decoding release JSON: %w", err)
	}
	return &rel, nil
}

// AssetURL finds the download URL for the archive matching the given OS/arch.
// Archive naming follows GoReleaser's template:
//
//	clim_{version}_{os}_{arch}.tar.gz  (or .zip for windows)
func AssetURL(rel *Release, goos, goarch string) (string, error) {
	ext := ".tar.gz"
	if goos == "windows" {
		ext = ".zip"
	}

	suffix := fmt.Sprintf("_%s_%s%s", goos, goarch, ext)

	for _, a := range rel.Assets {
		if strings.HasSuffix(a.Name, suffix) {
			return a.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("no release asset found for %s/%s in release %s",
		goos, goarch, rel.TagName)
}

func (g *GitHubClient) httpClient() *http.Client {
	if g.HTTPClient != nil {
		return g.HTTPClient
	}
	return http.DefaultClient
}

func (g *GitHubClient) owner() string {
	if g.Owner != "" {
		return g.Owner
	}
	return defaultOwner
}

func (g *GitHubClient) repo() string {
	if g.Repo != "" {
		return g.Repo
	}
	return defaultRepo
}

func (g *GitHubClient) baseURL() string {
	if g.BaseURL != "" {
		return g.BaseURL
	}
	return defaultBaseURL
}
