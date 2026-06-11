package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/registry"
)

// Tile-mode renderer for the My Tools / Marketplace tabs. Same
// visual language as the session-tile redesign (PR #93):
//
//   - colored rounded border signals install state
//   - bold title with a reserved 2-cell ⭐ slot so favorites don't
//     shift the title column
//   - colored package-source chip on the title row
//   - description line in dim foreground
//   - version row with current → latest chevron when an update is
//     available; right-anchored [x]/[ ] selection box on Updates tab
//
// The generic tile-layout primitives (chooseTileLayout, withGutters,
// padOrTruncTile, tile geometry constants) live in tile_layout.go.

// toolsViewMode controls whether tabInstalled / tabUpdates /
// tabFavorites / Marketplace-Tools renders as a dense table (default)
// or as a tile grid. Toggled by the `t` key on those tabs and
// remembered per-tab so the user can mix-and-match views.
type toolsViewMode int

// Tools view modes.
const (
	toolsViewList  toolsViewMode = 0 // default — dense table
	toolsViewTiles toolsViewMode = 1 // bordered tile grid
)

// next returns the view mode that follows v in the toggle cycle.
func (v toolsViewMode) next() toolsViewMode {
	if v == toolsViewList {
		return toolsViewTiles
	}
	return toolsViewList
}

// label returns the short human label for the current mode.
func (v toolsViewMode) label() string {
	if v == toolsViewTiles {
		return "tiles"
	}
	return "list"
}

// isToolsTab reports whether the given tab is one where tile mode
// applies: My Tools (Installed/Updates/Favorites) and Marketplace.
func isToolsTab(tab int) bool {
	switch tab {
	case tabInstalled, tabUpdates, tabFavorites, tabDiscover:
		return true
	}
	return false
}

// renderToolTile draws a single tool card at `width` columns.
//
// Unselected tiles have a neutral (dim) border — install state is
// signalled by the colored dot on the title row and the colored
// version line, NOT by the border. The cursor-selected tile is the
// only tile whose border is colored (bright cyan), so it pops
// without making the grid noisy.
//
// `showCheckbox` should be true only for the Updates tab — it adds a
// right-anchored [x]/[ ] selection box on the version row that
// mirrors the Space-to-select semantics of the list view.
//
// Precondition: `width >= tileMinWidth`. Callers narrower than that
// must fall back to list mode at the layout layer (buildToolTileLines
// enforces this). The renderer floors innerW at 4 so it won't panic
// on smaller widths, but the title / chip rows assume there's room
// for the full set of cells (dot + sp + star + sp + title + sp + chip).
func renderToolTile(t registry.Tool, width int, opts toolTileOpts) string {
	state := toolTileState(&t, opts.marketplaceStatus)
	borderColor := cyberFGDim
	if opts.selected {
		borderColor = cyberPrimary
	}
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(width).
		Padding(0, 1)

	// innerW is the budget inside the rounded border + 1-cell padding
	// on each side (4 cells reserved total). Never expand past the
	// caller-supplied width — earlier versions clamped innerW *up* to
	// tileMinWidth-4, which made content wider than the border on
	// narrow terminals and broke alignment / wrapped rows past the
	// card edge. Floor at 4 instead so per-row arithmetic that
	// subtracts leaders / chips doesn't go negative; the layout layer
	// (chooseTileLayout) is responsible for falling back to a single
	// column / list mode when the terminal is too narrow.
	innerW := width - 4
	if innerW < 4 {
		innerW = 4
	}

	rows := []string{
		renderToolStarsRow(&t, state, innerW),
		renderToolTitleRow(&t, opts.favorited, opts.selected, innerW),
		renderToolCategoryRow(&t, innerW),
		renderToolDescriptionRow(&t, innerW),
		renderToolVersionRow(&t, opts.showCheckbox, opts.checked, innerW),
	}
	return border.Render(strings.Join(rows, "\n"))
}

// toolTileOpts bundles the per-call options for renderToolTile. The
// caller (buildToolTileLines) knows whether the row is favorited,
// selected by cursor, etc.
type toolTileOpts struct {
	favorited    bool
	selected     bool
	showCheckbox bool // Updates tab only
	checked      bool // current state of the row's batch-select
	// marketplaceStatus is the per-tool diff label from the latest
	// market refresh — StatusNew, StatusChanged, or StatusUnchanged.
	// Replaces the old boolean `marketplaceNew` which conflated New
	// with Changed (list view shows separate NEW / UPDATED badges;
	// tile mode now matches).
	marketplaceStatus registry.MarketplaceStatus
}

// toolTileState classifies a tool into one of the visual states the
// border color maps from.
func toolTileState(t *registry.Tool, marketStatus registry.MarketplaceStatus) string {
	switch {
	case t.HasUpdate():
		return "update"
	case t.IsInstalled():
		return "installed"
	case marketStatus == registry.StatusNew:
		return "new"
	case marketStatus == registry.StatusChanged:
		return "changed"
	default:
		return "uninstalled"
	}
}

// toolStateColor returns the accent color for a tool state. Drives
// both the tile border and the version-row foreground.
func toolStateColor(state string) color.Color {
	switch state {
	case "installed":
		return cyberOK
	case "update", "changed":
		return cyberAccent
	case "new":
		return cyberPrimary
	}
	return cyberFGDim
}

// toolStateDot returns the colored state glyph for the title row.
func toolStateDot(state string) string {
	glyph := "○"
	switch state {
	case "installed":
		glyph = "●"
	case "update", "changed":
		glyph = "◐"
	case "new":
		glyph = "▲"
	}
	return lipgloss.NewStyle().Foreground(toolStateColor(state)).Render(glyph)
}

// renderToolStarsRow renders the top subtitle row: GitHub stars (when
// known) or the marketplace-new / marketplace-updated pip. Empty
// placeholder when neither.
func renderToolStarsRow(t *registry.Tool, state string, innerW int) string {
	parts := []string{}
	if t.GitHubInfo != nil && t.GitHubInfo.Stars > 0 {
		stars := lipgloss.NewStyle().Foreground(cyberAccent).
			Render("★ " + formatStars(t.GitHubInfo.Stars))
		parts = append(parts, stars)
	}
	switch state {
	case "new":
		parts = append(parts,
			lipgloss.NewStyle().Foreground(cyberPrimary).Bold(true).Render("● new"))
	case "changed":
		// Matches the list view's UPDATED badge so changed tools
		// don't masquerade as new ones.
		parts = append(parts,
			lipgloss.NewStyle().Foreground(cyberAccent).Bold(true).Render("◐ updated"))
	}
	if t.GitHubInfo != nil && t.GitHubInfo.Archived {
		warn := lipgloss.NewStyle().Foreground(cyberFGDim).Render("⌧ archived")
		parts = append(parts, warn)
	}
	if len(parts) == 0 {
		return padOrTruncTile(lipgloss.NewStyle().Foreground(cyberFGDim).Render("—"), innerW)
	}
	return padOrTruncTile(strings.Join(parts, "  "), innerW)
}

// renderToolTitleRow lays out: state dot + reserved star slot +
// bold tool name + colored package chip. The reserved slot keeps
// the title's column position identical whether starred or not.
func renderToolTitleRow(t *registry.Tool, favorited, selected bool, innerW int) string {
	dot := toolStateDot(toolTileState(t, registry.StatusUnchanged)) // dot mirrors install state, ignore new/changed flag here
	star := "  "
	if favorited {
		star = "⭐"
	}
	chip := toolPackageChip(t)
	titleStyle := lipgloss.NewStyle().Foreground(cyberFG).Bold(true)
	if selected {
		titleStyle = titleStyle.Foreground(cyberPrimary)
	}
	title := t.DisplayName
	if title == "" {
		title = t.Name
	}
	// Budget: innerW - dot(1) - sp(1) - star(2) - sp(1) - [sp(1) + chipW]
	// (the sp + chip group only counts when chip is non-empty, otherwise
	// it'd steal a column and append a trailing blank).
	chipBudget := 0
	if chip != "" {
		chipBudget = 1 + lipgloss.Width(chip)
	}
	titleW := innerW - 1 - 1 - 2 - 1 - chipBudget
	if titleW < 6 {
		titleW = 6
	}
	titleText := titleStyle.Render(padOrTruncTile(title, titleW))
	if chip == "" {
		return dot + " " + star + " " + titleText
	}
	return dot + " " + star + " " + titleText + " " + chip
}

// renderToolCategoryRow renders the category as a chip, falling back
// to the first GitHub topic when no category is set so the row keeps
// a constant height. Em-dash when both are empty.
func renderToolCategoryRow(t *registry.Tool, innerW int) string {
	text := t.Category
	if text == "" && t.GitHubInfo != nil && len(t.GitHubInfo.Topics) > 0 {
		text = t.GitHubInfo.Topics[0]
	}
	if text == "" {
		return padOrTruncTile(lipgloss.NewStyle().Foreground(cyberFGDim).Render("—"), innerW)
	}
	chip := lipgloss.NewStyle().Foreground(cyberFG).Background(cyberChipBg).Padding(0, 1).Render(text)
	return padOrTruncTile(chip, innerW)
}

// renderToolDescriptionRow renders the description line with a "▸"
// leader and dim foreground. Em-dash placeholder when missing.
func renderToolDescriptionRow(t *registry.Tool, innerW int) string {
	desc := ""
	if t.GitHubInfo != nil {
		desc = t.GitHubInfo.Description
	}
	if desc == "" {
		desc = "—"
	}
	const leader = "▸ "
	content := padOrTruncTile(desc, innerW-lipgloss.Width(leader))
	return lipgloss.NewStyle().Foreground(cyberFGDim).Render(leader + content)
}

// renderToolVersionRow renders the bottom version row. Three modes:
//   - not installed:  "latest 1.2.3"        (dim)
//   - installed:      "1.2.3"                (green)
//   - has update:     "1.2.3 → 1.3.0"       (amber)
//
// Plus an optional right-anchored "[x]" / "[ ]" checkbox for the
// Updates tab.
func renderToolVersionRow(t *registry.Tool, showCheckbox, checked bool, innerW int) string {
	var versionCell string
	switch {
	case t.HasUpdate():
		versionCell = lipgloss.NewStyle().Foreground(cyberAccent).
			Render(fmt.Sprintf("%s → %s", t.InstalledVersion(), t.Latest))
	case t.IsInstalled():
		versionCell = lipgloss.NewStyle().Foreground(cyberOK).Render(t.InstalledVersion())
	case t.Latest != "":
		versionCell = lipgloss.NewStyle().Foreground(cyberFGDim).Render("latest " + t.Latest)
	default:
		versionCell = lipgloss.NewStyle().Foreground(cyberFGDim).Render("—")
	}
	if !showCheckbox {
		return padOrTruncTile(versionCell, innerW)
	}
	box := "[ ]"
	if checked {
		box = lipgloss.NewStyle().Foreground(cyberPrimary).Bold(true).Render("[x]")
	}
	// Right-anchor the checkbox. Pad version to (innerW - boxWidth -
	// gap), then append the box.
	const gap = 2
	avail := innerW - lipgloss.Width(box) - gap
	if avail < 4 {
		avail = 4
	}
	left := padOrTruncTile(versionCell, avail)
	return left + strings.Repeat(" ", gap) + box
}

// toolPackageChip picks the first available package-manager source
// for the tool and renders it as a colored chip. Cyan when the tool
// is installed (matching how `klim` resolves the source), dim
// otherwise.
func toolPackageChip(t *registry.Tool) string {
	source := pickPackageSource(t)
	if source == "" {
		return ""
	}
	style := lipgloss.NewStyle().Foreground(cyberFGDim)
	if t.IsInstalled() {
		style = style.Foreground(cyberPrimary).Bold(true)
	}
	return style.Render("⌬ " + source)
}

// pickPackageSource returns the first non-empty package-manager name
// from the tool's Packages field. Order mirrors typical platform
// precedence (Winget on Windows first, then cross-platform sources).
func pickPackageSource(t *registry.Tool) string {
	switch {
	case t.Packages.Winget != "":
		return "winget"
	case t.Packages.Choco != "":
		return "choco"
	case t.Packages.Scoop != "":
		return "scoop"
	case t.Packages.Brew != "":
		return "brew"
	case t.Packages.Apt != "":
		return "apt"
	case t.Packages.Snap != "":
		return "snap"
	case t.Packages.NPM != "":
		return "npm"
	}
	return ""
}

// buildToolTileLines is the tile-mode counterpart to buildToolLines.
// Called from renderView when m.toolsViewMode[m.activeTab] ==
// toolsViewTiles. Renders the windowed slice of tools as a tile grid
// inside the same row budget the list mode uses.
func (m Model) buildToolTileLines(maxRows int) []string {
	if len(m.filteredIndex) == 0 {
		// Mirror buildToolLines: show the same empty-state message
		// (e.g. "No installed tools found." / "All tools are up to
		// date!") instead of a blank body. Without this, switching
		// to tile mode while a filter hid every tool produced a
		// completely empty view with no hint about why.
		lines := []string{m.renderHeader()}
		if m.phase >= phaseDone {
			if msg := m.emptyStateMessage(); msg != "" {
				lines = append(lines, dimVersion.Render(msg))
			}
		}
		return lines
	}
	// Header row (kept so the tab still looks like part of klim
	// rather than a free-floating grid).
	lines := []string{m.renderHeader()}

	// Compute layout from the available body width.
	bodyW, _ := m.bodyDims()
	tileW, cols := chooseTileLayout(bodyW)

	// Terminal too narrow for a usable tile: fall back to list
	// mode. chooseTileLayout's single-column branch returns whatever
	// the terminal width is — without this guard, narrow terminals
	// render tiles where the title row can't even fit the package
	// chip, producing wrapped rows that break the border. List mode
	// degrades gracefully via column truncation in renderRow.
	if tileW < tileMinWidth {
		return m.buildToolLines(maxRows)
	}

	// Budget the tile data rows. One tile-row = tileHeight visual
	// lines. We must also reserve room for:
	//   - the header row (already pushed above): 1 line
	//   - the "↑ N above" indicator: up to 1 line
	//   - the "↓ N below" indicator: up to 1 line
	// Without reserving the indicators, a near-full grid plus both
	// indicators overflows `maxRows` and renderView truncates the
	// last tile row mid-card. We always reserve both slots even
	// when only one (or neither) will ultimately render — the
	// trade-off is one wasted line when the cursor sits at either
	// end of the list, which is preferable to clipped tiles.
	const indicatorBudget = 2
	const headerBudget = 1
	tileRows := (maxRows - headerBudget - indicatorBudget) / tileHeight
	if tileRows < 1 {
		// Not enough vertical room for even one full tile row.
		// Previously we forced tileRows = 1 and accepted that
		// renderView would clip the card mid-render; reviewer
		// flagged that as a clearly-broken visual. Fall back to
		// list mode for the remaining row budget — it degrades
		// gracefully on tiny terminals (truncated rows instead of
		// half-rendered cards).
		return m.buildToolLines(maxRows)
	}
	maxTiles := tileRows * cols

	// Window the visible slice around the cursor — same indicator
	// scheme as buildToolLines (↑ N above / ↓ N below).
	total := len(m.filteredIndex)
	start, hiddenAbove, hiddenBelow, windowSize := windowWithIndicators(total, m.cursor, maxTiles)

	// Snap `start` to a multiple of `cols` so the grid stays
	// spatially stable as the cursor moves. windowWithIndicators
	// centres on the item index, so a cursor move that crosses a
	// row boundary can shift `start` by 1 — which makes every visible
	// tile re-flow to a different column on screen, producing the
	// "tiles reshuffle diagonally" effect the reviewer flagged. By
	// pinning the window's first index to a row boundary, navigation
	// only ever scrolls whole tile-rows.
	if cols > 1 && start%cols != 0 {
		snapped := start - (start % cols)
		// Snapping down only ever pulls the visible window earlier
		// — never past the cursor's row — so the cursor stays in
		// view. Recompute the hidden counts; the windowSize is
		// preserved (it was already capped at maxTiles).
		if snapped < 0 {
			snapped = 0
		}
		start = snapped
		hiddenAbove = start
		if start+windowSize > total {
			windowSize = total - start
		}
		hiddenBelow = total - start - windowSize
		if hiddenBelow < 0 {
			hiddenBelow = 0
		}
	}

	if hiddenAbove > 0 {
		lines = append(lines, "  "+dimVersion.Render(fmt.Sprintf("↑ %d above", hiddenAbove)))
	}

	// Render each visible tool to a tile.
	tiles := make([]string, 0, windowSize)
	for vi := start; vi < total && len(tiles) < windowSize; vi++ {
		toolIdx := m.filteredIndex[vi]
		tool := m.tools[toolIdx]
		opts := toolTileOpts{
			favorited:    m.favoriteNames[tool.Name],
			selected:     vi == m.cursor && !m.categoryPicker,
			showCheckbox: m.activeTab == tabUpdates,
			checked:      m.activeTab == tabUpdates && m.updateSelected[toolIdx],
		}
		// Tag the tile with its marketplace status (StatusNew /
		// StatusChanged / StatusUnchanged) so the renderer can
		// distinguish a brand-new tool from an updated one — list
		// view shows separate NEW / UPDATED badges and tile mode
		// now matches.
		if m.activeTab == tabDiscover {
			opts.marketplaceStatus = tool.MarketplaceStatus
		}
		tiles = append(tiles, renderToolTile(tool, tileW, opts))
	}

	// Stitch tiles into rows of `cols`.
	for s := 0; s < len(tiles); s += cols {
		end := s + cols
		if end > len(tiles) {
			end = len(tiles)
		}
		row := lipgloss.JoinHorizontal(lipgloss.Top, withGutters(tiles[s:end])...)
		lines = append(lines, strings.Split(row, "\n")...)
	}

	if hiddenBelow > 0 {
		lines = append(lines, "  "+dimVersion.Render(fmt.Sprintf("↓ %d below", hiddenBelow)))
	}

	return lines
}
