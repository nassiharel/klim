package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nassiharel/clim/internal/registry"
)

func TestValidGitHubSlug(t *testing.T) {
	cases := map[string]bool{
		"cli/cli":               true,
		"eza-community/eza":     true,
		"BurntSushi/ripgrep":    true,
		"owner/repo.with.dot":   true,
		"owner/repo_under":      true,
		"":                      false,
		"justowner":             false,
		"/missing-owner":        false,
		"owner/":                false,
		"owner//repo":           false,
		"-leading/hyphen":       false,
		"owner/repo with space": false,
	}
	for slug, want := range cases {
		if got := validGitHubSlug(slug); got != want {
			t.Errorf("validGitHubSlug(%q) = %v, want %v", slug, got, want)
		}
	}
}

func TestFetchRepo_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/cli/cli" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("missing/invalid Authorization header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"stargazers_count": 42000,
			"forks_count": 1234,
			"description": "GitHub on the command line",
			"homepage": "https://cli.github.com",
			"topics": ["cli", "github"],
			"archived": false,
			"pushed_at": "2024-01-02T03:04:05Z",
			"updated_at": "2024-01-02T03:04:05Z",
			"license": {"spdx_id": "MIT", "name": "MIT License"}
		}`))
	}))
	defer srv.Close()

	f := &githubFetcher{baseURL: srv.URL, token: "test-token", client: srv.Client()}
	info, err := f.fetchRepo(context.Background(), "cli/cli")
	if err != nil {
		t.Fatalf("fetchRepo: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil info")
	}
	if info.Stars != 42000 {
		t.Errorf("stars = %d, want 42000", info.Stars)
	}
	if info.License != "MIT" {
		t.Errorf("license = %q, want MIT", info.License)
	}
	if info.Description != "GitHub on the command line" {
		t.Errorf("unexpected description: %q", info.Description)
	}
	if len(info.Topics) != 2 {
		t.Errorf("topics = %v", info.Topics)
	}
	if info.FetchedAt == "" {
		t.Error("FetchedAt should be set")
	}
}

func TestFetchRepo_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	f := &githubFetcher{baseURL: srv.URL, client: srv.Client()}
	info, err := f.fetchRepo(context.Background(), "gone/gone")
	if err != nil {
		t.Fatalf("fetchRepo: %v", err)
	}
	if info != nil {
		t.Errorf("expected nil info for 404, got %+v", info)
	}
}

func TestFetchRepo_RateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	f := &githubFetcher{baseURL: srv.URL, client: srv.Client()}
	_, err := f.fetchRepo(context.Background(), "a/b")
	if err == nil {
		t.Fatal("expected error for rate-limited response")
	}
}

func TestEnrichWithGitHub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/ok/one":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"stargazers_count": 10}`))
		case "/repos/ok/two":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"stargazers_count": 20}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	tools := []registry.ToolDef{
		{Name: "a", GitHub: "ok/one"},
		{Name: "b", GitHub: "ok/two"},
		{Name: "c"}, // no github field — must be left alone
		{Name: "d", GitHub: "bad-slug"},
	}

	f := &githubFetcher{baseURL: srv.URL, client: srv.Client()}
	if err := enrichWithGitHub(context.Background(), tools, f, 2, false); err != nil {
		t.Fatalf("enrichWithGitHub: %v", err)
	}
	if tools[0].GitHubInfo == nil || tools[0].GitHubInfo.Stars != 10 {
		t.Errorf("tool a: unexpected info %+v", tools[0].GitHubInfo)
	}
	if tools[1].GitHubInfo == nil || tools[1].GitHubInfo.Stars != 20 {
		t.Errorf("tool b: unexpected info %+v", tools[1].GitHubInfo)
	}
	if tools[2].GitHubInfo != nil {
		t.Errorf("tool c: should have no GitHubInfo")
	}
	if tools[3].GitHubInfo != nil {
		t.Errorf("tool d: invalid slug should not populate GitHubInfo")
	}

	// Strict mode should surface the bad-slug error.
	tools2 := []registry.ToolDef{{Name: "d", GitHub: "bad-slug"}}
	if err := enrichWithGitHub(context.Background(), tools2, f, 1, true); err == nil {
		t.Error("expected strict mode to fail on bad slug")
	}
}
