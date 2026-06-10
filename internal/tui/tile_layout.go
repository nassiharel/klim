package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// Tile-layout primitives shared between every tile-style renderer in
// the TUI (session tiles, tools tiles, future plugin/MCP tiles).
//
// The geometry constants and layout helpers in this file are pure —
// they don't know which entity the caller is rendering. Each
// per-entity tile file (agents_session_tiles.go, tools_tile.go, …)
// imports these as siblings in the same package.
//
// Why this lives in its own file: when the first tile view shipped
// for sessions, the helpers were inlined there. Adding tiles to My
// Tools / Marketplace meant we'd have a second caller, so the layout
// math was extracted here in a no-behavior-change refactor.

// Tile geometry. Cards are sized to fit two columns at narrow widths
// and grow to three columns when the terminal can hold them. Each
// card is exactly tileHeight rows tall so the responsive row math in
// every caller stays simple.
const (
	tileMinWidth  = 32 // sub-this, fall back to single-column
	tileIdealMax  = 56 // wider tiles waste horizontal space
	tileGap       = 2  // horizontal space between cards
	tileHeight    = 7  // 5 content rows + the rounded-border top/bottom
	tileBodyLines = 5  // content lines inside the border
)

// chooseTileLayout picks (tileWidth, columnCount) for the available
// horizontal budget. Bias toward 2 columns until we have room for 3
// without each tile dropping below the ideal max; never more than 3
// because cards start to look like the table they're replacing.
func chooseTileLayout(totalWidth int) (int, int) {
	// Try 3 cols first.
	if w := (totalWidth - 2*tileGap) / 3; w >= tileMinWidth {
		if w > tileIdealMax {
			w = tileIdealMax
		}
		return w, 3
	}
	// Try 2 cols.
	if w := (totalWidth - tileGap) / 2; w >= tileMinWidth {
		if w > tileIdealMax {
			w = tileIdealMax
		}
		return w, 2
	}
	// Single column — clamp so a 200-col terminal doesn't produce a
	// single absurdly wide card.
	w := totalWidth
	if w > tileIdealMax {
		w = tileIdealMax
	}
	return w, 1
}

// withGutters interleaves blank gutter strings between the tiles in
// a row so JoinHorizontal produces the expected spacing.
func withGutters(tiles []string) []string {
	if len(tiles) <= 1 {
		return tiles
	}
	gutter := strings.Repeat(" ", tileGap)
	out := make([]string, 0, len(tiles)*2-1)
	for i, t := range tiles {
		if i > 0 {
			out = append(out, gutter)
		}
		out = append(out, t)
	}
	return out
}

// padOrTruncTile pads or truncates `s` to exactly `n` visual cells.
// Truncation uses a horizontal ellipsis so it's obvious to the user.
// Used inside tiles where we need each line to be a known width so
// the rounded border stays aligned.
func padOrTruncTile(s string, n int) string {
	if n <= 0 {
		return ""
	}
	w := lipgloss.Width(s)
	if w == n {
		return s
	}
	if w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return truncateANSI(s, n-1) + "…"
}
