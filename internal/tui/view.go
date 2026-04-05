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

	showPath := m.width >= 120
	showSource := m.width >= 90

	b.WriteString(m.renderHeader(showPath, showSource) + "\n")
	b.WriteString(m.renderSeparator(showPath, showSource) + "\n")

	visibleRows := m.height - 8
	if visibleRows < 3 {
		visibleRows = 3
	}

	start := 0
	if m.cursor >= visibleRows {
		start = m.cursor - visibleRows + 1
	}

	for vi := start; vi < len(m.filteredIndex) && vi < start+visibleRows; vi++ {
		tool := m.tools[m.filteredIndex[vi]]
		b.WriteString(m.renderToolRow(tool, vi == m.cursor, showPath, showSource) + "\n")
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
	ver, lat := "—", ""
	if m.phase < 2 {
		ver, lat = "…", "…"
	} else if primary != nil && primary.Version != "" {
		ver = primary.Version
	}
	if m.phase >= 2 {
		lat = tool.Latest
	}

	verStr := padRight(ver, 14)
	latStr := padRight(lat, 14)
	if ver == "—" || ver == "…" {
		verStr = loadingStyle.Render(verStr)
	}
	if lat == "…" {
		latStr = loadingStyle.Render(latStr)
	}

	parts := []string{cursor + padRight(tool.DisplayName, 18), verStr, latStr}

	if showSource && primary != nil {
		parts = append(parts, padRight(string(primary.Source), 10))
	} else if showSource {
		parts = append(parts, padRight("", 10))
	}

	statusText, statusSty := renderStatus(ver, tool.Latest, m.phase)
	parts = append(parts, statusSty.Render(padRight(statusText, 14)))

	if showPath {
		p := ""
		if primary != nil {
			p = registry.TruncatePath(primary.Path, 40)
		}
		parts = append(parts, padRight(p, 40))
	}

	line := strings.Join(parts, "  ")
	if selected {
		return selectedStyle.Render(line)
	}
	return line
}

func renderStatus(installed, latest string, phase int) (string, lipgloss.Style) {
	if phase < 2 {
		return "…", loadingStyle
	}
	status := registry.StatusString(installed, latest)
	switch status {
	case "✓ up to date":
		return status, upToDateStyle
	case "⬆ update":
		return status, upgradableStyle
	default:
		return status, loadingStyle
	}
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
