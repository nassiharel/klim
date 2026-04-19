package tui

import (
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"

	"github.com/nassiharel/clim/internal/build"
	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/logging"
	"github.com/nassiharel/clim/internal/registry"
)

const (
	colName      = 28 // width for name column
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
	body.WriteString(m.renderSearchBar() + "\n")

	// Marketplace sub-tab bar.
	if m.activeTab == tabDiscover {
		body.WriteString(m.renderDiscoverSubTabs() + "\n")
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
	visibleRows := m.height - 8
	if visibleRows < 3 {
		visibleRows = 3
	}

	sidebarLines := m.buildSidebarLines(visibleRows)
	toolLines := m.buildToolLines(visibleRows)

	totalLines := max(len(sidebarLines), len(toolLines))
	sidebarOnRight := m.cfg != nil && m.cfg.UI.SidebarRight

	for i := range totalLines {
		left := ""
		if i < len(sidebarLines) {
			left = sidebarLines[i]
		}
		right := ""
		if i < len(toolLines) {
			right = toolLines[i]
		}

		if sidebarOnRight {
			body.WriteString(right + " │ " + left + "\n")
		} else {
			body.WriteString(fixedWidthANSI(left, colSidebar) + " │ " + right + "\n")
		}
	}

	return m.layoutWithFooter(body.String(), m.renderHelp())
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
		{"Updates", tabUpdates},
		{"Marketplace", tabDiscover},
		{"Backup", tabBackup},
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

// renderSearchBar renders the search box.
// Press / to focus the search box. Press f to focus the filter sidebar.
func (m Model) renderSearchBar() string {
	var b strings.Builder

	// Search input.
	switch {
	case m.filtering:
		b.WriteString("  " + filterPromptStyle.Render("/") + " " + m.filterInput.View())
	case m.filterText != "":
		b.WriteString("  " + filterPromptStyle.Render("/") + " " + dimVersion.Render(m.filterText))
	default:
		b.WriteString("  " + dimVersion.Render("/ search..."))
	}

	return b.String()
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
			parts = append(parts, dimVersion.Render(l.name))
		}
	}
	return "  " + strings.Join(parts, "  ")
}

// renderPacksList renders the list of packs for the Packs sub-tab.
func (m Model) renderPacksList() string {
	var b strings.Builder

	if len(m.packs) == 0 {
		b.WriteString("\n  " + dimVersion.Render("No packs available.") + "\n")
		return b.String()
	}

	// Header.
	b.WriteString("  " +
		headerStyle.Render(fixedWidth("PACK", colName)) + "  " +
		headerStyle.Render(fixedWidth("TOOLS", colPackTools)) + "  " +
		headerStyle.Render("STATUS") + "\n")

	toolMap := make(map[string]bool, len(m.tools))
	for _, t := range m.tools {
		if t.IsInstalled() {
			toolMap[t.Name] = true
		}
	}

	visibleRows := m.height - 12
	if visibleRows < 3 {
		visibleRows = 3
	}
	start := 0
	if m.cursor >= visibleRows {
		start = m.cursor - visibleRows + 1
	}

	for vi := start; vi < len(m.packs) && vi < start+visibleRows; vi++ {
		pack := m.packs[vi]
		selected := vi == m.cursor

		cursor := "  "
		if selected {
			cursor = "▸ "
		}

		nameCell := nameStyle.Render(fixedWidth(pack.DisplayName, colName))
		toolCount := fixedWidth(fmt.Sprintf("%d tools", len(pack.ToolNames)), colPackTools)

		// Compute install status.
		installed := 0
		for _, name := range pack.ToolNames {
			if toolMap[name] {
				installed++
			}
		}
		var status string
		switch {
		case len(pack.ToolNames) == 0:
			status = dimVersion.Render("empty pack")
		case installed == len(pack.ToolNames):
			status = upToDateStyle.Render("✓ installed")
		case installed > 0:
			status = dimVersion.Render(fmt.Sprintf("%d/%d installed", installed, len(pack.ToolNames)))
		default:
			status = dimVersion.Render("not installed")
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

// renderForYouList renders the smart recommendations for the For You sub-tab.
func (m Model) renderForYouList() string {
	var b strings.Builder

	if len(m.recommendations) == 0 {
		b.WriteString("\n  " + dimVersion.Render("No recommendations — install some tools first!") + "\n")
		return b.String()
	}

	// Header.
	b.WriteString("  " +
		headerStyle.Render(fixedWidth("TOOL", colName)) + "  " +
		headerStyle.Render(fixedWidth("MATCH", 5)) + "  " +
		headerStyle.Render(fixedWidth("BECAUSE YOU HAVE", colReason)) + "  " +
		headerStyle.Render("STATUS") + "\n")

	visibleRows := m.height - 12
	if visibleRows < 3 {
		visibleRows = 3
	}
	start := 0
	if m.cursor >= visibleRows {
		start = m.cursor - visibleRows + 1
	}

	// Find max score for relative bar sizing.
	maxScore := 1
	for _, rec := range m.recommendations {
		if rec.score > maxScore {
			maxScore = rec.score
		}
	}

	for vi := start; vi < len(m.recommendations) && vi < start+visibleRows; vi++ {
		rec := m.recommendations[vi]
		if rec.toolIdx >= len(m.tools) {
			continue
		}
		tool := m.tools[rec.toolIdx]
		selected := vi == m.cursor

		cursor := "  "
		if selected {
			cursor = "▸ "
		}

		nameCell := nameStyle.Render(fixedWidth(tool.Name, colName))

		// Score bar: scale to 5 chars max.
		barLen := (rec.score * 5) / maxScore
		if barLen < 1 {
			barLen = 1
		}
		bar := upgradableStyle.Render(strings.Repeat("█", barLen) + strings.Repeat("░", 5-barLen))

		reasonCell := dimVersion.Render(fixedWidth(rec.reason, colReason))

		line := cursor + nameCell + "  " + bar + "  " + reasonCell
		if badge := githubStarsBadge(tool); badge != "" {
			line += "  " + fixedWidthANSI(dimVersion.Render(badge), colStars)
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

	// Pad remaining rows.
	rendered := max(min(len(m.recommendations)-start, visibleRows), 0)
	for range max(visibleRows-rendered, 0) {
		b.WriteString("\n")
	}

	return b.String()
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

	toolMap := make(map[string]bool, len(m.tools))
	toolByName := make(map[string]registry.Tool, len(m.tools))
	for _, t := range m.tools {
		toolByName[t.Name] = t
		if t.IsInstalled() {
			toolMap[t.Name] = true
		}
	}

	installed := 0
	for _, name := range pack.ToolNames {
		isInstalled := toolMap[name]
		if isInstalled {
			installed++
		}
		icon := dim("○")
		status := dim("not installed")
		if isInstalled {
			icon = upToDateStyle.Render("✓")
			status = upToDateStyle.Render("installed")
		}
		nameCell := fixedWidth(name, colPackName)
		statusCell := fixedWidthANSI(status, colPackStat)
		line := "    " + icon + "  " + nameCell + "  " + statusCell
		if t, ok := toolByName[name]; ok {
			if badge := githubStarsBadge(t); badge != "" {
				line += "  " + fixedWidthANSI(dim(badge), colStars)
			}
		}
		b.WriteString(line + "\n")
	}

	// Actions.
	if len(pack.ToolNames) == 0 {
		b.WriteString("  " + dim("This pack has no tools defined.") + "\n\n")
		hints := dim("Esc") + " back"
		b.WriteString("  " + helpStyle.Render(hints))
		return b.String()
	}

	fmt.Fprintf(&b, "\n  %s  %d/%d installed\n\n", label("Status:"), installed, len(pack.ToolNames))

	if installed < len(pack.ToolNames) {
		b.WriteString("  " + nameStyle.Render("Press Enter or i to install missing tools") + "\n")
	} else {
		b.WriteString("  " + upToDateStyle.Render("All tools in this pack are installed! ✓") + "\n")
	}
	if installed > 0 {
		b.WriteString("  " + dim("Press x to remove installed tools") + "\n")
	}
	b.WriteString("\n")

	// Help.
	hints := dim("Enter/i") + " install   " + dim("x") + " remove   " + dim("Esc") + " back"
	b.WriteString("  " + helpStyle.Render(hints))

	return b.String()
}

// --- Header ---

func (m Model) renderHeader() string {
	switch m.activeTab {
	case tabInstalled:
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
	case tabInstalled:
		line = m.renderInstalledRow(tool, selected)
	case tabUpdates:
		line = m.renderUpdateRow(tool, toolIdx, selected)
	case tabDiscover:
		line = m.renderDiscoverRow(tool, selected)
	}

	if selected {
		w := lipgloss.Width(line)
		if w < m.width {
			line += strings.Repeat(" ", m.width-w)
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

func (m Model) renderDetailView(tool registry.Tool) string {
	var b strings.Builder
	label := detailLabelStyle.Render
	dim := dimVersion.Render

	// ── Header ──────────────────────────────────────────────────
	nameLabel := tool.Name
	if tool.DisplayName != "" && !strings.EqualFold(tool.Name, tool.DisplayName) {
		nameLabel += " (" + tool.DisplayName + ")"
	}
	b.WriteString("  " + detailTitleStyle.Render(nameLabel))
	b.WriteString("  " + categoryStyle.Render(tool.Category))
	divLen := max(m.width-lipgloss.Width(nameLabel)-lipgloss.Width(tool.Category)-8, 10)
	b.WriteString("  " + strings.Repeat("─", divLen))
	b.WriteString("\n\n")

	// ── Description (word-wrapped) ──────────────────────────────
	if tool.GitHubInfo != nil && tool.GitHubInfo.Description != "" {
		maxW := m.width - 6
		if maxW < 20 {
			maxW = 20
		}
		for _, line := range wordWrap(tool.GitHubInfo.Description, maxW) {
			b.WriteString("  " + dim(line) + "\n")
		}
		b.WriteString("\n")
	} else {
		b.WriteString("  " + dim("No description available.") + "\n\n")
	}

	// ── Version & Status ────────────────────────────────────────
	if tool.IsInstalled() {
		ver := tool.InstalledVersion()
		if ver == "" {
			ver = "—"
		}
		b.WriteString("  " + label("Version:    ") + nameStyle.Render(ver))
		if tool.Latest != "" {
			if registry.VersionsMatch(ver, tool.Latest) {
				b.WriteString("  " + upToDateStyle.Render("✓ up to date"))
			} else if tool.HasUpdate() {
				b.WriteString("  " + upgradableStyle.Render("⬆ "+tool.Latest+" available"))
			}
			if tool.LatestFrom != "" {
				b.WriteString("  " + dim("(via "+tool.LatestFrom+")"))
			}
		}
		b.WriteString("\n")
	} else {
		b.WriteString("  " + label("Status:     ") + dim("Not installed") + "\n")
	}

	// ── Instances ───────────────────────────────────────────────
	if tool.IsInstalled() {
		b.WriteString("  " + label("Instances:  "))
		if len(tool.Instances) == 1 {
			b.WriteString(dim("1 installation") + "\n")
		} else {
			b.WriteString(upgradableStyle.Render(fmt.Sprintf("%d installations", len(tool.Instances))) + "\n")
		}
		for i, inst := range tool.Instances {
			bullet := "○"
			style := detailSecondary
			if i == 0 {
				bullet = "●"
				style = detailPrimary
			}
			instVer := inst.Version
			if instVer == "" {
				instVer = "—"
			}
			fmt.Fprintf(&b, "    %s  %-14s  %-8s  %s\n",
				style.Render(bullet),
				instVer,
				sourceStyle.Render(string(inst.Source)),
				dim(registry.TruncatePath(inst.Path, m.width-40)),
			)
		}
		b.WriteString("\n")

		// Smart recommendations for multiple instances.
		if len(tool.Instances) > 1 {
			b.WriteString(m.renderInstanceRecommendations(tool))
		}
	}

	// ── Supported Platforms ─────────────────────────────────────
	platforms := derivePlatforms(tool.Packages)
	if len(platforms) > 0 {
		b.WriteString("  " + label("Platforms:  ") + dim(strings.Join(platforms, ", ")) + "\n")
	}

	// ── Binary names ────────────────────────────────────────────
	if len(tool.BinaryNames) > 0 {
		b.WriteString("  " + label("Binaries:   ") + dim(strings.Join(tool.BinaryNames, ", ")) + "\n")
	}

	// ── Display name ────────────────────────────────────────────
	if tool.DisplayName != "" {
		b.WriteString("  " + label("Display:    ") + dim(tool.DisplayName) + "\n")
	}

	// ── Category ────────────────────────────────────────────────
	if tool.Category != "" {
		b.WriteString("  " + label("Category:   ") + dim(tool.Category) + "\n")
	}

	// ── Tags ────────────────────────────────────────────────────
	if len(tool.Tags) > 0 {
		b.WriteString("  " + label("Tags:       ") + dim(strings.Join(tool.Tags, ", ")) + "\n")
	}

	// ── Packages (package manager IDs) ──────────────────────────
	if pkgs := collectPackageEntries(tool.Packages); len(pkgs) > 0 {
		b.WriteString("  " + label("Packages:") + "\n")
		for _, p := range pkgs {
			fmt.Fprintf(&b, "    %-8s  %s\n",
				sourceStyle.Render(p.source),
				dim(p.id),
			)
		}
	}
	b.WriteString("\n")

	// ── GitHub repository metadata ─────────────────────────────
	b.WriteString(m.renderGitHubSection(tool))

	// ── Install / Upgrade / Remove commands ─────────────────────
	if tool.IsInstalled() {
		if primary := tool.PrimaryInstance(); primary != nil {
			if cmd := tool.Packages.UpgradeCmd(primary.Source); cmd != "" {
				b.WriteString("  " + label("Upgrade:    ") + detailCmdStyle.Render(cmd) + "\n")
			}
			if cmd := tool.Packages.RemoveCmd(primary.Source); cmd != "" {
				b.WriteString("  " + label("Remove:     ") + detailCmdStyle.Render(cmd) + "\n")
			}
		}
		b.WriteString("\n")
	}

	// Install commands for all available sources on this OS.
	installCmds := m.collectInstallCmds(tool)
	if len(installCmds) > 0 {
		b.WriteString("  " + label("Install:") + "\n")
		for _, ic := range installCmds {
			fmt.Fprintf(&b, "    %-8s  %s\n",
				sourceStyle.Render(ic.source),
				detailCmdStyle.Render(ic.cmd),
			)
		}
		b.WriteString("\n")
	}

	// ── You might also like ────────────────────────────────────
	if related := m.relatedTools(tool); len(related) > 0 {
		b.WriteString("  " + label("You might also like:") + "\n")
		maxScore := 1
		for _, r := range related {
			if r.score > maxScore {
				maxScore = r.score
			}
		}
		for _, r := range related {
			if r.toolIdx >= len(m.tools) {
				continue
			}
			rt := m.tools[r.toolIdx]
			barLen := (r.score * 5) / maxScore
			if barLen < 1 {
				barLen = 1
			}
			bar := upgradableStyle.Render(strings.Repeat("█", barLen) + strings.Repeat("░", 5-barLen))
			fmt.Fprintf(&b, "    %s %s\n",
				nameStyle.Render(fixedWidth(rt.Name, 16)),
				bar,
			)
		}
		b.WriteString("\n")
	}

	// ── Action menu ─────────────────────────────────────────────
	var footer strings.Builder
	if len(m.toolMenuItems) > 0 {
		footer.WriteString("  " + label("Actions:") + "\n")
		for i, item := range m.toolMenuItems {
			cursor := "  "
			if i == m.toolMenu {
				cursor = "▸ "
			}
			line := "  " + cursor + nameStyle.Render(item.label)
			if i == m.toolMenu {
				w := lipgloss.Width(line)
				if w < m.width {
					line += strings.Repeat(" ", m.width-w)
				}
				line = selectedRowStyle.Render(line)
			}
			footer.WriteString(line + "\n")
		}
		footer.WriteString("\n")
	}

	// ── Help bar ────────────────────────────────────────────────
	switch {
	case m.pendingAction != nil:
		prompt := confirmStyle.Render(fmt.Sprintf("  Run %s?", strings.Join(m.pendingAction.cmdArgs, " ")))
		keys := dim("y") + " confirm   " + dim("Esc") + " cancel"
		footer.WriteString(prompt + "  " + keys)
	default:
		hints := []string{
			dim("↑↓") + " navigate",
			dim("Enter") + " select",
			dim("Esc") + " back",
		}
		footer.WriteString("  " + helpStyle.Render(strings.Join(hints, "   ")))
	}

	return m.layoutWithFooter(b.String(), footer.String())
}

// installCmdEntry pairs a source label with the formatted command string.
type installCmdEntry struct {
	source string
	cmd    string
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

// collectInstallCmds returns install commands for all available sources on this OS.
func (m Model) collectInstallCmds(tool registry.Tool) []installCmdEntry {
	var entries []installCmdEntry
	for _, src := range registry.SourcesForOS() {
		if cmd := tool.Packages.InstallCmd(src); cmd != "" {
			entries = append(entries, installCmdEntry{
				source: string(src),
				cmd:    cmd,
			})
		}
	}
	return entries
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

	if pkgs.Winget != "" || pkgs.Choco != "" {
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
		visibleRows := m.height - 12
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
		visibleRows := m.height - 16
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
	visibleRows := m.height - 11
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
	case tabDashboard:
		parts = []string{
			dimVersion.Render("↑↓") + " scroll",
			dimVersion.Render("Home") + " top",
			dimVersion.Render("←→") + " tab",
			dimVersion.Render("r") + " refresh",
			dimVersion.Render("q") + " quit",
		}
	default:
		parts = []string{
			dimVersion.Render("↑↓") + " navigate",
			dimVersion.Render("←→") + " tab",
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
	}

	help := helpStyle.Render("  " + strings.Join(parts, "   "))
	if m.statusMsg != "" {
		help += "  " + upgradableStyle.Render(m.statusMsg)
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
	start := 0
	if m.cursor >= maxRows {
		start = m.cursor - maxRows + 1
	}

	rowCount := 0
	for vi := start; vi < len(m.filteredIndex) && rowCount < maxRows; vi++ {
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

	// Pad to maxRows + 1 (header + rows).
	for len(lines) < maxRows+1 {
		lines = append(lines, "")
	}

	return lines
}

// fixedWidthANSI pads a styled string (which may contain ANSI codes) to the
// given display width using lipgloss.Width for measurement.
func fixedWidthANSI(s string, width int) string {
	w := lipgloss.Width(s)
	if w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
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

// relatedTools returns up to 5 not-installed tools that share tags with the given tool.
func (m Model) relatedTools(tool registry.Tool) []recommendation {
	if len(tool.Tags) == 0 {
		return nil
	}
	tagSet := make(map[string]struct{}, len(tool.Tags))
	for _, tag := range tool.Tags {
		tagSet[tag] = struct{}{}
	}

	var recs []recommendation
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
		recs = append(recs, recommendation{toolIdx: i, score: score})
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
