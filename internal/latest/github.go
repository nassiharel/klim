package latest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// fetchGitHub queries the GitHub Releases API for the latest release tag.
// repo is "owner/repo" (e.g. "cli/cli").
func fetchGitHub(repo string) string {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	// Use GitHub token if available (increases rate limit from 60 to 5000/hr).
	if token := githubToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := httpClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return ""
	}
	defer resp.Body.Close()

	var result struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}

	return normalizeVersion(result.TagName)
}

// fetchPyPI queries the PyPI JSON API for the latest version.
func fetchPyPI(pkg string) string {
	url := fmt.Sprintf("https://pypi.org/pypi/%s/json", pkg)

	resp, err := httpClient.Get(url)
	if err != nil || resp.StatusCode != 200 {
		return ""
	}
	defer resp.Body.Close()

	var result struct {
		Info struct {
			Version string `json:"version"`
		} `json:"info"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}

	return normalizeVersion(result.Info.Version)
}

// fetchNPM queries the npm registry for the latest version.
func fetchNPM(pkg string) string {
	url := fmt.Sprintf("https://registry.npmjs.org/%s/latest", pkg)

	resp, err := httpClient.Get(url)
	if err != nil || resp.StatusCode != 200 {
		return ""
	}
	defer resp.Body.Close()

	var result struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}

	return normalizeVersion(result.Version)
}

// normalizeVersion strips common tag prefixes and suffixes to extract a clean
// semver-like version string.
func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)

	// Strip common prefixes: "v1.2.3", "go1.23", "release-1.2.3"
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "go")
	v = strings.TrimPrefix(v, "release-")

	// Handle git-for-windows tags like "2.47.1.windows.1" → "2.47.1"
	if idx := strings.Index(v, ".windows."); idx > 0 {
		v = v[:idx]
	}

	// Strip tool-name prefixes in GitHub tags like "docker-v29.3.1",
	// "jq-1.8.1", "tig-2.6.0", "azure-dev-cli_1.23.14".
	// Heuristic: find the last '-' or '_' that's followed by a digit or 'v'+digit.
	for i := len(v) - 1; i >= 0; i-- {
		if v[i] == '-' || v[i] == '_' {
			rest := v[i+1:]
			// Direct digit: "jq-1.8.1" → "1.8.1"
			if len(rest) > 0 && rest[0] >= '0' && rest[0] <= '9' {
				v = rest
				break
			}
			// "v" + digit: "docker-v29.3.1" → "v29.3.1" → stripped below
			if len(rest) > 1 && rest[0] == 'v' && rest[1] >= '0' && rest[1] <= '9' {
				v = rest
				break
			}
		}
	}

	// Strip leading "v" again (handles "docker-v29.3.1" → "v29.3.1" → "29.3.1")
	v = strings.TrimPrefix(v, "v")

	return v
}

// githubToken returns a GitHub personal access token from the environment.
func githubToken() string {
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("GH_TOKEN")
}
