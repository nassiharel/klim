package tui

import (
	"github.com/nassiharel/klim/internal/githubfmt"
	"github.com/nassiharel/klim/internal/registry"
)

// formatStars, formatGitHubDate and githubRepoURL are thin aliases for
// the canonical implementations in internal/githubfmt. They exist so
// the existing TUI call sites and tests don't churn — and so the CLI's
// `klim tool info` shares the same formatting contract. See
// internal/githubfmt/githubfmt_test.go for the behavioral tests.

func formatStars(n int) string          { return githubfmt.FormatStars(n) }
func formatGitHubDate(ts string) string { return githubfmt.FormatDate(ts) }
func githubRepoURL(slug string) string  { return githubfmt.RepoURL(slug) }

// githubStarsBadge returns a compact "★ 12k" badge for the given tool, or ""
// if the tool has no GitHub metadata or zero stars. The output includes no
// trailing whitespace so callers can place it freely.
func githubStarsBadge(tool registry.Tool) string {
	if tool.GitHubInfo == nil || tool.GitHubInfo.Stars <= 0 {
		return ""
	}
	return "★ " + formatStars(tool.GitHubInfo.Stars)
}
