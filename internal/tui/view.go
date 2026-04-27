package tui

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"

	"github.com/nassiharel/clim/internal/build"
	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/logging"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/scancache"
)

const (
	colName      = 22 // width for name column in tool lists
	colNameWide  = 32 // width for name column in recommendation cards
	colVersion   = 24 // width for version info column
	colSource    = 8  // width for source column
	colCategory  = 12 // width for category column
	colStatus    = 18 // width for backup status column
	colSidebar   = 18 // width for filter sidebar panel
	colStars     = 8  // width reserved for ★ NNNk badge column
	colPackTools = 8  // width for "N tools" column in packs list
	colPackName  = 20 // width for pack tool name column in pack detail
	colPackStat  = 16 // width for pack tool install-status column in pack detail
	colReason    = 32 // width for "BECAUSE YOU HAVE" reason column in For You
)

func (m Model) renderView() string {
	if m.quitting {
		return ""
	}

	// Detail view.
	if m.showDetail && m.detailIdx >= 0 && m.detailIdx < len(m.tools) {
		return m.renderDetailView(m.tools[m.detailIdx])
	}

	// Pack detail view.
	if m.showPackDetail && m.packDetailIdx >= 0 && m.packDetailIdx < len(m.packs) {
		return m.renderPackDetailView(m.packs[m.packDetailIdx])
	}

	var body strings.Builder

	body.WriteString(m.renderTitleBar() + "\n")
	body.WriteString(m.renderTabBar() + "\n\n")

	// Backup tab has its own rendering path.
	if m.activeTab == tabBackup {
		body.WriteString(m.renderBackupView())
		var footer string
		switch {
		case m.importingPath:
			footer = "  " + confirmStyle.Render("Import:") + " " + m.importInput.View() + "  " + dimVersion.Render("Enter") + " go   " + dimVersion.Render("Esc") + " cancel"
		case m.enteringToken:
			footer = "  " + confirmStyle.Render("Token:") + " " + m.tokenInput.View() + "  " + dimVersion.Render("Enter") + " go   " + dimVersion.Render("Esc") + " cancel"
		default:
			footer = m.renderHelp()
		}
		return m.layoutWithFooter(body.String(), footer)
	}

	// Project tab.
	if m.activeTab == tabProject {
		body.WriteString(m.renderProjectView())
		return m.layoutWithFooter(body.String(), m.renderHelp())
	}

	// Favorites tab — custom rendering for share token / empty state.
	if m.activeTab == tabFavorites {
		if custom := m.renderFavoritesView(); custom != "" {
			body.WriteString(custom)
			return m.layoutWithFooter(body.String(), m.renderHelp())
		}
		// Fall through to standard two-column layout for non-empty favorites list.
	}

	// Config tab — supports scrolling.
	if m.activeTab == tabConfig {
		content := m.renderConfigView()
		lines := strings.Split(content, "\n")

		footer := m.renderHelp()
		footerRows := visualRows(footer, m.width)
		const cfgHeaderRows = 3
		const cfgMinGap = 1
		visibleRows := m.height - cfgHeaderRows - footerRows - cfgMinGap
		if visibleRows < 5 {
			visibleRows = 5
		}

		maxScroll := len(lines) - visibleRows
		if maxScroll < 0 {
			maxScroll = 0
		}
		scroll := m.configScroll
		if scroll > maxScroll {
			scroll = maxScroll
		}

		if scroll > 0 && scroll < len(lines) {
			lines = lines[scroll:]
		}

		body.WriteString(strings.Join(lines, "\n"))
		if scroll > 0 {
			footer = "  " + dimVersion.Render("↑/↓ scroll") + "    " + footer
		} else if len(strings.Split(content, "\n")) > visibleRows {
			footer = "  " + dimVersion.Render("↓ more below") + "    " + footer
		}
		return m.layoutWithFooter(body.String(), footer)
	}

	// Dashboard tab has its own rendering path — supports scrolling.
	if m.activeTab == tabDashboard {
		content := m.renderDashboardView()
		lines := strings.Split(content, "\n")

		// Compute available visible rows: total height minus tab bar (2 lines),
		// footer, and 1-line gap between body and footer.
		footer := m.renderHelp()
		footerRows := visualRows(footer, m.width)
		const headerRows = 3 // title bar + tab bar + blank
		const minGap = 1
		visibleRows := m.height - headerRows - footerRows - minGap
		if visibleRows < 5 {
			visibleRows = 5
		}

		// Clamp scroll offset so last screenful of content stays visible.
		maxScroll := len(lines) - visibleRows
		if maxScroll < 0 {
			maxScroll = 0
		}
		scroll := m.dashboardScroll
		if scroll > maxScroll {
			scroll = maxScroll
		}

		// Apply scroll.
		if scroll > 0 && scroll < len(lines) {
			lines = lines[scroll:]
		}

		body.WriteString(strings.Join(lines, "\n"))
		if scroll > 0 {
			footer = "  " + dimVersion.Render("↑/↓ scroll   Home top") + "    " + footer
		} else if len(strings.Split(content, "\n")) > visibleRows {
			footer = "  " + dimVersion.Render("↓ scroll down") + "    " + footer
		}
		return m.layoutWithFooter(body.String(), footer)
	}

	// Search bar.
	body.WriteString(m.renderSearchBar() + "\n\n")

	// Marketplace sub-tab bar.
	if m.activeTab == tabDiscover {
		body.WriteString(m.renderDiscoverSubTabs() + "\n\n")
	}

	// Marketplace Packs sub-tab — separate rendering path.
	if m.activeTab == tabDiscover && m.discoverSubTab == discoverPacks {
		body.WriteString(m.renderPacksList())
		return m.layoutWithFooter(body.String(), m.renderHelp())
	}

	// Marketplace For You sub-tab — separate rendering path.
	if m.activeTab == tabDiscover && m.discoverSubTab == discoverForYou {
		body.WriteString(m.renderForYouList())
		return m.layoutWithFooter(body.String(), m.renderHelp())
	}

	// Two-column layout: sidebar | tool list.
	// Compute available rows dynamically based on actual footer height.
	footer := m.renderHelp()
	footerRows := visualRows(footer, m.width)
	// Overhead: title(1) + tabs(1) + blank(1) + search(1) + blank(1) + gap(1) + footer.
	overhead := 6 + footerRows
	if m.activeTab == tabDiscover {
		overhead += 2 // sub-tab bar + blank line
	}
	visibleRows := m.height - overhead
	if visibleRows < 3 {
		visibleRows = 3
	}

	sidebarOnRight := m.cfg != nil && m.cfg.UI.SidebarRight

	sidebarLines := m.buildSidebarLines(visibleRows)
	toolLines := m.buildToolLines(visibleRows)

	// Always produce exactly visibleRows lines so footer position is stable.
	totalLines := visibleRows

	for i := range totalLines {
		left := ""
		if i < len(sidebarLines) {
			left = sidebarLines[i]
		}
		right := ""
		if i < len(toolLines) {
			right = toolLines[i]
		}

		var line string
		if sidebarOnRight {
			toolWidth := m.width - colSidebar - 3 // 3 = " │ "
			if toolWidth < 20 {
				toolWidth = 20
			}
			line = fixedWidthANSI(right, toolWidth) + " │ " + left
		} else {
			line = fixedWidthANSI(left, colSidebar) + " │ " + right
		}

		// Truncate to terminal width to prevent wrapping (which destabilizes footer).
		if m.width > 0 && lipgloss.Width(line) > m.width {
			line = truncateANSI(line, m.width)
		}

		body.WriteString(line + "\n")
	}

	return m.layoutWithFooter(body.String(), footer)
}

// footerHeight returns the number of visual rows the help/status footer occupies.
func (m Model) footerHeight() int {
	return visualRows(m.renderHelp(), m.width)
}

// layoutWithFooter pads `body` with blank lines so that `footer` sticks to the
// bottom of the terminal viewport. If the combined body + footer would exceed
// the available height, the body is truncated from the bottom so the footer
// remains visible. When the height is unknown, a single blank separator line
// is inserted and the content is returned as-is.
//
// Row counts are computed visually: a line that is wider than the terminal
// wraps onto multiple physical rows, so each logical line contributes
// `ceil(width/m.width)` rows.
func (m Model) layoutWithFooter(body, footer string) string {
	// Normalize: ensure body ends with exactly one newline so subsequent line
	// counting and padding are predictable.
	body = strings.TrimRight(body, "\n") + "\n"

	// Always reserve at least one blank separator line between body and footer.
	const minGap = 1

	if m.height <= 0 {
		return body + strings.Repeat("\n", minGap) + footer
	}

	footerRows := visualRows(footer, m.width)
	bodyRows := visualRows(body, m.width)

	available := m.height - footerRows - minGap
	if available < 0 {
		available = 0
	}
	if bodyRows > available {
		// Body is too tall to fit alongside the footer. Drop lines from the
		// bottom until the remaining body fits, so the footer (action menu,
		// help hints, etc.) is never clipped by the terminal.
		lines := strings.SplitAfter(body, "\n")
		rows := 0
		kept := 0
		for _, ln := range lines {
			r := visualRows(ln, m.width)
			if rows+r > available {
				break
			}
			rows += r
			kept++
		}
		body = strings.Join(lines[:kept], "")
		bodyRows = rows
	}

	gap := m.height - bodyRows - footerRows
	if gap < minGap {
		gap = minGap
	}
	return body + strings.Repeat("\n", gap) + footer
}

// visualRows returns the number of terminal rows occupied by s when rendered
// at the given width, accounting for both explicit newlines and line wrapping
// when a line exceeds `width` cells. A single trailing newline is treated as
// a line terminator (not an additional empty row). A width of 0 falls back to
// counting explicit newlines only.
func visualRows(s string, width int) int {
	if s == "" {
		return 0
	}
	// A single trailing newline terminates the last line rather than starting
	// a new one; strip it before counting.
	trimmed := strings.TrimSuffix(s, "\n")
	if trimmed == "" {
		return 1
	}
	rows := 0
	for _, line := range strings.Split(trimmed, "\n") {
		if width <= 0 {
			rows++
			continue
		}
		w := lipgloss.Width(line)
		if w == 0 {
			rows++
			continue
		}
		rows += (w + width - 1) / width
	}
	return rows
}

// --- Title & Tabs ---

func (m Model) renderTitleBar() string {
	title := titleStyle.Render("  clim")

	if m.phase == phaseLoading {
		return title + "  " + loadingStyle.Render(m.spinner.View()+" Loading tools...")
	}
	if m.phase == phaseResolving && m.pending > 0 {
		return title + "  " + loadingStyle.Render(fmt.Sprintf("%s Checking versions (%d remaining)...", m.spinner.View(), m.pending))
	}

	inst, upd, notInst := m.stats()
	active := inst + notInst
	summary := fmt.Sprintf("%d/%d installed", inst, active)
	if upd > 0 {
		summary += " · " + upgradableStyle.Render(strconv.Itoa(upd)+" updates")
	}
	if notInst > 0 {
		summary += fmt.Sprintf(" · %d in marketplace", notInst)
	}
	return title + "  " + summaryStyle.Render(summary)
}

func (m Model) renderTabBar() string {
	tabs := []struct {
		label string
		idx   int
	}{
		{"Installed", tabInstalled},
		{"★ Favorites", tabFavorites},
		{"Updates", tabUpdates},
		{"Marketplace", tabDiscover},
		{"Backup", tabBackup},
		{"Project", tabProject},
		{"Dashboard", tabDashboard},
		{"Config", tabConfig},
	}

	var parts []string
	for _, tab := range tabs {
		style := inactiveTabStyle
		if tab.idx == m.activeTab {
			style = activeTabStyle
		}
		parts = append(parts, style.Render(tab.label))
	}

	return "  " + strings.Join(parts, " ")
}

// --- Search Bar ---

// renderSearchBar renders the search box with a styled border.
func (m Model) renderSearchBar() string {
	var content string
	switch {
	case m.filtering:
		content = filterPromptStyle.Render("🔍 ") + m.filterInput.View()
	case m.filterText != "":
		content = filterPromptStyle.Render("🔍 ") + dimVersion.Render(m.filterText) +
			"  " + dimVersion.Render("(/ edit  Esc clear)")
	default:
		content = dimVersion.Render("🔍 / search...")
	}

	// Active filter indicators.
	var filters []string
	if m.categoryFilter != "" {
		filters = append(filters, chipStyle.Render(m.categoryFilter))
	}
	if m.tagFilter != "" {
		filters = append(filters, chipStyle.Render(m.tagFilter))
	}
	if m.platformFilter != "" {
		filters = append(filters, chipStyle.Render(m.platformFilter))
	}
	if m.sortMode == sortByStars {
		filters = append(filters, chipStyle.Render("★ sort"))
	}
	if len(filters) > 0 {
		content += "    " + strings.Join(filters, " ")
	}

	return "  " + content
}

// --- Marketplace sub-tabs & packs ---

// renderDiscoverSubTabs renders the [Tools] [Packs] [For You] sub-tab bar.
func (m Model) renderDiscoverSubTabs() string {
	labels := []struct {
		name string
		idx  int
	}{
		{"Tools", discoverTools},
		{"Packs", discoverPacks},
		{"For You", discoverForYou},
	}
	var parts []string
	for _, l := range labels {
		if l.idx == m.discoverSubTab {
			parts = append(parts, activeTabStyle.Render(l.name))
		} else {
			parts = append(parts, inactiveTabStyle.Render(l.name))
		}
	}
	return "  " + strings.Join(parts, " ")
}

// renderPacksList renders the list of packs for the Packs sub-tab.
func (m Model) renderPacksList() string {
	var b strings.Builder

	if len(m.packs) == 0 {
		b.WriteString("\n  " + dimVersion.Render("No packs available.") + "\n")
		return b.String()
	}

	toolMap := registry.InstalledSet(m.tools)

	// Build sorted index based on packSortMode.
	packOrder := make([]int, len(m.packs))
	for i := range packOrder {
		packOrder[i] = i
	}

	// Compute install counts for sorting.
	packInstalled := make([]int, len(m.packs))
	for i, pack := range m.packs {
		for _, name := range pack.ToolNames {
			if toolMap[name] {
				packInstalled[i]++
			}
		}
	}

	if m.packSortMode == 1 {
		// Sort by name.
		sort.SliceStable(packOrder, func(a, b int) bool {
			return strings.ToLower(m.packs[packOrder[a]].DisplayName) < strings.ToLower(m.packs[packOrder[b]].DisplayName)
		})
	} else {
		// Sort by status (default): complete first, then partial (desc %), then not installed, then name.
		sort.SliceStable(packOrder, func(a, b int) bool {
			ai, bi := packOrder[a], packOrder[b]
			aTotal, bTotal := len(m.packs[ai].ToolNames), len(m.packs[bi].ToolNames)
			aInst, bInst := packInstalled[ai], packInstalled[bi]
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

	// Overhead: title(1) + tabs(1) + blank(1) + search(1) + sub-tabs(1) + header(1) + gap(1) + footer.
	visibleRows := m.height - 7 - m.footerHeight()
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
// accounting for the current sort mode.
func (m Model) packDisplayIndex(displayIdx int) int {
	if m.packSortMode == 1 || len(m.packs) == 0 {
		return displayIdx // name sort = natural order (already alpha from scan)
	}
	toolMap := registry.InstalledSet(m.tools)
	packOrder := make([]int, len(m.packs))
	for i := range packOrder {
		packOrder[i] = i
	}
	packInstalled := make([]int, len(m.packs))
	for i, pack := range m.packs {
		for _, name := range pack.ToolNames {
			if toolMap[name] {
				packInstalled[i]++
			}
		}
	}
	sort.SliceStable(packOrder, func(a, b int) bool {
		ai, bi := packOrder[a], packOrder[b]
		aTotal, bTotal := len(m.packs[ai].ToolNames), len(m.packs[bi].ToolNames)
		aInst, bInst := packInstalled[ai], packInstalled[bi]
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
	if displayIdx < len(packOrder) {
		return packOrder[displayIdx]
	}
	return displayIdx
}
func (m Model) renderForYouList() string {
	var b strings.Builder

	if len(m.recommendations) == 0 {
		b.WriteString("\n\n")
		b.WriteString("  " + dimVersion.Render("No recommendations yet.") + "\n\n")
		b.WriteString("  " + dimVersion.Render("Install a few tools and clim will suggest related ones") + "\n")
		b.WriteString("  " + dimVersion.Render("based on what you use.") + "\n")
		return b.String()
	}

	// Section header.
	b.WriteString("  " + dashSection.Render("Recommended for you") +
		"  " + dimVersion.Render(fmt.Sprintf("(%d suggestions)", len(m.recommendations))) + "\n\n")

	// Overhead: title(1) + tabs(1) + blank(1) + search(1) + sub-tabs(1) + section header(1) + blank(1) + gap(1) + footer.
	visibleLines := m.height - 8 - m.footerHeight()
	if visibleLines < 6 {
		visibleLines = 6
	}
	itemsPerPage := visibleLines / 3
	if itemsPerPage < 2 {
		itemsPerPage = 2
	}

	start := 0
	if m.cursor >= itemsPerPage {
		start = m.cursor - itemsPerPage + 1
	}

	end := start + itemsPerPage
	if end > len(m.recommendations) {
		end = len(m.recommendations)
	}

	for vi := start; vi < end; vi++ {
		rec := m.recommendations[vi]
		selected := vi == m.cursor
		b.WriteString(m.renderRecCard(rec, selected, false) + "\n")

		// Blank separator between cards (not after last).
		if vi < end-1 {
			b.WriteString("\n")
		}
	}

	// Pad remaining lines to prevent layout jitter.
	renderedItems := end - start
	renderedLines := renderedItems*2 + max(renderedItems-1, 0)
	for i := renderedLines; i < visibleLines; i++ {
		b.WriteString("\n")
	}

	return b.String()
}

// Fixed column widths for recommendation card alignment.
const (
	colCatFY     = 16 // category column (chip style adds padding)
	colStarsFY   = 11 // stars badge column
	colGaugeFY   = 12 // match gauge width
	colPctFY     = 5  // percentage column
	colReasonFY  = 34 // "You use: ..." column
)

// renderRecCard renders a single 2-line recommendation card.
// selected highlights both lines. compact omits row 2 (for inline use in detail views).
func (m Model) renderRecCard(rec recommendation, selected, compact bool) string {
	if rec.toolIdx >= len(m.tools) {
		return ""
	}
	tool := m.tools[rec.toolIdx]

	// --- Row 1: cursor + name + category + stars + gauge + pct ---
	cursor := "  "
	if selected {
		cursor = "▸ "
	}

	displayName := tool.DisplayName
	if displayName == "" {
		displayName = tool.Name
	}
	nameCell := nameStyle.Render(fixedWidth(displayName, colNameWide))

	catText := ""
	if rec.category != "" {
		catText = chipStyle.Render(rec.category)
	}
	catCell := fixedWidthANSI(catText, colCatFY)

	starsText := ""
	if rec.stars > 0 {
		starsText = dimVersion.Render("★ " + formatStars(rec.stars))
	}
	starsCell := fixedWidthANSI(starsText, colStarsFY)

	pct := rec.matchPct
	filled := pct * colGaugeFY / 100
	if filled < 1 && pct > 0 {
		filled = 1
	}
	bar := gauge(filled, colGaugeFY, colGaugeFY, dashGaugeFill, dashGaugeEmpty)
	pctCell := fixedWidthANSI(upgradableStyle.Render(fmt.Sprintf("%d%%", pct)), colPctFY)

	line1 := cursor + nameCell + " " + catCell + " " + starsCell + " " + bar + " " + pctCell

	if selected {
		w := lipgloss.Width(line1)
		if w < m.width {
			line1 += strings.Repeat(" ", m.width-w)
		}
		line1 = selectedRowStyle.Render(line1)
	}

	if compact {
		return line1
	}

	// --- Row 2: indent + description + reason ---
	desc := rec.description
	if desc == "" {
		desc = "No description"
	}
	desc = fixedWidth(desc, colNameWide)
	descCell := dimVersion.Render(desc)

	reasonText := ""
	if rec.reason != "" {
		reasonText = "You use: " + rec.reason
	}
	reasonCell := dimVersion.Render(fixedWidth(reasonText, colReasonFY))

	line2 := "  " + descCell + " " + reasonCell

	if selected {
		w := lipgloss.Width(line2)
		if w < m.width {
			line2 += strings.Repeat(" ", m.width-w)
		}
		line2 = selectedRowStyle.Render(line2)
	}

	return line1 + "\n" + line2
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

		if pending == 0 && !m.packInstalling {
			b.WriteString("\n  " + dim("Esc") + " back")
		}

		return b.String()
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

	if installed < len(pack.ToolNames) {
		b.WriteString("  " + nameStyle.Render("i to install missing tools") + "\n")
	} else {
		b.WriteString("  " + upToDateStyle.Render("All tools in this pack are installed! ✓") + "\n")
	}
	if installed > 0 {
		b.WriteString("  " + dim("x to remove installed tools") + "\n")
	}

	// Footer.
	footer := "  " + dim("↑↓") + " navigate   " + dim("Enter") + " tool detail   " + dim("i") + " install   " + dim("x") + " remove   " + dim("Esc") + " back"
	return m.layoutWithFooter(b.String(), footer)
}

// --- Header ---

func (m Model) renderHeader() string {
	switch m.activeTab {
	case tabInstalled, tabFavorites:
		return "  " +
			headerStyle.Render(fixedWidth("TOOL", colName)) + "  " +
			headerStyle.Render(fixedWidth("VERSION", colVersion)) + "  " +
			headerStyle.Render(fixedWidth("SOURCE", colSource)) + "  " +
			headerStyle.Render(fixedWidth("CATEGORY", colCategory))
	case tabUpdates:
		return "      " +
			headerStyle.Render(fixedWidth("TOOL", colName)) + "  " +
			headerStyle.Render(fixedWidth("UPDATE", colVersion)) + "  " +
			headerStyle.Render(fixedWidth("SOURCE", colSource)) + "  " +
			headerStyle.Render(fixedWidth("CATEGORY", colCategory))
	case tabDiscover:
		return "  " +
			headerStyle.Render(fixedWidth("TOOL", colName)) + "  " +
			headerStyle.Render(fixedWidth("CATEGORY", colCategory)) + "  " +
			headerStyle.Render(fixedWidth("STATUS", colStars))
	case tabBackup:
		return "    " +
			headerStyle.Render(fixedWidth("TOOL", colName)) + "  " +
			headerStyle.Render(fixedWidth("STATUS", colStatus)) + "  " +
			headerStyle.Render(fixedWidth("SOURCE", colSource))
	}
	return ""
}

// --- Row rendering per tab ---

func (m Model) renderRow(tool registry.Tool, toolIdx int, selected bool) string {
	var line string

	switch m.activeTab {
	case tabInstalled, tabFavorites:
		line = m.renderInstalledRow(tool, selected)
	case tabUpdates:
		line = m.renderUpdateRow(tool, toolIdx, selected)
	case tabDiscover:
		line = m.renderDiscoverRow(tool, selected)
	}

	// Star indicator for favorited tools — inserted after cursor prefix,
	// before the name. Keeps ▸ cursor visible for selected rows.
	if m.favoriteNames[tool.Name] {
		// Row format is "▸ NAME..." or "  NAME...". Replace the space
		// after cursor with a styled star.
		runes := []rune(line)
		if len(runes) >= 2 {
			// runes[0] = cursor char (▸ or space), runes[1] = space
			line = string(runes[0:1]) + upgradableStyle.Render("★") + string(runes[2:])
		}
	}

	if selected {
		// Pad to tool column width (not full terminal width) so the selection
		// highlight doesn't bleed into the sidebar column.
		padWidth := m.width
		hasSidebar := len(m.sidebarItems) > 0
		if hasSidebar {
			padWidth = m.width - colSidebar - 3
		}
		w := lipgloss.Width(line)
		if w < padWidth {
			line += strings.Repeat(" ", padWidth-w)
		}
		line = selectedRowStyle.Render(line)
	}

	return line
}

func (m Model) renderInstalledRow(tool registry.Tool, selected bool) string {
	cursor := "  "
	if selected {
		cursor = "▸ "
	}

	// Name column: plain text padded, then styled.
	nameText := toolLabel(tool)
	nameCell := nameStyle.Render(fixedWidth(nameText, colName))

	// Version column: styled version info, then pad to fixed width.
	verCell := fixedWidthANSI(m.versionInfoStyled(tool), colVersion)

	// Source column.
	src := ""
	if primary := tool.PrimaryInstance(); primary != nil {
		src = string(primary.Source)
	}
	srcCell := sourceStyle.Render(fixedWidth(src, colSource))
	catCell := categoryStyle.Render(fixedWidth(tool.Category, colCategory))

	line := cursor + nameCell + "  " + verCell + "  " + srcCell + "  " + catCell

	if badge := githubStarsBadge(tool); badge != "" {
		line += "  " + fixedWidthANSI(dimVersion.Render(badge), colStars)
	}

	if len(tool.Instances) > 1 {
		line += "  " + dimVersion.Render(fmt.Sprintf("(%d instances)", len(tool.Instances)))
	}

	return line
}

func (m Model) renderUpdateRow(tool registry.Tool, toolIdx int, selected bool) string {
	cursor := "  "
	if selected {
		cursor = "▸ "
	}

	// Selection checkbox.
	check := ""
	if m.updateSelected[toolIdx] {
		check = upToDateStyle.Render("[✓]") + " "
	} else {
		check = dimVersion.Render("[ ]") + " "
	}

	nameText := toolLabel(tool)
	nameCell := nameStyle.Render(fixedWidth(nameText, colName))

	ver := tool.InstalledVersion()
	updateText := ver + " → " + tool.Latest
	verCell := fixedWidth(updateText, colVersion)

	src := ""
	if primary := tool.PrimaryInstance(); primary != nil {
		src = string(primary.Source)
	}
	srcCell := sourceStyle.Render(fixedWidth(src, colSource))
	catCell := categoryStyle.Render(fixedWidth(tool.Category, colCategory))

	line := cursor + check + nameCell + "  " + verCell + "  " + srcCell + "  " + catCell
	if badge := githubStarsBadge(tool); badge != "" {
		line += "  " + fixedWidthANSI(dimVersion.Render(badge), colStars)
	}
	return line
}

func (m Model) renderDiscoverRow(tool registry.Tool, selected bool) string {
	cursor := "  "
	if selected {
		cursor = "▸ "
	}

	nameText := toolLabel(tool)
	nameCell := dimVersion.Render(fixedWidth(nameText, colName))
	catCell := categoryStyle.Render(fixedWidth(tool.Category, colCategory))

	// Stars column: pad to a fixed width (empty string pads to colStars too) so
	// the trailing marketplace-status badge lines up across rows.
	starsText := githubStarsBadge(tool)
	starsCell := fixedWidthANSI(dimVersion.Render(starsText), colStars)

	line := cursor + nameCell + "  " + catCell + "  " + starsCell

	var badge string
	switch tool.MarketplaceStatus {
	case registry.StatusNew:
		badge = "  " + upgradableStyle.Render("NEW")
	case registry.StatusChanged:
		badge = "  " + detailTitleStyle.Render("UPDATED")
	}

	return line + badge
}

// --- Version info (plain text, no ANSI) ---

func (m Model) versionInfoStyled(tool registry.Tool) string {
	// Tool still resolving — show spinner placeholder.
	if m.phase < phaseDone && !toolResolved(tool) {
		return "…"
	}

	primary := tool.PrimaryInstance()
	ver := ""
	if primary != nil {
		ver = primary.Version
	}
	latest := tool.Latest

	if ver == "" && latest == "" {
		return "—"
	}
	if ver != "" && latest == "" {
		return ver
	}
	if ver == "" && latest != "" {
		return "— → " + latest + " ?"
	}
	if tool.HasUpdate() {
		return ver + " → " + upgradableStyle.Render(latest+" ⬆")
	}
	return ver + " " + upToDateStyle.Render("✓")
}

// --- Detail view ---

// renderDetailView renders the tool detail page. Sections (from top):
//
//  1. Hero      — name, status badge, category, description, quick stats bar
//  2. Installed — installed version vs latest, source/path, all instances,
//     recommendations (only shown when the tool is installed)
//  3. Package Managers — unified table of every declared PM with package id,
//     availability on the current host and the `install` command
//  4. About     — tags + topics (deduped), platforms, binary names
//  5. Community — GitHub repo/homepage/license, stars gauge, forks, last push
//  6. Related   — "You might also like" with match bars
//  7. Actions   — footer menu + key hints
func (m Model) renderDetailView(tool registry.Tool) string {
	body := m.renderDetailBody(tool)

	// Footer: help bar only (actions are now inline in Package Managers section).
	var footer strings.Builder

	dim := dimVersion.Render
	switch {
	case m.pendingAction != nil:
		prompt := confirmStyle.Render(fmt.Sprintf("  Run %s?", strings.Join(m.pendingAction.cmdArgs, " ")))
		keys := dim("y") + " confirm   " + dim("Esc") + " cancel"
		footer.WriteString(prompt + "  " + keys)
	default:
		hints := []string{
			dim("↑↓") + " navigate",
			dim("PgUp/PgDn") + " scroll",
			dim("Enter") + " select",
			dim("Esc") + " back",
		}
		footer.WriteString("  " + helpStyle.Render(strings.Join(hints, "   ")))
	}

	return m.layoutDetailWithScroll(body, footer.String())
}

// renderDetailBody renders the scrollable body of the tool detail page
// (everything except the footer). Extracted so computeDetailMaxScroll
// can measure actual line count.
func (m Model) renderDetailBody(tool registry.Tool) string {
	var b strings.Builder

	divider := func(title string) string {
		section := dashSection.Render
		w := m.width - lipgloss.Width(title) - 8
		if w < 4 {
			w = 4
		}
		return "  " + dashDim.Render("▸ ") + section(title) + " " + dashDim.Render(strings.Repeat("─", w)) + "\n"
	}

	b.WriteString(m.renderHeroHeader(tool))

	if tool.IsInstalled() {
		b.WriteString(divider("Installed"))
		b.WriteString(m.renderInstalledStatus(tool))
	}

	pms := m.renderPackageManagers(tool)
	if pms != "" {
		b.WriteString(divider("Package Managers"))
		b.WriteString(pms)
	}

	about := m.renderAboutSection(tool)
	if about != "" {
		b.WriteString(divider("About"))
		b.WriteString(about)
	}

	community := m.renderCommunitySection(tool)
	if community != "" {
		b.WriteString(divider("Community"))
		b.WriteString(community)
	}

	if len(m.detailRelated) > 0 {
		b.WriteString(divider("You might also like"))
		for i, r := range m.detailRelated {
			selected := i == m.detailRelCursor
			b.WriteString(m.renderRecCard(r, selected, true) + "\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

// layoutDetailWithScroll applies m.detailScroll to the rendered body so the
// tool detail view can scroll vertically, then hands off to layoutWithFooter
// for bottom-pinning of the footer. Also clamps m.detailScroll in-place via
// the returned model... except we have a value receiver on renderDetailView,
// so we clamp locally and just use the clamped value here. The next user
// input re-renders and settles any over-scroll silently.
func (m Model) layoutDetailWithScroll(body, footer string) string {
	if m.height <= 0 {
		return m.layoutWithFooter(body, footer)
	}

	// Trim trailing newline to avoid inflating line count with an empty entry.
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")

	footerRows := visualRows(footer, m.width)
	const minGap = 1
	visibleRows := m.height - footerRows - minGap
	if visibleRows < 5 {
		visibleRows = 5
	}

	// Line-based scrolling: scroll and maxScroll are in logical lines,
	// matching the unit we slice by. No visual-row / logical-line mismatch.
	maxScroll := len(lines) - visibleRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := m.detailScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}

	// Slice lines by scroll offset.
	if scroll > 0 && scroll < len(lines) {
		lines = lines[scroll:]
	}

	// Annotate the footer with scroll position when applicable.
	if scroll > 0 || maxScroll > 0 {
		pct := 0
		if maxScroll > 0 {
			pct = (scroll * 100) / maxScroll
		}
		indicator := fmt.Sprintf("[%d%%]", pct)
		footer = "  " + dimVersion.Render(indicator) + "   " + strings.TrimLeft(footer, " ")
	}

	return m.layoutWithFooter(strings.Join(lines, "\n"), footer)
}

// renderHeroHeader renders the top-of-page "hero" block: name, status badge,
// category pill, description, and an at-a-glance stats bar (stars, forks,
// license, last push).
func (m Model) renderHeroHeader(tool registry.Tool) string {
	var b strings.Builder

	// Name + alias.
	displayName := tool.DisplayName
	if displayName == "" {
		displayName = tool.Name
	}
	name := detailTitleStyle.Render(displayName)
	if tool.DisplayName != "" && tool.DisplayName != tool.Name {
		name += "  " + dimVersion.Render("("+tool.Name+")")
	}

	// Status badge.
	var badge string
	switch {
	case tool.IsInstalled() && tool.HasUpdate():
		badge = upgradableStyle.Render(" ⬆ UPDATE AVAILABLE ")
	case tool.IsInstalled():
		badge = upToDateStyle.Render(" ✓ INSTALLED ")
	default:
		badge = dashDim.Render(" ○ NOT INSTALLED ")
	}

	// Category + archived chips.
	chips := []string{badge}
	if tool.Category != "" {
		chips = append(chips, chipStyle.Render(tool.Category))
	}
	if tool.GitHubInfo != nil && tool.GitHubInfo.Archived {
		chips = append(chips, upgradableStyle.Render(" ⚠ ARCHIVED "))
	}

	b.WriteString("  " + name + "  " + strings.Join(chips, "  ") + "\n")

	// Description — readable, not dim.
	if info := tool.GitHubInfo; info != nil && info.Description != "" {
		maxW := m.width - 6
		if maxW < 20 {
			maxW = 20
		}
		b.WriteString("\n")
		for _, line := range wordWrap(info.Description, maxW) {
			b.WriteString("  " + heroDescStyle.Render(line) + "\n")
		}
	}

	// Quick stats bar: ★ stars · ⑂ forks · 📜 license · 🕒 last push.
	// Shown at the top so the most-asked-for info is above the fold.
	// (The Community section below does not repeat these.)
	if stats := m.renderQuickStats(tool); stats != "" {
		b.WriteString("\n  " + stats + "\n")
	}

	b.WriteString("\n")
	return b.String()
}

// renderQuickStats renders the single-line summary of GitHub stats. Returns
// "" when the tool has no enriched metadata.
func (m Model) renderQuickStats(tool registry.Tool) string {
	info := tool.GitHubInfo
	if info == nil {
		return ""
	}
	var parts []string
	sep := dashDim.Render(" · ")

	if info.Stars > 0 {
		parts = append(parts, upgradableStyle.Render("★ ")+dashNumber.Render(formatStars(info.Stars)))
	}
	if info.Forks > 0 {
		parts = append(parts, dashDim.Render("⑂ ")+dashNumber.Render(formatStars(info.Forks)))
	}
	if info.License != "" {
		parts = append(parts, dashDim.Render("📜 ")+dimVersion.Render(info.License))
	}
	if d := formatGitHubDate(info.PushedAt); d != "" {
		parts = append(parts, dashDim.Render("🕒 ")+dimVersion.Render(d))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, sep)
}

// renderInstalledStatus renders the "Installed" section: primary version vs
// latest, source and path, and (when present) the list of additional instances
// plus actionable recommendations.
func (m Model) renderInstalledStatus(tool registry.Tool) string {
	var b strings.Builder
	label := detailLabelStyle.Render
	dim := dimVersion.Render

	ver := tool.InstalledVersion()
	if ver == "" {
		ver = "—"
	}

	line := "  " + label(fixedWidth("Version", 14)) + dashNumber.Render(ver)
	if tool.Latest != "" {
		switch {
		case registry.VersionsMatch(ver, tool.Latest):
			line += "  " + upToDateStyle.Render("✓ latest")
		case tool.HasUpdate():
			line += "  " + dashDim.Render("→") + "  " + upgradableStyle.Render(tool.Latest) + "  " + upgradableStyle.Render("available")
		}
		if tool.LatestFrom != "" {
			line += "  " + dim("via "+tool.LatestFrom)
		}
	}
	b.WriteString(line + "\n")

	if primary := tool.PrimaryInstance(); primary != nil {
		b.WriteString("  " + label(fixedWidth("Source", 14)) + sourceStyle.Render(string(primary.Source)) + "\n")
		b.WriteString("  " + label(fixedWidth("Path", 14)) + dim(registry.TruncatePath(primary.Path, m.width-20)) + "\n")
	}
	b.WriteString("\n")

	// Multiple instances.
	if len(tool.Instances) > 1 {
		b.WriteString("  " + detailLabelStyle.Render(fmt.Sprintf("%d installations found", len(tool.Instances))) + "\n")
		for i, inst := range tool.Instances {
			bullet := dashDim.Render("○")
			if i == 0 {
				bullet = upToDateStyle.Render("●")
			}
			instVer := inst.Version
			if instVer == "" {
				instVer = "—"
			}
			fmt.Fprintf(&b, "  %s  %s  %s  %s\n",
				bullet,
				nameStyle.Render(fixedWidth(instVer, 14)),
				sourceStyle.Render(fixedWidth(string(inst.Source), 8)),
				dim(registry.TruncatePath(inst.Path, m.width-36)),
			)
		}
		b.WriteString("\n")
		b.WriteString(m.renderInstanceRecommendations(tool))
	}
	return b.String()
}

// renderPackageManagers renders a unified view of every declared package
// manager: availability dot, PM name, and package id.
// Interactive: toolMenu cursor navigates PM rows. Enter to install/upgrade, x to remove.
func (m Model) renderPackageManagers(tool registry.Tool) string {
	pkgs := collectPackageEntries(tool.Packages)
	if len(pkgs) == 0 {
		return ""
	}

	avail := make(map[string]bool, len(registry.AllPMStatusForOS()))
	for _, pm := range registry.AllPMStatusForOS() {
		avail[string(pm.Source)] = pm.Available
	}

	interactive := len(m.toolMenuItems) > 0

	// Build installed source set for showing action hints.
	installedSources := make(map[string]bool)
	if tool.IsInstalled() {
		for _, inst := range tool.Instances {
			installedSources[string(inst.Source)] = true
		}
	}

	var b strings.Builder
	pmIdx := 0 // tracks index into m.toolMenuItems (only available PMs)

	for _, p := range pkgs {
		isAvailable, knownPM := avail[p.source]
		// Skip PMs not available on PATH.
		if knownPM && !isAvailable {
			continue
		}
		if !knownPM {
			continue // PM not applicable to this OS
		}

		// Bullet color: green = installed via this PM, orange = PM available but not used.
		bullet := upgradableStyle.Render("●") // orange — available but not installed via this PM
		if installedSources[p.source] {
			bullet = upToDateStyle.Render("●") // green — installed via this PM
		}

		cursor := "  "
		if interactive && pmIdx == m.toolMenu {
			cursor = "▸ "
		}

		pmName := sourceStyle.Render(fixedWidth(p.source, 8))
		pkgID := nameStyle.Render(p.id)

		// Action hints on the selected row.
		hint := ""
		if interactive && pmIdx == m.toolMenu {
			if tool.IsInstalled() && pmIdx < len(m.toolMenuItems) {
				item := m.toolMenuItems[pmIdx]
				var actions []string
				if item.picker != nil {
					if item.picker.action == actionUpgrade {
						actions = append(actions, dimVersion.Render("Enter")+" upgrade")
					} else {
						actions = append(actions, dimVersion.Render("Enter")+" install")
					}
				}
				if item.removePicker != nil {
					actions = append(actions, dimVersion.Render("x")+" remove")
				}
				if len(actions) > 0 {
					hint = "  " + strings.Join(actions, "  ")
				}
			} else {
				hint = "  " + dimVersion.Render("Enter") + " install"
			}
		}

		line := cursor + bullet + "  " + pmName + "  " + pkgID + hint

		if interactive && pmIdx == m.toolMenu {
			w := lipgloss.Width(line)
			if w < m.width {
				line += strings.Repeat(" ", m.width-w)
			}
			line = selectedRowStyle.Render(line)
		}

		b.WriteString(line + "\n")
		pmIdx++
	}
	b.WriteString("\n")

	return b.String()
}

// renderAboutSection renders consolidated metadata: binary names, platforms,
// and a deduped list of tags + GitHub topics.
func (m Model) renderAboutSection(tool registry.Tool) string {
	var b strings.Builder
	label := detailLabelStyle.Render
	dim := dimVersion.Render

	// Binaries.
	if len(tool.BinaryNames) > 0 {
		b.WriteString("  " + label(fixedWidth("Binaries", 14)) + dim(strings.Join(tool.BinaryNames, ", ")) + "\n")
	}

	// Platforms as colored pills (highlighted for the current OS).
	if platforms := derivePlatforms(tool.Packages); len(platforms) > 0 {
		current := currentOSLabel()
		line := "  " + label(fixedWidth("Platforms", 14))
		for i, p := range platforms {
			pill := chipStyle.Render(p)
			if p == current {
				pill = chipAccentStyle.Render(p + " (this host)")
			}
			line += pill
			if i < len(platforms)-1 {
				line += " "
			}
		}
		b.WriteString(line + "\n")
	}

	// Tags + topics (deduped, case-insensitive).
	if labels := combineTagsAndTopics(tool); len(labels) > 0 {
		line := "  " + label(fixedWidth("Tags", 14))
		for _, t := range labels {
			pill := chipStyle.Render(t)
			pillW := lipgloss.Width(pill) + 1
			if lipgloss.Width(line)+pillW > m.width-4 {
				b.WriteString(line + "\n")
				line = "  " + strings.Repeat(" ", 14)
			}
			line += pill + " "
		}
		b.WriteString(line + "\n")
	}

	if b.Len() > 0 {
		b.WriteString("\n")
	}
	return b.String()
}

// renderCommunitySection renders GitHub repo URL + homepage. Counts, license
// and activity are surfaced in the hero quick-stats bar to avoid duplication.
// Returns "" when the tool has no GitHub slug.
func (m Model) renderCommunitySection(tool registry.Tool) string {
	if tool.GitHubSlug == "" && tool.GitHubInfo == nil {
		return ""
	}

	var b strings.Builder
	label := detailLabelStyle.Render
	dim := dimVersion.Render

	if url := githubRepoURL(tool.GitHubSlug); url != "" {
		b.WriteString("  " + label(fixedWidth("GitHub", 14)) + dim(url) + "\n")
	}
	if info := tool.GitHubInfo; info != nil && info.Homepage != "" {
		b.WriteString("  " + label(fixedWidth("Homepage", 14)) + dim(info.Homepage) + "\n")
	}

	if b.Len() == 0 {
		return ""
	}
	b.WriteString("\n")
	return b.String()
}

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

// renderGitHubSection renders a multi-line block with GitHub repository
// metadata for the detail view. Returns "" when the tool has no GitHub slug
// and no fetched info.
func (m Model) renderGitHubSection(tool registry.Tool) string {
	if tool.GitHubSlug == "" && tool.GitHubInfo == nil {
		return ""
	}

	var b strings.Builder
	label := detailLabelStyle.Render
	dim := dimVersion.Render

	b.WriteString("  " + detailTitleStyle.Render("GitHub") + "\n")

	slug := tool.GitHubSlug
	if url := githubRepoURL(slug); url != "" {
		b.WriteString("  " + label("Repo:       ") + dim(url) + "\n")
	}

	info := tool.GitHubInfo
	if info == nil {
		// Slug present but no enriched info (e.g. catalog predates assembly,
		// or fetch failed) — nothing else to show.
		b.WriteString("\n")
		return b.String()
	}

	if info.Archived {
		b.WriteString("  " + upgradableStyle.Render("⚠ Repository is archived (no longer maintained)") + "\n")
	}

	// Stars / forks on a single line for compactness.
	var stats []string
	if info.Stars > 0 {
		stats = append(stats, "★ "+formatStars(info.Stars)+" stars")
	}
	if info.Forks > 0 {
		stats = append(stats, "⑂ "+formatStars(info.Forks)+" forks")
	}
	if len(stats) > 0 {
		b.WriteString("  " + label("Stats:      ") + strings.Join(stats, "   ") + "\n")
	}

	if info.License != "" {
		b.WriteString("  " + label("License:    ") + info.License + "\n")
	}

	if info.Homepage != "" {
		b.WriteString("  " + label("Homepage:   ") + dim(info.Homepage) + "\n")
	}

	if len(info.Topics) > 0 {
		b.WriteString("  " + label("Topics:     ") + dim(strings.Join(info.Topics, ", ")) + "\n")
	}

	if d := formatGitHubDate(info.PushedAt); d != "" {
		b.WriteString("  " + label("Last push:  ") + dim(d) + "\n")
	}

	b.WriteString("\n")
	return b.String()
}

// packageEntry is one package-manager → package-id pairing for the detail view.
type packageEntry struct {
	source string
	id     string
}

// collectPackageEntries returns the declared package IDs for each package manager
// in a stable display order. Empty IDs are omitted.
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

// derivePlatforms infers supported operating systems from which package manager
// IDs are defined. Returns human-readable labels like "Windows", "macOS", "Linux".
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

// renderInstanceRecommendations analyzes multiple installations and gives
// actionable advice: version conflicts, stale installs, PATH priority issues.
func (m Model) renderInstanceRecommendations(tool registry.Tool) string {
	var tips []string
	primary := tool.Instances[0]

	// Find the newest version among all instances.
	newestVer := primary.Version
	newestIdx := 0
	for i, inst := range tool.Instances {
		if inst.Version != "" && inst.Version != "—" {
			if newestVer == "" || !registry.VersionsMatch(inst.Version, newestVer) {
				// Compare versions numerically to find the newest.
				if registry.CompareVersions(inst.Version, newestVer) > 0 {
					newestVer = inst.Version
					newestIdx = i
				}
			}
		}
	}

	// Tip: primary is not the newest version.
	if newestIdx != 0 && newestVer != "" && primary.Version != "" &&
		!registry.VersionsMatch(primary.Version, newestVer) {
		newer := tool.Instances[newestIdx]
		tips = append(tips, upgradableStyle.Render("⚠")+fmt.Sprintf(
			"  PATH priority: %s (%s) is active, but %s (%s) has a newer version %s",
			sourceStyle.Render(string(primary.Source)),
			primary.Version,
			sourceStyle.Render(string(newer.Source)),
			newer.Version,
			newestVer,
		))
	}

	// Tip: stale manual installs with no version.
	for _, inst := range tool.Instances[1:] {
		if inst.Source == registry.SourceManual && inst.Version == "" {
			tips = append(tips, dimVersion.Render("⚠")+fmt.Sprintf(
				"  Unknown version at %s — consider removing this stale install",
				dimVersion.Render(registry.TruncatePath(inst.Path, 50)),
			))
		}
	}

	// Tip: multiple package managers managing the same tool.
	sources := make(map[registry.InstallSource]bool)
	for _, inst := range tool.Instances {
		if inst.Source != registry.SourceManual {
			sources[inst.Source] = true
		}
	}
	if len(sources) > 1 {
		srcNames := make([]string, 0, len(sources))
		for src := range sources {
			srcNames = append(srcNames, string(src))
		}
		// Sort for stable ordering across renders (map iteration is
		// non-deterministic and would otherwise cause visible flicker as
		// the detail view re-renders during background version resolution).
		sort.Strings(srcNames)
		tips = append(tips, dimVersion.Render("💡")+fmt.Sprintf(
			"  Multiple package managers (%s) — consider standardizing on one to avoid conflicts",
			strings.Join(srcNames, ", "),
		))
	}

	// Tip: suggest removing duplicates.
	if len(tool.Instances) >= 3 {
		tips = append(tips, dimVersion.Render("💡")+fmt.Sprintf(
			"  %d installations found — consider removing unused ones to simplify your PATH",
			len(tool.Instances),
		))
	}

	if len(tips) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("  " + detailLabelStyle.Render("Recommendations:") + "\n")
	for _, tip := range tips {
		b.WriteString("    " + tip + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

// --- Backup tab ---

func (m Model) renderBackupView() string {
	var b strings.Builder

	// Pack creation wizard.
	if m.creatingPack {
		return m.renderPackCreateView()
	}

	// My Packs view.
	if m.viewingMyPacks {
		return m.renderMyPacksView()
	}

	// My Backups view.
	if m.viewingMyBackups {
		return m.renderMyBackupsView()
	}

	if m.backupMode == backupModeIdle {
		b.WriteString("\n")

		type menuItem struct {
			label string
			desc  string
		}
		items := []menuItem{
			{"Export", "Save installed tools to a manifest file"},
			{"Import", "Reinstall tools from a manifest file"},
			{"Share", "Generate a share token for chat/messaging"},
			{"Open Token", "Install tools from a share token"},
			{"Create Pack", "Build a custom pack from marketplace tools"},
			{"My Packs", "View and manage your custom packs"},
			{"My Backups", "View and restore saved backups"},
		}

		for i, item := range items {
			cursor := "  "
			if i == m.cursor {
				cursor = "▸ "
			}
			line := cursor + nameStyle.Render(fixedWidth(item.label, 12)) + "  " + dimVersion.Render(item.desc)
			if i == m.cursor {
				w := lipgloss.Width(line)
				if w < m.width {
					line += strings.Repeat(" ", m.width-w)
				}
				line = selectedRowStyle.Render(line)
			}
			b.WriteString(line + "\n")
		}

		// Pad remaining space.
		visibleRows := m.height - 8 - m.footerHeight()
		for range max(visibleRows, 0) {
			b.WriteString("\n")
		}
		return b.String()
	}

	// Share token display mode.
	if m.backupMode == backupModeShare {
		b.WriteString("\n")
		b.WriteString("  " + detailTitleStyle.Render("Share Token") + "\n\n")
		b.WriteString("  " + dimVersion.Render("Send this token via Slack, Teams, or any chat:") + "\n\n")

		// Word-wrap the token to fit the terminal width.
		maxW := m.width - 6
		if maxW < 40 {
			maxW = 40
		}
		for _, line := range wordWrap(m.sharedToken, maxW) {
			b.WriteString("  " + dimVersion.Render(line) + "\n")
		}

		b.WriteString("\n")

		// Copy button.
		if m.tokenCopied {
			b.WriteString("  " + buttonDoneStyle.Render("✓ Copied to clipboard") + "\n")
		} else {
			b.WriteString("  " + buttonStyle.Render("⎘ Copy to clipboard (c)") + "\n")
		}

		b.WriteString("\n")
		b.WriteString("  " + dimVersion.Render("Recipients can install with:") + "  " + detailCmdStyle.Render("clim open <token>") + "\n")

		// Pad remaining space.
		visibleRows := m.height - 12 - m.footerHeight()
		for range max(visibleRows, 0) {
			b.WriteString("\n")
		}
		return b.String()
	}

	// Confirm mode — show review header instead of progress bar.
	if m.backupConfirm {
		pending := 0
		selected := 0
		skipped := 0
		for _, item := range m.backupItems {
			switch item.status {
			case backupPending:
				pending++
				if item.selected {
					selected++
				}
			case backupSkipped, backupFailed:
				skipped++
			}
		}
		b.WriteString("\n")
		b.WriteString(confirmStyle.Render("  Review import plan") + "  " +
			dimVersion.Render(fmt.Sprintf("%d selected of %d to install, %d skipped", selected, pending, skipped)) + "\n\n")
	} else {
		// Show currently installing tool + progress bar.
		total := len(m.backupItems)
		if total > 0 {
			// Find running item.
			for _, item := range m.backupItems {
				if item.status == backupRunning {
					fmt.Fprintf(&b, "  %s %s (%d/%d)\n",
						upgradableStyle.Render("Installing:"),
						itemLabel(item.name, item.display),
						m.backupDone+1, total,
					)
					break
				}
			}

			frac := float64(m.backupDone) / float64(total)
			barWidth := m.width - 30
			if barWidth < 20 {
				barWidth = 20
			}
			m.backupBar.SetWidth(barWidth)
			fmt.Fprintf(&b, "  %s  %s  %d/%d\n\n",
				detailLabelStyle.Render("Progress:"),
				m.backupBar.ViewAs(frac),
				m.backupDone, total,
			)
		}
	}

	// Header.
	b.WriteString(m.renderHeader() + "\n")

	// Backup rows.
	visibleRows := m.height - 7 - m.footerHeight()
	if visibleRows < 3 {
		visibleRows = 3
	}

	start := 0
	if m.cursor >= visibleRows {
		start = m.cursor - visibleRows + 1
	}

	for vi := start; vi < len(m.backupItems) && vi < start+visibleRows; vi++ {
		item := m.backupItems[vi]
		selected := vi == m.cursor
		b.WriteString(m.renderBackupRow(item, selected, m.backupConfirm) + "\n")
	}

	// Pad.
	rendered := min(len(m.backupItems)-start, visibleRows)
	for range max(visibleRows-rendered, 0) {
		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) renderBackupRow(item backupItem, selected bool, confirmMode bool) string {
	cursor := "  "
	if selected {
		cursor = "▸ "
	}

	// Status icon (fixed 2-char width: icon + space).
	var icon string
	var statusLabel string
	var statusStyle lipgloss.Style
	switch item.status {
	case backupPending:
		if confirmMode {
			if item.selected {
				icon = upToDateStyle.Render("✓ ")
			} else {
				icon = dimVersion.Render("· ")
			}
		} else {
			icon = dimVersion.Render("○ ")
		}
		statusLabel = "pending"
		statusStyle = dimVersion
	case backupRunning:
		icon = upgradableStyle.Render("◉ ")
		statusLabel = "installing"
		statusStyle = upgradableStyle
	case backupDone:
		icon = upToDateStyle.Render("✓ ")
		statusLabel = "done"
		statusStyle = upToDateStyle
	case backupFailed:
		icon = upgradableStyle.Render("✗ ")
		if item.errMsg != "" {
			statusLabel = item.errMsg
		} else {
			statusLabel = "failed"
		}
		statusStyle = upgradableStyle
	case backupSkipped:
		icon = dimVersion.Render("– ")
		if item.errMsg != "" {
			statusLabel = item.errMsg
		} else {
			statusLabel = "skipped"
		}
		statusStyle = dimVersion
	}

	nameCell := nameStyle.Render(fixedWidth(itemLabel(item.name, item.display), colName))
	statusCell := statusStyle.Render(fixedWidth(statusLabel, colStatus))
	sourceCell := sourceStyle.Render(fixedWidth(item.source, colSource))

	line := cursor + icon + nameCell + "  " + statusCell + "  " + sourceCell

	if selected {
		// Pad to full width for selection highlight.
		w := lipgloss.Width(line)
		if w < m.width {
			line += strings.Repeat(" ", m.width-w)
		}
		line = selectedRowStyle.Render(line)
	}

	// Show attempted command for failed items when selected.
	if selected && item.status == backupFailed && len(item.cmdArgs) > 0 {
		line += "\n    " + dimVersion.Render("→ "+strings.Join(item.cmdArgs, " "))
	}

	return line
}

// --- Config tab ---

func (m Model) renderConfigView() string {
	var b strings.Builder
	label := detailLabelStyle.Render
	dim := dimVersion.Render

	// Version info.
	b.WriteString("\n")
	fmt.Fprintf(&b, "  %s  %s\n", label(fixedWidth("Version", 18)), build.Info())
	fmt.Fprintf(&b, "  %s  %s / %s\n", label(fixedWidth("OS / Arch", 18)), runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "  %s  %s\n", label(fixedWidth("Go", 18)), runtime.Version())

	// File paths.
	b.WriteString("\n")
	configPath := dim("(unknown)")
	if p, err := config.Path(); err == nil {
		configPath = p
	}
	fmt.Fprintf(&b, "  %s  %s\n", label(fixedWidth("Config", 18)), configPath)

	logPath := logging.Path()
	if logPath == "" {
		logPath = dim("(unavailable)")
	}
	fmt.Fprintf(&b, "  %s  %s\n", label(fixedWidth("Log", 18)), logPath)

	// Last scan time.
	scanTime := dim("(never)")
	if p, err := scancache.Path(); err == nil {
		if info, err := os.Stat(p); err == nil {
			ago := time.Since(info.ModTime())
			scanTime = info.ModTime().Format("2006-01-02 15:04:05")
			switch {
			case ago < time.Minute:
				scanTime += dim("  (just now)")
			case ago < time.Hour:
				scanTime += dim(fmt.Sprintf("  (%d min ago)", int(ago.Minutes())))
			case ago < 24*time.Hour:
				scanTime += dim(fmt.Sprintf("  (%d hours ago)", int(ago.Hours())))
			default:
				scanTime += dim(fmt.Sprintf("  (%d days ago)", int(ago.Hours()/24)))
			}
		}
	}
	fmt.Fprintf(&b, "  %s  %s\n", label(fixedWidth("Last Scan", 18)), scanTime)

	// Package managers.
	b.WriteString("\n")
	b.WriteString("  " + label("Package Managers") + "\n")
	for _, pm := range registry.AllPMStatusForOS() {
		icon := upgradableStyle.Render("✗")
		status := dim("not found")
		if pm.Available {
			icon = upToDateStyle.Render("✓")
			status = upToDateStyle.Render("installed")
		}
		fmt.Fprintf(&b, "    %s  %-10s %s\n", icon, string(pm.Source), status)
	}

	// Config warnings.
	if len(m.configWarnings) > 0 {
		b.WriteString("\n  " + upgradableStyle.Render("⚠ Config Warnings") + "\n\n")
		for _, w := range m.configWarnings {
			b.WriteString("  " + upgradableStyle.Render("•") + "  " + dimVersion.Render(w) + "\n")
		}
	}

	// Editable settings.
	b.WriteString(m.renderConfigEditor())

	return b.String()
}

// --- Help ---

func (m Model) renderHelp() string {
	// Confirmation mode — show prompt instead of normal help.
	if m.pendingAction != nil {
		prompt := confirmStyle.Render(fmt.Sprintf("  Run %s?", strings.Join(m.pendingAction.cmdArgs, " ")))
		keys := dimVersion.Render("y") + " confirm   " + dimVersion.Render("Esc") + " cancel"
		return prompt + "  " + keys
	}

	var parts []string

	switch m.activeTab {
	case tabBackup:
		switch {
		case m.backupMode == backupModeIdle:
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("←→") + " tab",
				dimVersion.Render("Enter") + " select",
				dimVersion.Render("q") + " quit",
			}
		case m.backupConfirm:
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("Space") + " toggle",
				dimVersion.Render("a") + " select all",
				dimVersion.Render("Enter") + " confirm",
				dimVersion.Render("Esc") + " cancel",
			}
		case m.backupMode == backupModeShare:
			parts = []string{
				dimVersion.Render("c") + " copy to clipboard",
				dimVersion.Render("Esc") + " back",
				dimVersion.Render("q") + " quit",
			}
		default:
			if m.isImportRunning() {
				parts = []string{
					dimVersion.Render("Esc") + " cancel",
				}
			} else {
				parts = []string{
					dimVersion.Render("↑↓") + " navigate",
					dimVersion.Render("←→") + " tab",
					dimVersion.Render("Esc") + " back",
					dimVersion.Render("q") + " quit",
				}
			}
		}
	case tabConfig:
		if m.configEditing {
			parts = []string{
				dimVersion.Render("Enter") + " confirm",
				dimVersion.Render("Esc") + " cancel",
			}
		} else {
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("Enter") + " edit",
				dimVersion.Render("S") + " save",
				dimVersion.Render("r") + " reset",
				dimVersion.Render("←→") + " tab",
				dimVersion.Render("q") + " quit",
			}
		}
	case tabProject:
		switch m.projectView {
		case projectViewList:
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("Enter") + " open",
				dimVersion.Render("←→") + " tab",
				dimVersion.Render("q") + " quit",
			}
		case projectViewAddTool:
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("Enter") + " add",
				dimVersion.Render("Esc") + " cancel",
				dimVersion.Render("type") + " filter",
			}
		default:
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("Enter") + " select",
				dimVersion.Render("Esc") + " back",
				dimVersion.Render("q") + " quit",
			}
		}
	case tabDashboard:
		parts = []string{
			dimVersion.Render("↑↓") + " scroll",
			dimVersion.Render("Home") + " top",
			dimVersion.Render("←→") + " tab",
			dimVersion.Render("r") + " refresh",
			dimVersion.Render("q") + " quit",
		}
	case tabFavorites:
		if m.favClearConfirm {
			parts = []string{
				dimVersion.Render("y") + " confirm",
				dimVersion.Render("n/Esc") + " cancel",
			}
		} else if m.favMode == "share" && m.sharedToken != "" {
			parts = []string{
				dimVersion.Render("c") + " copy to clipboard",
				dimVersion.Render("Esc") + " back",
				dimVersion.Render("q") + " quit",
			}
		} else {
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("*") + " unfavorite",
				dimVersion.Render("e") + " export",
				dimVersion.Render("s") + " share",
				dimVersion.Render("x") + " clear all",
				dimVersion.Render("q") + " quit",
			}
		}
	default:
		sortLabel := "s sort:name"
		if m.sortMode == sortByStars {
			sortLabel = "s sort:★"
		}
		parts = []string{
			dimVersion.Render("↑↓") + " navigate",
			dimVersion.Render("←→") + " tab",
			dimVersion.Render("*") + " favorite",
			dimVersion.Render(sortLabel),
			dimVersion.Render("Enter") + " detail",
			dimVersion.Render("f") + " filter",
			dimVersion.Render("r") + " refresh",
			dimVersion.Render("q") + " quit",
		}
		if m.activeTab == tabUpdates {
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("Space") + " toggle",
				dimVersion.Render("a") + " select all",
				dimVersion.Render("u") + " upgrade",
				dimVersion.Render("f") + " category",
				dimVersion.Render("Enter") + " detail",
				dimVersion.Render("q") + " quit",
			}
		}
		if m.activeTab == tabDiscover && m.discoverSubTab == discoverForYou {
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("Enter") + " detail",
				dimVersion.Render("i") + " install",
				dimVersion.Render("*") + " favorite",
				dimVersion.Render("←→") + " tab",
				dimVersion.Render("q") + " quit",
			}
		}
	}

	help := helpStyle.Render("  " + strings.Join(parts, "   "))
	if m.statusMsg != "" {
		// Status bar on its own line above help keys.
		status := "  " + upgradableStyle.Render(m.statusMsg)
		return status + "\n" + help
	}
	return help
}

// --- Two-column layout builders ---

// buildSidebarLines renders the filter sidebar as a slice of fixed-width strings.
func (m Model) buildSidebarLines(maxRows int) []string {
	if len(m.sidebarItems) == 0 {
		return nil
	}

	// Render all sidebar items into a full list.
	all := make([]string, 0, len(m.sidebarItems))

	for i, item := range m.sidebarItems {
		if item.isHeader {
			// Section header.
			all = append(all, headerStyle.Render(fixedWidth(item.label, colSidebar-2)))
			continue
		}

		cursor := "  "
		if m.categoryPicker && i == m.sidebarIdx {
			cursor = "▸ "
		}

		// Highlight the currently active filter value.
		style := dimVersion
		isActive := false
		switch item.section {
		case "category":
			isActive = (item.value == "" && m.categoryFilter == "") ||
				(item.value != "" && strings.EqualFold(item.value, m.categoryFilter))
		case "tag":
			isActive = (item.value == "" && m.tagFilter == "") ||
				(item.value != "" && strings.EqualFold(item.value, m.tagFilter))
		case "platform":
			isActive = (item.value == "" && m.platformFilter == "") ||
				(item.value != "" && strings.EqualFold(item.value, m.platformFilter))
		}
		if isActive {
			style = nameStyle
		}

		label := fixedWidth(item.label, colSidebar-4)
		line := cursor + style.Render(label)

		if m.categoryPicker && i == m.sidebarIdx {
			line = selectedRowStyle.Render(fixedWidth(line, colSidebar))
		}

		all = append(all, line)
	}

	// Apply scrolling viewport to keep sidebarIdx visible.
	if len(all) <= maxRows {
		// Everything fits — pad and return.
		for len(all) < maxRows {
			all = append(all, "")
		}
		return all
	}

	// Find which rendered line corresponds to sidebarIdx.
	cursorLine := 0
	if m.categoryPicker {
		cursorLine = m.sidebarIdx // items and rendered lines are 1:1
		if cursorLine >= len(all) {
			cursorLine = len(all) - 1
		}
	}

	// Compute scroll start so cursor is visible.
	start := 0
	if cursorLine >= maxRows {
		start = cursorLine - maxRows + 1
	}
	if start+maxRows > len(all) {
		start = len(all) - maxRows
	}
	if start < 0 {
		start = 0
	}

	return all[start : start+maxRows]
}

// buildToolLines renders the header + tool rows + empty state as a slice of strings.
func (m Model) buildToolLines(maxRows int) []string {
	lines := make([]string, 0, maxRows+1)

	// Header row.
	if m.phase >= phaseResolving && len(m.filteredIndex) > 0 {
		lines = append(lines, m.renderHeader())
	} else {
		lines = append(lines, "") // blank header line for alignment
	}

	// Tool rows.
	// Header takes 1 line from maxRows, so effective data capacity is maxRows-1.
	dataRows := maxRows - 1
	if dataRows < 1 {
		dataRows = 1
	}
	start := 0
	if m.cursor >= dataRows {
		start = m.cursor - dataRows + 1
	}

	rowCount := 0
	for vi := start; vi < len(m.filteredIndex) && rowCount < dataRows; vi++ {
		toolIdx := m.filteredIndex[vi]
		tool := m.tools[toolIdx]
		selected := vi == m.cursor && !m.categoryPicker
		lines = append(lines, m.renderRow(tool, toolIdx, selected))
		rowCount++
	}

	// Empty state.
	if len(m.filteredIndex) == 0 && m.phase >= phaseDone {
		msg := ""
		noCatalog := len(m.tools) == 0
		switch m.activeTab {
		case tabInstalled:
			if noCatalog {
				msg = "No tools loaded."
			} else {
				msg = "No installed tools found."
			}
		case tabUpdates:
			if noCatalog {
				msg = "No tools loaded."
			} else {
				msg = "All tools are up to date! ✓"
			}
		case tabDiscover:
			if noCatalog {
				msg = "No tools loaded."
			} else {
				msg = "All marketplace tools are installed!"
			}
		}
		if msg != "" {
			lines = append(lines, dimVersion.Render(msg))
		}
	}

	// No padding here — layoutWithFooter handles bottom alignment.

	return lines
}

// fixedWidthANSI pads a styled string (which may contain ANSI codes) to the
// given display width using lipgloss.Width for measurement.
func fixedWidthANSI(s string, width int) string {
	w := lipgloss.Width(s)
	if w < width {
		return s + strings.Repeat(" ", width-w)
	}
	if w > width {
		return truncateANSI(s, width)
	}
	return s
}

// truncateANSI truncates a string containing ANSI escape codes to the given
// display width. It preserves escape sequences intact and tracks visible width
// incrementally via runewidth (O(n), not O(n²)).
func truncateANSI(s string, maxWidth int) string {
	if lipgloss.Width(s) <= maxWidth {
		return s
	}

	var buf strings.Builder
	visW := 0
	i := 0
	runes := []rune(s)
	truncated := false
	for i < len(runes) {
		// Detect CSI sequence: ESC [ <params> <final byte>.
		// Final byte is 0x40–0x7E. Parameter/intermediate bytes are 0x20–0x3F.
		if runes[i] == '\x1b' && i+1 < len(runes) && runes[i+1] == '[' {
			buf.WriteRune(runes[i]) // ESC
			i++
			buf.WriteRune(runes[i]) // [
			i++
			// Copy parameter and intermediate bytes (0x20–0x3F), then final byte (0x40–0x7E).
			for i < len(runes) {
				buf.WriteRune(runes[i])
				if runes[i] >= 0x40 && runes[i] <= 0x7E {
					i++
					break
				}
				i++
			}
			continue
		}

		rw := runewidth.RuneWidth(runes[i])
		if visW+rw > maxWidth {
			truncated = true
			break
		}
		buf.WriteRune(runes[i])
		visW += rw
		i++
	}

	// Always append an explicit ANSI reset when truncation occurred
	// to prevent style bleed into subsequent content.
	if truncated {
		buf.WriteString("\x1b[0m")
	}

	return buf.String()
}

// --- Helpers ---

func toolResolved(tool registry.Tool) bool {
	if tool.Latest != "" || tool.LatestFrom != "" {
		return true
	}
	for _, inst := range tool.Instances {
		if inst.Version != "" {
			return true
		}
	}
	return false
}

// toolLabel returns the tool's short name for list rows.
func toolLabel(tool registry.Tool) string {
	return tool.Name
}

// relatedTools returns up to 5 not-installed tools that share tags with the given tool,
// enriched with display metadata.
func (m Model) relatedTools(tool registry.Tool) []recommendation {
	if len(tool.Tags) == 0 {
		return nil
	}
	tagSet := make(map[string]struct{}, len(tool.Tags))
	for _, tag := range tool.Tags {
		tagSet[tag] = struct{}{}
	}

	var recs []recommendation
	maxScore := 0
	for i, t := range m.tools {
		if t.Name == tool.Name || t.IsInstalled() {
			continue
		}
		score := 0
		for _, tag := range t.Tags {
			if _, ok := tagSet[tag]; ok {
				score++
			}
		}
		if score == 0 {
			continue
		}
		desc := ""
		stars := 0
		if t.GitHubInfo != nil {
			desc = t.GitHubInfo.Description
			stars = t.GitHubInfo.Stars
		}
		recs = append(recs, recommendation{
			toolIdx:     i,
			score:       score,
			category:    t.Category,
			description: desc,
			stars:       stars,
		})
		if score > maxScore {
			maxScore = score
		}
	}

	sort.Slice(recs, func(i, j int) bool {
		if recs[i].score != recs[j].score {
			return recs[i].score > recs[j].score
		}
		return m.tools[recs[i].toolIdx].Name < m.tools[recs[j].toolIdx].Name
	})

	if len(recs) > 5 {
		recs = recs[:5]
	}

	// Compute matchPct.
	if maxScore > 0 {
		for i := range recs {
			recs[i].matchPct = recs[i].score * 100 / maxScore
			if recs[i].matchPct < 1 {
				recs[i].matchPct = 1
			}
		}
	}
	return recs
}

// itemLabel returns display if non-empty, otherwise name.
func itemLabel(name, display string) string {
	if display != "" {
		return display
	}
	return name
}

// fixedWidth pads or truncates a plain string to exactly `width` display columns.
// Uses runewidth to correctly handle CJK characters and emoji (which occupy
// two columns). Must be called BEFORE applying lipgloss styles, not after.
func fixedWidth(s string, width int) string {
	if width <= 0 {
		return s
	}
	sw := runewidth.StringWidth(s)
	if sw > width {
		if width <= 1 {
			return "…"
		}
		return runewidth.Truncate(s, width-1, "") + "…"
	}
	if sw < width {
		return s + strings.Repeat(" ", width-sw)
	}
	return s
}
