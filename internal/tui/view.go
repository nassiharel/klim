package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/clim/internal/version"
)

// renderView renders the full TUI.
func (m Model) renderView() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Title.
	title := titleStyle.Render("clim — CLI Manager")
	if m.loading > 0 {
		title += " " + loadingStyle.Render(m.spinner.View()+" scanning...")
	}
	b.WriteString(title + "\n\n")

	// Calculate column widths based on terminal width.
	showPath := m.width >= 100
	tableWidth := m.width - 6 // account for border padding

	// Header.
	header := m.renderHeader(showPath)
	b.WriteString(header + "\n")

	// Separator.
	sep := m.renderSeparator(showPath)
	b.WriteString(sep + "\n")

	// Rows.
	visibleRows := m.height - 8 // title + header + sep + help + borders
	if visibleRows < 3 {
		visibleRows = 3
	}

	// Scrolling: ensure cursor is visible.
	start := 0
	if m.cursor >= visibleRows {
		start = m.cursor - visibleRows + 1
	}

	for vi := start; vi < len(m.filteredIndex) && vi < start+visibleRows; vi++ {
		idx := m.filteredIndex[vi]
		row := m.tools[idx]
		selected := vi == m.cursor
		b.WriteString(m.renderRow(row, selected, showPath, tableWidth) + "\n")
	}

	// Pad remaining lines.
	rendered := len(m.filteredIndex) - start
	if rendered < visibleRows {
		for i := 0; i < visibleRows-rendered; i++ {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")

	// Filter bar (if active).
	if m.filtering {
		b.WriteString(filterPromptStyle.Render("/ ") + m.filterInput.View() + "\n")
	}

	// Help.
	help := m.renderHelp()
	b.WriteString(help)

	_ = tableWidth

	return borderStyle.Render(b.String())
}

func (m Model) renderHeader(showPath bool) string {
	name := headerStyle.Render(padRight("Tool", 20))
	ver := headerStyle.Render(padRight("Version", 12))
	latest := headerStyle.Render(padRight("Latest", 12))
	status := headerStyle.Render(padRight("Status", 20))

	if showPath {
		path := headerStyle.Render(padRight("Path", 35))
		return fmt.Sprintf("  %s  %s  %s  %s  %s", name, ver, latest, path, status)
	}
	return fmt.Sprintf("  %s  %s  %s  %s", name, ver, latest, status)
}

func (m Model) renderSeparator(showPath bool) string {
	s := "  " + strings.Repeat("─", 20) + "  " +
		strings.Repeat("─", 12) + "  " +
		strings.Repeat("─", 12) + "  "
	if showPath {
		s += strings.Repeat("─", 35) + "  "
	}
	s += strings.Repeat("─", 20)
	return lipgloss.NewStyle().Foreground(dimColor).Render(s)
}

func (m Model) renderRow(row ToolRow, selected, showPath bool, maxWidth int) string {
	cursor := "  "
	if selected {
		cursor = "▸ "
	}

	name := padRight(row.Tool.DisplayName, 20)
	installed := padRight(valueOr(row.InstalledVer, "—"), 12)
	latest := padRight(valueOr(row.LatestVer, "…"), 12)

	// Style based on status.
	statusText := ""
	var statusStyle lipgloss.Style
	switch row.Status {
	case version.StatusUpToDate:
		statusText = "✓ up to date"
		statusStyle = upToDateStyle
	case version.StatusUpgradable:
		statusText = "⬆ upgrade"
		statusStyle = upgradableStyle
	case version.StatusNotInstalled:
		statusText = "✗ not found"
		statusStyle = notFoundStyle
	case version.StatusLoading:
		statusText = "… loading"
		statusStyle = loadingStyle
	case version.StatusError:
		statusText = "? error"
		statusStyle = errorStatusStyle
	}

	styledStatus := statusStyle.Render(padRight(statusText, 20))

	var line string
	if showPath {
		path := padRight(truncatePath(row.Path, 35), 35)
		line = fmt.Sprintf("%s%s  %s  %s  %s  %s", cursor, name, installed, latest, path, styledStatus)
	} else {
		line = fmt.Sprintf("%s%s  %s  %s  %s", cursor, name, installed, latest, styledStatus)
	}

	if selected {
		return selectedStyle.Render(line)
	}
	return line
}

func (m Model) renderHelp() string {
	parts := []string{
		"↑/↓ navigate",
		"/ filter",
		"r refresh",
		"q quit",
	}
	return helpStyle.Render(strings.Join(parts, "  "))
}

// --- Helpers ---

func padRight(s string, width int) string {
	if len(s) >= width {
		return s[:width]
	}
	return s + strings.Repeat(" ", width-len(s))
}

func truncatePath(path string, maxLen int) string {
	if path == "" {
		return "—"
	}
	if len(path) <= maxLen {
		return path
	}
	return "..." + path[len(path)-maxLen+3:]
}

func valueOr(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}
