package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nassiharel/clim/internal/registry"
)

// formatStars formats a star count compactly (e.g. 12.3k, 1.2M) for row display.
func formatStars(n int) string {
	switch {
	case n < 1000:
		return strconv.Itoa(n)
	case n < 10_000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	case n < 1_000_000:
		return fmt.Sprintf("%dk", n/1000)
	default:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
}

// githubStarsBadge returns a compact "★ 12.3k" badge for the given tool, or ""
// if the tool has no GitHub metadata or zero stars. The output includes no
// trailing whitespace so callers can place it freely.
func githubStarsBadge(tool registry.Tool) string {
	if tool.GitHubInfo == nil || tool.GitHubInfo.Stars <= 0 {
		return ""
	}
	return "★ " + formatStars(tool.GitHubInfo.Stars)
}

// formatGitHubDate converts a GitHub ISO-8601 timestamp into a short, human
// readable "N days ago" / "N months ago" / "on 2024-03-14" string.
// Returns the raw input if it cannot be parsed.
func formatGitHubDate(ts string) string {
	if ts == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		// Tolerate legacy "2006-01-02T15:04:05Z" without timezone name.
		if t2, err2 := time.Parse("2006-01-02T15:04:05Z", ts); err2 == nil {
			t = t2
		} else {
			return ts
		}
	}
	delta := time.Since(t)
	switch {
	case delta < 0:
		return t.Format("2006-01-02")
	case delta < 48*time.Hour:
		h := int(delta.Hours())
		if h <= 1 {
			return "just now"
		}
		return fmt.Sprintf("%d hours ago", h)
	case delta < 60*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(delta.Hours()/24))
	case delta < 365*24*time.Hour:
		return fmt.Sprintf("%d months ago", int(delta.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%.1f years ago", delta.Hours()/(24*365))
	}
}

// githubRepoURL returns the canonical https URL for a "owner/repo" slug, or
// "" if the slug is empty.
func githubRepoURL(slug string) string {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return ""
	}
	return "https://github.com/" + slug
}
