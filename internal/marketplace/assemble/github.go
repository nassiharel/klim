package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/nassiharel/clim/internal/registry"
)

// ghSlugPattern validates the "owner/repo" form used by the source `github`
// field. GitHub logins allow ASCII letters, digits and hyphens; repo names
// additionally allow ._-.
var ghSlugPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+$`)

// validGitHubSlug reports whether s is a well-formed "owner/repo" slug.
func validGitHubSlug(s string) bool {
	return ghSlugPattern.MatchString(s)
}

// githubRepo mirrors the subset of fields from the GitHub REST API response
// at https://api.github.com/repos/{owner}/{repo} that we care about.
type githubRepo struct {
	StargazersCount int      `json:"stargazers_count"`
	ForksCount      int      `json:"forks_count"`
	Description     string   `json:"description"`
	Homepage        string   `json:"homepage"`
	Topics          []string `json:"topics"`
	Archived        bool     `json:"archived"`
	PushedAt        string   `json:"pushed_at"`
	UpdatedAt       string   `json:"updated_at"`
	License         *struct {
		SPDXID string `json:"spdx_id"`
		Name   string `json:"name"`
	} `json:"license"`
}

// githubFetcher fetches repository metadata from the GitHub REST API.
type githubFetcher struct {
	baseURL string
	token   string
	client  *http.Client
}

func newGitHubFetcher() *githubFetcher {
	base := strings.TrimRight(os.Getenv("GITHUB_API_URL"), "/")
	if base == "" {
		base = "https://api.github.com"
	}
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	return &githubFetcher{
		baseURL: base,
		token:   token,
		client:  &http.Client{Timeout: 15 * time.Second},
	}
}

// fetchRepo retrieves repository info for an "owner/repo" slug. It returns
// nil info (and nil error) on 404 so callers can treat missing/renamed repos
// as non-fatal.
func (f *githubFetcher) fetchRepo(ctx context.Context, slug string) (*registry.GitHubInfo, error) {
	url := fmt.Sprintf("%s/repos/%s", f.baseURL, slug)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "clim-marketplace-assemble")
	if f.token != "" {
		req.Header.Set("Authorization", "Bearer "+f.token)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling GitHub API: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// fall through
	case http.StatusNotFound:
		return nil, nil
	case http.StatusForbidden, http.StatusTooManyRequests:
		// Rate limit. Surface the reset hint when available so CI logs are
		// actionable.
		reset := resp.Header.Get("X-RateLimit-Reset")
		remaining := resp.Header.Get("X-RateLimit-Remaining")
		return nil, fmt.Errorf("rate limited (remaining=%s, reset=%s): HTTP %d", remaining, reset, resp.StatusCode)
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var r githubRepo
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	info := &registry.GitHubInfo{
		Stars:       r.StargazersCount,
		Forks:       r.ForksCount,
		Description: r.Description,
		Homepage:    r.Homepage,
		Topics:      r.Topics,
		Archived:    r.Archived,
		PushedAt:    r.PushedAt,
		UpdatedAt:   r.UpdatedAt,
		FetchedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if r.License != nil {
		if r.License.SPDXID != "" && r.License.SPDXID != "NOASSERTION" {
			info.License = r.License.SPDXID
		} else {
			info.License = r.License.Name
		}
	}
	return info, nil
}

// enrichWithGitHub populates the GitHubInfo field of each tool that declares
// a `github:` slug. Fetches are performed concurrently with a bounded worker
// pool. Per-tool failures are logged and do not abort assembly unless
// strict is true.
func enrichWithGitHub(ctx context.Context, tools []registry.ToolDef, fetcher *githubFetcher, concurrency int, strict bool) error {
	if concurrency <= 0 {
		concurrency = 4
	}

	type job struct {
		idx  int
		slug string
	}
	jobs := make(chan job)
	var wg sync.WaitGroup

	var mu sync.Mutex
	var firstErr error

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				info, err := fetcher.fetchRepo(ctx, j.slug)
				if err != nil {
					fmt.Fprintf(os.Stderr, "warning: github fetch failed for %s (%s): %v\n", tools[j.idx].Name, j.slug, err)
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
					continue
				}
				if info == nil {
					fmt.Fprintf(os.Stderr, "warning: github repo %s for tool %s not found\n", j.slug, tools[j.idx].Name)
					continue
				}
				tools[j.idx].GitHubInfo = info
			}
		}()
	}

	for i, t := range tools {
		slug := strings.TrimSpace(t.GitHub)
		if slug == "" {
			continue
		}
		if !validGitHubSlug(slug) {
			err := fmt.Errorf("tool %q: invalid github slug %q (want owner/repo)", t.Name, slug)
			fmt.Fprintf(os.Stderr, "warning: %v\n", err)
			mu.Lock()
			if firstErr == nil {
				firstErr = err
			}
			mu.Unlock()
			continue
		}
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return ctx.Err()
		case jobs <- job{idx: i, slug: slug}:
		}
	}
	close(jobs)
	wg.Wait()

	if strict && firstErr != nil {
		return firstErr
	}
	return nil
}
