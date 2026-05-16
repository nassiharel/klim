package tui

import "charm.land/lipgloss/v2"

// Cyber theme — the single source of truth for every color the TUI
// renders. styles.go aliases the legacy token names (primaryColor,
// successColor, …) onto these definitions so older render paths
// pick up the cyber look automatically.
//
// Colors are hex (true-color) — lipgloss auto-degrades to the
// nearest 256-color slot on terminals without true-color support.
// Conservative naming: every token is semantic ("alert", "ok",
// "panel") rather than literal ("red", "green") so a future
// re-skin only changes this file.
var (
	// ── Accents ─────────────────────────────────────────────────
	// Primary: electric cyber cyan. The interface's signature
	// colour — used for focus, brand, "live" indicators.
	cyberPrimary    = lipgloss.Color("#00d9ff")
	cyberPrimaryDim = lipgloss.Color("#0080a0")
	// Secondary: hot magenta. Reserved for selection cues and
	// accent highlights that need to read distinctly from cyan.
	cyberSecondary = lipgloss.Color("#ff4dd2")

	// ── Semantic ────────────────────────────────────────────────
	// OK / installed / pass. Brighter, more saturated than the
	// old mint green for contrast against the dark canvas.
	cyberOK = lipgloss.Color("#39ff14")
	// Warning / outdated / hint. Amber pulls in the "Iron Man"
	// HUD tone and pairs well with cyan.
	cyberAccent = lipgloss.Color("#ffb000")
	// Alert / blocked / fail.
	cyberAlert = lipgloss.Color("#ff4757")
	// Info / muted hint. Soft cyan tint.
	cyberInfo = lipgloss.Color("#94e1ff")

	// ── Text tiers ──────────────────────────────────────────────
	cyberFG    = lipgloss.Color("#e8f4f8") // near-white, faint cyan tint
	cyberFGDim = lipgloss.Color("#90a4b0")

	// ── Surfaces ────────────────────────────────────────────────
	cyberSelectedBg = lipgloss.Color("#0d1a2e") // selection / cursor row
	cyberChipBg     = lipgloss.Color("#152033") // chip / pill bg

	// ── Rules / borders ─────────────────────────────────────────
	cyberRule       = lipgloss.Color("#1f2d40") // subtle separator
	cyberRuleBright = lipgloss.Color("#3b6080") // mid-tier divider / active boundary

	// ── Brand colours for the Agents tab ────────────────────────
	// Anthropic's signature warm orange — used as the circle dot
	// next to "claude" entities. Sampled from the Claude product
	// chrome (the brand colour Anthropic uses on claude.ai).
	claudeBrand = lipgloss.Color("#cc785c")
	// GitHub Copilot's product purple — used for "copilot" entities.
	copilotBrand = lipgloss.Color("#8957e5")
	// MCP Registry gets the same amber accent we use for warnings
	// — distinct from the two CLI providers.
	mcpBrand = lipgloss.Color("#ffb000")
)

// Reusable style tokens — every render path should reach for one of
// these rather than constructing inline lipgloss styles, so a
// future palette tweak edits one file.
var (
	// ── HUD / header ────────────────────────────────────────────
	hudBracketStyle = lipgloss.NewStyle().Foreground(cyberPrimaryDim).Bold(true)
	hudBrandStyle   = lipgloss.NewStyle().Foreground(cyberPrimary).Bold(true)
	hudLabelStyle   = lipgloss.NewStyle().Foreground(cyberFGDim)
	hudValueStyle   = lipgloss.NewStyle().Foreground(cyberFG).Bold(true)
	hudSepStyle     = lipgloss.NewStyle().Foreground(cyberRuleBright)
	hudAlertStyle   = lipgloss.NewStyle().Foreground(cyberAccent).Bold(true)
	hudOKStyle      = lipgloss.NewStyle().Foreground(cyberOK).Bold(true)

	// ── Tabs ────────────────────────────────────────────────────
	cyberTabActiveStyle = lipgloss.NewStyle().
				Foreground(cyberPrimary).
				Bold(true).
				Padding(0, 1)
	cyberTabInactiveStyle = lipgloss.NewStyle().
				Foreground(cyberFGDim).
				Padding(0, 1)
	cyberTabBracketStyle      = lipgloss.NewStyle().Foreground(cyberPrimary).Bold(true)
	cyberTabUnderlineStyle    = lipgloss.NewStyle().Foreground(cyberPrimary)
	cyberTabUnderlineDimStyle = lipgloss.NewStyle().Foreground(cyberRuleBright)

	// Subtab strip — same colour vocabulary as the parent tabs,
	// distinguished by bracket shape only.
	cyberSubtabActiveLabelStyle   = lipgloss.NewStyle().Foreground(cyberPrimary).Bold(true)
	cyberSubtabInactiveLabelStyle = lipgloss.NewStyle().Foreground(cyberFGDim)

	// ── Boot splash ─────────────────────────────────────────────
	bootGlowStyle     = lipgloss.NewStyle().Foreground(cyberPrimary).Bold(true)
	bootHaloStyle     = lipgloss.NewStyle().Foreground(cyberPrimaryDim)
	bootScanStyle     = lipgloss.NewStyle().Foreground(cyberInfo)
	bootSubtitleStyle = lipgloss.NewStyle().Foreground(cyberFGDim).Italic(true)

	// ── Spinners / pulses ───────────────────────────────────────
	cyberSpinnerStyle    = lipgloss.NewStyle().Foreground(cyberPrimary).Bold(true)
	hudPulseDimStyle     = lipgloss.NewStyle().Foreground(cyberPrimaryDim).Bold(true)
	hudPulsePrimaryStyle = lipgloss.NewStyle().Foreground(cyberPrimary).Bold(true)
	hudPulseInfoStyle    = lipgloss.NewStyle().Foreground(cyberInfo).Bold(true)

	// ── Chips / pills ───────────────────────────────────────────
	cyberChipStyle = lipgloss.NewStyle().
			Foreground(cyberFG).
			Background(cyberChipBg).
			Padding(0, 1)
	cyberChipAccentStyle = lipgloss.NewStyle().
				Foreground(cyberPrimary).
				Background(cyberChipBg).
				Bold(true).
				Padding(0, 1)

	// ── Buttons / actions ───────────────────────────────────────
	cyberButtonStyle = lipgloss.NewStyle().
				Foreground(cyberPrimary).
				Bold(true).
				Padding(0, 2)
	cyberButtonDoneStyle = lipgloss.NewStyle().
				Foreground(cyberOK).
				Bold(true).
				Padding(0, 2)

	// ── Selection (cursor row) ──────────────────────────────────
	cyberSelectedRowStyle = lipgloss.NewStyle().Background(cyberSelectedBg)

	// ── Misc decorations ────────────────────────────────────────
	cyberRuleStyle = lipgloss.NewStyle().Foreground(cyberRule)
)

// pulseStyle returns the prebuilt pulse style for the current
// animation frame, producing a soft breathing effect on "live"
// indicators. Reusable style tokens (no allocation per call) keep
// per-frame render cheap even at the 10 fps tick rate.
func pulseStyle(frame int) lipgloss.Style {
	switch (frame / 3) % 6 {
	case 0, 5:
		return hudPulseDimStyle
	case 1, 4:
		return hudPulsePrimaryStyle
	default:
		return hudPulseInfoStyle
	}
}

// pulseDot returns a glowing dot character whose intensity tracks
// the animation frame. Renders as a single visible cell.
func pulseDot(frame int) string {
	return pulseStyle(frame).Render("◉")
}

// staticDot returns a non-animated dim cyan dot, used when the
// animation loop is idle so the HUD still shows the activity
// indicator without consuming a frame budget.
func staticDot() string {
	return hudPulseDimStyle.Render("◉")
}
