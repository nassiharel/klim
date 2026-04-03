package version

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

type githubRelease struct {
	TagName string `json:"tag_name"`
}

func (c *HTTPChecker) latestGitHub(ctx context.Context, repo string) LatestVersion {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	if c.baseURL != "" {
		url = fmt.Sprintf("%s/repos/%s/releases/latest", c.baseURL, repo)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return LatestVersion{Error: fmt.Errorf("create request: %w", err)}
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.githubToken)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return LatestVersion{Error: fmt.Errorf("github request: %w", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden {
		return LatestVersion{Error: fmt.Errorf("github rate limited — set GITHUB_TOKEN env var")}
	}
	if resp.StatusCode != http.StatusOK {
		return LatestVersion{Error: fmt.Errorf("github %s: status %d", repo, resp.StatusCode)}
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return LatestVersion{Error: fmt.Errorf("github decode: %w", err)}
	}

	version := strings.TrimPrefix(release.TagName, "v")
	return LatestVersion{Version: extractSemver(version)}
}

// semverRe matches a semver-like version string (X.Y.Z) anywhere in text.
var semverRe = regexp.MustCompile(`(\d+\.\d+\.\d+)`)

// extractSemver pulls out the first semver-like substring from s.
// Falls back to the original string if no match.
func extractSemver(s string) string {
	match := semverRe.FindString(s)
	if match != "" {
		return match
	}
	return s
}
