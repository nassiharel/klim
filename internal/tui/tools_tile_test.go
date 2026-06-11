package tui

// Tests for the tile-mode renderer used on the My Tools and
// Marketplace tabs. Verifies state mapping, the favorite-slot
// alignment guarantee (same regression that bit session tiles),
// the Updates-tab checkbox column, and that the toggle cycle
// stays binary.

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/registry"
)

// mkTool builds a registry.Tool covering a minimal set of fields for
// the tile renderer to exercise. State is selected via the variadic
// install / latest version pair.
func mkTool(name, installed, latest string, packages registry.PackageIDs) registry.Tool {
	tool := registry.Tool{
		Name:        name,
		DisplayName: name,
		Category:    "test",
		Packages:    packages,
		Latest:      latest,
	}
	if installed != "" {
		tool.Instances = []registry.Instance{{Path: "/fake/" + name, Version: installed, Source: registry.SourceWinget}}
	}
	return tool
}

// TestRenderToolTile_PerState pins the state classifier so the
// border / dot color stays semantic.
func TestRenderToolTile_PerState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		installed string
		latest    string
		marketSt  registry.MarketplaceStatus
		want      string
	}{
		{"not installed", "", "1.0.0", registry.StatusUnchanged, "uninstalled"},
		{"installed current", "1.0.0", "1.0.0", registry.StatusUnchanged, "installed"},
		{"installed has update", "1.0.0", "1.2.0", registry.StatusUnchanged, "update"},
		{"marketplace new", "", "", registry.StatusNew, "new"},
		// Regression: StatusChanged used to fall through to "new"
		// because the caller passed a boolean that conflated the
		// two. Pin them as distinct visual states.
		{"marketplace changed", "", "", registry.StatusChanged, "changed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := mkTool("foo", tt.installed, tt.latest, registry.PackageIDs{Winget: "Foo"})
			got := toolTileState(&tool, tt.marketSt)
			if got != tt.want {
				t.Errorf("toolTileState = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRenderToolTile_ChangedTileShowsUpdatedBadgeNotNew is the
// regression test for the "tile view mislabels marketplace-changed
// tools as new" PR comment. Before the fix, toolTileOpts had a single
// boolean `marketplaceNew` that was set for both StatusNew and
// StatusChanged, so a changed tool rendered "● new". The fix passes
// the full MarketplaceStatus through; this test asserts a Changed
// tool emits the "updated" pip and NOT the "new" pip.
func TestRenderToolTile_ChangedTileShowsUpdatedBadgeNotNew(t *testing.T) {
	t.Parallel()
	tool := mkTool("foo", "", "1.0.0", registry.PackageIDs{Winget: "Foo"})

	changed := stripANSIForTest(renderToolTile(tool, 60, toolTileOpts{
		marketplaceStatus: registry.StatusChanged,
	}))
	if !strings.Contains(changed, "updated") {
		t.Errorf("StatusChanged tile missing 'updated' pip:\n%s", changed)
	}
	if strings.Contains(changed, "● new") {
		t.Errorf("StatusChanged tile should NOT show 'new' pip:\n%s", changed)
	}

	fresh := stripANSIForTest(renderToolTile(tool, 60, toolTileOpts{
		marketplaceStatus: registry.StatusNew,
	}))
	if !strings.Contains(fresh, "new") {
		t.Errorf("StatusNew tile missing 'new' pip:\n%s", fresh)
	}
}

// TestRenderToolTile_FavoriteSlotPreservesTitleColumn is the
// regression test that pairs with the same guarantee for session
// tiles: starring a tile must not shift the title column.
func TestRenderToolTile_FavoriteSlotPreservesTitleColumn(t *testing.T) {
	t.Parallel()
	tool := mkTool("MarkerXYZ", "1.0.0", "1.0.0", registry.PackageIDs{Winget: "Foo"})
	unstarred := renderToolTile(tool, 50, toolTileOpts{favorited: false})
	starred := renderToolTile(tool, 50, toolTileOpts{favorited: true})

	// Title row is index 2 (border top + stars row + title row).
	unRow := stripANSIForTest(strings.Split(unstarred, "\n")[2])
	stRow := stripANSIForTest(strings.Split(starred, "\n")[2])

	unCol := visualColumn(unRow, "MarkerXYZ")
	stCol := visualColumn(stRow, "MarkerXYZ")
	if unCol < 0 || stCol < 0 {
		t.Fatalf("title text not found:\n unstarred: %q\n starred:   %q", unRow, stRow)
	}
	if unCol != stCol {
		t.Errorf("title visual column shifted by star: unstarred=%d starred=%d", unCol, stCol)
	}
}

// TestRenderToolTile_UpdatesCheckbox confirms the checkbox renders
// only when showCheckbox is true. Catches a regression that would
// crowd the version row with a checkbox on Installed/Favorites tabs.
func TestRenderToolTile_UpdatesCheckbox(t *testing.T) {
	t.Parallel()
	tool := mkTool("foo", "1.0.0", "1.2.0", registry.PackageIDs{Winget: "Foo"})

	withBox := renderToolTile(tool, 50, toolTileOpts{showCheckbox: true, checked: true})
	if !strings.Contains(stripANSIForTest(withBox), "[x]") {
		t.Errorf("expected [x] in tile with showCheckbox=true, checked=true:\n%s", withBox)
	}

	unchecked := renderToolTile(tool, 50, toolTileOpts{showCheckbox: true, checked: false})
	if !strings.Contains(stripANSIForTest(unchecked), "[ ]") {
		t.Errorf("expected [ ] when showCheckbox=true, checked=false")
	}

	without := renderToolTile(tool, 50, toolTileOpts{showCheckbox: false})
	if strings.Contains(stripANSIForTest(without), "[x]") || strings.Contains(stripANSIForTest(without), "[ ]") {
		t.Errorf("checkbox leaked into tile when showCheckbox=false:\n%s", without)
	}
}

// TestRenderToolTile_PackageChipReflectsFirstSource walks the
// package-source priority order. Picks first non-empty.
func TestRenderToolTile_PackageChipReflectsFirstSource(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		pkgs registry.PackageIDs
		want string
	}{
		{"winget wins", registry.PackageIDs{Winget: "X", Brew: "Y"}, "winget"},
		{"brew when no winget/choco/scoop", registry.PackageIDs{Brew: "X"}, "brew"},
		{"npm last resort", registry.PackageIDs{NPM: "X"}, "npm"},
		{"empty when no source", registry.PackageIDs{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := mkTool("foo", "", "", tt.pkgs)
			got := pickPackageSource(&tool)
			if got != tt.want {
				t.Errorf("pickPackageSource = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestToolsViewMode_NextCycle pins the toggle path so it stays
// binary.
func TestToolsViewMode_NextCycle(t *testing.T) {
	t.Parallel()
	if toolsViewList.next() != toolsViewTiles {
		t.Errorf("list → tiles")
	}
	if toolsViewTiles.next() != toolsViewList {
		t.Errorf("tiles → list")
	}
	if toolsViewList.label() != "list" || toolsViewTiles.label() != "tiles" {
		t.Errorf("labels wrong")
	}
}

// TestIsToolsTab confirms the toggle only applies to row-list tabs.
func TestIsToolsTab(t *testing.T) {
	t.Parallel()
	cases := map[int]bool{
		tabInstalled: true,
		tabFavorites: true,
		tabUpdates:   true,
		tabDiscover:  true,
		tabAgents:    false,
		tabHealth:    false,
		tabProject:   false,
	}
	for tab, want := range cases {
		if got := isToolsTab(tab); got != want {
			t.Errorf("isToolsTab(%d) = %v, want %v", tab, got, want)
		}
	}
}

// TestRenderToolTile_EachIsRightHeight pins the per-tile row count
// so the grid stays aligned. Same invariant as session tiles.
func TestRenderToolTile_EachIsRightHeight(t *testing.T) {
	t.Parallel()
	tool := mkTool("foo", "1.0.0", "1.0.0", registry.PackageIDs{Winget: "Foo"})
	out := renderToolTile(tool, 50, toolTileOpts{})
	lines := strings.Split(out, "\n")
	if len(lines) != tileHeight {
		t.Errorf("expected %d lines per tile, got %d:\n%s", tileHeight, len(lines), out)
	}
	// All lines must be the same visual width (border alignment).
	w0 := lipgloss.Width(lines[0])
	for i, ln := range lines {
		if w := lipgloss.Width(ln); w != w0 {
			t.Errorf("line %d width %d != line 0 width %d", i, w, w0)
		}
	}
}

// TestRenderToolTile_HighlightedHasDifferentBar — selected tile must
// visibly differ from idle tile (cyan border vs state color).
func TestRenderToolTile_HighlightedHasDifferentBar(t *testing.T) {
	t.Parallel()
	tool := mkTool("foo", "1.0.0", "1.0.0", registry.PackageIDs{Winget: "Foo"})
	idle := renderToolTile(tool, 50, toolTileOpts{selected: false})
	sel := renderToolTile(tool, 50, toolTileOpts{selected: true})
	if idle == sel {
		t.Error("selected tile must differ from idle tile")
	}
}

// TestBuildToolTileLines_EmptyFilteredIndexShowsMessage pins the
// fix for the "tile mode is completely blank when filters hide all
// tools" bug. Tile mode used to return []string{""} for an empty
// filtered index, bypassing the list-mode empty-state messages.
// Now it mirrors buildToolLines and renders e.g.
// "All tools are up to date! ✓" inside the tile body so the user
// understands why the grid is empty.
func TestBuildToolTileLines_EmptyFilteredIndexShowsMessage(t *testing.T) {
	t.Parallel()
	cases := []struct {
		tab     int
		hasCat  bool
		wantSub string
	}{
		{tabInstalled, true, "No installed tools found."},
		{tabUpdates, true, "All tools are up to date!"},
		{tabDiscover, true, "All marketplace tools are installed!"},
		{tabInstalled, false, "No tools loaded."},
	}
	for _, tc := range cases {
		m := Model{
			activeTab:     tc.tab,
			filteredIndex: nil,
			phase:         phaseDone,
			width:         120,
			height:        40,
		}
		if tc.hasCat {
			m.tools = []registry.Tool{mkTool("foo", "1.0.0", "1.0.0", registry.PackageIDs{Winget: "F"})}
		}
		lines := m.buildToolTileLines(20)
		joined := stripANSIForTest(strings.Join(lines, "\n"))
		if !strings.Contains(joined, tc.wantSub) {
			t.Errorf("tab=%d hasCat=%v: expected empty-state %q in output, got:\n%s",
				tc.tab, tc.hasCat, tc.wantSub, joined)
		}
	}
}

// TestRenderToolTile_NarrowWidthKeepsContentInsideBorder is the
// regression for the "innerW expanded past width-4 on narrow tiles"
// PR comment. Previously, renderToolTile clamped innerW *up* to
// tileMinWidth-4 — so a tile rendered at the minimum width still
// got content sized for a wider box, breaking alignment / wrapping
// rows past the card edge.
//
// `renderToolTile` is documented to require width >= tileMinWidth
// (callers shorter than that fall back to list mode at the layout
// level — see buildToolTileLines). The widths exercised here are
// all valid tile widths; the assertion is that content fits inside
// the border at the *exact* width supplied (no upward clamping).
func TestRenderToolTile_NarrowWidthKeepsContentInsideBorder(t *testing.T) {
	t.Parallel()
	tool := mkTool("foo-bar-baz-with-a-long-name", "1.0.0", "1.0.0",
		registry.PackageIDs{Winget: "Foo"})

	for _, width := range []int{tileMinWidth, tileMinWidth + 4, 40, tileIdealMax} {
		tile := renderToolTile(tool, width, toolTileOpts{})
		lines := strings.Split(tile, "\n")
		if len(lines) != tileHeight {
			t.Errorf("width=%d: expected %d lines, got %d:\n%s",
				width, tileHeight, len(lines), tile)
			continue
		}
		// Every rendered line must be exactly `width` cells — the
		// contract padOrTruncTile + lipgloss border enforce when
		// content fits inside innerW. A wider line means innerW
		// overflowed the border.
		for i, ln := range lines {
			if w := lipgloss.Width(ln); w != width {
				t.Errorf("width=%d line %d: visual width %d, want %d:\n%q",
					width, i, w, width, ln)
			}
		}
	}
}

// TestRenderToolTitleRow_NoTrailingBlankWhenChipMissing is the
// regression for the "title row reserves a trailing space for the
// package-source chip even when chip is empty" PR comment. The pre-
// fix renderer always appended ` + " " + chip`, which (with chip=="")
// shrank the title budget by 1 cell. The fix: when chip is empty,
// the title row uses the freed-up cell.
//
// We assert this by rendering two title rows at the same innerW —
// one with a chip, one without — and confirming the no-chip variant
// gets one extra cell for the title text.
func TestRenderToolTitleRow_NoTrailingBlankWhenChipMissing(t *testing.T) {
	t.Parallel()
	const innerW = 40
	// Use a long name so we can measure where padOrTruncTile cuts.
	const longName = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	noChipTool := mkTool(longName, "", "", registry.PackageIDs{})
	withChipTool := mkTool(longName, "", "", registry.PackageIDs{Winget: "X"})

	noChipRow := stripANSIForTest(renderToolTitleRow(&noChipTool, false, false, innerW))
	withChipRow := stripANSIForTest(renderToolTitleRow(&withChipTool, false, false, innerW))

	// Count the A's that survived truncation in each row. The
	// no-chip row should keep at least one MORE 'A' than the
	// with-chip row (the cell that used to be the trailing blank
	// is now used for title content).
	noChipAs := strings.Count(noChipRow, "A")
	withChipAs := strings.Count(withChipRow, "A")
	if noChipAs <= withChipAs {
		t.Errorf("no-chip row should fit more title text than with-chip row;\nno chip: %q (A×%d)\nwith chip: %q (A×%d)",
			noChipRow, noChipAs, withChipRow, withChipAs)
	}
	// And the chip variant must still actually render its chip — so
	// we haven't accidentally suppressed it via the no-chip branch.
	if !strings.Contains(withChipRow, "winget") {
		t.Errorf("chip should still render when Packages.Winget is set: %q", withChipRow)
	}
}

// TestBuildToolTileLines_SnapsStartToColumnBoundary is the regression
// for the "tile window reshuffles diagonally" PR comment. windowWith-
// Indicators centres start on the cursor index; in tile mode that
// means a cursor move across a row boundary can shift `start` by 1,
// reshuffling every visible tile by one column. The fix pins start
// to a multiple of `cols` (with cols > 1) so the grid is spatially
// stable as the cursor walks the list.
//
// We exercise the build indirectly via a deterministic dataset and
// assert that across consecutive cursor positions within a single
// "row" the rendered tile sequence shifts by entire rows only.
func TestBuildToolTileLines_SnapsStartToColumnBoundary(t *testing.T) {
	t.Parallel()
	// 30 tools so a 3-col grid windows them and `start` is not pinned
	// to zero.
	tools := make([]registry.Tool, 0, 30)
	idx := make([]int, 0, 30)
	for i := 0; i < 30; i++ {
		// Distinct DisplayName so the rendered tiles are easy to
		// distinguish in the assertion.
		name := fmt.Sprintf("Tool%02d", i)
		tools = append(tools, mkTool(name, "1.0.0", "1.0.0",
			registry.PackageIDs{Winget: "F"}))
		idx = append(idx, i)
	}

	// Run with a width that comfortably yields cols=3
	// (chooseTileLayout's 3-column branch needs >= 3*tileMinWidth +
	// 2*tileGap = 100).
	const termWidth = 130

	// Build a tile snapshot at cursor=cursorIdx and return the set
	// of DisplayName strings rendered as tiles (ignoring header /
	// indicators).
	snapAt := func(cursorIdx int) []string {
		m := Model{
			activeTab:     tabInstalled,
			tools:         tools,
			filteredIndex: idx,
			phase:         phaseDone,
			width:         termWidth,
			height:        40,
			cursor:        cursorIdx,
		}
		lines := m.buildToolTileLines(40)
		var seen []string
		joined := stripANSIForTest(strings.Join(lines, "\n"))
		for i := 0; i < 30; i++ {
			name := fmt.Sprintf("Tool%02d", i)
			if strings.Contains(joined, name) {
				seen = append(seen, name)
			}
		}
		return seen
	}

	// At cursor=8, the row containing the cursor is row 2 (indices
	// 6,7,8 with cols=3). At cursor=7 (same row) the snapshot's
	// first tile MUST match the one at cursor=8 — i.e. moving the
	// cursor within a row never shifts the visible grid.
	at7 := snapAt(7)
	at8 := snapAt(8)
	if len(at7) == 0 || len(at8) == 0 {
		t.Fatalf("expected non-empty tile sets, got at7=%v at8=%v", at7, at8)
	}
	if at7[0] != at8[0] {
		t.Errorf("moving cursor within a row shifted the grid: cursor=7 first=%q cursor=8 first=%q",
			at7[0], at8[0])
	}
	// All starts must be multiples of cols (=3) given termWidth >=
	// 3-col threshold. We check via the first tile name (Tool00 →
	// idx 0, Tool03 → idx 3, …).
	if name := at8[0]; name != "" {
		var startIdx int
		_, _ = fmt.Sscanf(name, "Tool%02d", &startIdx)
		if startIdx%3 != 0 {
			t.Errorf("window start %d (from tile %q) is not aligned to col=3 boundary", startIdx, name)
		}
	}
}

// TestBuildToolTileLines_FallsBackToListOnTinyTerminal is the
// regression for the "tileRows forced to min 1 even when too small"
// PR comment. Previously, when maxRows could not fit a full tile
// (header + indicators + tileHeight = 10 lines), the builder still
// forced tileRows = 1 and accepted that renderView would clip the
// card mid-render. The fix falls back to list mode for the remaining
// row budget so the user sees truncated rows instead of half-cards.
func TestBuildToolTileLines_FallsBackToListOnTinyTerminal(t *testing.T) {
	t.Parallel()
	tools := []registry.Tool{mkTool("foo", "1.0.0", "1.0.0",
		registry.PackageIDs{Winget: "F"})}
	m := Model{
		activeTab:     tabInstalled,
		tools:         tools,
		filteredIndex: []int{0},
		phase:         phaseDone,
		width:         120,
		height:        40,
	}
	// maxRows=5 leaves (5 - 1 header - 2 indicators) / 7 tileHeight
	// = 0/7 → pre-fix path went tileRows = 1 (clipped tiles).
	lines := m.buildToolTileLines(5)
	joined := stripANSIForTest(strings.Join(lines, "\n"))
	// List mode emits the header row in a different shape than the
	// tile mode (it uses tab-separated columns). The signature we
	// rely on: list mode renders "foo" without the rounded border
	// glyphs (╭ ╮ │ ╰ ╯). If any tile-border glyph appears we know
	// we fell into the broken clipped-tile path instead of list
	// mode.
	for _, g := range []string{"╭", "╮", "╰", "╯"} {
		if strings.Contains(joined, g) {
			t.Errorf("expected list-mode fallback when maxRows is too small for a tile; tile-border glyph %q found:\n%s", g, joined)
		}
	}
	if !strings.Contains(joined, "foo") {
		t.Errorf("list-mode fallback should still render the tool row; got:\n%s", joined)
	}
}
