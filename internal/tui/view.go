package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m Model) renderView() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Title.
	title := titleStyle.Render("clim — CLI Manager")
	if m.scanning {
		title += " " + loadingStyle.Render(m.spinner.View()+" scanning PATH...")
	} else if m.pending > 0 {
		title += " " + loadingStyle.Render(fmt.Sprintf("%s checking %d...", m.spinner.View(), m.pending))
	} else {
		title += " " + loadingStyle.Render(fmt.Sprintf("(%d tools)", len(m.tools)))
	}
	b.WriteString(title + "\n\n")

	// Layout: adapt columns to terminal width.
	showPath := m.width >= 110
	showLatest := m.width >= 80

	// Header.
	b.WriteString(m.renderHeader(showPath, showLatest) + "\n")
	b.WriteString(m.renderSeparator(showPath, showLatest) + "\n")

	// Rows.
	visibleRows := m.height - 8
	if visibleRows < 3 {
		visibleRows = 3
	}

	start := 0
	if m.cursor >= visibleRows {
		start = m.cursor - visibleRows + 1
	}

	for vi := start; vi < len(m.filteredIndex) && vi < start+visibleRows; vi++ {
		idx := m.filteredIndex[vi]
		row := m.tools[idx]
		selected := vi == m.cursor
		b.WriteString(m.renderRow(row, selected, showPath, showLatest) + "\n")
	}

	rendered := min(len(m.filteredIndex)-start, visibleRows)
	if rendered < visibleRows {
		for range visibleRows - rendered {
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")

	if m.filtering {
		b.WriteString(filterPromptStyle.Render("/ ") + m.filterInput.View() + "\n")
	}

	b.WriteString(m.renderHelp())

	return borderStyle.Render(b.String())
}

func (m Model) renderHeader(showPath, showLatest bool) string {
	name := headerStyle.Render(padRight("Tool", 20))
	ver := headerStyle.Render(padRight("Version", 14))
	var parts []string
	parts = append(parts, "  "+name, ver)
	if showLatest {
		parts = append(parts, headerStyle.Render(padRight("Latest", 14)))
		parts = append(parts, headerStyle.Render(padRight("Status", 14)))
	}
	if showPath {
		parts = append(parts, headerStyle.Render(padRight("Path", 40)))
	}
	return strings.Join(parts, "  ")
}

func (m Model) renderSeparator(showPath, showLatest bool) string {
	s := "  " + strings.Repeat("─", 20) + "  " + strings.Repeat("─", 14)
	if showLatest {
		s += "  " + strings.Repeat("─", 14) + "  " + strings.Repeat("─", 14)
	}
	if showPath {
		s += "  " + strings.Repeat("─", 40)
	}
	return lipgloss.NewStyle().Foreground(dimColor).Render(s)
}

func (m Model) renderRow(row ToolRow, selected, showPath, showLatest bool) string {
	cursor := "  "
	if selected {
		cursor = "▸ "
	}

	name := padRight(row.DisplayName, 20)

	// Version column.
	var verStr string
	if !row.VersionDone {
		verStr = loadingStyle.Render(padRight("…", 14))
	} else if row.Version == "—" {
		verStr = loadingStyle.Render(padRight("—", 14))
	} else {
		verStr = padRight(row.Version, 14)
	}

	var parts []string
	parts = append(parts, cursor+name, verStr)

	if showLatest {
		// Latest column.
		var latStr string
		if !row.LatestDone {
			latStr = loadingStyle.Render(padRight("…", 14))
		} else if row.LatestVersion == "" {
			latStr = padRight("", 14)
		} else {
			latStr = padRight(row.LatestVersion, 14)
		}
		parts = append(parts, latStr)

		// Status column.
		statusText, statusStyle := computeRowStatus(row)
		parts = append(parts, statusStyle.Render(padRight(statusText, 14)))
	}

	if showPath {
		parts = append(parts, padRight(truncatePath(row.Path, 40), 40))
	}

	line := strings.Join(parts, "  ")

	if selected {
		return selectedStyle.Render(line)
	}
	return line
}

func computeRowStatus(row ToolRow) (string, lipgloss.Style) {
	if !row.VersionDone || !row.LatestDone {
		return "…", loadingStyle
	}

	if row.Version == "—" {
		return "?", loadingStyle
	}

	if row.LatestVersion == "" {
		return "", lipgloss.NewStyle() // no latest source
	}

	if versionEqual(row.Version, row.LatestVersion) {
		return "✓ up to date", upToDateStyle
	}
	return "⬆ update", upgradableStyle
}

// versionEqual checks if two version strings refer to the same version.
func versionEqual(a, b string) bool {
	na := normalizeVer(a)
	nb := normalizeVer(b)
	if na == nb {
		return true
	}
	if len(na) > len(nb) {
		return strings.HasPrefix(na, nb+".")
	}
	return strings.HasPrefix(nb, na+".")
}

func normalizeVer(v string) string {
	for {
		if len(v) >= 2 && v[len(v)-2:] == ".0" {
			v = v[:len(v)-2]
		} else {
			break
		}
	}
	return v
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
