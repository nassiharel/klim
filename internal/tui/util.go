package tui

import (
	"runtime"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/clim/internal/registry"
)

// combineTagsAndTopics merges catalog tags and GitHub topics, de-duplicating
// case-insensitively while preserving the first-seen original casing. Tags
// come first (curated), then topics (crowd-sourced).
func combineTagsAndTopics(tool registry.Tool) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(s string) {
		key := strings.ToLower(strings.TrimSpace(s))
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		out = append(out, s)
	}
	for _, t := range tool.Tags {
		add(t)
	}
	if tool.GitHubInfo != nil {
		for _, t := range tool.GitHubInfo.Topics {
			add(t)
		}
	}
	return out
}

// currentOSLabel returns the "Windows" / "macOS" / "Linux" label matching the
// runtime platform, matching the strings produced by derivePlatforms.
func currentOSLabel() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	}
	return ""
}

// packageEntry is one package-manager → package-id pairing for the detail view.
type packageEntry struct {
	source string
	id     string
}

// collectPackageEntries returns the declared package IDs for each package
// manager in a stable display order. Empty IDs are omitted.
func collectPackageEntries(pkgs registry.PackageIDs) []packageEntry {
	all := []packageEntry{
		{source: string(registry.SourceWinget), id: pkgs.Winget},
		{source: string(registry.SourceChoco), id: pkgs.Choco},
		{source: string(registry.SourceScoop), id: pkgs.Scoop},
		{source: string(registry.SourceBrew), id: pkgs.Brew},
		{source: string(registry.SourceApt), id: pkgs.Apt},
		{source: string(registry.SourceSnap), id: pkgs.Snap},
		{source: string(registry.SourceNPM), id: pkgs.NPM},
	}
	entries := make([]packageEntry, 0, len(all))
	for _, e := range all {
		if e.id != "" {
			entries = append(entries, e)
		}
	}
	return entries
}

// derivePlatforms infers supported operating systems from which package
// manager IDs are defined. Returns human-readable labels like "Windows",
// "macOS", "Linux".
func derivePlatforms(pkgs registry.PackageIDs) []string {
	var platforms []string
	seen := make(map[string]bool)

	add := func(label string) {
		if !seen[label] {
			seen[label] = true
			platforms = append(platforms, label)
		}
	}

	if pkgs.Winget != "" || pkgs.Choco != "" || pkgs.Scoop != "" {
		add("Windows")
	}
	if pkgs.Brew != "" {
		add("macOS")
		add("Linux")
	}
	if pkgs.Apt != "" || pkgs.Snap != "" {
		add("Linux")
	}
	if pkgs.NPM != "" {
		add("Windows")
		add("macOS")
		add("Linux")
	}
	return platforms
}

// wordWrap breaks text into lines that fit within maxWidth display columns.
// Uses lipgloss.Width for correct handling of multi-byte UTF-8 and wide characters.
func wordWrap(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	var lines []string
	current := words[0]
	for _, word := range words[1:] {
		if lipgloss.Width(current)+1+lipgloss.Width(word) > maxWidth {
			lines = append(lines, current)
			current = word
		} else {
			current += " " + word
		}
	}
	lines = append(lines, current)
	return lines
}
