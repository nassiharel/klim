package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/agents"
)

// Tile-mode renderer for the Sessions sub-tab. Inspired by
// ghcpCliDashboard's SessionGrid: one bordered card per session,
// packed with the most-used metadata (state, title, branch, turns,
// recent activity).
//
// Lives in its own file because the rendering is independent of
// the table-based renderRow / renderHeader machinery — different
// layout primitive, different sizing rules.

// Tile geometry. Cards are sized to fit two columns at narrow
// widths and grow to three columns when the terminal can hold them
// without horizontal scroll. Each card is exactly tileHeight rows
// tall so the responsive row math in the caller stays simple.
const (
	tileMinWidth  = 32 // sub-this, fall back to single-column
	tileIdealMax  = 52 // wider tiles waste space; cap at this
	tileGap       = 2  // horizontal space between cards
	tileHeight    = 6  // border-top + 4 content + border-bottom
	tileBodyLines = 4  // content lines inside the border
)

// tileBorderActive is the highlight border for the cursor-selected
// tile. Plain rounded border for the rest.
var (
	tileBorderActive = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(cyberAccent)
	tileBorderIdle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(cyberFGDim)
)

// renderSessionTiles renders the given session rows as a grid of
// bordered cards. `cursor` is the index of the highlighted row
// (-1 to skip highlighting); rows that aren't sessions are skipped
// silently so a stray mixed-content slice doesn't crash the
// renderer.
//
// `totalWidth` is the horizontal budget the caller has — usually
// `m.width - agentsSidebarColWidth - 3` so the grid composites
// cleanly with the sidebar.
//
// Returns a string with one or more newline-terminated rows of
// tiles, ready to be inserted into the agents body buffer.
func renderSessionTiles(rows []agentRow, cursor int, totalWidth int) string {
	if totalWidth < tileMinWidth+4 {
		// Terminal too narrow for tiles — fall back to a one-line
		// summary per session so the user still sees something.
		var b strings.Builder
		for i, r := range rows {
			if r.session == nil {
				continue
			}
			marker := "  "
			if i == cursor {
				marker = "▸ "
			}
			b.WriteString(marker + sessionTileFallback(*r.session) + "\n")
		}
		return b.String()
	}

	tileW, cols := chooseTileLayout(totalWidth)

	// Build per-session pre-rendered tile strings, then stitch them
	// into rows of `cols` columns. Each tile is exactly tileHeight
	// visual rows (lipgloss border + tileBodyLines + closing border).
	tiles := make([]string, 0, len(rows))
	for i, r := range rows {
		if r.session == nil {
			continue
		}
		tiles = append(tiles, renderOneTile(*r.session, tileW, i == cursor))
	}

	var b strings.Builder
	for start := 0; start < len(tiles); start += cols {
		end := start + cols
		if end > len(tiles) {
			end = len(tiles)
		}
		// Each tile is a multi-line string; lipgloss.JoinHorizontal
		// lays them side by side without re-padding.
		row := lipgloss.JoinHorizontal(lipgloss.Top, withGutters(tiles[start:end])...)
		b.WriteString(row)
		b.WriteByte('\n')
	}
	return b.String()
}

// chooseTileLayout picks (tileWidth, columnCount) for the available
// horizontal budget. Bias toward 2 columns until we have room for 3
// without each tile dropping below the ideal max; never more than 3
// because at that point cards start to look like the table they're
// replacing.
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
	// Single column — clamp to a sensible max so a 200-col terminal
	// doesn't produce a single absurdly wide card.
	w := totalWidth
	if w > tileIdealMax {
		w = tileIdealMax
	}
	return w, 1
}

// withGutters interleaves blank gutter strings between the tiles in
// a row so `JoinHorizontal` produces the expected spacing.
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

// renderOneTile draws a single session card at `width` columns.
//
// Layout (4 content lines inside the rounded border):
//
//	● ⭐ <title>           <provider>
//	  ⎇ <branch>           <modified>
//	  <recent activity>
//	  <state> · <turns>t · <mcps>
//
// The title gets the most space; everything else is single-line.
// Long values are truncated with `…` so the card height stays at
// exactly tileBodyLines.
func renderOneTile(s agents.Session, width int, selected bool) string {
	style := tileBorderIdle
	if selected {
		style = tileBorderActive
	}
	// Inner content width is width - 2 (one column for each border).
	innerW := width - 2
	if innerW < tileMinWidth-2 {
		innerW = tileMinWidth - 2
	}

	dot := stateDot(s.LiveState)
	star := " "
	if s.Starred {
		star = "★"
	}
	title := tileDisplayTitle(s)
	provider := providerShort(s.Provider)
	// Title row: dot + star + title (grows) + provider on the right.
	titleW := innerW - lipgloss.Width(dot) - lipgloss.Width(star) - lipgloss.Width(provider) - 3 // 3 = spaces
	if titleW < 8 {
		titleW = 8
	}
	titleRow := fmt.Sprintf("%s %s %s %s",
		dot, star,
		padOrTruncTile(title, titleW),
		dimVersion.Render(provider),
	)

	// Branch + modified row.
	branch := s.Branch
	if branch == "" {
		branch = "—"
	}
	modified := humaniseTime(s.LastModified)
	branchRow := fmt.Sprintf("  ⎇ %s %s",
		padOrTruncTile(branch, innerW-lipgloss.Width(modified)-5),
		dimVersion.Render(modified),
	)

	// Recent activity row (the most diagnostic line — what is the
	// agent actually doing right now).
	recent := s.RecentActivity
	if recent == "" {
		recent = "—"
	}
	recentRow := "  " + dimVersion.Render(padOrTruncTile(recent, innerW-2))

	// Footer row: state label + turn count + mcp count.
	footer := fmt.Sprintf("  %s · %s · %s",
		stateLabel(s.LiveState),
		turnCountLabel(s.TurnCount),
		mcpCountLabel(len(s.MCPServers)),
	)
	footerRow := padOrTruncTile(footer, innerW)

	body := strings.Join([]string{titleRow, branchRow, recentRow, footerRow}, "\n")
	return style.Width(width).Render(body)
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
	// Truncate. truncateANSI handles ANSI escapes correctly.
	return truncateANSI(s, n-1) + "…"
}

// tileDisplayTitle picks the most useful string for the tile's
// header: explicit Title (the first user message), then repository,
// then the project's last path segment, then the short ID. Always
// non-empty so the tile never has a blank header row.
func tileDisplayTitle(s agents.Session) string {
	if s.Title != "" {
		return s.Title
	}
	if s.Repository != "" {
		return s.Repository
	}
	if s.ProjectPath != "" {
		// Take last segment.
		if i := strings.LastIndexAny(s.ProjectPath, `/\`); i >= 0 && i < len(s.ProjectPath)-1 {
			return s.ProjectPath[i+1:]
		}
		return s.ProjectPath
	}
	return sessionShortID(s.ID)
}

// sessionTileFallback is the one-line summary used when the
// terminal is too narrow for tile rendering. Keeps the screen
// usable rather than emitting an empty body.
func sessionTileFallback(s agents.Session) string {
	return fmt.Sprintf("%s %s · %s · %s",
		stateDot(s.LiveState),
		truncAgentRow(tileDisplayTitle(s), 40),
		humaniseTime(s.LastModified),
		stateLabel(s.LiveState),
	)
}

// stateDot returns the colored live-state indicator. Matches the
// state-glyph scheme used by `klim agents sessions list` so the two
// surfaces look at-a-glance identical.
func stateDot(st agents.LiveState) string {
	switch st {
	case agents.StateWorking:
		return lipgloss.NewStyle().Foreground(cyberOK).Render("●")
	case agents.StateThinking:
		return lipgloss.NewStyle().Foreground(cyberPrimary).Render("◐")
	case agents.StateWaiting:
		return lipgloss.NewStyle().Foreground(cyberAccent).Render("▲")
	case agents.StateIdle:
		return lipgloss.NewStyle().Foreground(cyberFGDim).Render("○")
	}
	return lipgloss.NewStyle().Foreground(cyberFGDim).Render("·")
}

// stateLabel returns the live state as a short word, or "—" when
// no state was derived.
func stateLabel(st agents.LiveState) string {
	if st == "" {
		return "—"
	}
	return string(st)
}

// turnCountLabel renders the turn count with a trailing "t" for
// compactness (5t instead of "5 turns"). Zero turns → "—".
func turnCountLabel(n int) string {
	if n <= 0 {
		return "—"
	}
	return fmt.Sprintf("%dt", n)
}

// mcpCountLabel renders the MCP server count with a leading 🔌 chip
// when non-zero. Zero → "—" to keep the footer width consistent.
func mcpCountLabel(n int) string {
	if n <= 0 {
		return "—"
	}
	return fmt.Sprintf("🔌%d", n)
}
