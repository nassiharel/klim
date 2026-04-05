package tui

import (
	"fmt"
	"strings"

	"github.com/nassiharel/clim/internal/registry"
)

const nameCol = 20

func (m Model) renderView() string {
	if m.quitting {
		return ""
	}

	// Detail view.
	if m.showDetail && m.detailIdx >= 0 && m.detailIdx < len(m.tools) {
		return m.renderDetailView(m.tools[m.detailIdx])
	}

	var b strings.Builder

	// Title + tabs + summary.
	b.WriteString(m.renderTitleBar() + "\n")
	b.WriteString(m.renderTabBar() + "\n\n")

	// Rows.
	visibleRows := m.height - 6
	if visibleRows < 3 {
		visibleRows = 3
	}

	start := 0
	if m.cursor >= visibleRows {
		start = m.cursor - visibleRows + 1
	}

	for vi := start; vi < len(m.filteredIndex) && vi < start+visibleRows; vi++ {
		tool := m.tools[m.filteredIndex[vi]]
		selected := vi == m.cursor
		b.WriteString(m.renderRow(tool, selected) + "\n")
	}

	// Pad.
	rendered := min(len(m.filteredIndex)-start, visibleRows)
	for range max(visibleRows-rendered, 0) {
		b.WriteString("\n")
	}

	// Empty state.
	if len(m.filteredIndex) == 0 && m.phase >= 2 {
		msg := ""
		switch m.activeTab {
		case tabInstalled:
			msg = "  No installed tools found."
		case tabUpdates:
			msg = "  All tools are up to date! ✓"
		case tabDiscover:
			msg = "  All curated tools are already installed!"
		}
		b.WriteString("\n" + dimVersion.Render(msg) + "\n")
	}

	b.WriteString("\n")

	// Filter or help.
	if m.filtering {
		b.WriteString("  " + filterPromptStyle.Render("/") + " " + m.filterInput.View())
	} else {
		b.WriteString(m.renderHelp())
	}

	return b.String()
}

// --- Title & Tabs ---

func (m Model) renderTitleBar() string {
	title := titleStyle.Render("  clim")

	if m.phase == 0 {
		return title + "  " + loadingStyle.Render(m.spinner.View()+" finding tools...")
	}
	if m.phase == 1 {
		return title + "  " + loadingStyle.Render(m.spinner.View()+" checking versions...")
	}

	inst, upd, notInst := m.stats()
	summary := fmt.Sprintf("%d/%d installed", inst, len(m.tools))
	if upd > 0 {
		summary += fmt.Sprintf(" · %s", upgradableStyle.Render(fmt.Sprintf("%d updates", upd)))
	}
	if notInst > 0 {
		summary += fmt.Sprintf(" · %d to discover", notInst)
	}

	return title + "  " + summaryStyle.Render(summary)
}

func (m Model) renderTabBar() string {
	tabs := []struct {
		label string
		idx   int
	}{
		{"Installed", tabInstalled},
		{"Updates", tabUpdates},
		{"Discover", tabDiscover},
	}

	var parts []string
	for _, tab := range tabs {
		style := inactiveTabStyle
		if tab.idx == m.activeTab {
			style = activeTabStyle
		}
		parts = append(parts, style.Render(tab.label))
	}

	return "  " + strings.Join(parts, " ")
}

// --- Row rendering per tab ---

func (m Model) renderRow(tool registry.Tool, selected bool) string {
	var line string

	switch m.activeTab {
	case tabInstalled:
		line = m.renderInstalledRow(tool, selected)
	case tabUpdates:
		line = m.renderUpdateRow(tool, selected)
	case tabDiscover:
		line = m.renderDiscoverRow(tool, selected)
	}

	if selected {
		lineWidth := runeLen(line)
		if lineWidth < m.width {
			line += strings.Repeat(" ", m.width-lineWidth)
		}
		line = selectedRowStyle.Render(line)
	}

	return line
}

func (m Model) renderInstalledRow(tool registry.Tool, selected bool) string {
	cursor := "  "
	if selected {
		cursor = "▸ "
	}

	name := nameStyle.Render(pad(tool.DisplayName, nameCol))
	verInfo := m.renderVersionInfo(tool)

	src := ""
	if primary := tool.PrimaryInstance(); primary != nil && primary.Source != "" {
		src = sourceStyle.Render(string(primary.Source))
	}

	line := cursor + name + "  " + verInfo
	if src != "" {
		line += "  " + src
	}

	// Show instance count if multiple.
	if len(tool.Instances) > 1 {
		line += "  " + dimVersion.Render(fmt.Sprintf("(%d instances)", len(tool.Instances)))
	}

	return line
}

func (m Model) renderUpdateRow(tool registry.Tool, selected bool) string {
	cursor := "  "
	if selected {
		cursor = "▸ "
	}

	name := nameStyle.Render(pad(tool.DisplayName, nameCol))
	ver := tool.InstalledVersion()
	verStr := versionStyle.Render(ver) + arrowStyle.Render(" → ") + upgradableStyle.Render(tool.Latest)

	// Show upgrade command.
	cmd := ""
	if primary := tool.PrimaryInstance(); primary != nil {
		cmd = tool.Packages.UpgradeCmd(primary.Source)
	}

	line := cursor + name + "  " + verStr
	if cmd != "" {
		line += "  " + detailCmdStyle.Render(cmd)
	}

	return line
}

func (m Model) renderDiscoverRow(tool registry.Tool, selected bool) string {
	cursor := "  "
	if selected {
		cursor = "▸ "
	}

	name := pad(tool.DisplayName, nameCol)
	cat := categoryStyle.Render(pad(tool.Category, 12))
	cmd := tool.Packages.BestInstallCmd()

	line := cursor + dimVersion.Render(name) + "  " + cat
	if cmd != "" {
		line += "  " + detailCmdStyle.Render(cmd)
	}

	return line
}

// --- Version info (Installed tab) ---

func (m Model) renderVersionInfo(tool registry.Tool) string {
	if m.phase < 2 {
		return loadingStyle.Render("…")
	}

	primary := tool.PrimaryInstance()
	ver := ""
	if primary != nil {
		ver = primary.Version
	}
	latest := tool.Latest

	if ver == "" && latest == "" {
		return dimVersion.Render("—")
	}
	if ver != "" && latest == "" {
		return versionStyle.Render(ver)
	}
	if ver == "" && latest != "" {
		return dimVersion.Render("—") + arrowStyle.Render(" → ") + versionStyle.Render(latest) + " " + dimVersion.Render("?")
	}
	if registry.VersionsMatch(ver, latest) {
		return versionStyle.Render(ver) + " " + upToDateStyle.Render("✓")
	}
	return versionStyle.Render(ver) + arrowStyle.Render(" → ") + upgradableStyle.Render(latest) + " " + upgradableStyle.Render("⬆")
}

// --- Detail view ---

func (m Model) renderDetailView(tool registry.Tool) string {
	var b strings.Builder

	// Header.
	b.WriteString("  " + detailTitleStyle.Render(tool.DisplayName))
	b.WriteString("  " + categoryStyle.Render(tool.Category))
	b.WriteString("  " + strings.Repeat("─", max(m.width-runeLen(tool.DisplayName)-runeLen(tool.Category)-8, 10)))
	b.WriteString("\n\n")

	if tool.IsInstalled() {
		// Instances.
		b.WriteString("  " + detailLabelStyle.Render("Instances:") + "\n")
		for i, inst := range tool.Instances {
			bullet := "○"
			style := detailSecondary
			if i == 0 {
				bullet = "●"
				style = detailPrimary
			}
			ver := inst.Version
			if ver == "" {
				ver = "—"
			}
			b.WriteString(fmt.Sprintf("    %s  %s  %s  %s\n",
				style.Render(bullet),
				pad(ver, 14),
				sourceStyle.Render(pad(string(inst.Source), 8)),
				dimVersion.Render(registry.TruncatePath(inst.Path, m.width-40)),
			))
		}
		b.WriteString("\n")

		// Version status.
		if tool.Latest != "" {
			if registry.VersionsMatch(tool.InstalledVersion(), tool.Latest) {
				b.WriteString("  " + upToDateStyle.Render("✓ Up to date") + "  " + dimVersion.Render("("+tool.Latest+")") + "\n")
			} else {
				b.WriteString("  " + upgradableStyle.Render("⬆ Update available: "+tool.Latest) + "\n")
			}
			b.WriteString("\n")
		}

		// Commands.
		if primary := tool.PrimaryInstance(); primary != nil {
			if cmd := tool.Packages.UpgradeCmd(primary.Source); cmd != "" {
				b.WriteString("  " + detailLabelStyle.Render("Upgrade:") + "  " + detailCmdStyle.Render(cmd) + "\n")
			}
			if cmd := tool.Packages.RemoveCmd(primary.Source); cmd != "" {
				b.WriteString("  " + detailLabelStyle.Render("Remove: ") + "  " + detailCmdStyle.Render(cmd) + "\n")
			}
		}
	} else {
		b.WriteString("  " + dimVersion.Render("Not installed") + "\n")
		b.WriteString("  " + dimVersion.Render("Recommended developer tool") + "\n\n")
	}

	// Install commands — only for the current OS.
	b.WriteString("\n  " + detailLabelStyle.Render("Install:") + "\n")
	for _, src := range registry.SourcesForOS() {
		if cmd := tool.Packages.InstallCmd(src); cmd != "" {
			b.WriteString("    " + sourceStyle.Render(pad(string(src), 8)) + detailCmdStyle.Render(cmd) + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString("  " + helpStyle.Render("Esc back"))

	return b.String()
}

// --- Help ---

func (m Model) renderHelp() string {
	parts := []string{
		dimVersion.Render("↑↓") + " navigate",
		dimVersion.Render("Tab") + " switch",
		dimVersion.Render("Enter") + " detail",
		dimVersion.Render("/") + " filter",
		dimVersion.Render("r") + " refresh",
		dimVersion.Render("q") + " quit",
	}
	return helpStyle.Render("  " + strings.Join(parts, "   "))
}

// --- Helpers ---

func pad(s string, width int) string {
	r := []rune(s)
	if len(r) >= width {
		return string(r[:width])
	}
	return s + strings.Repeat(" ", width-len(r))
}

func runeLen(s string) int {
	return len([]rune(s))
}
