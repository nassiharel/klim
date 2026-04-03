package version

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// latestCustom handles non-standard version APIs (Go, Node.js, Python).
func (c *HTTPChecker) latestCustom(ctx context.Context, urlPattern string) LatestVersion {
	switch {
	case strings.Contains(urlPattern, "go.dev"):
		return c.latestGo(ctx)
	case strings.Contains(urlPattern, "nodejs.org"):
		return c.latestNode(ctx)
	case strings.Contains(urlPattern, "endoflife.date"):
		return c.latestPython(ctx)
	default:
		return LatestVersion{Error: fmt.Errorf("unknown custom URL pattern: %s", urlPattern)}
	}
}

// latestGo fetches the latest stable Go version from go.dev.
func (c *HTTPChecker) latestGo(ctx context.Context) LatestVersion {
	url := "https://go.dev/dl/?mode=json"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return LatestVersion{Error: err}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return LatestVersion{Error: fmt.Errorf("go.dev request: %w", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return LatestVersion{Error: fmt.Errorf("go.dev: status %d", resp.StatusCode)}
	}

	var releases []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return LatestVersion{Error: fmt.Errorf("go.dev decode: %w", err)}
	}

	for _, r := range releases {
		if r.Stable {
			version := strings.TrimPrefix(r.Version, "go")
			return LatestVersion{Version: version}
		}
	}

	return LatestVersion{Error: fmt.Errorf("go.dev: no stable release found")}
}

// latestNode fetches the latest LTS Node.js version from nodejs.org.
func (c *HTTPChecker) latestNode(ctx context.Context) LatestVersion {
	url := "https://nodejs.org/dist/index.json"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return LatestVersion{Error: err}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return LatestVersion{Error: fmt.Errorf("nodejs.org request: %w", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return LatestVersion{Error: fmt.Errorf("nodejs.org: status %d", resp.StatusCode)}
	}

	var releases []struct {
		Version string `json:"version"`
		LTS     any    `json:"lts"` // Can be false (bool) or a string like "Jod"
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return LatestVersion{Error: fmt.Errorf("nodejs.org decode: %w", err)}
	}

	// Return the first entry — it's the latest release (sorted newest first).
	if len(releases) > 0 {
		version := strings.TrimPrefix(releases[0].Version, "v")
		return LatestVersion{Version: version}
	}

	return LatestVersion{Error: fmt.Errorf("nodejs.org: no releases found")}
}

// latestPython fetches the latest Python version from endoflife.date.
func (c *HTTPChecker) latestPython(ctx context.Context) LatestVersion {
	url := "https://endoflife.date/api/python.json"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return LatestVersion{Error: err}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return LatestVersion{Error: fmt.Errorf("endoflife.date request: %w", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return LatestVersion{Error: fmt.Errorf("endoflife.date: status %d", resp.StatusCode)}
	}

	var releases []struct {
		Latest string `json:"latest"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return LatestVersion{Error: fmt.Errorf("endoflife.date decode: %w", err)}
	}

	if len(releases) > 0 {
		return LatestVersion{Version: releases[0].Latest}
	}

	return LatestVersion{Error: fmt.Errorf("endoflife.date: no Python releases found")}
}
