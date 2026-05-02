package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/search"
)

// applyFilter recomputes m.filteredIndex based on the active tab, sidebar
// selections (category/tag/platform/status), and free-text search filter.
// It also rebuilds the sidebar items for the active tab.
func (m *Model) applyFilter() {
	m.filteredIndex = nil
	filter := strings.ToLower(m.filterText)

	// First pass: collect tools matching the current tab (for contextual sidebar).
	var tabTools []registry.Tool
	for _, tool := range m.tools {
		if m.matchesTab(tool) {
			tabTools = append(tabTools, tool)
		}
	}

	// Rebuild sidebar items from tab-scoped tools only.
	m.categories = collectCategories(tabTools)
	m.tags = collectTags(tabTools)
	m.platforms = collectPlatforms(tabTools)
	m.sidebarItems = buildSidebarItems(m.categories, m.tags, m.platforms, tabTools)

	// Prepend STATUS filter section on the Marketplace tab.
	if m.activeTab == tabDiscover {
		installedCount := 0
		for _, t := range tabTools {
			if t.IsInstalled() {
				installedCount++
			}
		}
		availableCount := len(tabTools) - installedCount
		statusItems := []sidebarItem{
			{label: "STATUS", isHeader: true},
			{label: fmt.Sprintf("All (%d)", len(tabTools)), section: "status", value: ""},
			{label: fmt.Sprintf("Installed (%d)", installedCount), section: "status", value: "installed"},
			{label: fmt.Sprintf("Available (%d)", availableCount), section: "status", value: "available"},
		}
		m.sidebarItems = append(statusItems, m.sidebarItems...)
	}

	// Pre-compute search scores for all tools when filter is active.
	var searchScores map[int]int
	if filter != "" {
		// Build pointer→index map once (O(n)), then map results (O(m)).
		ptrToIdx := make(map[*registry.Tool]int, len(m.tools))
		for i := range m.tools {
			ptrToIdx[&m.tools[i]] = i
		}
		searchScores = make(map[int]int)
		results := search.Search(m.tools, filter)
		for _, r := range results {
			if idx, ok := ptrToIdx[r.Tool]; ok {
				searchScores[idx] = r.Score
			}
		}
	}

	// Second pass: apply all filters (sidebar + text search).
	for i, tool := range m.tools {
		if !m.matchesTab(tool) {
			continue
		}
		// Status filter (Marketplace tab only).
		if m.activeTab == tabDiscover && m.statusFilter == "installed" && !tool.IsInstalled() {
			continue
		}
		if m.activeTab == tabDiscover && m.statusFilter == "available" && tool.IsInstalled() {
			continue
		}
		// Structured category filter.
		if m.categoryFilter != "" && !strings.EqualFold(tool.Category, m.categoryFilter) {
			continue
		}
		// Tag filter.
		if m.tagFilter != "" && !hasTag(tool.Tags, m.tagFilter) {
			continue
		}
		// Platform filter.
		if m.platformFilter != "" && !hasPlatform(tool.Packages, m.platformFilter) {
			continue
		}
		if filter != "" {
			if searchScores[i] <= 0 {
				continue
			}
		}
		m.filteredIndex = append(m.filteredIndex, i)
	}

	// Apply sort mode.
	if m.sortMode == sortByStars {
		sort.SliceStable(m.filteredIndex, func(a, b int) bool {
			starsA, starsB := 0, 0
			if m.tools[m.filteredIndex[a]].GitHubInfo != nil {
				starsA = m.tools[m.filteredIndex[a]].GitHubInfo.Stars
			}
			if m.tools[m.filteredIndex[b]].GitHubInfo != nil {
				starsB = m.tools[m.filteredIndex[b]].GitHubInfo.Stars
			}
			return starsA > starsB
		})
	}

	if m.cursor >= len(m.filteredIndex) {
		m.cursor = max(0, len(m.filteredIndex)-1)
	}
}

// hasTag reports whether the tool has an exact tag match (case-insensitive).
func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// hasPlatform reports whether the tool supports the given platform.
func hasPlatform(pkgs registry.PackageIDs, platform string) bool {
	for _, p := range derivePlatforms(pkgs) {
		if strings.EqualFold(p, platform) {
			return true
		}
	}
	return false
}

// matchesTab reports whether a tool should appear on the active tab.
func (m *Model) matchesTab(tool registry.Tool) bool {
	switch m.activeTab {
	case tabInstalled:
		return tool.IsInstalled()
	case tabFavorites:
		return m.favoriteNames[tool.Name]
	case tabUpdates:
		return tool.HasUpdate()
	case tabDiscover:
		return true // all tools; statusFilter applied in applyFilter
	case tabBackup:
		return false // Backup tab renders from backupItems, not tools
	case tabProject:
		return false // Project tab renders check results, not tools
	case tabDashboard:
		return false // Dashboard tab renders aggregate stats, not tools
	case tabConfig:
		return false // Config tab renders static content, not tools
	}
	return false
}

// collectCategories returns sorted unique category names from the tool list.
func collectCategories(tools []registry.Tool) []string {
	seen := make(map[string]struct{})
	for _, t := range tools {
		if t.Category != "" {
			seen[t.Category] = struct{}{}
		}
	}
	cats := make([]string, 0, len(seen))
	for cat := range seen {
		cats = append(cats, cat)
	}
	sort.Strings(cats)
	return cats
}

// collectTags returns sorted unique tag names from the tool list.
func collectTags(tools []registry.Tool) []string {
	seen := make(map[string]struct{})
	for _, t := range tools {
		for _, tag := range t.Tags {
			if tag != "" {
				seen[tag] = struct{}{}
			}
		}
	}
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

// collectPlatforms returns sorted unique platform names inferred from all tools.
func collectPlatforms(tools []registry.Tool) []string {
	seen := make(map[string]struct{})
	for _, t := range tools {
		for _, p := range derivePlatforms(t.Packages) {
			seen[p] = struct{}{}
		}
	}
	platforms := make([]string, 0, len(seen))
	for p := range seen {
		platforms = append(platforms, p)
	}
	sort.Strings(platforms)
	return platforms
}

// buildSidebarItems constructs the flat sidebar item list from categories,
// tags, and platforms. tabTools are the tools visible on the current tab,
// used for counting.
func buildSidebarItems(categories, tags, platforms []string, tabTools []registry.Tool) []sidebarItem {
	var items []sidebarItem

	// Count tools per category.
	catCount := make(map[string]int, len(categories))
	for _, t := range tabTools {
		if t.Category != "" {
			catCount[t.Category]++
		}
	}

	// Count tools per tag.
	tagCount := make(map[string]int, len(tags))
	for _, t := range tabTools {
		for _, tag := range t.Tags {
			tagCount[tag]++
		}
	}

	// Count tools per platform.
	platCount := make(map[string]int, len(platforms))
	for _, t := range tabTools {
		for _, p := range derivePlatforms(t.Packages) {
			platCount[p]++
		}
	}

	totalCount := len(tabTools)

	// Category section.
	items = append(items,
		sidebarItem{label: "CATEGORY", isHeader: true},
		sidebarItem{label: fmt.Sprintf("All (%d)", totalCount), section: "category", value: ""},
	)
	for _, cat := range categories {
		items = append(items, sidebarItem{label: fmt.Sprintf("%s (%d)", cat, catCount[cat]), section: "category", value: cat})
	}

	// Platform section.
	if len(platforms) > 0 {
		items = append(items,
			sidebarItem{label: "PLATFORM", isHeader: true},
			sidebarItem{label: fmt.Sprintf("All (%d)", totalCount), section: "platform", value: ""},
		)
		for _, p := range platforms {
			items = append(items, sidebarItem{label: fmt.Sprintf("%s (%d)", p, platCount[p]), section: "platform", value: p})
		}
	}

	// Tag section.
	if len(tags) > 0 {
		items = append(items,
			sidebarItem{label: "TAG", isHeader: true},
			sidebarItem{label: fmt.Sprintf("All (%d)", totalCount), section: "tag", value: ""},
		)
		for _, tag := range tags {
			items = append(items, sidebarItem{label: fmt.Sprintf("%s (%d)", tag, tagCount[tag]), section: "tag", value: tag})
		}
	}

	return items
}
