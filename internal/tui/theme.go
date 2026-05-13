package tui

import (
	"charm.land/lipgloss/v2"
)

// Cyber theme — electric cyber-modern palette.
//
// Colors are hex (true-color) — lipgloss auto-degrades to the nearest
// 256-color slot on terminals without true-color support, so we still
// get a recognisable look on Windows cmd / older xterm.
var (
	// Bright accents (the "active" / "live" layer).
	cyberPrimary = lipgloss.Color("#00d9ff") // electric cyber cyan
	cyberAccent  = lipgloss.Color("#ffb000") // amber for warnings / hints
	cyberOK      = lipgloss.Color("#39ff14") // bright green for OK / installed
	cyberInfo    = lipgloss.Color("#94e1ff") // soft cyan tint for info

	// Dim halo shade (one notch below the bright twin).
	cyberPrimaryDim = lipgloss.Color("#0080a0")

	// Text tiers.
	cyberFG    = lipgloss.Color("#e8f4f8") // near-white with cyan tint
	cyberFGDim = lipgloss.Color("#90a4b0")

	// Panel surface for the active-tab fill.
	cyberBGActive = lipgloss.Color("#0066a0")

	// Brighter rule tone for the underline glow.
	cyberRuleBright = lipgloss.Color("#3b6080")
)

// Reusable style tokens — every render path should reach for one of
// these rather than constructing inline lipgloss styles, so a future
// palette tweak only edits this file.
var (
	// HUD / header.
	hudBracketStyle = lipgloss.NewStyle().Foreground(cyberPrimaryDim).Bold(true)
	hudBrandStyle   = lipgloss.NewStyle().Foreground(cyberPrimary).Bold(true)
	hudLabelStyle   = lipgloss.NewStyle().Foreground(cyberFGDim)
	hudValueStyle   = lipgloss.NewStyle().Foreground(cyberFG).Bold(true)
	hudSepStyle     = lipgloss.NewStyle().Foreground(cyberRuleBright)
	hudAlertStyle   = lipgloss.NewStyle().Foreground(cyberAccent).Bold(true)
	hudOKStyle      = lipgloss.NewStyle().Foreground(cyberOK).Bold(true)

	// Tabs.
	cyberTabActiveStyle = lipgloss.NewStyle().
				Foreground(cyberFG).
				Background(cyberBGActive).
				Bold(true).
				Padding(0, 1)
	cyberTabInactiveStyle = lipgloss.NewStyle().
				Foreground(cyberFGDim).
				Padding(0, 1)
	cyberTabBracketStyle      = lipgloss.NewStyle().Foreground(cyberPrimary).Bold(true)
	cyberTabUnderlineStyle    = lipgloss.NewStyle().Foreground(cyberPrimary)
	cyberTabUnderlineDimStyle = lipgloss.NewStyle().Foreground(cyberRuleBright)

	// Boot splash.
	bootGlowStyle     = lipgloss.NewStyle().Foreground(cyberPrimary).Bold(true)
	bootHaloStyle     = lipgloss.NewStyle().Foreground(cyberPrimaryDim)
	bootScanStyle     = lipgloss.NewStyle().Foreground(cyberInfo)
	bootSubtitleStyle = lipgloss.NewStyle().Foreground(cyberFGDim).Italic(true)

	// Cyber spinner.
	cyberSpinnerStyle = lipgloss.NewStyle().Foreground(cyberPrimary).Bold(true)

	// Subtabs — slimmer accent for nested tab strips.
	cyberSubtabActiveLabelStyle   = lipgloss.NewStyle().Foreground(cyberPrimary).Bold(true)
	cyberSubtabInactiveLabelStyle = lipgloss.NewStyle().Foreground(cyberFGDim)

	// HUD pulse — prebuilt for each brightness tier so the per-
	// frame render path doesn't allocate a fresh style every call.
	hudPulseDimStyle     = lipgloss.NewStyle().Foreground(cyberPrimaryDim).Bold(true)
	hudPulsePrimaryStyle = lipgloss.NewStyle().Foreground(cyberPrimary).Bold(true)
	hudPulseInfoStyle    = lipgloss.NewStyle().Foreground(cyberInfo).Bold(true)
)

// pulseColor returns the prebuilt pulse style for the current
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
