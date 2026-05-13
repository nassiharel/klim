package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/registry"
)

// Dashboard color palette — aliases into the cyber theme tokens.
var (
	dashGaugeEmpty = lipgloss.NewStyle().Foreground(cyberRule)
	dashGaugeFill  = lipgloss.NewStyle().Foreground(cyberOK)
	dashGaugeWarn  = lipgloss.NewStyle().Foreground(cyberAccent)
	dashGaugeInfo  = lipgloss.NewStyle().Foreground(cyberPrimary)
	dashNumber     = lipgloss.NewStyle().Bold(true).Foreground(cyberFG)
	dashLabel      = lipgloss.NewStyle().Foreground(cyberFGDim)
	dashSection    = lipgloss.NewStyle().Bold(true).Foreground(cyberPrimary)
	dashDim        = lipgloss.NewStyle().Foreground(cyberFGDim)
)

// namedCount is a generic name+count pair used in dashboard bar charts.
type namedCount struct {
	name  string
	count int
}

// sortByCountDesc sorts named counts by count descending, name ascending.
func sortByCountDesc(items []namedCount) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].count != items[j].count {
			return items[i].count > items[j].count
		}
		return items[i].name < items[j].name
	})
}

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

// renderStatCard renders a single bordered stat card with rounded corners.
func renderStatCard(icon, value, label string, iconStyle lipgloss.Style, width int) string {
	if width < 8 {
		width = 8
	}
	// Build card content, then frame it.
	inner := width - 4 // padding (1 each side) + border (1 each side)
	if inner < 4 {
		inner = 4
	}

	content := iconStyle.Render(icon) + " " + dashNumber.Render(value)
	labelStr := dashLabel.Render(label)

	// Pad content lines to inner width.
	contentW := lipgloss.Width(content)
	if contentW < inner {
		content += strings.Repeat(" ", inner-contentW)
	}
	labelW := lipgloss.Width(labelStr)
	if labelW < inner {
		labelStr += strings.Repeat(" ", inner-labelW)
	}

	border := lipgloss.NewStyle().Foreground(cyberRule)
	top := border.Render("╭" + strings.Repeat("─", inner+2) + "╮")
	mid1 := border.Render("│") + " " + content + " " + border.Render("│")
	mid2 := border.Render("│") + " " + labelStr + " " + border.Render("│")
	bot := border.Render("╰" + strings.Repeat("─", inner+2) + "╯")

	return top + "\n" + mid1 + "\n" + mid2 + "\n" + bot
}

// renderDashboardView renders the Dashboard tab with aggregate stats.
func (m Model) renderDashboardView() string {
	var b strings.Builder

	// Show loading state while scan is in progress.
	if m.phase < phaseDone {
		b.WriteString("\n")
		b.WriteString("  " + dashSection.Render("Dashboard") + "\n\n")
		b.WriteString("  " + cyberSpinnerStyle.Render(spinnerArc(m.animFrame)) + " " + dashDim.Render("Scanning tools...") + "\n")
		return b.String()
	}

	installed, updates, notInstalled := m.stats()
	total := installed + notInstalled
	upToDate := installed - updates

	// §1  HERO STAT CARDS — 4 bordered boxes side by side.
	b.WriteString("\n")

	// Each card = inner + 4 (border+padding). 4 cards + 3 gaps + 2 indent = total.
	cardW := (m.width - 5) / 4 // total width per card including border
	if cardW < 12 {
		cardW = 12
	}
	if cardW > 22 {
		cardW = 22
	}

	cards := []struct {
		icon  string
		value string
		label string
		style lipgloss.Style
	}{
		{"●", strconv.Itoa(installed), "Installed", dashGaugeFill},
		{"▲", strconv.Itoa(updates), "Updates", dashGaugeWarn},
		{"○", strconv.Itoa(notInstalled), "Available", dashGaugeInfo},
		{"★", strconv.Itoa(len(m.favoriteNames)), "Favorites", dashGaugeWarn},
	}

	var cardStrs []string
	for i, c := range cards {
		cardStrs = append(cardStrs, renderStatCard(c.icon, c.value, c.label, c.style, cardW))
		if i < len(cards)-1 {
			cardStrs = append(cardStrs, " ") // gap between cards
		}
	}
	joined := lipgloss.JoinHorizontal(lipgloss.Top, cardStrs...)
	// Indent every line (JoinHorizontal produces multi-line output).
	for i, line := range strings.Split(joined, "\n") {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("  " + line)
	}
	b.WriteString("\n")

	// ═══════════════════════════════════════════════════
	// §2  TOOL COVERAGE — gauges
	// ═══════════════════════════════════════════════════
	b.WriteString("\n  " + dashSection.Render("Tool Coverage") + "\n\n")

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

	b.WriteString(fmt.Sprintf("  %s / %s installed  ",
		dashNumber.Render(strconv.Itoa(installed)),
		dashDim.Render(strconv.Itoa(total)),
	))
	b.WriteString(gauge(installed, total, gaugeWidth, dashGaugeFill, dashGaugeEmpty))
	b.WriteString(fmt.Sprintf("  %s\n", dashNumber.Render(fmt.Sprintf("%d%%", pctInstalled))))

	if installed > 0 {
		pctUpToDate := upToDate * 100 / installed
		b.WriteString(fmt.Sprintf("  %s / %s up to date  ",
			dashNumber.Render(strconv.Itoa(upToDate)),
			dashDim.Render(strconv.Itoa(installed)),
		))
		updateStyle := dashGaugeFill
		if updates > 0 {
			updateStyle = dashGaugeWarn
		}
		b.WriteString(gauge(upToDate, installed, gaugeWidth, updateStyle, dashGaugeEmpty))
		b.WriteString(fmt.Sprintf("  %s\n", dashNumber.Render(fmt.Sprintf("%d%%", pctUpToDate))))
	}

	// ═══════════════════════════════════════════════════
	// §3  ATTENTION NEEDED
	// ═══════════════════════════════════════════════════
	b.WriteString("\n  " + dashSection.Render("Attention") + "\n\n")

	var alerts []string

	// Updates available.
	if updates > 0 {
		var updateNames []string
		for _, t := range m.tools {
			if t.HasUpdate() && len(updateNames) < 2 {
				updateNames = append(updateNames, t.Name)
			}
		}
		msg := fmt.Sprintf("%d tools have updates available", updates)
		if len(updateNames) > 0 {
			msg += " (" + strings.Join(updateNames, ", ")
			if updates > len(updateNames) {
				msg += ", ..."
			}
			msg += ")"
		}
		alerts = append(alerts, "  "+upgradableStyle.Render("▲")+"  "+upgradableStyle.Render(msg))
	}

	// Favorite tools with updates.
	favUpdates := 0
	for _, t := range m.tools {
		if t.HasUpdate() && m.favoriteNames[t.Name] {
			favUpdates++
		}
	}
	if favUpdates > 0 {
		alerts = append(alerts, "  "+dashGaugeWarn.Render("★")+"  "+dashGaugeWarn.Render(
			fmt.Sprintf("%d favorite tool(s) have updates", favUpdates)))
	}

	// New marketplace additions.
	newCount := 0
	for _, t := range m.tools {
		if t.MarketplaceStatus == registry.StatusNew {
			newCount++
		}
	}
	if newCount > 0 {
		alerts = append(alerts, "  "+dashGaugeInfo.Render("●")+"  "+dashGaugeInfo.Render(
			fmt.Sprintf("%d new tools added to marketplace", newCount))+"  "+chipStyle.Render("NEW"))
	}

	if len(alerts) == 0 {
		b.WriteString("  " + upToDateStyle.Render("✓ All good! Everything is up to date.") + "\n")
	} else {
		for _, a := range alerts {
			b.WriteString(a + "\n")
		}
	}

	// ═══════════════════════════════════════════════════
	// §4  GITHUB HIGHLIGHTS — top starred installed tools
	// ═══════════════════════════════════════════════════
	type starredTool struct {
		name     string
		stars    int
		pushedAt string
	}
	var starred []starredTool
	for _, t := range m.tools {
		if t.IsInstalled() && t.GitHubInfo != nil && t.GitHubInfo.Stars > 0 {
			starred = append(starred, starredTool{
				name:     t.DisplayName,
				stars:    t.GitHubInfo.Stars,
				pushedAt: t.GitHubInfo.PushedAt,
			})
		}
	}
	if len(starred) > 0 {
		sort.Slice(starred, func(i, j int) bool {
			return starred[i].stars > starred[j].stars
		})
		if len(starred) > 5 {
			starred = starred[:5]
		}

		b.WriteString("\n  " + dashSection.Render("GitHub Highlights") + "\n\n")
		for _, s := range starred {
			starsStr := dashGaugeWarn.Render(fixedWidth("★ "+formatStars(s.stars), 10))
			nameStr := nameStyle.Render(fixedWidth(s.name, 22))
			pushStr := ""
			if s.pushedAt != "" {
				pushStr = dashDim.Render("pushed " + formatGitHubDate(s.pushedAt))
			}
			b.WriteString(fmt.Sprintf("  %s  %s  %s\n", starsStr, nameStr, pushStr))
		}
	}

	// ═══════════════════════════════════════════════════
	// §5  TOP PICKS FOR YOU — recommendation preview
	// ═══════════════════════════════════════════════════
	if len(m.recommendations) > 0 {
		b.WriteString("\n  " + dashSection.Render("Top Picks for You") + "\n\n")
		shown := len(m.recommendations)
		if shown > 3 {
			shown = 3
		}
		for i := 0; i < shown; i++ {
			b.WriteString(m.renderRecCard(m.recommendations[i], false, true) + "\n")
		}
		if len(m.recommendations) > 3 {
			b.WriteString("  " + dashDim.Render(fmt.Sprintf("… and %d more in Marketplace → For You", len(m.recommendations)-3)) + "\n")
		}
	}

	// ═══════════════════════════════════════════════════
	// §6  PACKAGE MANAGERS — horizontal bar chart
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
		var pms []namedCount
		maxCount := 0
		for name, count := range pmCounts {
			pms = append(pms, namedCount{name, count})
			if count > maxCount {
				maxCount = count
			}
		}
		sortByCountDesc(pms)

		barMax := m.width - 26
		if barMax < 10 {
			barMax = 10
		}
		if barMax > 50 {
			barMax = 50
		}

		// Per-PM bar colors — rotate through the cyber accent
		// palette so each PM gets a distinct but on-theme hue.
		pmColors := []lipgloss.Style{
			lipgloss.NewStyle().Foreground(cyberOK),
			lipgloss.NewStyle().Foreground(cyberPrimary),
			lipgloss.NewStyle().Foreground(cyberAccent),
			lipgloss.NewStyle().Foreground(cyberSecondary),
			lipgloss.NewStyle().Foreground(cyberInfo),
			lipgloss.NewStyle().Foreground(cyberAlert),
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
				dashNumber.Render(fixedWidth(strconv.Itoa(pm.count), 3)),
				dashDim.Render(fmt.Sprintf("(%d%%)", pct)),
			))
		}
	} else {
		b.WriteString("  " + dashDim.Render("No installed tools detected.") + "\n")
	}

	// ═══════════════════════════════════════════════════
	// §7  CATEGORIES — mini bar chart
	// ═══════════════════════════════════════════════════
	b.WriteString("\n  " + dashSection.Render("Categories") + "\n\n")

	catCounts := make(map[string]int)
	for _, tool := range m.tools {
		if tool.IsInstalled() && tool.Category != "" {
			catCounts[tool.Category]++
		}
	}

	if len(catCounts) > 0 {
		var cats []namedCount
		maxCat := 0
		for name, count := range catCounts {
			cats = append(cats, namedCount{name, count})
			if count > maxCat {
				maxCat = count
			}
		}
		sortByCountDesc(cats)

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
				dashNumber.Render(strconv.Itoa(cat.count)),
			))
		}
	} else {
		b.WriteString("  " + dashDim.Render("No categories.") + "\n")
	}

	// ═══════════════════════════════════════════════════
	// §8  TOP TAGS — pill/badge style
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
		var tags []namedCount
		for name, count := range tagCounts {
			tags = append(tags, namedCount{name, count})
		}
		sortByCountDesc(tags)
		if len(tags) > 12 {
			tags = tags[:12]
		}

		tagPill := cyberChipStyle

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
	// §9  PACKS
	// ═══════════════════════════════════════════════════
	b.WriteString("\n  " + dashSection.Render("Packs") + "\n\n")

	installedSet := registry.InstalledSet(m.tools)

	fullMarket, partialMarket := 0, 0
	for _, pack := range m.packs {
		have := 0
		for _, name := range pack.ToolNames {
			if installedSet[name] {
				have++
			}
		}
		if have == len(pack.ToolNames) {
			fullMarket++
		} else if have > 0 {
			partialMarket++
		}
	}

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

	b.WriteString(fmt.Sprintf("  %s  %s  %s\n",
		dashLabel.Render(fixedWidth("Marketplace", 14)),
		gauge(fullMarket, len(m.packs), 15, dashGaugeFill, dashGaugeEmpty),
		fmt.Sprintf("%s / %s complete  %s partial",
			dashNumber.Render(strconv.Itoa(fullMarket)),
			dashDim.Render(strconv.Itoa(len(m.packs))),
			dashGaugeWarn.Render(strconv.Itoa(partialMarket)),
		),
	))

	b.WriteString(fmt.Sprintf("  %s  %s  %s\n",
		dashLabel.Render(fixedWidth("Custom", 14)),
		gauge(fullCustom, len(m.customPacks), 15, dashGaugeInfo, dashGaugeEmpty),
		fmt.Sprintf("%s / %s complete  %s partial",
			dashNumber.Render(strconv.Itoa(fullCustom)),
			dashDim.Render(strconv.Itoa(len(m.customPacks))),
			dashGaugeWarn.Render(strconv.Itoa(partialCustom)),
		),
	))

	// ═══════════════════════════════════════════════════
	// §10  BACKUPS
	// ═══════════════════════════════════════════════════
	b.WriteString("\n  " + dashSection.Render("Backups") + "\n\n")

	backups := m.myBackupFiles
	backupCount := len(backups)

	if backupCount == 0 {
		b.WriteString("  " + dashDim.Render("No backups yet. Export from the Backup tab to create one.") + "\n")
	} else {
		b.WriteString(fmt.Sprintf("  %s backup file(s)\n\n",
			dashNumber.Render(strconv.Itoa(backupCount)),
		))
		shown := backupCount
		if shown > 3 {
			shown = 3
		}
		for i := 0; i < shown; i++ {
			bf := backups[i]
			b.WriteString(fmt.Sprintf("    %s  %s  %s tools\n",
				dashDim.Render("•"),
				dashLabel.Render(bf.modTime.Format("2006-01-02 15:04")),
				dashNumber.Render(strconv.Itoa(bf.toolCount)),
			))
		}
		if backupCount > 3 {
			b.WriteString(fmt.Sprintf("    %s\n",
				dashDim.Render(fmt.Sprintf("… and %d more", backupCount-3)),
			))
		}
	}

	return b.String()
}
