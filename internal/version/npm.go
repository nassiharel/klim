package version

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type npmPackage struct {
	Version string `json:"version"`
}

func (c *HTTPChecker) latestNPM(ctx context.Context, packageName string) LatestVersion {
	// Scoped packages like @githubnext/github-copilot-cli need URL encoding.
	encoded := url.PathEscape(packageName)
	apiURL := fmt.Sprintf("https://registry.npmjs.org/%s/latest", encoded)
	if c.baseURL != "" {
		apiURL = fmt.Sprintf("%s/%s/latest", c.baseURL, encoded)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return LatestVersion{Error: fmt.Errorf("create request: %w", err)}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return LatestVersion{Error: fmt.Errorf("npm request: %w", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return LatestVersion{Error: fmt.Errorf("npm %s: status %d", packageName, resp.StatusCode)}
	}

	var pkg npmPackage
	if err := json.NewDecoder(resp.Body).Decode(&pkg); err != nil {
		return LatestVersion{Error: fmt.Errorf("npm decode: %w", err)}
	}

	return LatestVersion{Version: pkg.Version}
}
