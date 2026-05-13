package tui

import "charm.land/lipgloss/v2"

// styles.go is the legacy palette / style-token layer. Every name
// below is preserved for backwards compatibility — callers from the
// pre-cyber era still reference primaryColor, successColor,
// activeTabStyle, etc. — but the values now alias the canonical
// cyber palette declared in theme.go. New code should import from
// theme.go directly; this file exists only so the rest of the
// package doesn't need a sweeping rename.
var (
	// ── Color aliases ───────────────────────────────────────────
	// Only the aliases still referenced by older render paths are
	// retained. Unused legacy names (successColor, dimColor,
	// warningColor, selectedBg, tabActiveBg, borderColor) have
	// been dropped now that every consumer migrated to the cyber
	// tokens directly.
	primaryColor   = cyberPrimary
	highlightColor = cyberFG
	subtleColor    = cyberFGDim

	// ── Tabs (legacy filled-pill style; left for any render path
	// that hasn't migrated to cyberTab*Style yet — the cyber HUD
	// tab bar in view.go uses the cyber tokens directly).
	activeTabStyle = lipgloss.NewStyle().
			Foreground(cyberPrimary).
			Bold(true).
			Padding(0, 1)
	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(cyberFGDim).
				Padding(0, 1)

	// ── Inline text ─────────────────────────────────────────────
	nameStyle    = lipgloss.NewStyle().Bold(true).Foreground(cyberFG)
	headerStyle  = lipgloss.NewStyle().Foreground(cyberFGDim)
	dimVersion   = lipgloss.NewStyle().Foreground(cyberFGDim)
	loadingStyle = lipgloss.NewStyle().Foreground(cyberFGDim)
	helpStyle    = lipgloss.NewStyle().Foreground(cyberFGDim)

	// ── Status colors ───────────────────────────────────────────
	upToDateStyle   = lipgloss.NewStyle().Foreground(cyberOK)
	upgradableStyle = lipgloss.NewStyle().Foreground(cyberAccent).Bold(true)

	// ── Inline cells ────────────────────────────────────────────
	sourceStyle      = lipgloss.NewStyle().Foreground(cyberFGDim)
	categoryStyle    = lipgloss.NewStyle().Foreground(cyberInfo)
	confirmStyle     = lipgloss.NewStyle().Foreground(cyberAccent).Bold(true)
	selectedRowStyle = cyberSelectedRowStyle

	// ── Detail view ─────────────────────────────────────────────
	detailTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(cyberPrimary)
	detailLabelStyle = lipgloss.NewStyle().Foreground(cyberFGDim)
	detailCmdStyle   = lipgloss.NewStyle().Foreground(cyberAccent)

	// Hero description — readable, not dim.
	heroDescStyle = lipgloss.NewStyle().Foreground(cyberFG)

	// ── Filter input ────────────────────────────────────────────
	filterPromptStyle = lipgloss.NewStyle().Foreground(cyberPrimary).Bold(true)

	// ── Pills / chips ───────────────────────────────────────────
	chipStyle       = cyberChipStyle
	chipAccentStyle = cyberChipAccentStyle

	// ── Buttons ─────────────────────────────────────────────────
	buttonStyle     = cyberButtonStyle
	buttonDoneStyle = cyberButtonDoneStyle

	// ── Rules ───────────────────────────────────────────────────
	ruleStyle = cyberRuleStyle
)
