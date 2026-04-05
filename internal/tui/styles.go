package tui

import (
	"charm.land/lipgloss/v2"
)

var (
	// Colors.
	primaryColor   = lipgloss.Color("39")  // Bright cyan-blue
	successColor   = lipgloss.Color("42")  // Green
	dimColor       = lipgloss.Color("241") // Medium gray
	highlightColor = lipgloss.Color("15")  // Bright white
	warningColor   = lipgloss.Color("214") // Orange
	subtleColor    = lipgloss.Color("244") // Lighter gray
	selectedBg     = lipgloss.Color("236") // Subtle dark gray background
	tabActiveBg    = lipgloss.Color("62")  // Muted purple for active tab

	// Title bar.
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(primaryColor)
	summaryStyle = lipgloss.NewStyle().Foreground(dimColor)

	// Tabs.
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
	versionStyle = lipgloss.NewStyle()
	dimVersion   = lipgloss.NewStyle().Foreground(dimColor)
	arrowStyle   = lipgloss.NewStyle().Foreground(dimColor)

	// Status icons.
	upToDateStyle   = lipgloss.NewStyle().Foreground(successColor)
	upgradableStyle = lipgloss.NewStyle().Foreground(warningColor)

	// Source label.
	sourceStyle = lipgloss.NewStyle().Foreground(subtleColor)

	// Selected row.
	selectedRowStyle = lipgloss.NewStyle().Background(selectedBg)

	// Loading spinner.
	loadingStyle = lipgloss.NewStyle().Foreground(dimColor)

	// Help bar.
	helpStyle = lipgloss.NewStyle().Foreground(dimColor)

	// Filter input.
	filterPromptStyle = lipgloss.NewStyle().Foreground(warningColor).Bold(true)

	// Detail view.
	detailTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(primaryColor)
	detailLabelStyle = lipgloss.NewStyle().Foreground(subtleColor)
	detailCmdStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("222")) // Yellow for commands
	detailPrimary    = lipgloss.NewStyle().Foreground(successColor)
	detailSecondary  = lipgloss.NewStyle().Foreground(dimColor)

	// Category badge.
	categoryStyle = lipgloss.NewStyle().Foreground(subtleColor)
)
