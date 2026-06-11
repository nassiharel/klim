package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/agents"
)

// Tile-mode renderer for the Sessions sub-tab. Visual design ported
// from ghcpCliDashboard's SessionTile.tsx: a colored left-edge accent
// bar signals live state, a bold title sits next to a colored
// provider chip, the branch is demoted into a dim pill, recent
// activity is colored by state, and turn/MCP counts live as pill
// badges instead of dim text crammed onto the same row as the state
// label.
//
// Lives in its own file because the rendering is independent of the
// table-based renderRow / renderHeader machinery — different layout
// primitive, different sizing rules. The generic tile-layout helpers
// (chooseTileLayout, withGutters, padOrTruncTile, tile geometry
// constants) live in tile_layout.go and are shared with other
// per-entity tile renderers (tools, plugins, …).

// renderSessionTiles renders the given session rows as a grid of
// bordered cards. `cursor` is the index of the highlighted row (-1
// to skip highlighting); rows that aren't sessions are skipped
// silently so a stray mixed-content slice doesn't crash the renderer.
//
// `totalWidth` is the horizontal budget the caller has — usually
// `m.width - agentsSidebarColWidth - 3` so the grid composites
// cleanly with the sidebar.
//
// Returns a string with one or more newline-terminated rows of tiles,
// ready to be inserted into the agents body buffer.
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
		row := lipgloss.JoinHorizontal(lipgloss.Top, withGutters(tiles[start:end])...)
		b.WriteString(row)
		b.WriteByte('\n')
	}
	return b.String()
}

// renderOneTile draws a single session card at `width` columns.
//
// Unselected tiles use a neutral (dim) border — state is signalled
// by the colored dot on the title row and the bold state label in
// the badges row, not by the border. The cursor-selected tile is
// the ONLY tile whose border carries color (bright cyan), so it
// pops without making the grid noisy.
//
// Star slot is always reserved (2 cells) so the title's column
// doesn't shift between starred and unstarred tiles.
func renderOneTile(s agents.Session, width int, selected bool) string {
	borderColor := cyberFGDim
	if selected {
		borderColor = cyberPrimary
	}
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(width).
		Padding(0, 1)

	// Inner content width = width - 2 (border cells) - 2 (padding).
	innerW := width - 4
	if innerW < tileMinWidth-4 {
		innerW = tileMinWidth - 4
	}

	// Tile subtitle: humanised "<relative> ago" off the field
	// actually displayed (LastModified). Previous wording said
	// "started <relative>" but used LastModified, which conflated
	// session-creation with last-activity time — the latter is far
	// more useful for spotting stale sessions, so we keep the field
	// and update the label to match.
	subtitle := lipgloss.NewStyle().Foreground(cyberFGDim).
		Render(padOrTruncTile("modified "+humaniseTime(s.LastModified), innerW))

	titleRow := renderTitleRow(s, innerW, selected)
	branchRow := renderBranchRow(s, innerW)
	activityRow := renderActivityRow(s, innerW)
	badgesRow := renderBadgesRow(s, innerW)

	body := strings.Join([]string{subtitle, titleRow, branchRow, activityRow, badgesRow}, "\n")
	return border.Render(body)
}

// renderTitleRow lays out: dot + reserved-star-slot + bold title +
// right-aligned provider chip. The reserved 2-cell star slot keeps
// the title's column position identical whether or not the session
// is starred — fixes the visible misalignment in the original design.
func renderTitleRow(s agents.Session, innerW int, selected bool) string {
	dot := stateDot(s.LiveState)
	star := "  " // always two cells reserved
	if s.Starred {
		star = "⭐"
	}
	chip := providerChip(s.Provider)
	titleStyle := lipgloss.NewStyle().Foreground(cyberFG).Bold(true)
	if selected {
		titleStyle = titleStyle.Foreground(cyberPrimary)
	}
	title := tileDisplayTitle(s)
	// Budget: innerW - dot(1) - space(1) - star(2) - space(1) -
	// [space-before-chip(1) + chip width]. The chip group only counts
	// when chip is non-empty — otherwise we'd steal a column and
	// leave a trailing blank at end-of-line (same regression pattern
	// as renderToolTitleRow). Sessions with an unknown Provider
	// (chip == "") use the freed cell for title content.
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

// renderBranchRow renders the demoted branch pill. Empty branch →
// dim em-dash so the row height stays consistent.
func renderBranchRow(s agents.Session, innerW int) string {
	pill := branchPill(s.Branch)
	return padOrTruncTile(pill, innerW)
}

// renderActivityRow renders the recent-activity line, prefix-stripped
// for readability and colored by state. Waiting sessions get amber so
// the user can spot pending prompts at a glance.
func renderActivityRow(s agents.Session, innerW int) string {
	text := stripActivityPrefix(s.RecentActivity)
	if text == "" {
		text = "—"
	}
	color := cyberFG
	switch s.LiveState {
	case agents.StateWaiting:
		color = cyberAccent
	case agents.StateIdle:
		color = cyberFGDim
	}
	const leader = "▸ "
	content := padOrTruncTile(text, innerW-lipgloss.Width(leader))
	return lipgloss.NewStyle().Foreground(color).Render(leader + content)
}

// renderBadgesRow renders the bottom line: turn count + MCP count +
// state label. Counts use chip-style backgrounds so they read as
// pills rather than dim text.
func renderBadgesRow(s agents.Session, innerW int) string {
	chipStyle := lipgloss.NewStyle().Foreground(cyberFG).Background(cyberChipBg).Padding(0, 1)
	stateStyle := lipgloss.NewStyle().Foreground(stateBarColor(s.LiveState)).Bold(true)

	parts := []string{}
	if s.TurnCount > 0 {
		parts = append(parts, chipStyle.Render(fmt.Sprintf("💬 %d", s.TurnCount)))
	}
	if n := len(s.MCPServers); n > 0 {
		parts = append(parts, chipStyle.Render(fmt.Sprintf("🔌 %d", n)))
	}
	if s.LiveState != "" {
		parts = append(parts, stateStyle.Render(string(s.LiveState)))
	}
	joined := strings.Join(parts, "  ")
	return padOrTruncTile(joined, innerW)
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
		if i := strings.LastIndexAny(s.ProjectPath, `/\`); i >= 0 && i < len(s.ProjectPath)-1 {
			return s.ProjectPath[i+1:]
		}
		return s.ProjectPath
	}
	return sessionShortID(s.ID)
}

// sessionTileFallback is the one-line summary used when the terminal
// is too narrow for tile rendering. Keeps the screen usable rather
// than emitting an empty body.
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
	return lipgloss.NewStyle().Foreground(stateBarColor(st)).Render(stateGlyph(st))
}

// stateGlyph returns the bare glyph for a live state, no styling.
func stateGlyph(st agents.LiveState) string {
	switch st {
	case agents.StateWorking:
		return "●"
	case agents.StateThinking:
		return "◐"
	case agents.StateWaiting:
		return "▲"
	case agents.StateIdle:
		return "○"
	}
	return "·"
}

// stateBarColor returns the accent color for a live state. Drives
// both the tile border and the state-label foreground so the visual
// language is consistent everywhere.
func stateBarColor(st agents.LiveState) color.Color {
	switch st {
	case agents.StateWorking:
		return cyberOK
	case agents.StateThinking:
		return cyberPrimary
	case agents.StateWaiting:
		return cyberAccent
	case agents.StateIdle:
		return cyberFGDim
	}
	return cyberFGDim
}

// stateLabel returns the live state as a short word, or "—" when no
// state was derived.
func stateLabel(st agents.LiveState) string {
	if st == "" {
		return "—"
	}
	return string(st)
}

// providerChip renders a colored provider badge to sit next to the
// title. Claude uses amber, Copilot uses cyan — matches the chip
// styling in the dashboard SessionTile.tsx port.
func providerChip(p agents.ProviderID) string {
	switch p {
	case agents.ProviderClaudeCode:
		return lipgloss.NewStyle().
			Foreground(cyberAccent).Bold(true).
			Render("✦ Claude")
	case agents.ProviderCopilotCLI:
		return lipgloss.NewStyle().
			Foreground(cyberPrimary).Bold(true).
			Render("⌬ Copilot")
	}
	return ""
}

// branchPill renders the branch as a dim "⎇ <branch>" string, or
// returns the em-dash placeholder when no branch is known so the
// row keeps a constant height.
func branchPill(branch string) string {
	if branch == "" {
		return lipgloss.NewStyle().Foreground(cyberFGDim).Render("⎇ —")
	}
	style := lipgloss.NewStyle().Foreground(cyberFG).Background(cyberChipBg).Padding(0, 1)
	return style.Render("⎇ " + branch)
}

// stripActivityPrefix removes the noisy role-marker prefixes the
// JSONL parser emits ("[user]", "[assistant]", "[tool]", "tool:",
// "asking:") so the activity line reads as plain English.
func stripActivityPrefix(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for _, p := range []string{
		"[user]", "[assistant]", "[tool]",
		"[user] ", "[assistant] ", "[tool] ",
	} {
		if strings.HasPrefix(s, p) {
			s = strings.TrimPrefix(s, p)
			s = strings.TrimSpace(s)
			break
		}
	}
	for _, p := range []string{"tool: ", "tool:", "asking: ", "asking:"} {
		if strings.HasPrefix(s, p) {
			s = strings.TrimPrefix(s, p)
			s = strings.TrimSpace(s)
			break
		}
	}
	return s
}
