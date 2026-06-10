package tui

import (
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/agents"
)

// Tile-mode renderer for the non-session Agents sub-tabs:
// Marketplaces, Plugins, Skills, MCPs. Same visual language as
// the session-tile redesign (PR #93), with one twist: per user
// feedback only the cursor-selected tile wears a colored border
// (cyan). Unselected tiles have a neutral dim border and signal
// state via the dot glyph + colored badges row.
//
// Sessions retains its own renderSessionTiles in
// agents_session_tiles.go since the data shape is meaningfully
// different (live state, recent activity, branch, turns).
//
// The generic tile-layout primitives live in tile_layout.go.

// renderAgentsTiles renders an arbitrary slice of agentRow rows as
// a tile grid. Dispatches to the correct per-entity tile renderer
// based on which payload pointer (marketplace / plugin / skill /
// mcp) is set on the row. Session rows go through
// renderSessionTiles instead — callers must route accordingly.
//
// `cursor` is the highlighted row's index (-1 to skip selection).
// `totalWidth` is the horizontal budget the caller has.
func renderAgentsTiles(rows []agentRow, cursor int, totalWidth int) string {
	if totalWidth < tileMinWidth+4 {
		// Terminal too narrow — one-line fallback.
		var b strings.Builder
		for i, r := range rows {
			marker := "  "
			if i == cursor {
				marker = "▸ "
			}
			b.WriteString(marker + agentRowFallback(r) + "\n")
		}
		return b.String()
	}
	tileW, cols := chooseTileLayout(totalWidth)

	tiles := make([]string, 0, len(rows))
	for i, r := range rows {
		tile := renderOneAgentEntityTile(r, tileW, i == cursor)
		if tile == "" {
			continue
		}
		tiles = append(tiles, tile)
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

// renderOneAgentEntityTile dispatches on the row's payload pointer.
// Returns "" for rows that don't match any supported entity (e.g.
// session rows, which use a different renderer).
func renderOneAgentEntityTile(r agentRow, width int, selected bool) string {
	switch {
	case r.marketplace != nil:
		return renderMarketplaceTile(*r.marketplace, width, selected)
	case r.plugin != nil:
		return renderPluginTile(*r.plugin, width, selected)
	case r.skill != nil:
		return renderSkillTile(*r.skill, r.bookmarked, width, selected)
	case r.mcp != nil:
		return renderMCPTile(*r.mcp, width, selected)
	}
	return ""
}

// agentRowFallback is the one-line summary for narrow terminals
// where the grid can't render. Mirrors sessionTileFallback in shape.
func agentRowFallback(r agentRow) string {
	subtitle := r.subtitle
	if subtitle != "" {
		subtitle = " · " + truncAgentRow(subtitle, 30)
	}
	return r.name + subtitle
}

// ─── Marketplace tile ─────────────────────────────────────────────

// entityBorder returns the shared border style every agents-entity
// tile uses. Unselected: neutral dim border, state lives in the dot
// and badges. Selected: bright cyan border — the only tile in the
// grid that wears a colored frame, so the cursor pops without
// turning the whole grid into a rainbow.
func entityBorder(width int, selected bool) lipgloss.Style {
	borderColor := cyberFGDim
	if selected {
		borderColor = cyberPrimary
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).Width(width).Padding(0, 1)
}

// renderMarketplaceTile is the tile-mode renderer for a single
// marketplace entry. State is shown via the dot glyph and badges
// row (✓ installed / + catalog); border stays neutral.
//
// Body rows (5):
//  1. dim subtitle: install spec / URL (truncated)
//  2. dot + reserved spacer + bold name + provider chip
//  3. owner pill (or "—")
//  4. dim description leader
//  5. badges: 📦 plugin count, ✓/+ installed/catalog, last-synced
func renderMarketplaceTile(m agents.Marketplace, width int, selected bool) string {
	state := "installed"
	if !m.Installed {
		state = "catalog"
	}
	border := entityBorder(width, selected)
	innerW := width - 4
	if innerW < tileMinWidth-4 {
		innerW = tileMinWidth - 4
	}

	subtitle := m.InstallSpec
	if subtitle == "" {
		subtitle = m.URL
	}
	if subtitle == "" {
		subtitle = "—"
	}
	subRow := padOrTruncTile(lipgloss.NewStyle().Foreground(cyberFGDim).Render(subtitle), innerW)

	titleRow := entityTitleRow(agentEntityStateDot(state), false,
		entityDisplayName(m.Name, m.DisplayName), agentsProviderTileChip(m.Provider), innerW, selected)

	owner := m.Owner
	if owner == "" {
		owner = "—"
	}
	ownerRow := padOrTruncTile(entityPill("◎ "+owner), innerW)

	descRow := entityDescriptionRow(m.Description, innerW)

	badges := []string{}
	if m.PluginCount > 0 {
		badges = append(badges, entityChip("📦 "+itoa(m.PluginCount)))
	}
	if m.Installed {
		badges = append(badges, entityChip("✓ installed"))
	} else {
		badges = append(badges, entityChip("+ catalog"))
	}
	if !m.LastSynced.IsZero() {
		badges = append(badges,
			lipgloss.NewStyle().Foreground(cyberFGDim).Render("synced "+humaniseTime(m.LastSynced)))
	}
	badgeRow := padOrTruncTile(strings.Join(badges, "  "), innerW)

	body := strings.Join([]string{subRow, titleRow, ownerRow, descRow, badgeRow}, "\n")
	return border.Render(body)
}

// ─── Plugin tile ─────────────────────────────────────────────────

// renderPluginTile renders one Plugin entry. State color lives in
// the dot + version chip + state badge; border stays neutral.
func renderPluginTile(p agents.Plugin, width int, selected bool) string {
	state := pluginState(&p)
	border := entityBorder(width, selected)
	innerW := width - 4
	if innerW < tileMinWidth-4 {
		innerW = tileMinWidth - 4
	}

	version := p.Version
	if version == "" {
		version = "—"
	}
	subRow := padOrTruncTile(lipgloss.NewStyle().Foreground(cyberFGDim).Render("v"+version), innerW)

	titleRow := entityTitleRow(agentEntityStateDot(state), false,
		entityDisplayName(p.Name, p.DisplayName), agentsProviderTileChip(p.Provider), innerW, selected)

	source := p.Marketplace
	if source == "" {
		source = string(p.Source)
	}
	if source == "" {
		source = "—"
	}
	sourceRow := padOrTruncTile(entityPill("◎ "+source), innerW)

	descRow := entityDescriptionRow(p.Description, innerW)

	badges := []string{}
	if p.SkillCount > 0 {
		badges = append(badges, entityChip("⚙ "+itoa(p.SkillCount)+" skills"))
	}
	if p.MCPCount > 0 {
		badges = append(badges, entityChip("🔌 "+itoa(p.MCPCount)))
	}
	switch state {
	case "enabled":
		badges = append(badges, lipgloss.NewStyle().Foreground(cyberOK).Bold(true).Render("enabled"))
	case "disabled":
		badges = append(badges, lipgloss.NewStyle().Foreground(cyberAccent).Bold(true).Render("disabled"))
	default:
		badges = append(badges, lipgloss.NewStyle().Foreground(cyberFGDim).Render("catalog"))
	}
	badgeRow := padOrTruncTile(strings.Join(badges, "  "), innerW)

	body := strings.Join([]string{subRow, titleRow, sourceRow, descRow, badgeRow}, "\n")
	return border.Render(body)
}

// ─── Skill tile ─────────────────────────────────────────────────

// renderSkillTile renders one Skill entry. State color lives in
// the dot + enabled/disabled badge; border stays neutral.
// User-invocable skills get a 👤 chip.
func renderSkillTile(s agents.Skill, _bookmarked bool, width int, selected bool) string {
	state := "enabled"
	if !s.Enabled {
		state = "disabled"
	}
	border := entityBorder(width, selected)
	innerW := width - 4
	if innerW < tileMinWidth-4 {
		innerW = tileMinWidth - 4
	}

	scope := string(s.Scope)
	if scope == "" {
		scope = "—"
	}
	subRow := padOrTruncTile(lipgloss.NewStyle().Foreground(cyberFGDim).Render("scope: "+scope), innerW)

	titleRow := entityTitleRow(agentEntityStateDot(state), false,
		s.Name, agentsProviderTileChip(s.Provider), innerW, selected)

	plugin := s.SourcePlugin
	if plugin == "" {
		plugin = "—"
	}
	pluginRow := padOrTruncTile(entityPill("◎ "+plugin), innerW)

	descRow := entityDescriptionRow(s.Description, innerW)

	badges := []string{}
	if s.Model != "" {
		badges = append(badges, entityChip("🧠 "+s.Model))
	}
	if s.UserInvocable {
		badges = append(badges, entityChip("👤 user"))
	}
	switch state {
	case "enabled":
		badges = append(badges, lipgloss.NewStyle().Foreground(cyberOK).Bold(true).Render("enabled"))
	case "disabled":
		badges = append(badges, lipgloss.NewStyle().Foreground(cyberAccent).Bold(true).Render("disabled"))
	}
	badgeRow := padOrTruncTile(strings.Join(badges, "  "), innerW)

	body := strings.Join([]string{subRow, titleRow, pluginRow, descRow, badgeRow}, "\n")
	return border.Render(body)
}

// ─── MCP tile ────────────────────────────────────────────────────

// renderMCPTile renders one MCP server entry. State color lives in
// the dot + enabled/disabled badge; transport chip distinguishes
// stdio / http / sse. Border stays neutral.
func renderMCPTile(m agents.MCP, width int, selected bool) string {
	state := "enabled"
	if !m.Enabled {
		state = "disabled"
	}
	border := entityBorder(width, selected)
	innerW := width - 4
	if innerW < tileMinWidth-4 {
		innerW = tileMinWidth - 4
	}

	transport := m.Transport
	if transport == "" {
		transport = "stdio"
	}
	subRow := padOrTruncTile(lipgloss.NewStyle().Foreground(cyberFGDim).Render("transport: "+transport), innerW)

	titleRow := entityTitleRow(agentEntityStateDot(state), false,
		m.Name, agentsProviderTileChip(m.Provider), innerW, selected)

	endpoint := m.URL
	if endpoint == "" && m.Command != "" {
		endpoint = m.Command
	}
	if endpoint == "" {
		endpoint = "—"
	}
	endpointRow := padOrTruncTile(entityPill("⇆ "+endpoint), innerW)

	descRow := padOrTruncTile(lipgloss.NewStyle().Foreground(cyberFGDim).
		Render("▸ "+padOrTruncTile(scopeLabel(m.Scope), innerW-2)), innerW)

	badges := []string{}
	if n := len(m.Tools); n > 0 {
		badges = append(badges, entityChip("⚙ "+itoa(n)+" tools"))
	}
	if n := len(m.EnvKeys); n > 0 {
		badges = append(badges, entityChip("⚙ env "+itoa(n)))
	}
	switch state {
	case "enabled":
		badges = append(badges, lipgloss.NewStyle().Foreground(cyberOK).Bold(true).Render("enabled"))
	case "disabled":
		badges = append(badges, lipgloss.NewStyle().Foreground(cyberAccent).Bold(true).Render("disabled"))
	}
	badgeRow := padOrTruncTile(strings.Join(badges, "  "), innerW)

	body := strings.Join([]string{subRow, titleRow, endpointRow, descRow, badgeRow}, "\n")
	return border.Render(body)
}

// ─── Shared helpers ──────────────────────────────────────────────

// pluginState classifies a plugin into its visual state.
func pluginState(p *agents.Plugin) string {
	switch {
	case p.Enabled && p.Installed:
		return "enabled"
	case p.Installed:
		return "disabled"
	}
	return "catalog"
}

// agentEntityStateColor returns the accent color for the canonical
// states used by the agent-entity tile renderers.
func agentEntityStateColor(state string) color.Color {
	switch state {
	case "enabled", "installed":
		return cyberOK
	case "disabled":
		return cyberAccent
	case "catalog":
		return cyberFGDim
	}
	return cyberFGDim
}

// agentEntityStateDot returns the colored glyph for a state.
func agentEntityStateDot(state string) string {
	glyph := "○"
	switch state {
	case "enabled", "installed":
		glyph = "●"
	case "disabled":
		glyph = "◐"
	}
	return lipgloss.NewStyle().Foreground(agentEntityStateColor(state)).Render(glyph)
}

// entityTitleRow lays out the dot + reserved spacer + bold title +
// right-aligned provider chip. Mirrors the session-tile title row so
// every tile in the Agents tab feels like part of the same family.
// `bookmark` is currently unused for agent entities (only sessions
// get the ⭐ slot) — passing false reserves 2 cells for symmetry.
func entityTitleRow(dot string, bookmark bool, title, chip string, innerW int, selected bool) string {
	star := "  "
	if bookmark {
		star = "⭐"
	}
	titleStyle := lipgloss.NewStyle().Foreground(cyberFG).Bold(true)
	if selected {
		titleStyle = titleStyle.Foreground(cyberPrimary)
	}
	// innerW - dot(1) - sp(1) - star(2) - sp(1) - chipW - sp(1).
	titleW := innerW - 1 - 1 - 2 - 1 - lipgloss.Width(chip) - 1
	if titleW < 6 {
		titleW = 6
	}
	titleText := titleStyle.Render(padOrTruncTile(title, titleW))
	return dot + " " + star + " " + titleText + " " + chip
}

// entityDescriptionRow renders the description line with a ▸ leader
// and dim foreground. Em-dash when empty so the row keeps a
// constant height.
func entityDescriptionRow(desc string, innerW int) string {
	if desc == "" {
		desc = "—"
	}
	const leader = "▸ "
	content := padOrTruncTile(desc, innerW-lipgloss.Width(leader))
	return lipgloss.NewStyle().Foreground(cyberFGDim).Render(leader + content)
}

// entityPill renders the dim chip used for source / owner / plugin
// rows below the title.
func entityPill(text string) string {
	return lipgloss.NewStyle().Foreground(cyberFG).Background(cyberChipBg).Padding(0, 1).Render(text)
}

// entityChip renders the smaller chips on the bottom badges row.
func entityChip(text string) string {
	return lipgloss.NewStyle().Foreground(cyberFG).Background(cyberChipBg).Padding(0, 1).Render(text)
}

// entityDisplayName picks the most useful display string.
func entityDisplayName(name, displayName string) string {
	if displayName != "" {
		return displayName
	}
	return name
}

// agentsProviderTileChip renders the colored "✦ Claude" / "⌬ Copilot"
// chip used in agent-entity tiles. Mirrors providerChip in
// agents_session_tiles.go but accepts the agents.ProviderID type
// directly so callers don't have to import the alias.
func agentsProviderTileChip(p agents.ProviderID) string {
	switch p {
	case agents.ProviderClaudeCode:
		return lipgloss.NewStyle().Foreground(cyberAccent).Bold(true).Render("✦ Claude")
	case agents.ProviderCopilotCLI:
		return lipgloss.NewStyle().Foreground(cyberPrimary).Bold(true).Render("⌬ Copilot")
	}
	return ""
}

// scopeLabel returns a human-readable scope label, falling back to
// "—" so the row width stays predictable.
func scopeLabel(s agents.Scope) string {
	if s == "" {
		return "—"
	}
	return "scope: " + string(s)
}

// itoa wraps strconv.Itoa for readability inside tile renderers,
// where most numeric inserts are tiny counts (skill count, MCP
// count, plugin count).
func itoa(n int) string {
	return strconv.Itoa(n)
}
