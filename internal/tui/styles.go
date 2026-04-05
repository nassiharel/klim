package tui

import (
	"charm.land/lipgloss/v2"
)

var (
	// Colors.
	primaryColor   = lipgloss.Color("39")  // Blue
	successColor   = lipgloss.Color("42")  // Green
	dimColor       = lipgloss.Color("241") // Gray
	highlightColor = lipgloss.Color("15")  // White/bright
	warningColor   = lipgloss.Color("214") // Orange/Yellow

	// Title.
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			PaddingLeft(1).
			PaddingRight(1)

	// Help bar.
	helpStyle = lipgloss.NewStyle().
			Foreground(dimColor).
			PaddingLeft(1)

	// Table header.
	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(highlightColor).
			Underline(true)

	// Selected row.
	selectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(highlightColor)

	// Loading / dim text.
	loadingStyle = lipgloss.NewStyle().Foreground(dimColor)

	// Status styles.
	upToDateStyle  = lipgloss.NewStyle().Foreground(successColor)
	upgradableStyle = lipgloss.NewStyle().Foreground(warningColor)

	// Border.
	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(1, 2)

	// Filter input.
	filterPromptStyle = lipgloss.NewStyle().
				Foreground(warningColor).
				Bold(true)
)
