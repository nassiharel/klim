package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/nassiharel/klim/internal/favorites"
	"github.com/nassiharel/klim/internal/registry"
)

// loadFavoriteNames loads favorites from disk into a map for quick lookups.
func loadFavoriteNames() map[string]bool {
	m, err := favorites.Set()
	if err != nil {
		return make(map[string]bool)
	}
	return m
}

// handleKeyFavorites handles keys when the Favorites tab is active.
func (m Model) handleKeyFavorites(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Clear transient status on keypress.
	if len(m.tools) > 0 && !m.favClearConfirm {
		m.statusMsg = ""
	}

	// Clear-all confirmation mode.
	if m.favClearConfirm {
		switch msg.String() {
		case "y", "Y":
			m.favClearConfirm = false
			if err := favorites.Save(nil); err != nil {
				m.statusMsg = fmt.Sprintf("⚠ %v", err)
			} else {
				m.favoriteNames = make(map[string]bool)
				m.statusMsg = "✓ All favorites cleared"
				m.applyFilter()
				m.cursor = 0
			}
			return m, nil
		case "n", "N", "esc":
			m.favClearConfirm = false
			m.statusMsg = ""
			return m, nil
		}
		return m, nil
	}

	// Share token display mode — only allow copy and dismiss.
	if m.favMode == "share" && m.sharedToken != "" {
		switch msg.String() {
		case "c":
			if err := m.clip.WriteAll(m.sharedToken); err != nil {
				m.statusMsg = "⚠ Clipboard unavailable"
			} else {
				m.tokenCopied = true
				m.statusMsg = "✓ Copied to clipboard!"
			}
			return m, nil
		case "esc":
			m.favMode = ""
			m.sharedToken = ""
			m.tokenCopied = false
			return m, nil
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil
	}

	// Export finished — esc goes back.
	if m.favMode != "" {
		switch msg.String() {
		case "esc":
			m.favMode = ""
			return m, nil
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil
	}

	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "*":
		// Unfavorite highlighted tool.
		if m.cursor < len(m.filteredIndex) {
			idx := m.filteredIndex[m.cursor]
			if idx < len(m.tools) {
				name := m.tools[idx].Name
				if err := favorites.Remove(name); err != nil {
					m.statusMsg = fmt.Sprintf("⚠ %v", err)
				} else {
					delete(m.favoriteNames, name)
					m.statusMsg = "☆ Removed from favorites"
					m.applyFilter()
					if m.cursor >= len(m.filteredIndex) && m.cursor > 0 {
						m.cursor--
					}
				}
			}
		}
		return m, nil
	case "e":
		// Export favorites.
		favTools := m.favoritesTools()
		if len(favTools) == 0 {
			m.statusMsg = "No favorites to export"
			return m, nil
		}
		m.favMode = "export"
		return m, exportFavoritesCmd(m.tools, m.favoriteNames)
	case "s":
		// Share favorites.
		if len(m.favoriteNames) == 0 {
			m.statusMsg = "No favorites to share"
			return m, nil
		}
		m.favMode = "share"
		return m, shareFavoritesCmd(m.favoriteNames)
	case "x":
		// Clear all favorites — enter confirmation mode.
		if len(m.favoriteNames) == 0 {
			m.statusMsg = "No favorites to clear"
			return m, nil
		}
		m.favClearConfirm = true
		m.statusMsg = fmt.Sprintf("Clear all %d favorites? (y/n)", len(m.favoriteNames))
		return m, nil
	case "enter":
		// Open tool detail.
		if m.cursor < len(m.filteredIndex) {
			m.openDetailView(m.filteredIndex[m.cursor])
		}
		return m, nil
	case "/":
		m.filtering = true
		return m, m.filterInput.Focus()
	case "f":
		if len(m.sidebarItems) > 0 {
			m.categoryPicker = true
			m.sidebarIdx = 0
			for i, item := range m.sidebarItems {
				if !item.isHeader {
					m.sidebarIdx = i
					break
				}
			}
		}
		return m, nil

	// Navigation.
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < m.rowCount()-1 {
			m.cursor++
		}
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = max(0, m.rowCount()-1)

	// Tab switching. Favorites is a My Tools subtab, so left/right
	// must cycle the My Tools subtab strip (Installed ↔ Updates ↔
	// Favorites) before stepping to the next/previous parent. This
	// mirrors the handleKeyDefault behaviour and prevents Favorites
	// from skipping straight to Marketplace/Config on arrow keys.
	case "right", "tab":
		if isMyToolsTab(m.activeTab) {
			i := myToolsSubIndex(m.activeTab)
			if i >= 0 && i < len(myToolsSubOrder)-1 {
				m.activeTab = myToolsSubOrder[i+1]
				m.cursor = 0
				m.applyFilter()
				return m, nil
			}
		}
		next := parentTabOrder[(parentIndex(m.activeTab)+1)%len(parentTabOrder)]
		m.discoverSubTab = discoverTools
		return m.gotoParentTab(next)
	case "left", "shift+tab":
		if isMyToolsTab(m.activeTab) {
			i := myToolsSubIndex(m.activeTab)
			if i > 0 {
				m.activeTab = myToolsSubOrder[i-1]
				m.cursor = 0
				m.applyFilter()
				return m, nil
			}
		}
		prev := parentTabOrder[(parentIndex(m.activeTab)+len(parentTabOrder)-1)%len(parentTabOrder)]
		m.discoverSubTab = discoverTools
		return m.gotoParentTab(prev)
	case "1", "2", "3", "4", "5", "6", "7", "8":
		if handled, cmd := m.switchToTabByNumber(msg.String()); handled {
			return m, cmd
		}
		return m, nil
	case "r":
		cmd := m.startScan()
		return m, cmd
	}
	return m, nil
}

// favoritesTools returns the subset of tools that are in the favorites set.
func (m Model) favoritesTools() []registry.Tool {
	var result []registry.Tool
	for _, t := range m.tools {
		if m.favoriteNames[t.Name] {
			result = append(result, t)
		}
	}
	return result
}

// renderFavoritesView renders the Favorites tab content when it needs a custom
// rendering path (share token, empty state). Returns "" when the standard
// two-column layout should be used.
func (m Model) renderFavoritesView() string {
	if m.favMode == "share" && m.sharedToken != "" {
		return m.renderFavShareToken()
	}

	if len(m.filteredIndex) == 0 && m.favMode == "" {
		if len(m.favoriteNames) == 0 {
			return m.renderFavEmptyState()
		}
		// Has favorites but current filter/search hides them all.
		var b strings.Builder
		b.WriteString("\n\n")
		b.WriteString("  " + dimVersion.Render("No favorites match the current filter.") + "\n")
		return b.String()
	}

	return ""
}

// renderFavEmptyState renders the empty state for the favorites tab.
func (m Model) renderFavEmptyState() string {
	var b strings.Builder
	b.WriteString("\n\n")
	b.WriteString("  " + dimVersion.Render("No favorites yet.") + "\n\n")
	b.WriteString("  " + dimVersion.Render("Press") + " * " + dimVersion.Render("on any tool to add it to favorites.") + "\n")
	return b.String()
}

// renderFavShareToken renders the share token display for favorites.
func (m Model) renderFavShareToken() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  " + confirmStyle.Render("★ Favorites Share Token") + "\n\n")

	// Word-wrap the token.
	token := m.sharedToken
	lineWidth := m.width - 6
	if lineWidth < 40 {
		lineWidth = 40
	}
	for len(token) > 0 {
		end := lineWidth
		if end > len(token) {
			end = len(token)
		}
		b.WriteString("  " + token[:end] + "\n")
		token = token[end:]
	}

	b.WriteString("\n")
	if m.tokenCopied {
		b.WriteString("  " + upgradableStyle.Render("✓ Copied to clipboard!") + "\n")
	} else {
		b.WriteString("  " + dimVersion.Render("Press c to copy to clipboard") + "\n")
	}
	b.WriteString("\n")
	b.WriteString("  " + dimVersion.Render("Recipients can import with:") + "\n")
	b.WriteString("  klim open <token>\n")

	return b.String()
}
