package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/clim/internal/registry"
)

func (m Model) renderView() string {
	if m.quitting {
		return ""
	}

	var b strings.Builder

	// Title.
	title := titleStyle.Render("clim — CLI Manager")
	switch m.phase {
	case 0:
		title += " " + loadingStyle.Render(m.spinner.View()+" finding tools...")
	case 1:
		title += " " + loadingStyle.Render(m.spinner.View()+" checking versions...")
	default:
		title += " " + loadingStyle.Render(fmt.Sprintf("(%d tools)", len(m.filteredIndex)))
	}
	b.WriteString(title + "\n\n")

	// Layout.
	showPath := m.width >= 120
	showSource := m.width >= 90

	b.WriteString(m.renderHeader(showPath, showSource) + "\n")
	b.WriteString(m.renderSeparator(showPath, showSource) + "\n")

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
		tool := m.tools[idx]
		selected := vi == m.cursor
		b.WriteString(m.renderToolRow(tool, selected, showPath, showSource) + "\n")
	}

	rendered := min(len(m.filteredIndex)-start, visibleRows)
	for range visibleRows - max(rendered, 0) {
		b.WriteString("\n")
	}

	b.WriteString("\n")
	if m.filtering {
		b.WriteString(filterPromptStyle.Render("/ ") + m.filterInput.View() + "\n")
	}
	b.WriteString(m.renderHelp())

	return borderStyle.Render(b.String())
}

func (m Model) renderHeader(showPath, showSource bool) string {
	parts := []string{
		"  " + headerStyle.Render(padRight("Tool", 18)),
		headerStyle.Render(padRight("Version", 14)),
		headerStyle.Render(padRight("Latest", 14)),
	}
	if showSource {
		parts = append(parts, headerStyle.Render(padRight("Source", 10)))
	}
	parts = append(parts, headerStyle.Render(padRight("Status", 14)))
	if showPath {
		parts = append(parts, headerStyle.Render(padRight("Path", 40)))
	}
	return strings.Join(parts, "  ")
}

func (m Model) renderSeparator(showPath, showSource bool) string {
	s := "  " + strings.Repeat("─", 18) + "  " + strings.Repeat("─", 14) + "  " + strings.Repeat("─", 14)
	if showSource {
		s += "  " + strings.Repeat("─", 10)
	}
	s += "  " + strings.Repeat("─", 14)
	if showPath {
		s += "  " + strings.Repeat("─", 40)
	}
	return lipgloss.NewStyle().Foreground(dimColor).Render(s)
}

func (m Model) renderToolRow(tool registry.Tool, selected, showPath, showSource bool) string {
	cursor := "  "
	if selected {
		cursor = "▸ "
	}

	primary := tool.PrimaryInstance()
	name := padRight(tool.DisplayName, 18)

	ver := "—"
	if m.phase < 2 {
		ver = "…"
	} else if primary != nil && primary.Version != "" {
		ver = primary.Version
	}
	verStr := padRight(ver, 14)
	if ver == "—" || ver == "…" {
		verStr = loadingStyle.Render(verStr)
	}

	lat := ""
	if m.phase < 2 {
		lat = "…"
	} else {
		lat = tool.Latest
	}
	latStr := padRight(lat, 14)
	if lat == "…" {
		latStr = loadingStyle.Render(latStr)
	}

	parts := []string{cursor + name, verStr, latStr}

	if showSource {
		src := ""
		if primary != nil {
			src = primary.Source
		}
		parts = append(parts, padRight(src, 10))
	}

	statusText, statusSty := computeRowStatus(ver, tool.Latest, m.phase)
	parts = append(parts, statusSty.Render(padRight(statusText, 14)))

	if showPath {
		p := ""
		if primary != nil {
			p = truncatePath(primary.Path, 40)
		}
		parts = append(parts, padRight(p, 40))
	}

	line := strings.Join(parts, "  ")
	if selected {
		return selectedStyle.Render(line)
	}
	return line
}

func computeRowStatus(installed, latest string, phase int) (string, lipgloss.Style) {
	if phase < 2 {
		return "…", loadingStyle
	}
	if installed == "—" || installed == "" {
		if latest != "" {
			return "?", loadingStyle
		}
		return "", lipgloss.NewStyle()
	}
	if latest == "" {
		return "", lipgloss.NewStyle()
	}
	if registry.VersionsMatch(installed, latest) {
		return "✓ up to date", upToDateStyle
	}
	return "⬆ update", upgradableStyle
}

func (m Model) renderHelp() string {
	return helpStyle.Render(strings.Join([]string{
		"↑/↓ navigate", "/ filter", "r refresh", "q quit",
	}, "  "))
}

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
