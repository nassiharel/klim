package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/clim/internal/registry"
)

// packSortOrder returns the indices into m.packs in display order, accounting
// for the current packSortMode. Used by both renderPacksList (for rendering)
// and packDisplayIndex (for resolving cursor → real pack index on Enter).
//
// Modes:
//   - 0 (default): status sort — complete > partial (desc %) > not-installed,
//     ties broken by display name.
//   - 1: name sort — case-insensitive by display name.
func (m Model) packSortOrder() []int {
	order := make([]int, len(m.packs))
	for i := range order {
		order[i] = i
	}
	if len(m.packs) == 0 {
		return order
	}

	if m.packSortMode == 1 {
		sort.SliceStable(order, func(a, b int) bool {
			return strings.ToLower(m.packs[order[a]].DisplayName) < strings.ToLower(m.packs[order[b]].DisplayName)
		})
		return order
	}

	// Status sort: precompute install counts.
	toolMap := registry.InstalledSet(m.tools)
	installed := make([]int, len(m.packs))
	for i, pack := range m.packs {
		for _, name := range pack.ToolNames {
			if toolMap[name] {
				installed[i]++
			}
		}
	}
	sort.SliceStable(order, func(a, b int) bool {
		ai, bi := order[a], order[b]
		aTotal, bTotal := len(m.packs[ai].ToolNames), len(m.packs[bi].ToolNames)
		aInst, bInst := installed[ai], installed[bi]
		aComplete := aTotal > 0 && aInst == aTotal
		bComplete := bTotal > 0 && bInst == bTotal
		aPartial := aInst > 0 && !aComplete
		bPartial := bInst > 0 && !bComplete

		aRank := 2
		if aComplete {
			aRank = 0
		} else if aPartial {
			aRank = 1
		}
		bRank := 2
		if bComplete {
			bRank = 0
		} else if bPartial {
			bRank = 1
		}
		if aRank != bRank {
			return aRank < bRank
		}
		if aPartial && bPartial && aTotal > 0 && bTotal > 0 {
			aPct := aInst * 100 / aTotal
			bPct := bInst * 100 / bTotal
			if aPct != bPct {
				return aPct > bPct
			}
		}
		return strings.ToLower(m.packs[ai].DisplayName) < strings.ToLower(m.packs[bi].DisplayName)
	})
	return order
}

// renderPacksList renders the list of packs for the Packs sub-tab.
func (m Model) renderPacksList() string {
	var b strings.Builder

	if len(m.packs) == 0 {
		b.WriteString("\n  " + dimVersion.Render("No packs available.") + "\n")
		return b.String()
	}

	toolMap := registry.InstalledSet(m.tools)
	packOrder := m.packSortOrder()

	// Compute install counts for the status column.
	packInstalled := make([]int, len(m.packs))
	for i, pack := range m.packs {
		for _, name := range pack.ToolNames {
			if toolMap[name] {
				packInstalled[i]++
			}
		}
	}

	// Header with sort indicator.
	sortLabel := "status"
	if m.packSortMode == 1 {
		sortLabel = "name"
	}
	b.WriteString("  " +
		headerStyle.Render(fixedWidth("PACK", colName)) + "  " +
		headerStyle.Render(fixedWidth("TOOLS", colPackTools)) + "  " +
		headerStyle.Render("STATUS") +
		"  " + dashDim.Render("[s: sort by "+sortLabel+"]") + "\n")

	// Overhead: title(1) + tabs(1) + rule(1) + blank(1) + search(1) + sub-tabs(1) + header(1) + gap(1) + footer.
	visibleRows := m.height - 8 - m.footerHeight()
	if visibleRows < 3 {
		visibleRows = 3
	}
	start := 0
	if m.cursor >= visibleRows {
		start = m.cursor - visibleRows + 1
	}

	for vi := start; vi < len(packOrder) && vi < start+visibleRows; vi++ {
		pi := packOrder[vi]
		pack := m.packs[pi]
		selected := vi == m.cursor

		cursor := "  "
		if selected {
			cursor = "▸ "
		}

		nameCell := nameStyle.Render(fixedWidth(pack.DisplayName, colName))
		toolCount := fixedWidth(fmt.Sprintf("%d tools", len(pack.ToolNames)), colPackTools)

		// Compute install status with gauge.
		installed := packInstalled[pi]
		var status string
		switch {
		case len(pack.ToolNames) == 0:
			status = dimVersion.Render("empty pack")
		case installed == len(pack.ToolNames):
			status = upToDateStyle.Render("✓ COMPLETE") + "  " +
				gauge(installed, len(pack.ToolNames), 10, dashGaugeFill, dashGaugeEmpty) +
				"  " + dimVersion.Render(fmt.Sprintf("%d / %d", installed, len(pack.ToolNames)))
		case installed > 0:
			pct := installed * 100 / len(pack.ToolNames)
			status = dashGaugeWarn.Render("◐ PARTIAL ") + "  " +
				gauge(installed, len(pack.ToolNames), 10, dashGaugeWarn, dashGaugeEmpty) +
				"  " + dimVersion.Render(fmt.Sprintf("%d / %d  (%d%%)", installed, len(pack.ToolNames), pct))
		default:
			status = dashDim.Render("○ NOT INSTALLED")
		}

		line := cursor + nameCell + "  " + toolCount + "  " + status
		if selected {
			w := lipgloss.Width(line)
			if w < m.width {
				line += strings.Repeat(" ", m.width-w)
			}
			line = selectedRowStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	// Pad remaining rows.
	rendered := max(min(len(m.packs)-start, visibleRows), 0)
	for range max(visibleRows-rendered, 0) {
		b.WriteString("\n")
	}

	return b.String()
}

// packDisplayIndex returns the real pack index for the given display position,
// using the same sort order that renderPacksList rendered.
func (m Model) packDisplayIndex(displayIdx int) int {
	order := m.packSortOrder()
	if displayIdx < 0 || displayIdx >= len(order) {
		return displayIdx
	}
	return order[displayIdx]
}

// renderPackDetailView renders the detail view for a selected pack.
func (m Model) renderPackDetailView(pack registry.Pack) string {
	var b strings.Builder
	label := detailLabelStyle.Render
	dim := dimVersion.Render

	// Header.
	title := detailTitleStyle.Render("📦 " + pack.DisplayName)
	b.WriteString("  " + title)
	divLen := max(m.width-lipgloss.Width(title)-6, 10)
	b.WriteString("  " + strings.Repeat("─", divLen))
	b.WriteString("\n\n")

	// Description.
	if pack.Description != "" {
		maxW := m.width - 6
		if maxW < 20 {
			maxW = 20
		}
		for _, line := range wordWrap(pack.Description, maxW) {
			b.WriteString("  " + dim(line) + "\n")
		}
		b.WriteString("\n")
	}

	// If a pack operation is in progress, show live progress.
	if len(m.packItems) > 0 {
		b.WriteString("  " + label("Progress:") + "\n\n")

		for _, item := range m.packItems {
			var icon, status string
			switch item.status {
			case packItemPending:
				icon = dim("○")
				status = dim("pending")
			case packItemRunning:
				icon = upgradableStyle.Render("◉")
				status = upgradableStyle.Render("running...")
			case packItemDone:
				icon = upToDateStyle.Render("✓")
				status = upToDateStyle.Render("done")
			case packItemFailed:
				icon = upgradableStyle.Render("✗")
				if item.errMsg != "" {
					status = upgradableStyle.Render(item.errMsg)
				} else {
					status = upgradableStyle.Render("failed")
				}
			case packItemSkipped:
				icon = dim("–")
				if item.errMsg != "" {
					status = dim(item.errMsg)
				} else {
					status = dim("skipped")
				}
			}
			fmt.Fprintf(&b, "    %s  %s  %s\n", icon, fixedWidth(itemLabel(item.name, item.display), colPackName), fixedWidthANSI(status, colPackStat))
		}

		pending := 0
		for _, item := range m.packItems {
			if item.status == packItemPending || item.status == packItemRunning {
				pending++
			}
		}
		fmt.Fprintf(&b, "\n  %d/%d complete\n", m.packDone, len(m.packItems))

		var progressFooter string
		if pending == 0 && !m.packInstalling {
			progressFooter = "  " + dim("Esc") + " back"
		} else {
			// Show current item name + progress + skip/cancel hints.
			current := ""
			for _, item := range m.packItems {
				if item.status == packItemRunning {
					current = item.display
					if current == "" {
						current = item.name
					}
					break
				}
			}
			verb := m.packAction
			if verb == "" {
				verb = "Installing"
			}
			var status string
			if current == "" {
				status = fmt.Sprintf("  %s... (%d/%d)", verb, m.packDone, len(m.packItems))
			} else {
				status = fmt.Sprintf("  %s %s (%d/%d)", verb, current, m.packDone, len(m.packItems))
			}
			progressFooter = upgradableStyle.Render(status) + "   " +
				dim("s") + " skip   " + dim("Esc") + " cancel   " + dim("q") + " dismiss"
		}

		return m.layoutWithFooter(b.String(), progressFooter)
	}

	// Static view — show tool list with install status.
	b.WriteString("  " + label("Tools:") + "\n\n")

	toolMap := registry.InstalledSet(m.tools)
	toolByName := make(map[string]registry.Tool, len(m.tools))
	for _, t := range m.tools {
		toolByName[t.Name] = t
	}

	installed := 0
	for i, name := range pack.ToolNames {
		isInstalled := toolMap[name]
		if isInstalled {
			installed++
		}
		selected := i == m.packToolCursor
		icon := dim("○")
		status := dim("not installed")
		if isInstalled {
			icon = upToDateStyle.Render("✓")
			status = upToDateStyle.Render("installed")
		}
		cursor := "    "
		if selected {
			cursor = "  ▸ "
		}
		nameCell := fixedWidth(name, colPackName)
		statusCell := fixedWidthANSI(status, colPackStat)
		line := cursor + icon + "  " + nameCell + "  " + statusCell
		if t, ok := toolByName[name]; ok {
			if badge := githubStarsBadge(t); badge != "" {
				line += "  " + fixedWidthANSI(dim(badge), colStars)
			}
		}
		if selected {
			w := lipgloss.Width(line)
			if w < m.width {
				line += strings.Repeat(" ", m.width-w)
			}
			line = selectedRowStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	// Actions.
	if len(pack.ToolNames) == 0 {
		b.WriteString("  " + dim("This pack has no tools defined.") + "\n")
		footer := "  " + dim("Esc") + " back"
		return m.layoutWithFooter(b.String(), footer)
	}

	fmt.Fprintf(&b, "\n  %s  %d/%d installed\n", label("Status:"), installed, len(pack.ToolNames))

	if installed >= len(pack.ToolNames) {
		b.WriteString("  " + upToDateStyle.Render("All tools in this pack are installed! ✓") + "\n")
	}

	// Footer.
	footer := "  " + dim("↑↓") + " navigate   " + dim("Enter") + " tool detail   " + dim("i") + " install   " + dim("x") + " remove   " + dim("Esc") + " back"
	return m.layoutWithFooter(b.String(), footer)
}

// --- Header ---
