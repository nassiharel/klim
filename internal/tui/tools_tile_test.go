package tui

// Tests for the tile-mode renderer used on the My Tools and
// Marketplace tabs. Verifies state mapping, the favorite-slot
// alignment guarantee (same regression that bit session tiles),
// the Updates-tab checkbox column, and that the toggle cycle
// stays binary.

import (
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
		marketNew bool
		want      string
	}{
		{"not installed", "", "1.0.0", false, "uninstalled"},
		{"installed current", "1.0.0", "1.0.0", false, "installed"},
		{"installed has update", "1.0.0", "1.2.0", false, "update"},
		{"marketplace new", "", "", true, "new"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := mkTool("foo", tt.installed, tt.latest, registry.PackageIDs{Winget: "Foo"})
			got := toolTileState(&tool, tt.marketNew)
			if got != tt.want {
				t.Errorf("toolTileState = %q, want %q", got, tt.want)
			}
		})
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
