package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/clim/internal/registry"
)

const (
	colName    = 28 // width for name column
	colVersion = 24 // width for version info column
	colSource  = 8  // width for source column
)

func (m Model) renderView() string {
	if m.quitting {
		return ""
	}

	// Detail view.
	if m.showDetail && m.detailIdx >= 0 && m.detailIdx < len(m.tools) {
		return m.renderDetailView(m.tools[m.detailIdx])
	}

	var b strings.Builder

	b.WriteString(m.renderTitleBar() + "\n")
	b.WriteString(m.renderTabBar() + "\n\n")

	// Header row.
	if m.phase >= 1 && len(m.filteredIndex) > 0 {
		b.WriteString(m.renderHeader() + "\n")
	}

	// Rows.
	visibleRows := m.height - 7
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

// --- Header ---

func (m Model) renderHeader() string {
	switch m.activeTab {
	case tabInstalled:
		return "  " +
			headerStyle.Render(fixedWidth("TOOL", colName)) + "  " +
			headerStyle.Render(fixedWidth("VERSION", colVersion)) + "  " +
			headerStyle.Render(fixedWidth("SOURCE", colSource))
	case tabUpdates:
		return "  " +
			headerStyle.Render(fixedWidth("TOOL", colName)) + "  " +
			headerStyle.Render(fixedWidth("UPDATE", colVersion)) + "  " +
			headerStyle.Render("COMMAND")
	case tabDiscover:
		return "  " +
			headerStyle.Render(fixedWidth("TOOL", colName)) + "  " +
			headerStyle.Render(fixedWidth("CATEGORY", 12)) + "  " +
			headerStyle.Render("INSTALL COMMAND")
	}
	return ""
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
		w := lipgloss.Width(line)
		if w < m.width {
			line += strings.Repeat(" ", m.width-w)
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

	// Name column: plain text padded, then styled.
	nameText := toolLabel(tool)
	nameCell := nameStyle.Render(fixedWidth(nameText, colName))

	// Version column: build plain version info, then pad.
	verCell := fixedWidth(m.versionInfoPlain(tool), colVersion)

	// Source column.
	src := ""
	if primary := tool.PrimaryInstance(); primary != nil {
		src = string(primary.Source)
	}
	srcCell := sourceStyle.Render(fixedWidth(src, colSource))

	line := cursor + nameCell + "  " + verCell + "  " + srcCell

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

	nameText := toolLabel(tool)
	nameCell := nameStyle.Render(fixedWidth(nameText, colName))

	ver := tool.InstalledVersion()
	updateText := ver + " → " + tool.Latest
	verCell := fixedWidth(updateText, colVersion)

	cmd := ""
	if primary := tool.PrimaryInstance(); primary != nil {
		cmd = tool.Packages.UpgradeCmd(primary.Source)
	}

	return cursor + nameCell + "  " + verCell + "  " + detailCmdStyle.Render(cmd)
}

func (m Model) renderDiscoverRow(tool registry.Tool, selected bool) string {
	cursor := "  "
	if selected {
		cursor = "▸ "
	}

	nameText := toolLabel(tool)
	nameCell := dimVersion.Render(fixedWidth(nameText, colName))
	catCell := categoryStyle.Render(fixedWidth(tool.Category, 12))
	cmd := tool.Packages.BestInstallCmd()

	return cursor + nameCell + "  " + catCell + "  " + detailCmdStyle.Render(cmd)
}

// --- Version info (plain text, no ANSI) ---

func (m Model) versionInfoPlain(tool registry.Tool) string {
	if m.phase < 2 {
		return "…"
	}

	primary := tool.PrimaryInstance()
	ver := ""
	if primary != nil {
		ver = primary.Version
	}
	latest := tool.Latest

	if ver == "" && latest == "" {
		return "—"
	}
	if ver != "" && latest == "" {
		return ver
	}
	if ver == "" && latest != "" {
		return "— → " + latest + " ?"
	}
	if registry.VersionsMatch(ver, latest) {
		return ver + " ✓"
	}
	return ver + " → " + latest + " ⬆"
}

// --- Detail view ---

func (m Model) renderDetailView(tool registry.Tool) string {
	var b strings.Builder

	// Header.
	label := tool.Name
	if tool.DisplayName != "" && !strings.EqualFold(tool.Name, tool.DisplayName) {
		label += " (" + tool.DisplayName + ")"
	}
	b.WriteString("  " + detailTitleStyle.Render(label))
	b.WriteString("  " + categoryStyle.Render(tool.Category))
	b.WriteString("  " + strings.Repeat("─", max(m.width-len(label)-len(tool.Category)-8, 10)))
	b.WriteString("\n\n")

	if tool.IsInstalled() {
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
			b.WriteString(fmt.Sprintf("    %s  %-14s  %-8s  %s\n",
				style.Render(bullet),
				ver,
				sourceStyle.Render(string(inst.Source)),
				dimVersion.Render(registry.TruncatePath(inst.Path, m.width-40)),
			))
		}
		b.WriteString("\n")

		if tool.Latest != "" {
			if registry.VersionsMatch(tool.InstalledVersion(), tool.Latest) {
				b.WriteString("  " + upToDateStyle.Render("✓ Up to date") + "  " + dimVersion.Render("("+tool.Latest+")") + "\n")
			} else {
				b.WriteString("  " + upgradableStyle.Render("⬆ Update available: "+tool.Latest) + "\n")
			}
			b.WriteString("\n")
		}

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
			b.WriteString(fmt.Sprintf("    %-8s  %s\n",
				sourceStyle.Render(string(src)),
				detailCmdStyle.Render(cmd),
			))
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

// toolLabel returns "name (DisplayName)" or just "name" if they match.
func toolLabel(tool registry.Tool) string {
	if tool.DisplayName == "" || strings.EqualFold(tool.Name, tool.DisplayName) {
		return tool.Name
	}
	return tool.Name + " (" + tool.DisplayName + ")"
}

// fixedWidth pads or truncates a plain string to exactly `width` characters.
// Must be called BEFORE applying lipgloss styles, not after.
func fixedWidth(s string, width int) string {
	r := []rune(s)
	if len(r) > width {
		return string(r[:width-1]) + "…"
	}
	if len(r) < width {
		return s + strings.Repeat(" ", width-len(r))
	}
	return s
}
