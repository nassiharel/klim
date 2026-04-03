package version

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type pypiResponse struct {
	Info struct {
		Version string `json:"version"`
	} `json:"info"`
}

func (c *HTTPChecker) latestPyPI(ctx context.Context, packageName string) LatestVersion {
	url := fmt.Sprintf("https://pypi.org/pypi/%s/json", packageName)
	if c.baseURL != "" {
		url = fmt.Sprintf("%s/pypi/%s/json", c.baseURL, packageName)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return LatestVersion{Error: fmt.Errorf("create request: %w", err)}
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return LatestVersion{Error: fmt.Errorf("pypi request: %w", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return LatestVersion{Error: fmt.Errorf("pypi %s: status %d", packageName, resp.StatusCode)}
	}

	var result pypiResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return LatestVersion{Error: fmt.Errorf("pypi decode: %w", err)}
	}

	return LatestVersion{Version: result.Info.Version}
}
