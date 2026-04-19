package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
)

// Dashboard color palette.
var (
	dashGaugeEmpty = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	dashGaugeFill  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))  // green
	dashGaugeWarn  = lipgloss.NewStyle().Foreground(lipgloss.Color("214")) // orange
	dashGaugeInfo  = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))  // cyan
	dashNumber     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	dashLabel      = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	dashSection    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	dashDim        = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

// gauge renders a horizontal gauge bar: [████████░░░░░░░░] 75%
func gauge(filled, total, width int, fillStyle, emptyStyle lipgloss.Style) string {
	if total == 0 {
		return emptyStyle.Render(strings.Repeat("░", width))
	}
	pct := filled * width / total
	if pct > width {
		pct = width
	}
	if filled > 0 && pct == 0 {
		pct = 1
	}
	bar := fillStyle.Render(strings.Repeat("█", pct)) +
		emptyStyle.Render(strings.Repeat("░", width-pct))
	return bar
}

// renderDashboardView renders the Dashboard tab with aggregate stats.
func (m Model) renderDashboardView() string {
	var b strings.Builder

	// Show loading state while scan is in progress.
	if m.phase < phaseDone {
		b.WriteString("\n")
		b.WriteString("  " + dashSection.Render("Dashboard") + "\n\n")
		b.WriteString("  " + m.spinner.View() + " " + dashDim.Render("Scanning tools...") + "\n")
		return b.String()
	}

	installed, updates, notInstalled := m.stats()
	total := installed + notInstalled
	upToDate := installed - updates

	// ═══════════════════════════════════════════════════
	// SUMMARY CARDS
	// ═══════════════════════════════════════════════════
	b.WriteString("\n")

	// Installation gauge.
	gaugeWidth := m.width - 30
	if gaugeWidth < 20 {
		gaugeWidth = 20
	}
	if gaugeWidth > 60 {
		gaugeWidth = 60
	}

	pctInstalled := 0
	if total > 0 {
		pctInstalled = installed * 100 / total
	}

	b.WriteString("  " + dashSection.Render("Tool Coverage") + "\n\n")
	b.WriteString(fmt.Sprintf("  %s / %s tools installed  ",
		dashNumber.Render(fmt.Sprintf("%d", installed)),
		dashDim.Render(fmt.Sprintf("%d", total)),
	))
	b.WriteString(gauge(installed, total, gaugeWidth, dashGaugeFill, dashGaugeEmpty))
	b.WriteString(fmt.Sprintf("  %s\n", dashNumber.Render(fmt.Sprintf("%d%%", pctInstalled))))

	// Update status gauge.
	if installed > 0 {
		pctUpToDate := upToDate * 100 / installed
		b.WriteString(fmt.Sprintf("  %s / %s up to date      ",
			dashNumber.Render(fmt.Sprintf("%d", upToDate)),
			dashDim.Render(fmt.Sprintf("%d", installed)),
		))
		updateStyle := dashGaugeFill
		if updates > 0 {
			updateStyle = dashGaugeWarn
		}
		b.WriteString(gauge(upToDate, installed, gaugeWidth, updateStyle, dashGaugeEmpty))
		b.WriteString(fmt.Sprintf("  %s\n", dashNumber.Render(fmt.Sprintf("%d%%", pctUpToDate))))
	}

	// Quick stats row.
	b.WriteString("\n  ")
	statCards := []struct {
		icon  string
		label string
		value string
		style lipgloss.Style
	}{
		{"●", "Installed", fmt.Sprintf("%d", installed), dashGaugeFill},
		{"▲", "Updates", fmt.Sprintf("%d", updates), dashGaugeWarn},
		{"○", "Available", fmt.Sprintf("%d", notInstalled), dashGaugeInfo},
	}
	for i, card := range statCards {
		if i > 0 {
			b.WriteString("   ")
		}
		b.WriteString(card.style.Render(card.icon) + " ")
		b.WriteString(dashNumber.Render(card.value) + " ")
		b.WriteString(dashLabel.Render(card.label))
	}
	b.WriteString("\n")

	// ═══════════════════════════════════════════════════
	// PACKAGE MANAGERS — horizontal bar chart
	// ═══════════════════════════════════════════════════
	b.WriteString("\n  " + dashSection.Render("Package Managers") + "\n\n")

	pmCounts := make(map[string]int)
	for _, tool := range m.tools {
		if !tool.IsInstalled() {
			continue
		}
		primary := tool.PrimaryInstance()
		if primary != nil {
			pmCounts[string(primary.Source)]++
		}
	}

	if len(pmCounts) > 0 {
		type pmEntry struct {
			name  string
			count int
		}
		var pms []pmEntry
		maxCount := 0
		for name, count := range pmCounts {
			pms = append(pms, pmEntry{name, count})
			if count > maxCount {
				maxCount = count
			}
		}
		sort.Slice(pms, func(i, j int) bool {
			if pms[i].count != pms[j].count {
				return pms[i].count > pms[j].count
			}
			return pms[i].name < pms[j].name
		})

		barMax := m.width - 26
		if barMax < 10 {
			barMax = 10
		}
		if barMax > 50 {
			barMax = 50
		}

		// Assign colors by rank.
		pmColors := []lipgloss.Style{
			lipgloss.NewStyle().Foreground(lipgloss.Color("42")),  // green
			lipgloss.NewStyle().Foreground(lipgloss.Color("39")),  // cyan
			lipgloss.NewStyle().Foreground(lipgloss.Color("214")), // orange
			lipgloss.NewStyle().Foreground(lipgloss.Color("135")), // purple
			lipgloss.NewStyle().Foreground(lipgloss.Color("222")), // yellow
			lipgloss.NewStyle().Foreground(lipgloss.Color("167")), // red
		}

		for i, pm := range pms {
			barLen := pm.count * barMax / maxCount
			if barLen < 1 {
				barLen = 1
			}
			pct := pm.count * 100 / installed
			colorStyle := pmColors[i%len(pmColors)]

			bar := colorStyle.Render(strings.Repeat("█", barLen)) +
				dashGaugeEmpty.Render(strings.Repeat("░", barMax-barLen))

			b.WriteString(fmt.Sprintf("  %s  %s  %s  %s\n",
				dashLabel.Render(fixedWidth(pm.name, 8)),
				bar,
				dashNumber.Render(fixedWidth(fmt.Sprintf("%d", pm.count), 3)),
				dashDim.Render(fmt.Sprintf("(%d%%)", pct)),
			))
		}
	} else {
		b.WriteString("  " + dashDim.Render("No installed tools detected.") + "\n")
	}

	// ═══════════════════════════════════════════════════
	// CATEGORIES — mini bar chart with sparklines
	// ═══════════════════════════════════════════════════
	b.WriteString("\n  " + dashSection.Render("Categories") + "\n\n")

	catCounts := make(map[string]int)
	for _, tool := range m.tools {
		if tool.IsInstalled() && tool.Category != "" {
			catCounts[tool.Category]++
		}
	}

	if len(catCounts) > 0 {
		type catEntry struct {
			name  string
			count int
		}
		var cats []catEntry
		maxCat := 0
		for name, count := range catCounts {
			cats = append(cats, catEntry{name, count})
			if count > maxCat {
				maxCat = count
			}
		}
		sort.Slice(cats, func(i, j int) bool {
			if cats[i].count != cats[j].count {
				return cats[i].count > cats[j].count
			}
			return cats[i].name < cats[j].name
		})

		miniW := 10
		for _, cat := range cats {
			barLen := cat.count * miniW / maxCat
			if barLen < 1 {
				barLen = 1
			}
			miniGauge := dashGaugeInfo.Render(strings.Repeat("▓", barLen)) +
				dashGaugeEmpty.Render(strings.Repeat("░", miniW-barLen))
			b.WriteString(fmt.Sprintf("  %s  %s  %s\n",
				dashLabel.Render(fixedWidth(cat.name, 18)),
				miniGauge,
				dashNumber.Render(fmt.Sprintf("%d", cat.count)),
			))
		}
	} else {
		b.WriteString("  " + dashDim.Render("No categories.") + "\n")
	}

	// ═══════════════════════════════════════════════════
	// TOP TAGS — pill/badge style
	// ═══════════════════════════════════════════════════
	b.WriteString("\n  " + dashSection.Render("Top Tags") + "\n\n")

	tagCounts := make(map[string]int)
	for _, tool := range m.tools {
		if !tool.IsInstalled() {
			continue
		}
		for _, tag := range tool.Tags {
			if tag != "" {
				tagCounts[tag]++
			}
		}
	}

	if len(tagCounts) > 0 {
		type tagEntry struct {
			name  string
			count int
		}
		var tags []tagEntry
		for name, count := range tagCounts {
			tags = append(tags, tagEntry{name, count})
		}
		sort.Slice(tags, func(i, j int) bool {
			if tags[i].count != tags[j].count {
				return tags[i].count > tags[j].count
			}
			return tags[i].name < tags[j].name
		})
		if len(tags) > 12 {
			tags = tags[:12]
		}

		tagPill := lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("237")).
			Padding(0, 1)

		line := "  "
		for _, tag := range tags {
			pill := tagPill.Render(fmt.Sprintf("%s %d", tag.name, tag.count))
			pillW := lipgloss.Width(pill) + 1
			if lipgloss.Width(line)+pillW > m.width-4 {
				b.WriteString(line + "\n")
				line = "  "
			}
			line += pill + " "
		}
		if line != "  " {
			b.WriteString(line + "\n")
		}
	} else {
		b.WriteString("  " + dashDim.Render("No tags.") + "\n")
	}

	// ═══════════════════════════════════════════════════
	// PACKS & BACKUPS — summary with sparkline
	// ═══════════════════════════════════════════════════
	b.WriteString("\n  " + dashSection.Render("Packs & Backups") + "\n\n")

	// Build installed tool set for pack status.
	installedSet := make(map[string]bool, len(m.tools))
	for _, tool := range m.tools {
		if tool.IsInstalled() {
			installedSet[tool.Name] = true
		}
	}

	// Count fully/partially installed marketplace packs.
	fullPacks, partialPacks := 0, 0
	for _, pack := range m.packs {
		have := 0
		for _, name := range pack.ToolNames {
			if installedSet[name] {
				have++
			}
		}
		if have == len(pack.ToolNames) {
			fullPacks++
		} else if have > 0 {
			partialPacks++
		}
	}

	// Count fully/partially installed custom packs.
	fullCustom, partialCustom := 0, 0
	for _, pack := range m.customPacks {
		have := 0
		for _, name := range pack.ToolNames {
			if installedSet[name] {
				have++
			}
		}
		if have == len(pack.ToolNames) {
			fullCustom++
		} else if have > 0 {
			partialCustom++
		}
	}

	backups := m.myBackupFiles
	backupCount := len(backups)

	// Marketplace packs gauge.
	b.WriteString(fmt.Sprintf("  %s  %s  %s\n",
		dashLabel.Render(fixedWidth("Marketplace", 14)),
		gauge(fullPacks, len(m.packs), 15, dashGaugeFill, dashGaugeEmpty),
		fmt.Sprintf("%s / %s complete  %s partial",
			dashNumber.Render(fmt.Sprintf("%d", fullPacks)),
			dashDim.Render(fmt.Sprintf("%d", len(m.packs))),
			dashGaugeWarn.Render(fmt.Sprintf("%d", partialPacks)),
		),
	))

	// Custom packs gauge.
	b.WriteString(fmt.Sprintf("  %s  %s  %s\n",
		dashLabel.Render(fixedWidth("Custom", 14)),
		gauge(fullCustom, len(m.customPacks), 15, dashGaugeInfo, dashGaugeEmpty),
		fmt.Sprintf("%s / %s complete  %s partial",
			dashNumber.Render(fmt.Sprintf("%d", fullCustom)),
			dashDim.Render(fmt.Sprintf("%d", len(m.customPacks))),
			dashGaugeWarn.Render(fmt.Sprintf("%d", partialCustom)),
		),
	))

	// Backups summary.
	b.WriteString(fmt.Sprintf("\n  %s  %s\n",
		dashLabel.Render(fixedWidth("Backups", 14)),
		dashNumber.Render(fmt.Sprintf("%d", backupCount)),
	))
	if backupCount > 0 {
		// Show most recent backups (up to 3).
		shown := backupCount
		if shown > 3 {
			shown = 3
		}
		for i := 0; i < shown; i++ {
			bf := backups[i]
			b.WriteString(fmt.Sprintf("    %s  %s  %s tools\n",
				dashDim.Render("•"),
				dashLabel.Render(bf.modTime.Format("2006-01-02 15:04")),
				dashNumber.Render(fmt.Sprintf("%d", bf.toolCount)),
			))
		}
		if backupCount > 3 {
			b.WriteString(fmt.Sprintf("    %s\n",
				dashDim.Render(fmt.Sprintf("… and %d more", backupCount-3)),
			))
		}
	}

	// ═══════════════════════════════════════════════════
	// PLATFORM COVERAGE — donut-style gauge per platform
	// ═══════════════════════════════════════════════════
	b.WriteString("\n  " + dashSection.Render("Platform Coverage") + "\n\n")

	platCounts := make(map[string]int)
	for _, tool := range m.tools {
		for _, p := range derivePlatforms(tool.Packages) {
			platCounts[p]++
		}
	}
	if len(platCounts) > 0 {
		type pe struct {
			name  string
			count int
		}
		var plats []pe
		for name, count := range platCounts {
			plats = append(plats, pe{name, count})
		}
		sort.Slice(plats, func(i, j int) bool {
			if plats[i].count != plats[j].count {
				return plats[i].count > plats[j].count
			}
			return plats[i].name < plats[j].name
		})

		for _, p := range plats {
			pct := p.count * 100 / total
			miniG := gauge(p.count, total, 15, dashGaugeInfo, dashGaugeEmpty)
			b.WriteString(fmt.Sprintf("  %s  %s  %s %s\n",
				dashLabel.Render(fixedWidth(p.name, 10)),
				miniG,
				dashNumber.Render(fixedWidth(fmt.Sprintf("%d", p.count), 4)),
				dashDim.Render(fmt.Sprintf("(%d%%)", pct)),
			))
		}
	}

	return b.String()
}
