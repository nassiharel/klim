package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// renderView renders the full TUI.
func (m Model) renderView() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Title.
	title := titleStyle.Render("clim — PATH Explorer")
	if m.loading {
		title += " " + loadingStyle.Render(m.spinner.View()+" scanning PATH...")
	} else {
		title += " " + loadingStyle.Render(fmt.Sprintf("(%d binaries)", len(m.tools)))
	}
	b.WriteString(title + "\n\n")

	// Header.
	header := m.renderHeader()
	b.WriteString(header + "\n")

	// Separator.
	sep := m.renderSeparator()
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
		b.WriteString(m.renderRow(row, selected) + "\n")
	}

	// Pad remaining lines.
	rendered := min(len(m.filteredIndex)-start, visibleRows)
	if rendered < visibleRows {
		for range visibleRows - rendered {
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

	return borderStyle.Render(b.String())
}

func (m Model) renderHeader() string {
	name := headerStyle.Render(padRight("Name", 25))
	path := headerStyle.Render(padRight("Path", 50))
	return fmt.Sprintf("  %s  %s", name, path)
}

func (m Model) renderSeparator() string {
	s := "  " + strings.Repeat("─", 25) + "  " + strings.Repeat("─", 50)
	return lipgloss.NewStyle().Foreground(dimColor).Render(s)
}

func (m Model) renderRow(row ToolRow, selected bool) string {
	cursor := "  "
	if selected {
		cursor = "▸ "
	}

	name := padRight(row.Name, 25)
	path := padRight(truncatePath(row.Path, 50), 50)
	line := fmt.Sprintf("%s%s  %s", cursor, name, path)

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
