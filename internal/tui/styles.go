package tui

import (
	"charm.land/lipgloss/v2"
)

var (
	// Colors — distinctive teal/mint palette.
	primaryColor   = lipgloss.Color("37")  // Teal (more distinctive than stock cyan)
	successColor   = lipgloss.Color("78")  // Mint green (warmer than pure green)
	dimColor       = lipgloss.Color("241") // Medium gray
	highlightColor = lipgloss.Color("15")  // Bright white
	warningColor   = lipgloss.Color("179") // Warm gold (softer than orange)
	subtleColor    = lipgloss.Color("244") // Lighter gray
	selectedBg     = lipgloss.Color("236") // Subtle dark gray background
	tabActiveBg    = lipgloss.Color("24")  // Deep teal for active tab
	borderColor    = lipgloss.Color("238") // Subtle border gray

	// Title bar.
	brandStyle   = lipgloss.NewStyle().Bold(true).Foreground(highlightColor).Background(primaryColor).Padding(0, 1)
	summaryStyle = lipgloss.NewStyle().Foreground(dimColor)

	// Tabs — cleaner pill-style.
	activeTabStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(highlightColor).
			Background(tabActiveBg).
			Padding(0, 1)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(subtleColor).
				Padding(0, 1)

	// Tool name.
	nameStyle = lipgloss.NewStyle().Bold(true).Foreground(highlightColor)

	// Table header.
	headerStyle = lipgloss.NewStyle().Foreground(subtleColor)

	// Version text.
	dimVersion = lipgloss.NewStyle().Foreground(dimColor)

	// Status icons.
	upToDateStyle   = lipgloss.NewStyle().Foreground(successColor)
	upgradableStyle = lipgloss.NewStyle().Foreground(warningColor)

	// Source label.
	sourceStyle = lipgloss.NewStyle().Foreground(subtleColor)

	// Selected row — accent left border effect via prefix.
	selectedRowStyle = lipgloss.NewStyle().Background(selectedBg)

	// Loading spinner.
	loadingStyle = lipgloss.NewStyle().Foreground(dimColor)

	// Help bar — with subtle top rule.
	helpStyle = lipgloss.NewStyle().Foreground(dimColor)

	// Filter input.
	filterPromptStyle = lipgloss.NewStyle().Foreground(primaryColor).Bold(true)

	// Detail view.
	detailTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(primaryColor)
	detailLabelStyle = lipgloss.NewStyle().Foreground(subtleColor)
	detailCmdStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("222")) // Yellow for commands

	// Hero description — readable, not dim.
	heroDescStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))

	// Pill backgrounds for metadata chips.
	chipStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("237")).
			Padding(0, 1)
	chipAccentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Background(tabActiveBg).
			Padding(0, 1).
			Bold(true)

	// Category badge.
	categoryStyle = lipgloss.NewStyle().Foreground(subtleColor)

	// Confirmation prompt.
	confirmStyle = lipgloss.NewStyle().Foreground(warningColor).Bold(true)

	// Button.
	buttonStyle = lipgloss.NewStyle().
			Foreground(highlightColor).
			Background(tabActiveBg).
			Padding(0, 2).
			Bold(true)

	buttonDoneStyle = lipgloss.NewStyle().
			Foreground(highlightColor).
			Background(successColor).
			Padding(0, 2).
			Bold(true)

	// Accent rule style for separators.
	ruleStyle = lipgloss.NewStyle().Foreground(borderColor)
)
