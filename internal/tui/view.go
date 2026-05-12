package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"

	"github.com/nassiharel/klim/internal/recommend"
	"github.com/nassiharel/klim/internal/registry"
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

// fitToVisibleRows is the universal stabiliser for scrollable tab
// bodies. It mirrors what the My Tools two-column path has always
// done — produce a body of EXACTLY `rows` visible lines (slice when
// the content is longer, pad with blanks when shorter) so the
// layoutWithFooter footer lands at a predictable terminal row
// regardless of how much content the renderer emitted.
//
// scroll is clamped: negative scrolls land at 0; scrolls past the
// end land at max so the bottom of the content stays visible.
//
// Returns the joined string (without a trailing newline — callers
// can append one if needed) and the clamped scroll value the caller
// should write back to its model so a stale scroll never persists.
func fitToVisibleRows(content string, scroll, rows int) (body string, clampedScroll, totalLines int) {
	if rows < 1 {
		return "", 0, 0
	}
	lines := strings.Split(content, "\n")
	total := len(lines)
	maxScroll := total - rows
	if maxScroll < 0 {
		maxScroll = 0
	}
	if scroll < 0 {
		scroll = 0
	}
	if scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll > 0 && scroll < len(lines) {
		lines = lines[scroll:]
	}
	if len(lines) > rows {
		lines = lines[:rows]
	}
	for len(lines) < rows {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n"), scroll, total
}

func (m Model) renderView() string {
	if m.quitting {
		return ""
	}

	// Boot splash — full-screen Jarvis-style cold-start visual.
	// Shown while the catalog is loading; replaced by the normal
	// layout the moment the first toolset arrives.
	if m.phase == phaseLoading {
		return m.renderBootSplash()
	}

	// Plan / checkpoint browser modal — full-screen overlay. Takes
	// priority over every other view including the health fix modal.
	if m.viewingPlan {
		if m.viewingCheckpoints {
			return m.renderCheckpointBrowser()
		}
		return m.renderPlanView()
	}

	// Health fix modal — full-screen overlay; takes priority over everything else.
	if m.fixModal.Open {
		return m.renderFixModal()
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
		footer := m.renderHelp()
		footerRows := m.footerHeight()
		cfgHeaderRows := 4 + m.subtabRows()
		const cfgMinGap = 1
		visibleRows := m.height - cfgHeaderRows - footerRows - cfgMinGap
		if visibleRows < 5 {
			visibleRows = 5
		}
		// fitToVisibleRows clamps the scroll internally and returns
		// the body sized to exactly visibleRows. The receiver here
		// is a value receiver (bubbletea's View() requires it), so
		// the clamped scroll can't be written back to the model
		// from this path — key handlers in Update() do that work
		// to keep the canonical state authoritative.
		fitted, scroll, total := fitToVisibleRows(m.renderConfigView(), m.configScroll, visibleRows)
		body.WriteString(fitted)
		if scroll > 0 {
			footer = "  " + dimVersion.Render("↑/↓ scroll") + "    " + footer
		} else if total > visibleRows {
			footer = "  " + dimVersion.Render("↓ more below") + "    " + footer
		}
		return m.layoutWithFooter(body.String(), footer)
	}

	// Dashboard tab — scrollable.
	if m.activeTab == tabDashboard {
		footer := m.renderHelp()
		footerRows := m.footerHeight()
		headerRows := 4 + m.subtabRows()
		const minGap = 1
		visibleRows := m.height - headerRows - footerRows - minGap
		if visibleRows < 5 {
			visibleRows = 5
		}
		fitted, scroll, total := fitToVisibleRows(m.renderDashboardView(), m.dashboardScroll, visibleRows)
		body.WriteString(fitted)
		if scroll > 0 {
			footer = "  " + dimVersion.Render("↑/↓ scroll   Home top") + "    " + footer
		} else if total > visibleRows {
			footer = "  " + dimVersion.Render("↓ scroll down") + "    " + footer
		}
		return m.layoutWithFooter(body.String(), footer)
	}

	// Health tab — scrollable like dashboard, plus its own PATH sub-view.
	if m.activeTab == tabHealth {
		footer := m.renderHelp()
		footerRows := m.footerHeight()
		headerRows := 4 + m.subtabRows()
		const minGap = 1
		visibleRows := m.height - headerRows - footerRows - minGap
		if visibleRows < 5 {
			visibleRows = 5
		}
		fitted, scroll, total := fitToVisibleRows(m.renderHealthView(), m.healthScroll, visibleRows)
		body.WriteString(fitted)
		if scroll > 0 {
			footer = "  " + dimVersion.Render("↑/↓ scroll   Home top") + "    " + footer
		} else if total > visibleRows {
			footer = "  " + dimVersion.Render("↓ scroll down") + "    " + footer
		}
		return m.layoutWithFooter(body.String(), footer)
	}

	// Security (Doctor) tab — scrollable.
	if m.activeTab == tabDoctor {
		footer := m.renderHelp()
		footerRows := m.footerHeight()
		headerRows := 4 + m.subtabRows()
		const minGap = 1
		visibleRows := m.height - headerRows - footerRows - minGap
		if visibleRows < 5 {
			visibleRows = 5
		}
		fitted, scroll, total := fitToVisibleRows(m.renderDoctorView(), m.doctorScroll, visibleRows)
		body.WriteString(fitted)
		if scroll > 0 {
			footer = "  " + dimVersion.Render("↑/↓ scroll   Home top") + "    " + footer
		} else if total > visibleRows {
			footer = "  " + dimVersion.Render("↓ scroll down") + "    " + footer
		}
		return m.layoutWithFooter(body.String(), footer)
	}

	// My Profile tab — same exactly-N-rows pattern, plus scroll support
	// when the My Score + Env Profile sections together exceed the
	// viewport (common on smaller terminals).
	if m.activeTab == tabProfile && m.viewingEnv {
		footer := m.renderHelp()
		footerRows := m.footerHeight()
		headerRows := 4 + m.subtabRows()
		const minGap = 1
		visibleRows := m.height - headerRows - footerRows - minGap
		if visibleRows < 5 {
			visibleRows = 5
		}
		fitted, scroll, total := fitToVisibleRows(m.renderEnvSubview(), m.profileScroll, visibleRows)
		body.WriteString(fitted)
		if scroll > 0 {
			footer = "  " + dimVersion.Render("↑/↓ scroll   Home top") + "    " + footer
		} else if total > visibleRows {
			footer = "  " + dimVersion.Render("↓ scroll down") + "    " + footer
		}
		return m.layoutWithFooter(body.String(), footer)
	}

	// My Profile tab — render the env sub-view directly. We gate
	// on m.viewingEnv so transient flows that intentionally drop
	// out of the env sub-view (e.g. the apply pipeline that hands
	// off to the import progress UI) can take over the screen
	// without this tab swallowing them.
	if m.activeTab == tabProfile && m.viewingEnv {
		body.WriteString(m.renderEnvSubview())
		return m.layoutWithFooter(body.String(), m.renderHelp())
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

	// Marketplace Onboard sub-tab — role-based recommendations.
	if m.activeTab == tabDiscover && m.discoverSubTab == discoverOnboard {
		body.WriteString(m.renderOnboardList())
		return m.layoutWithFooter(body.String(), m.renderHelp())
	}

	// Two-column layout: sidebar | tool list.
	// Compute available rows dynamically based on actual footer height.
	footer := m.renderHelp()
	footerRows := m.footerHeight()
	// Overhead: title(1) + tabs(1) + rule(1) + blank(1) + search(1) + blank(1) + gap(1) + footer.
	overhead := 7 + footerRows + m.subtabRows()
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

// footerHeight returns the number of visual rows the help/status footer occupies,
// including the rule separator line above it.
func (m Model) footerHeight() int {
	return visualRows(m.renderHelp(), m.width) + 1 // +1 for rule line
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
		// Unknown height — render rule + footer without padding.
		ruleLen := 40
		rule := "  " + ruleStyle.Render(strings.Repeat("─", ruleLen))
		return body + "\n" + rule + "\n" + footer
	}

	footerRows := visualRows(footer, m.width) + 1 // +1 for rule line above footer
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
	// Subtle rule above footer.
	ruleLen := m.width - 4
	if ruleLen < 1 {
		ruleLen = 1
	}
	rule := "  " + ruleStyle.Render(strings.Repeat("─", ruleLen))
	// Layout: body (bodyRows) + gap blank rows + rule (1 row, counted in footerRows)
	// + footer (footerRows-1 rows). Total = m.height. Using `gap` (not gap-1) here
	// is the fix for the off-by-one that left the bottom row of the terminal blank
	// and made footers look "floating" on tabs whose body is shorter than the
	// viewport.
	return body + strings.Repeat("\n", gap) + rule + "\n" + footer
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
	return m.renderCyberHUD()
}

// subtabRows returns the number of additional header rows occupied by
// the parent-tab subtab strip (0 when no subtab strip is shown for the
// active parent). My Tools and My Profile each render a single-row
// subtab strip beneath the main tab bar's rule.
func (m Model) subtabRows() int {
	if isMyToolsTab(m.activeTab) {
		return 1
	}
	if m.activeTab == tabProfile {
		return 1
	}
	if m.activeTab == tabHealth {
		return 1
	}
	return 0
}

// renderTabBar draws the parent tab labels with a cyber underline
// that brightens directly beneath the active label, giving the strip
// a focus indicator without resorting to pill backgrounds for every
// tab. Below the parent strip, an active-tab subtab strip is drawn
// for parents that own subtabs (My Tools, My Profile, Health).
func (m Model) renderTabBar() string {
	parents := []struct {
		label string
		idx   int // representative tab constant
	}{
		{"My Tools", tabInstalled},
		{"Marketplace", tabDiscover},
		{"Project", tabProject},
		{"Dashboard", tabDashboard},
		{"My Profile", tabProfile},
		{"Health", tabHealth},
		{"Security", tabDoctor},
		{"Backup", tabBackup},
		{"Config", tabConfig},
	}

	curParent := parentIndex(m.activeTab)
	tabLine, ranges := buildCyberTabLine(parents, curParent)

	// Glow underline — bright cell directly under the active tab,
	// dim rule everywhere else. Gives the strip its "scanning focus"
	// without claiming the cells the active label is using.
	ruleLen := m.width - 4
	if ruleLen < 1 {
		ruleLen = 1
	}
	underline := buildCyberUnderline(ranges, curParent, ruleLen)

	bar := tabLine + "\n" + underline

	// Subtab strip — rendered with a milder accent so it visually
	// nests inside the parent.
	if isMyToolsTab(m.activeTab) {
		subs := []struct {
			label string
			tab   int
		}{
			{"Installed", tabInstalled},
			{"Updates", tabUpdates},
			{"Favorites", tabFavorites},
		}
		var subParts []string
		for _, s := range subs {
			if s.tab == m.activeTab {
				subParts = append(subParts, cyberSubtabActive(s.label))
			} else {
				subParts = append(subParts, cyberSubtabInactive(s.label))
			}
		}
		bar += "\n  " + strings.Join(subParts, "  ")
	}

	if m.activeTab == tabProfile {
		bar += "\n  " + cyberSubtabActive("Env Profile")
	}

	if m.activeTab == tabHealth {
		subs := []struct {
			label string
			idx   int
		}{
			{"Issues", healthSubIssues},
			{"PATH", healthSubPath},
		}
		var subParts []string
		for _, s := range subs {
			if s.idx == m.healthSubTab {
				subParts = append(subParts, cyberSubtabActive(s.label))
			} else {
				subParts = append(subParts, cyberSubtabInactive(s.label))
			}
		}
		bar += "\n  " + strings.Join(subParts, "  ")
	}

	return bar
}

// buildCyberTabLine renders the parent-tab labels and reports each
// label's visible column range so the underline builder can paint
// the bright slice in the right place.
//
// Returns the rendered line (with the 2-cell indent already applied)
// and a slice of (start, end) inclusive column ranges (1-based, after
// the indent) for each parent label.
func buildCyberTabLine(parents []struct {
	label string
	idx   int
}, curParent int) (string, [][2]int) {
	var b strings.Builder
	b.WriteString("  ")
	col := 2 // account for indent
	ranges := make([][2]int, len(parents))
	for i, t := range parents {
		var rendered string
		labelLen := visualLen(t.label) + 2 // padding 1 cell each side
		if i == curParent {
			rendered = cyberTabBracketStyle.Render("[") + cyberTabActiveStyle.Render(t.label) + cyberTabBracketStyle.Render("]")
			labelLen += 2 // brackets
		} else {
			rendered = " " + cyberTabInactiveStyle.Render(t.label) + " "
		}
		ranges[i] = [2]int{col, col + labelLen - 1}
		col += labelLen
		b.WriteString(rendered)
	}
	return b.String(), ranges
}

// buildCyberUnderline draws the per-cell underline strip. Cells that
// fall under the active tab's label get the bright accent; the rest
// get a dim rule. The strip starts at the same 2-cell indent the tab
// line uses so the brackets visually align.
func buildCyberUnderline(ranges [][2]int, curParent, ruleLen int) string {
	var b strings.Builder
	b.WriteString("  ")
	if curParent < 0 || curParent >= len(ranges) {
		b.WriteString(cyberTabUnderlineDimStyle.Render(strings.Repeat("─", ruleLen)))
		return b.String()
	}
	lo, hi := ranges[curParent][0], ranges[curParent][1]
	// Convert from absolute column to relative (within the rule).
	lo -= 2
	hi -= 2
	if hi >= ruleLen {
		hi = ruleLen - 1
	}
	if lo < 0 {
		lo = 0
	}
	left := strings.Repeat("─", lo)
	mid := strings.Repeat("━", hi-lo+1) // heavier bar under the active
	right := strings.Repeat("─", ruleLen-hi-1)
	b.WriteString(cyberTabUnderlineDimStyle.Render(left))
	b.WriteString(cyberTabUnderlineStyle.Render(mid))
	b.WriteString(cyberTabUnderlineDimStyle.Render(right))
	return b.String()
}

func cyberSubtabActive(label string) string {
	return cyberTabBracketStyle.Render("‹") + " " +
		lipgloss.NewStyle().Foreground(cyberPrimary).Bold(true).Render(label) + " " +
		cyberTabBracketStyle.Render("›")
}

func cyberSubtabInactive(label string) string {
	return "  " + lipgloss.NewStyle().Foreground(cyberFGDim).Render(label) + "  "
}

// --- Search Bar ---

// renderSearchBar renders the search box with a styled border.
func (m Model) renderSearchBar() string {
	var content string
	searchIcon := filterPromptStyle.Render(">")
	switch {
	case m.filtering:
		content = searchIcon + " " + m.filterInput.View()
	case m.filterText != "":
		content = searchIcon + " " + dimVersion.Render(m.filterText) +
			"  " + dimVersion.Render("(/ edit  Esc clear)")
	default:
		content = dimVersion.Render("> / search...")
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

// renderDiscoverSubTabs moved to view_discover.go.

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
			headerStyle.Render(fixedWidth("STARS", colStars)) + "  " +
			headerStyle.Render("DESCRIPTION")
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

	// Star indicator for favorited tools — replace second char of cursor
	// prefix with a styled star.
	if m.favoriteNames[tool.Name] {
		runes := []rune(line)
		if len(runes) >= 2 {
			line = string(runes[0:1]) + upgradableStyle.Render("★") + string(runes[2:])
		}
	}

	if selected {
		padWidth := m.width
		if len(m.sidebarItems) > 0 {
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

	if badge := m.complianceBadge(tool.Name); badge != "" {
		line += "  " + badge
	}

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
	if badge := m.complianceBadge(tool.Name); badge != "" {
		line += "  " + badge
	}
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
	var nameCell string
	if tool.IsInstalled() {
		nameCell = upToDateStyle.Render(fixedWidth(nameText, colName))
	} else {
		nameCell = nameStyle.Render(fixedWidth(nameText, colName))
	}

	catCell := categoryStyle.Render(fixedWidth(tool.Category, colCategory))

	// Stars column.
	starsText := githubStarsBadge(tool)
	starsCell := fixedWidthANSI(dimVersion.Render(starsText), colStars)

	// Compliance badge: only present when a policy is loaded AND the
	// tool isn't compliant. We render it before the description so it
	// stays visible even when the description is truncated for narrow
	// terminals — discovering "this would violate policy" matters more
	// than the last few words of the GitHub blurb.
	compBadge := m.complianceBadge(tool.Name)
	compWidth := lipgloss.Width(compBadge)
	compCell := ""
	if compBadge != "" {
		compCell = "  " + compBadge
	}

	// Description preview — fill remaining width.
	desc := ""
	if tool.GitHubInfo != nil && tool.GitHubInfo.Description != "" {
		desc = tool.GitHubInfo.Description
	}
	descWidth := m.width - colName - colCategory - colStars - 10 // cursor + spacing
	if compWidth > 0 {
		descWidth -= compWidth + 2 // 2 = "  " separator before badge
	}
	if len(m.sidebarItems) > 0 {
		descWidth -= colSidebar + 3
	}
	if descWidth < 10 {
		descWidth = 0
	}
	descCell := ""
	if descWidth > 0 && desc != "" {
		descCell = "  " + dimVersion.Render(fixedWidth(desc, descWidth))
	}

	line := cursor + nameCell + "  " + catCell + "  " + starsCell + compCell + descCell

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

// renderInstanceRecommendations moved to view_detail.go.
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
		case "status":
			isActive = (item.value == "" && m.statusFilter == "") ||
				(item.value != "" && m.statusFilter == item.value)
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

// relatedTools returns up to 5 not-installed tools that share tags
// with the given tool. Delegates to recommend.Related so the same
// scoring runs in the web UI's tool detail page; this method is kept
// as a thin wrapper to minimise churn at TUI call sites.
func (m Model) relatedTools(tool registry.Tool) []recommendation {
	return recommend.Related(tool, m.tools, 5)
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
