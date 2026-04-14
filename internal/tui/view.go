package tui

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/clim/internal/build"
	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/registry"
)

const (
	colName     = 28 // width for name column
	colVersion  = 24 // width for version info column
	colSource   = 8  // width for source column
	colCategory = 12 // width for category column
	colStatus   = 18 // width for backup status column
	colSidebar  = 18 // width for filter sidebar panel
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

	// Backup tab has its own rendering path.
	if m.activeTab == tabBackup {
		b.WriteString(m.renderBackupView())
		b.WriteString("\n")
		switch {
		case m.importingPath:
			b.WriteString("  " + confirmStyle.Render("Import:") + " " + m.importInput.View() + "  " + dimVersion.Render("Enter") + " go   " + dimVersion.Render("Esc") + " cancel")
		case m.enteringToken:
			b.WriteString("  " + confirmStyle.Render("Token:") + " " + m.tokenInput.View() + "  " + dimVersion.Render("Enter") + " go   " + dimVersion.Render("Esc") + " cancel")
		default:
			b.WriteString(m.renderHelp())
		}
		return b.String()
	}

	// Config tab has its own rendering path.
	if m.activeTab == tabConfig {
		b.WriteString(m.renderConfigView())
		b.WriteString("\n")
		b.WriteString(m.renderHelp())
		return b.String()
	}

	// Search bar.
	b.WriteString(m.renderSearchBar() + "\n")

	// Two-column layout: sidebar | tool list.
	visibleRows := m.height - 8
	if visibleRows < 3 {
		visibleRows = 3
	}

	sidebarLines := m.buildSidebarLines(visibleRows)
	toolLines := m.buildToolLines(visibleRows)

	totalLines := max(len(sidebarLines), len(toolLines))
	sidebarOnRight := m.cfg != nil && m.cfg.UI.SidebarRight

	for i := range totalLines {
		left := ""
		if i < len(sidebarLines) {
			left = sidebarLines[i]
		}
		right := ""
		if i < len(toolLines) {
			right = toolLines[i]
		}

		if sidebarOnRight {
			b.WriteString(right + " │ " + left + "\n")
		} else {
			b.WriteString(fixedWidthANSI(left, colSidebar) + " │ " + right + "\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(m.renderHelp())

	return b.String()
}

// --- Title & Tabs ---

func (m Model) renderTitleBar() string {
	title := titleStyle.Render("  clim")

	if m.phase == phaseScanning {
		return title + "  " + loadingStyle.Render(m.spinner.View()+" finding tools...")
	}
	if m.phase == phaseResolving && m.pending > 0 {
		return title + "  " + loadingStyle.Render(fmt.Sprintf("%s checking versions (%d remaining)...", m.spinner.View(), m.pending))
	}

	inst, upd, notInst := m.stats()
	active := inst + notInst
	summary := fmt.Sprintf("%d/%d installed", inst, active)
	if upd > 0 {
		summary += " · " + upgradableStyle.Render(strconv.Itoa(upd)+" updates")
	}
	if notInst > 0 {
		summary += fmt.Sprintf(" · %d in marketplace", notInst)
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
		{"Marketplace", tabDiscover},
		{"Backup", tabBackup},
		{"Config", tabConfig},
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

// --- Search Bar ---

// renderSearchBar renders the search box.
// Press / to focus the search box. Press f to focus the filter sidebar.
func (m Model) renderSearchBar() string {
	var b strings.Builder

	// Search input.
	switch {
	case m.filtering:
		b.WriteString("  " + filterPromptStyle.Render("/") + " " + m.filterInput.View())
	case m.filterText != "":
		b.WriteString("  " + filterPromptStyle.Render("/") + " " + dimVersion.Render(m.filterText))
	default:
		b.WriteString("  " + dimVersion.Render("/ search..."))
	}

	return b.String()
}

// --- Header ---

func (m Model) renderHeader() string {
	switch m.activeTab {
	case tabInstalled:
		return "  " +
			headerStyle.Render(fixedWidth("TOOL", colName)) + "  " +
			headerStyle.Render(fixedWidth("VERSION", colVersion)) + "  " +
			headerStyle.Render(fixedWidth("SOURCE", colSource)) + "  " +
			headerStyle.Render(fixedWidth("CATEGORY", colCategory))
	case tabUpdates:
		return "      " +
			headerStyle.Render(fixedWidth("TOOL", colName)) + "  " +
			headerStyle.Render(fixedWidth("UPDATE", colVersion)) + "  " +
			headerStyle.Render(fixedWidth("SOURCE", colSource)) + "  " +
			headerStyle.Render(fixedWidth("CATEGORY", colCategory))
	case tabDiscover:
		return "  " +
			headerStyle.Render(fixedWidth("TOOL", colName)) + "  " +
			headerStyle.Render(fixedWidth("CATEGORY", colCategory)) + "  " +
			headerStyle.Render("STATUS")
	case tabBackup:
		return "  " +
			headerStyle.Render(fixedWidth("TOOL", colName)) + "  " +
			headerStyle.Render(fixedWidth("STATUS", colStatus)) + "  " +
			headerStyle.Render(fixedWidth("SOURCE", colSource))
	}
	return ""
}

// --- Row rendering per tab ---

func (m Model) renderRow(tool registry.Tool, toolIdx int, selected bool) string {
	var line string

	switch m.activeTab {
	case tabInstalled:
		line = m.renderInstalledRow(tool, selected)
	case tabUpdates:
		line = m.renderUpdateRow(tool, toolIdx, selected)
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

	// Version column: styled version info, then pad to fixed width.
	verCell := fixedWidthANSI(m.versionInfoStyled(tool), colVersion)

	// Source column.
	src := ""
	if primary := tool.PrimaryInstance(); primary != nil {
		src = string(primary.Source)
	}
	srcCell := sourceStyle.Render(fixedWidth(src, colSource))
	catCell := categoryStyle.Render(fixedWidth(tool.Category, colCategory))

	line := cursor + nameCell + "  " + verCell + "  " + srcCell + "  " + catCell

	if len(tool.Instances) > 1 {
		line += "  " + dimVersion.Render(fmt.Sprintf("(%d instances)", len(tool.Instances)))
	}

	return line
}

func (m Model) renderUpdateRow(tool registry.Tool, toolIdx int, selected bool) string {
	cursor := "  "
	if selected {
		cursor = "▸ "
	}

	// Selection checkbox.
	check := ""
	if m.updateSelected[toolIdx] {
		check = upToDateStyle.Render("[✓]") + " "
	} else {
		check = dimVersion.Render("[ ]") + " "
	}

	nameText := toolLabel(tool)
	nameCell := nameStyle.Render(fixedWidth(nameText, colName))

	ver := tool.InstalledVersion()
	updateText := ver + " → " + tool.Latest
	verCell := fixedWidth(updateText, colVersion)

	src := ""
	if primary := tool.PrimaryInstance(); primary != nil {
		src = string(primary.Source)
	}
	srcCell := sourceStyle.Render(fixedWidth(src, colSource))
	catCell := categoryStyle.Render(fixedWidth(tool.Category, colCategory))

	return cursor + check + nameCell + "  " + verCell + "  " + srcCell + "  " + catCell
}

func (m Model) renderDiscoverRow(tool registry.Tool, selected bool) string {
	cursor := "  "
	if selected {
		cursor = "▸ "
	}

	nameText := toolLabel(tool)
	nameCell := dimVersion.Render(fixedWidth(nameText, colName))
	catCell := categoryStyle.Render(fixedWidth(tool.Category, colCategory))

	var badge string
	switch tool.MarketplaceStatus {
	case registry.StatusNew:
		badge = "  " + upgradableStyle.Render("NEW")
	case registry.StatusChanged:
		badge = "  " + detailTitleStyle.Render("UPDATED")
	}

	return cursor + nameCell + "  " + catCell + badge
}

// --- Version info (plain text, no ANSI) ---

func (m Model) versionInfoStyled(tool registry.Tool) string {
	// Tool still resolving — show spinner placeholder.
	if m.phase < phaseDone && !toolResolved(tool) {
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
	if tool.HasUpdate() {
		return ver + " → " + upgradableStyle.Render(latest+" ⬆")
	}
	return ver + " " + upToDateStyle.Render("✓")
}

// --- Detail view ---

func (m Model) renderDetailView(tool registry.Tool) string {
	var b strings.Builder
	label := detailLabelStyle.Render
	dim := dimVersion.Render

	// ── Header ──────────────────────────────────────────────────
	nameLabel := tool.Name
	if tool.DisplayName != "" && !strings.EqualFold(tool.Name, tool.DisplayName) {
		nameLabel += " (" + tool.DisplayName + ")"
	}
	b.WriteString("  " + detailTitleStyle.Render(nameLabel))
	b.WriteString("  " + categoryStyle.Render(tool.Category))
	divLen := max(m.width-lipgloss.Width(nameLabel)-lipgloss.Width(tool.Category)-8, 10)
	b.WriteString("  " + strings.Repeat("─", divLen))
	b.WriteString("\n\n")

	// ── Description (word-wrapped) ──────────────────────────────
	switch {
	case tool.Info != nil:
		if tool.Info.Description != "" {
			maxW := m.width - 6
			if maxW < 20 {
				maxW = 20
			}
			for _, line := range wordWrap(tool.Info.Description, maxW) {
				b.WriteString("  " + dim(line) + "\n")
			}
			b.WriteString("\n")
		}
		// Metadata table.
		if tool.Info.Publisher != "" {
			b.WriteString("  " + label("Publisher:  ") + tool.Info.Publisher + "\n")
		}
		if tool.Info.Homepage != "" {
			b.WriteString("  " + label("Homepage:   ") + dim(tool.Info.Homepage) + "\n")
		}
		if tool.Info.License != "" {
			b.WriteString("  " + label("License:    ") + tool.Info.License + "\n")
		}
		if tool.Info.ReleaseDate != "" {
			b.WriteString("  " + label("Released:   ") + tool.Info.ReleaseDate + "\n")
		}
		b.WriteString("\n")
	case tool.InfoFetched:
		b.WriteString("  " + dim("No metadata available.") + "\n\n")
	default:
		b.WriteString("  " + loadingStyle.Render("Loading info...") + "\n\n")
	}

	// ── Version & Status ────────────────────────────────────────
	if tool.IsInstalled() {
		ver := tool.InstalledVersion()
		if ver == "" {
			ver = "—"
		}
		b.WriteString("  " + label("Version:    ") + nameStyle.Render(ver))
		if tool.Latest != "" {
			if registry.VersionsMatch(ver, tool.Latest) {
				b.WriteString("  " + upToDateStyle.Render("✓ up to date"))
			} else if tool.HasUpdate() {
				b.WriteString("  " + upgradableStyle.Render("⬆ "+tool.Latest+" available"))
			}
			if tool.LatestFrom != "" {
				b.WriteString("  " + dim("(via "+tool.LatestFrom+")"))
			}
		}
		b.WriteString("\n")
	} else {
		b.WriteString("  " + label("Status:     ") + dim("Not installed") + "\n")
	}

	// ── Instances ───────────────────────────────────────────────
	if tool.IsInstalled() {
		b.WriteString("  " + label("Instances:  "))
		if len(tool.Instances) == 1 {
			b.WriteString(dim("1 installation") + "\n")
		} else {
			b.WriteString(upgradableStyle.Render(fmt.Sprintf("%d installations", len(tool.Instances))) + "\n")
		}
		for i, inst := range tool.Instances {
			bullet := "○"
			style := detailSecondary
			if i == 0 {
				bullet = "●"
				style = detailPrimary
			}
			instVer := inst.Version
			if instVer == "" {
				instVer = "—"
			}
			fmt.Fprintf(&b, "    %s  %-14s  %-8s  %s\n",
				style.Render(bullet),
				instVer,
				sourceStyle.Render(string(inst.Source)),
				dim(registry.TruncatePath(inst.Path, m.width-40)),
			)
		}
		b.WriteString("\n")

		// Smart recommendations for multiple instances.
		if len(tool.Instances) > 1 {
			b.WriteString(m.renderInstanceRecommendations(tool))
		}
	}

	// ── Supported Platforms ─────────────────────────────────────
	platforms := derivePlatforms(tool.Packages)
	if len(platforms) > 0 {
		b.WriteString("  " + label("Platforms:  ") + dim(strings.Join(platforms, ", ")) + "\n")
	}

	// ── Binary names ────────────────────────────────────────────
	if len(tool.BinaryNames) > 0 {
		b.WriteString("  " + label("Binaries:   ") + dim(strings.Join(tool.BinaryNames, ", ")) + "\n")
	}
	b.WriteString("\n")

	// ── Install / Upgrade / Remove commands ─────────────────────
	if tool.IsInstalled() {
		if primary := tool.PrimaryInstance(); primary != nil {
			if cmd := tool.Packages.UpgradeCmd(primary.Source); cmd != "" {
				b.WriteString("  " + label("Upgrade:    ") + detailCmdStyle.Render(cmd) + "\n")
			}
			if cmd := tool.Packages.RemoveCmd(primary.Source); cmd != "" {
				b.WriteString("  " + label("Remove:     ") + detailCmdStyle.Render(cmd) + "\n")
			}
		}
		b.WriteString("\n")
	}

	// Install commands for all available sources on this OS.
	installCmds := m.collectInstallCmds(tool)
	if len(installCmds) > 0 {
		b.WriteString("  " + label("Install:") + "\n")
		for _, ic := range installCmds {
			fmt.Fprintf(&b, "    %-8s  %s\n",
				sourceStyle.Render(ic.source),
				detailCmdStyle.Render(ic.cmd),
			)
		}
		b.WriteString("\n")
	}

	// ── Action menu ─────────────────────────────────────────────
	if len(m.toolMenuItems) > 0 {
		b.WriteString("  " + label("Actions:") + "\n")
		for i, item := range m.toolMenuItems {
			cursor := "  "
			if i == m.toolMenu {
				cursor = "▸ "
			}
			line := "  " + cursor + nameStyle.Render(item.label)
			if i == m.toolMenu {
				w := lipgloss.Width(line)
				if w < m.width {
					line += strings.Repeat(" ", m.width-w)
				}
				line = selectedRowStyle.Render(line)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}

	// ── Help bar ────────────────────────────────────────────────
	switch {
	case m.pendingAction != nil:
		prompt := confirmStyle.Render(fmt.Sprintf("  Run %s?", strings.Join(m.pendingAction.cmdArgs, " ")))
		keys := dim("y") + " confirm   " + dim("Esc") + " cancel"
		b.WriteString(prompt + "  " + keys)
	default:
		hints := []string{
			dim("↑↓") + " navigate",
			dim("Enter") + " select",
			dim("Esc") + " back",
		}
		b.WriteString("  " + helpStyle.Render(strings.Join(hints, "   ")))
	}

	return b.String()
}

// installCmdEntry pairs a source label with the formatted command string.
type installCmdEntry struct {
	source string
	cmd    string
}

// collectInstallCmds returns install commands for all available sources on this OS.
func (m Model) collectInstallCmds(tool registry.Tool) []installCmdEntry {
	var entries []installCmdEntry
	for _, src := range registry.SourcesForOS() {
		if cmd := tool.Packages.InstallCmd(src); cmd != "" {
			entries = append(entries, installCmdEntry{
				source: string(src),
				cmd:    cmd,
			})
		}
	}
	return entries
}

// derivePlatforms infers supported operating systems from which package manager
// IDs are defined. Returns human-readable labels like "Windows", "macOS", "Linux".
func derivePlatforms(pkgs registry.PackageIDs) []string {
	var platforms []string
	seen := make(map[string]bool)

	add := func(label string) {
		if !seen[label] {
			seen[label] = true
			platforms = append(platforms, label)
		}
	}

	if pkgs.Winget != "" || pkgs.Choco != "" {
		add("Windows")
	}
	if pkgs.Brew != "" {
		add("macOS")
		add("Linux")
	}
	if pkgs.Apt != "" || pkgs.Snap != "" {
		add("Linux")
	}
	if pkgs.NPM != "" {
		add("Windows")
		add("macOS")
		add("Linux")
	}
	return platforms
}

// wordWrap breaks text into lines that fit within maxWidth display columns.
// Uses lipgloss.Width for correct handling of multi-byte UTF-8 and wide characters.
func wordWrap(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	var lines []string
	current := words[0]
	for _, word := range words[1:] {
		if lipgloss.Width(current)+1+lipgloss.Width(word) > maxWidth {
			lines = append(lines, current)
			current = word
		} else {
			current += " " + word
		}
	}
	lines = append(lines, current)
	return lines
}

// renderInstanceRecommendations analyzes multiple installations and gives
// actionable advice: version conflicts, stale installs, PATH priority issues.
func (m Model) renderInstanceRecommendations(tool registry.Tool) string {
	var tips []string
	primary := tool.Instances[0]

	// Find the newest version among all instances.
	newestVer := primary.Version
	newestIdx := 0
	for i, inst := range tool.Instances {
		if inst.Version != "" && inst.Version != "—" {
			if newestVer == "" || !registry.VersionsMatch(inst.Version, newestVer) {
				// Compare versions numerically to find the newest.
				if registry.CompareVersions(inst.Version, newestVer) > 0 {
					newestVer = inst.Version
					newestIdx = i
				}
			}
		}
	}

	// Tip: primary is not the newest version.
	if newestIdx != 0 && newestVer != "" && primary.Version != "" &&
		!registry.VersionsMatch(primary.Version, newestVer) {
		newer := tool.Instances[newestIdx]
		tips = append(tips, upgradableStyle.Render("⚠")+fmt.Sprintf(
			"  PATH priority: %s (%s) is active, but %s (%s) has a newer version %s",
			sourceStyle.Render(string(primary.Source)),
			primary.Version,
			sourceStyle.Render(string(newer.Source)),
			newer.Version,
			newestVer,
		))
	}

	// Tip: stale manual installs with no version.
	for _, inst := range tool.Instances[1:] {
		if inst.Source == registry.SourceManual && inst.Version == "" {
			tips = append(tips, dimVersion.Render("⚠")+fmt.Sprintf(
				"  Unknown version at %s — consider removing this stale install",
				dimVersion.Render(registry.TruncatePath(inst.Path, 50)),
			))
		}
	}

	// Tip: multiple package managers managing the same tool.
	sources := make(map[registry.InstallSource]bool)
	for _, inst := range tool.Instances {
		if inst.Source != registry.SourceManual {
			sources[inst.Source] = true
		}
	}
	if len(sources) > 1 {
		var srcNames []string
		for src := range sources {
			srcNames = append(srcNames, string(src))
		}
		tips = append(tips, dimVersion.Render("💡")+fmt.Sprintf(
			"  Multiple package managers (%s) — consider standardizing on one to avoid conflicts",
			strings.Join(srcNames, ", "),
		))
	}

	// Tip: suggest removing duplicates.
	if len(tool.Instances) >= 3 {
		tips = append(tips, dimVersion.Render("💡")+fmt.Sprintf(
			"  %d installations found — consider removing unused ones to simplify your PATH",
			len(tool.Instances),
		))
	}

	if len(tips) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("  " + detailLabelStyle.Render("Recommendations:") + "\n")
	for _, tip := range tips {
		b.WriteString("    " + tip + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

// --- Backup tab ---

func (m Model) renderBackupView() string {
	var b strings.Builder

	if m.backupMode == backupModeIdle {
		b.WriteString("\n")

		type menuItem struct {
			label string
			desc  string
		}
		items := []menuItem{
			{"Export", "Save installed tools to a manifest file"},
			{"Import", "Reinstall tools from a manifest file"},
			{"Share", "Generate a share token for chat/messaging"},
			{"Open Token", "Install tools from a share token"},
		}

		for i, item := range items {
			cursor := "  "
			if i == m.cursor {
				cursor = "▸ "
			}
			line := cursor + nameStyle.Render(fixedWidth(item.label, 12)) + "  " + dimVersion.Render(item.desc)
			if i == m.cursor {
				w := lipgloss.Width(line)
				if w < m.width {
					line += strings.Repeat(" ", m.width-w)
				}
				line = selectedRowStyle.Render(line)
			}
			b.WriteString(line + "\n")
		}

		// Pad remaining space.
		visibleRows := m.height - 12
		for range max(visibleRows, 0) {
			b.WriteString("\n")
		}
		return b.String()
	}

	// Share token display mode.
	if m.backupMode == backupModeShare {
		b.WriteString("\n")
		b.WriteString("  " + detailTitleStyle.Render("Share Token") + "\n\n")
		b.WriteString("  " + dimVersion.Render("Send this token via Slack, Teams, or any chat:") + "\n\n")

		// Word-wrap the token to fit the terminal width.
		maxW := m.width - 6
		if maxW < 40 {
			maxW = 40
		}
		for _, line := range wordWrap(m.sharedToken, maxW) {
			b.WriteString("  " + dimVersion.Render(line) + "\n")
		}

		b.WriteString("\n")

		// Copy button.
		if m.tokenCopied {
			b.WriteString("  " + buttonDoneStyle.Render("✓ Copied to clipboard") + "\n")
		} else {
			b.WriteString("  " + buttonStyle.Render("⎘ Copy to clipboard (c)") + "\n")
		}

		b.WriteString("\n")
		b.WriteString("  " + dimVersion.Render("Recipients can install with:") + "  " + detailCmdStyle.Render("clim open <token>") + "\n")

		// Pad remaining space.
		visibleRows := m.height - 16
		for range max(visibleRows, 0) {
			b.WriteString("\n")
		}
		return b.String()
	}

	// Confirm mode — show review header instead of progress bar.
	if m.backupConfirm {
		pending := 0
		selected := 0
		skipped := 0
		for _, item := range m.backupItems {
			switch item.status {
			case backupPending:
				pending++
				if item.selected {
					selected++
				}
			case backupSkipped, backupFailed:
				skipped++
			}
		}
		b.WriteString("\n")
		b.WriteString(confirmStyle.Render("  Review import plan") + "  " +
			dimVersion.Render(fmt.Sprintf("%d selected of %d to install, %d skipped", selected, pending, skipped)) + "\n\n")
	} else {
		// Progress bar.
		total := len(m.backupItems)
		if total > 0 {
			frac := float64(m.backupDone) / float64(total)
			barWidth := m.width - 30
			if barWidth < 20 {
				barWidth = 20
			}
			m.backupBar.SetWidth(barWidth)
			fmt.Fprintf(&b, "  %s  %s  %d/%d\n\n",
				detailLabelStyle.Render("Progress:"),
				m.backupBar.ViewAs(frac),
				m.backupDone, total,
			)
		}
	}

	// Header.
	b.WriteString(m.renderHeader() + "\n")

	// Backup rows.
	visibleRows := m.height - 11
	if visibleRows < 3 {
		visibleRows = 3
	}

	start := 0
	if m.cursor >= visibleRows {
		start = m.cursor - visibleRows + 1
	}

	for vi := start; vi < len(m.backupItems) && vi < start+visibleRows; vi++ {
		item := m.backupItems[vi]
		selected := vi == m.cursor
		b.WriteString(m.renderBackupRow(item, selected, m.backupConfirm) + "\n")
	}

	// Pad.
	rendered := min(len(m.backupItems)-start, visibleRows)
	for range max(visibleRows-rendered, 0) {
		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) renderBackupRow(item backupItem, selected bool, confirmMode bool) string {
	cursor := "  "
	if selected {
		cursor = "▸ "
	}

	// Status icon + label. Compute plain text first, style after fixedWidth.
	var icon string
	var statusLabel string
	var statusStyle lipgloss.Style
	switch item.status {
	case backupPending:
		if confirmMode {
			// Show selection checkbox during confirm mode.
			if item.selected {
				icon = upToDateStyle.Render("[✓]")
			} else {
				icon = dimVersion.Render("[ ]")
			}
		} else {
			icon = dimVersion.Render(" ○ ")
		}
		statusLabel = "pending"
		statusStyle = dimVersion
	case backupRunning:
		icon = upgradableStyle.Render(" ◉ ")
		statusLabel = "installing"
		statusStyle = upgradableStyle
	case backupDone:
		icon = upToDateStyle.Render(" ✓ ")
		statusLabel = "done"
		statusStyle = upToDateStyle
	case backupFailed:
		icon = upgradableStyle.Render(" ✗ ")
		if item.errMsg != "" {
			statusLabel = item.errMsg
		} else {
			statusLabel = "failed"
		}
		statusStyle = upgradableStyle
	case backupSkipped:
		icon = dimVersion.Render(" – ")
		if item.errMsg != "" {
			statusLabel = item.errMsg
		} else {
			statusLabel = "skipped"
		}
		statusStyle = dimVersion
	}

	nameCell := nameStyle.Render(fixedWidth(item.display, colName))
	statusCell := statusStyle.Render(fixedWidth(statusLabel, colStatus))
	sourceCell := sourceStyle.Render(fixedWidth(item.source, colSource))

	line := cursor + icon + " " + nameCell + "  " + statusCell + "  " + sourceCell

	if selected {
		// Pad to full width for selection highlight.
		w := lipgloss.Width(line)
		if w < m.width {
			line += strings.Repeat(" ", m.width-w)
		}
		line = selectedRowStyle.Render(line)
	}

	return line
}

// --- Config tab ---

func (m Model) renderConfigView() string {
	var b strings.Builder
	label := detailLabelStyle.Render
	dim := dimVersion.Render

	// Version info.
	b.WriteString("\n")
	fmt.Fprintf(&b, "  %s  %s\n", label(fixedWidth("Version", 18)), build.Info())
	fmt.Fprintf(&b, "  %s  %s / %s\n", label(fixedWidth("OS / Arch", 18)), runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "  %s  %s\n", label(fixedWidth("Go", 18)), runtime.Version())

	// File paths.
	b.WriteString("\n")
	marketplacePath := dim("(unknown)")
	if p, err := registry.ToolsPath(); err == nil {
		marketplacePath = p
	}
	fmt.Fprintf(&b, "  %s  %s\n", label(fixedWidth("Marketplace", 18)), marketplacePath)

	if m.cfg != nil && m.cfg.Marketplace.URL != "" {
		fmt.Fprintf(&b, "  %s  %s\n", label(fixedWidth("Catalog URL", 18)), m.cfg.Marketplace.URL)
	}

	configPath := dim("(unknown)")
	if p, err := config.Path(); err == nil {
		configPath = p
	}
	fmt.Fprintf(&b, "  %s  %s\n", label(fixedWidth("Config", 18)), configPath)

	// Editor.
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		editor = dim("(not set)")
	} else {
		editor += "  " + dim("($EDITOR)")
	}
	fmt.Fprintf(&b, "  %s  %s\n", label(fixedWidth("Editor", 18)), editor)

	// Configuration values.
	if m.cfg != nil {
		b.WriteString("\n")
		b.WriteString("  " + label("Configuration") + "\n")
		if m.cfg.Marketplace.URL != "" {
			fmt.Fprintf(&b, "    %-22s %s\n", dim("marketplace.url"), m.cfg.Marketplace.URL)
		} else {
			fmt.Fprintf(&b, "    %-22s %s\n", dim("marketplace.url"), dim("(default)"))
		}
		fmt.Fprintf(&b, "    %-22s %v\n", dim("marketplace.auto_refresh"), m.cfg.Marketplace.AutoRefresh)
		fmt.Fprintf(&b, "    %-22s %s\n", dim("marketplace.interval"), m.cfg.Marketplace.RefreshInterval.Duration)
		fmt.Fprintf(&b, "    %-22s %d %s\n", dim("performance.concurrency"), m.cfg.Performance.Concurrency, dim("(0=auto)"))
		fmt.Fprintf(&b, "    %-22s %s\n", dim("performance.timeout"), m.cfg.Performance.CommandTimeout.Duration)
		fmt.Fprintf(&b, "    %-22s %s\n", dim("ui.default_tab"), m.cfg.UI.DefaultTab)
		fmt.Fprintf(&b, "    %-22s %v\n", dim("ui.show_path"), m.cfg.UI.ShowPath)
	}

	// Package managers.
	b.WriteString("\n")
	b.WriteString("  " + label("Package Managers") + "\n")
	for _, pm := range registry.AllPMStatusForOS() {
		icon := upgradableStyle.Render("✗")
		status := dim("not found")
		if pm.Available {
			icon = upToDateStyle.Render("✓")
			status = upToDateStyle.Render("installed")
		}
		fmt.Fprintf(&b, "    %s  %-10s %s\n", icon, string(pm.Source), status)
	}

	// Tool stats.
	b.WriteString("\n")
	inst, upd, notInst := m.stats()
	total := inst + notInst
	fmt.Fprintf(&b, "  %s  %d total · %d installed · %d updates\n",
		label(fixedWidth("Tools", 18)), total, inst, upd)

	// Pad remaining height.
	used := 18 + len(registry.AllPMStatusForOS())
	if remaining := m.height - used - 6; remaining > 0 {
		b.WriteString(strings.Repeat("\n", remaining))
	}

	return b.String()
}

// --- Help ---

func (m Model) renderHelp() string {
	// Confirmation mode — show prompt instead of normal help.
	if m.pendingAction != nil {
		prompt := confirmStyle.Render(fmt.Sprintf("  Run %s?", strings.Join(m.pendingAction.cmdArgs, " ")))
		keys := dimVersion.Render("y") + " confirm   " + dimVersion.Render("Esc") + " cancel"
		return prompt + "  " + keys
	}

	var parts []string

	switch m.activeTab {
	case tabBackup:
		switch {
		case m.backupMode == backupModeIdle:
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("←→") + " tab",
				dimVersion.Render("Enter") + " select",
				dimVersion.Render("q") + " quit",
			}
		case m.backupConfirm:
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("Space") + " toggle",
				dimVersion.Render("a") + " select all",
				dimVersion.Render("Enter") + " confirm",
				dimVersion.Render("Esc") + " cancel",
			}
		case m.backupMode == backupModeShare:
			parts = []string{
				dimVersion.Render("c") + " copy to clipboard",
				dimVersion.Render("Esc") + " back",
				dimVersion.Render("q") + " quit",
			}
		default:
			parts = []string{
				dimVersion.Render("←→") + " tab",
				dimVersion.Render("Esc") + " back",
				dimVersion.Render("q") + " quit",
			}
		}
	case tabConfig:
		parts = []string{
			dimVersion.Render("←→") + " tab",
			dimVersion.Render("r") + " refresh",
			dimVersion.Render("q") + " quit",
		}
	default:
		parts = []string{
			dimVersion.Render("↑↓") + " navigate",
			dimVersion.Render("←→") + " tab",
			dimVersion.Render("Enter") + " detail",
			dimVersion.Render("f") + " filter",
			dimVersion.Render("r") + " refresh",
			dimVersion.Render("q") + " quit",
		}
		if m.activeTab == tabUpdates {
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("Space") + " toggle",
				dimVersion.Render("a") + " select all",
				dimVersion.Render("u") + " upgrade",
				dimVersion.Render("f") + " category",
				dimVersion.Render("Enter") + " detail",
				dimVersion.Render("q") + " quit",
			}
		}
	}

	help := helpStyle.Render("  " + strings.Join(parts, "   "))
	if m.statusMsg != "" {
		help += "  " + upgradableStyle.Render(m.statusMsg)
	}
	return help
}

// --- Two-column layout builders ---

// buildSidebarLines renders the filter sidebar as a slice of fixed-width strings.
func (m Model) buildSidebarLines(maxRows int) []string {
	if len(m.sidebarItems) == 0 {
		return nil
	}

	// Render all sidebar items into a full list.
	all := make([]string, 0, len(m.sidebarItems))

	for i, item := range m.sidebarItems {
		if item.isHeader {
			// Section header.
			all = append(all, headerStyle.Render(fixedWidth(item.label, colSidebar-2)))
			continue
		}

		cursor := "  "
		if m.categoryPicker && i == m.sidebarIdx {
			cursor = "▸ "
		}

		// Highlight the currently active filter value.
		style := dimVersion
		isActive := false
		switch item.section {
		case "category":
			isActive = (item.value == "" && m.categoryFilter == "") ||
				(item.value != "" && strings.EqualFold(item.value, m.categoryFilter))
		case "tag":
			isActive = (item.value == "" && m.tagFilter == "") ||
				(item.value != "" && strings.EqualFold(item.value, m.tagFilter))
		case "platform":
			isActive = (item.value == "" && m.platformFilter == "") ||
				(item.value != "" && strings.EqualFold(item.value, m.platformFilter))
		}
		if isActive {
			style = nameStyle
		}

		label := fixedWidth(item.label, colSidebar-4)
		line := cursor + style.Render(label)

		if m.categoryPicker && i == m.sidebarIdx {
			line = selectedRowStyle.Render(fixedWidth(line, colSidebar))
		}

		all = append(all, line)
	}

	// Apply scrolling viewport to keep sidebarIdx visible.
	if len(all) <= maxRows {
		// Everything fits — pad and return.
		for len(all) < maxRows {
			all = append(all, "")
		}
		return all
	}

	// Find which rendered line corresponds to sidebarIdx.
	cursorLine := 0
	if m.categoryPicker {
		cursorLine = m.sidebarIdx // items and rendered lines are 1:1
		if cursorLine >= len(all) {
			cursorLine = len(all) - 1
		}
	}

	// Compute scroll start so cursor is visible.
	start := 0
	if cursorLine >= maxRows {
		start = cursorLine - maxRows + 1
	}
	if start+maxRows > len(all) {
		start = len(all) - maxRows
	}
	if start < 0 {
		start = 0
	}

	return all[start : start+maxRows]
}

// buildToolLines renders the header + tool rows + empty state as a slice of strings.
func (m Model) buildToolLines(maxRows int) []string {
	lines := make([]string, 0, maxRows+1)

	// Header row.
	if m.phase >= phaseResolving && len(m.filteredIndex) > 0 {
		lines = append(lines, m.renderHeader())
	} else {
		lines = append(lines, "") // blank header line for alignment
	}

	// Tool rows.
	start := 0
	if m.cursor >= maxRows {
		start = m.cursor - maxRows + 1
	}

	rowCount := 0
	for vi := start; vi < len(m.filteredIndex) && rowCount < maxRows; vi++ {
		toolIdx := m.filteredIndex[vi]
		tool := m.tools[toolIdx]
		selected := vi == m.cursor && !m.categoryPicker
		lines = append(lines, m.renderRow(tool, toolIdx, selected))
		rowCount++
	}

	// Empty state.
	if len(m.filteredIndex) == 0 && m.phase >= phaseDone {
		msg := ""
		noCatalog := len(m.tools) == 0
		switch m.activeTab {
		case tabInstalled:
			if noCatalog {
				msg = "No tools loaded."
			} else {
				msg = "No installed tools found."
			}
		case tabUpdates:
			if noCatalog {
				msg = "No tools loaded."
			} else {
				msg = "All tools are up to date! ✓"
			}
		case tabDiscover:
			if noCatalog {
				msg = "No tools loaded."
			} else {
				msg = "All marketplace tools are installed!"
			}
		}
		if msg != "" {
			lines = append(lines, dimVersion.Render(msg))
		}
	}

	// Pad to maxRows + 1 (header + rows).
	for len(lines) < maxRows+1 {
		lines = append(lines, "")
	}

	return lines
}

// fixedWidthANSI pads a styled string (which may contain ANSI codes) to the
// given display width using lipgloss.Width for measurement.
func fixedWidthANSI(s string, width int) string {
	w := lipgloss.Width(s)
	if w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}

// --- Helpers ---

func toolResolved(tool registry.Tool) bool {
	if tool.Latest != "" || tool.LatestFrom != "" {
		return true
	}
	for _, inst := range tool.Instances {
		if inst.Version != "" {
			return true
		}
	}
	return false
}

// toolLabel returns the tool's short name for list rows.
func toolLabel(tool registry.Tool) string {
	return tool.Name
}

// fixedWidth pads or truncates a plain string to exactly `width` characters.
// Must be called BEFORE applying lipgloss styles, not after.
func fixedWidth(s string, width int) string {
	if width <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) > width {
		if width <= 1 {
			return "…"
		}
		return string(r[:width-1]) + "…"
	}
	if len(r) < width {
		return s + strings.Repeat(" ", width-len(r))
	}
	return s
}
