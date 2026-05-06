package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/nassiharel/klim/internal/registry"
)

func TestFormatStars(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{42, "42"},
		{999, "999"},
		{1_000, "1.0k"},
		{1_234, "1.2k"},
		{9_999, "10.0k"},
		{12_345, "12k"},
		{234_567, "234k"},
		{1_500_000, "1.5M"},
	}
	for _, tt := range tests {
		if got := formatStars(tt.in); got != tt.want {
			t.Errorf("formatStars(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestGitHubStarsBadge(t *testing.T) {
	tests := []struct {
		name string
		tool registry.Tool
		want string
	}{
		{"no info", registry.Tool{}, ""},
		{"zero stars", registry.Tool{GitHubInfo: &registry.GitHubInfo{Stars: 0}}, ""},
		{"with stars", registry.Tool{GitHubInfo: &registry.GitHubInfo{Stars: 2500}}, "★ 2.5k"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := githubStarsBadge(tt.tool); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGitHubRepoURL(t *testing.T) {
	if got := githubRepoURL(""); got != "" {
		t.Errorf("empty slug = %q, want empty", got)
	}
	if got := githubRepoURL("  cli/cli  "); got != "https://github.com/cli/cli" {
		t.Errorf("got %q", got)
	}
}

func TestFormatGitHubDate(t *testing.T) {
	if got := formatGitHubDate(""); got != "" {
		t.Errorf("empty = %q", got)
	}
	if got := formatGitHubDate("not a date"); got != "not a date" {
		t.Errorf("invalid passthrough = %q", got)
	}
	// 10 days ago.
	ts := time.Now().Add(-10 * 24 * time.Hour).UTC().Format(time.RFC3339)
	if got := formatGitHubDate(ts); !strings.Contains(got, "days ago") {
		t.Errorf("10-day-ago = %q, want 'days ago'", got)
	}
}

func TestRenderGitHubSection(t *testing.T) {
	m := Model{width: 80}

	t.Run("no github data", func(t *testing.T) {
		got := m.renderGitHubSection(registry.Tool{Name: "foo"})
		if got != "" {
			t.Errorf("want empty, got %q", got)
		}
	})

	t.Run("slug only", func(t *testing.T) {
		got := m.renderGitHubSection(registry.Tool{Name: "foo", GitHubSlug: "cli/cli"})
		if !strings.Contains(got, "github.com/cli/cli") {
			t.Errorf("missing repo URL: %q", got)
		}
	})

	t.Run("full info", func(t *testing.T) {
		tool := registry.Tool{
			Name:       "foo",
			GitHubSlug: "cli/cli",
			GitHubInfo: &registry.GitHubInfo{
				Stars:       1500,
				Forks:       200,
				License:     "MIT",
				Homepage:    "https://cli.github.com",
				Topics:      []string{"cli", "github"},
				PushedAt:    time.Now().Add(-5 * 24 * time.Hour).UTC().Format(time.RFC3339),
				Description: "GitHub CLI",
			},
		}
		got := m.renderGitHubSection(tool)
		for _, want := range []string{
			"GitHub",
			"github.com/cli/cli",
			"1.5k stars",
			"200 forks",
			"MIT",
			"https://cli.github.com",
			"cli, github",
			"days ago",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in:\n%s", want, got)
			}
		}
	})

	t.Run("archived warning", func(t *testing.T) {
		tool := registry.Tool{
			GitHubSlug: "old/tool",
			GitHubInfo: &registry.GitHubInfo{Archived: true, Stars: 10},
		}
		got := m.renderGitHubSection(tool)
		if !strings.Contains(got, "archived") {
			t.Errorf("missing archived warning: %q", got)
		}
	})
}
