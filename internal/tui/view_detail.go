package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/registry"
)

// renderDetailView renders the tool detail page. Sections (from top):
//
//  1. Hero      — name, status badge, category, description, quick stats bar
//  2. Installed — installed version vs latest, source/path, all instances,
//     recommendations (only shown when the tool is installed)
//  3. Package Managers — unified table of every declared PM with package id,
//     availability on the current host and the `install` command
//  4. About     — tags + topics (deduped), platforms, binary names
//  5. Community — GitHub repo/homepage/license, stars gauge, forks, last push
//  6. Related   — "You might also like" with match bars
//  7. Actions   — footer menu + key hints
func (m Model) renderDetailView(tool registry.Tool) string {
	body := m.renderDetailBody(tool)

	// Footer: help bar only (actions are now inline in Package Managers section).
	var footer strings.Builder

	dim := dimVersion.Render
	switch {
	case m.pendingAction != nil:
		prompt := confirmStyle.Render(fmt.Sprintf("  Run %s?", strings.Join(m.pendingAction.cmdArgs, " ")))
		keys := dim("y") + " confirm   " + dim("Esc") + " cancel"
		footer.WriteString(prompt + "  " + keys)
	default:
		hints := []string{
			dim("↑↓") + " navigate",
			dim("PgUp/PgDn") + " scroll",
			dim("Enter") + " select",
			dim("Esc") + " back",
		}
		footer.WriteString("  " + helpStyle.Render(strings.Join(hints, "   ")))
	}

	return m.layoutDetailWithScroll(body, footer.String())
}

// renderDetailBody renders the scrollable body of the tool detail page
// (everything except the footer). Extracted so computeDetailMaxScroll
// can measure actual line count.

func (m Model) renderDetailBody(tool registry.Tool) string {
	var b strings.Builder

	divider := func(title string) string {
		section := dashSection.Render
		w := m.width - lipgloss.Width(title) - 8
		if w < 4 {
			w = 4
		}
		return "  " + dashDim.Render("▸ ") + section(title) + " " + dashDim.Render(strings.Repeat("─", w)) + "\n"
	}

	b.WriteString(m.renderHeroHeader(tool))

	if tool.IsInstalled() {
		b.WriteString(divider("Installed"))
		b.WriteString(m.renderInstalledStatus(tool))
	}

	pms := m.renderPackageManagers(tool)
	if pms != "" {
		b.WriteString(divider("Package Managers"))
		b.WriteString(pms)
	}

	sec := m.renderSecuritySection(tool)
	if sec != "" {
		b.WriteString(divider("Security"))
		b.WriteString(sec)
	}

	comp := m.renderToolComplianceSection(tool)
	if comp != "" {
		b.WriteString(divider("Compliance"))
		b.WriteString(comp)
	}

	about := m.renderAboutSection(tool)
	if about != "" {
		b.WriteString(divider("About"))
		b.WriteString(about)
	}

	community := m.renderCommunitySection(tool)
	if community != "" {
		b.WriteString(divider("Community"))
		b.WriteString(community)
	}

	refs := m.renderReferencesSection(tool)
	if refs != "" {
		b.WriteString(divider("Referenced By"))
		b.WriteString(refs)
	}

	if len(m.detailRelated) > 0 {
		b.WriteString(divider("You might also like"))
		for i, r := range m.detailRelated {
			selected := i == m.detailRelCursor
			b.WriteString(m.renderRecCard(r, selected, true) + "\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

// layoutDetailWithScroll applies m.detailScroll to the rendered body so the
// tool detail view can scroll vertically, then hands off to layoutWithFooter
// for bottom-pinning of the footer. Also clamps m.detailScroll in-place via
// the returned model... except we have a value receiver on renderDetailView,
// so we clamp locally and just use the clamped value here. The next user
// input re-renders and settles any over-scroll silently.

func (m Model) layoutDetailWithScroll(body, footer string) string {
	if m.height <= 0 {
		return m.layoutWithFooter(body, footer)
	}

	// Trim trailing newline to avoid inflating line count with an empty entry.
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")

	footerRows := visualRows(footer, m.width)
	const minGap = 1
	visibleRows := m.height - footerRows - minGap
	if visibleRows < 5 {
		visibleRows = 5
	}

	// Line-based scrolling: scroll and maxScroll are in logical lines,
	// matching the unit we slice by. No visual-row / logical-line mismatch.
	maxScroll := len(lines) - visibleRows
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := m.detailScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	if scroll < 0 {
		scroll = 0
	}

	// Slice lines by scroll offset.
	if scroll > 0 && scroll < len(lines) {
		lines = lines[scroll:]
	}

	// Annotate the footer with scroll position when applicable.
	if scroll > 0 || maxScroll > 0 {
		pct := 0
		if maxScroll > 0 {
			pct = (scroll * 100) / maxScroll
		}
		indicator := fmt.Sprintf("[%d%%]", pct)
		footer = "  " + dimVersion.Render(indicator) + "   " + strings.TrimLeft(footer, " ")
	}

	return m.layoutWithFooter(strings.Join(lines, "\n"), footer)
}

// renderHeroHeader renders the top-of-page "hero" block: name, status badge,
// category pill, description, and an at-a-glance stats bar (stars, forks,
// license, last push).

func (m Model) renderHeroHeader(tool registry.Tool) string {
	var b strings.Builder

	// Name + alias.
	displayName := tool.DisplayName
	if displayName == "" {
		displayName = tool.Name
	}
	name := detailTitleStyle.Render(displayName)
	if tool.DisplayName != "" && tool.DisplayName != tool.Name {
		name += "  " + dimVersion.Render("("+tool.Name+")")
	}

	// Status badge.
	var badge string
	switch {
	case tool.IsInstalled() && tool.HasUpdate():
		badge = upgradableStyle.Render(" ⬆ UPDATE AVAILABLE ")
	case tool.IsInstalled():
		badge = upToDateStyle.Render(" ✓ INSTALLED ")
	default:
		badge = dashDim.Render(" ○ NOT INSTALLED ")
	}

	// Category + archived chips.
	chips := []string{badge}
	if tool.Category != "" {
		chips = append(chips, chipStyle.Render(tool.Category))
	}
	if tool.GitHubInfo != nil && tool.GitHubInfo.Archived {
		chips = append(chips, upgradableStyle.Render(" ⚠ ARCHIVED "))
	}
	if compChip := m.complianceVerdictChip(tool.Name); compChip != "" {
		chips = append(chips, compChip)
	}

	b.WriteString("  " + name + "  " + strings.Join(chips, "  ") + "\n")

	// Description — readable, not dim.
	if info := tool.GitHubInfo; info != nil && info.Description != "" {
		maxW := m.width - 6
		if maxW < 20 {
			maxW = 20
		}
		b.WriteString("\n")
		for _, line := range wordWrap(info.Description, maxW) {
			b.WriteString("  " + heroDescStyle.Render(line) + "\n")
		}
	}

	// Quick stats bar: ★ stars · ⑂ forks · 📜 license · 🕒 last push.
	// Shown at the top so the most-asked-for info is above the fold.
	// (The Community section below does not repeat these.)
	if stats := m.renderQuickStats(tool); stats != "" {
		b.WriteString("\n  " + stats + "\n")
	}

	b.WriteString("\n")
	return b.String()
}

// renderQuickStats renders the single-line summary of GitHub stats. Returns
// "" when the tool has no enriched metadata.

func (m Model) renderQuickStats(tool registry.Tool) string {
	info := tool.GitHubInfo
	if info == nil {
		return ""
	}
	var parts []string
	sep := dashDim.Render(" · ")

	if info.Stars > 0 {
		parts = append(parts, upgradableStyle.Render("★ ")+dashNumber.Render(formatStars(info.Stars)))
	}
	if info.Forks > 0 {
		parts = append(parts, dashDim.Render("⑂ ")+dashNumber.Render(formatStars(info.Forks)))
	}
	if info.License != "" {
		parts = append(parts, dashDim.Render("📜 ")+dimVersion.Render(info.License))
	}
	if d := formatGitHubDate(info.PushedAt); d != "" {
		parts = append(parts, dashDim.Render("🕒 ")+dimVersion.Render(d))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, sep)
}

// renderInstalledStatus renders the "Installed" section: primary version vs
// latest, source and path, and (when present) the list of additional instances
// plus actionable recommendations.

func (m Model) renderInstalledStatus(tool registry.Tool) string {
	var b strings.Builder
	label := detailLabelStyle.Render
	dim := dimVersion.Render

	ver := tool.InstalledVersion()
	if ver == "" {
		ver = "—"
	}

	line := "  " + label(fixedWidth("Version", 14)) + dashNumber.Render(ver)
	if tool.Latest != "" {
		switch {
		case registry.VersionsMatch(ver, tool.Latest):
			line += "  " + upToDateStyle.Render("✓ latest")
		case tool.HasUpdate():
			line += "  " + dashDim.Render("→") + "  " + upgradableStyle.Render(tool.Latest) + "  " + upgradableStyle.Render("available")
		}
		if tool.LatestFrom != "" {
			line += "  " + dim("via "+tool.LatestFrom)
		}
	}
	b.WriteString(line + "\n")

	if primary := tool.PrimaryInstance(); primary != nil {
		b.WriteString("  " + label(fixedWidth("Source", 14)) + sourceStyle.Render(string(primary.Source)) + "\n")
		b.WriteString("  " + label(fixedWidth("Path", 14)) + dim(registry.TruncatePath(primary.Path, m.width-20)) + "\n")
	}
	b.WriteString("\n")

	// Multiple instances.
	if len(tool.Instances) > 1 {
		b.WriteString("  " + detailLabelStyle.Render(fmt.Sprintf("%d installations found", len(tool.Instances))) + "\n")
		for i, inst := range tool.Instances {
			bullet := dashDim.Render("○")
			if i == 0 {
				bullet = upToDateStyle.Render("●")
			}
			instVer := inst.Version
			if instVer == "" {
				instVer = "—"
			}
			fmt.Fprintf(&b, "  %s  %s  %s  %s\n",
				bullet,
				nameStyle.Render(fixedWidth(instVer, 14)),
				sourceStyle.Render(fixedWidth(string(inst.Source), 8)),
				dim(registry.TruncatePath(inst.Path, m.width-36)),
			)
		}
		b.WriteString("\n")
		b.WriteString(m.renderInstanceRecommendations(tool))
	}
	return b.String()
}

// renderPackageManagers renders a unified view of every declared package
// manager: availability dot, PM name, and package id.
// Interactive: toolMenu cursor navigates PM rows. Enter to install/upgrade, x to remove.

func (m Model) renderPackageManagers(tool registry.Tool) string {
	pkgs := collectPackageEntries(tool.Packages)
	if len(pkgs) == 0 {
		return ""
	}

	avail := make(map[string]bool, len(registry.AllPMStatusForOS()))
	for _, pm := range registry.AllPMStatusForOS() {
		avail[string(pm.Source)] = pm.Available
	}

	interactive := len(m.toolMenuItems) > 0

	// Build installed source set for showing action hints.
	installedSources := make(map[string]bool)
	if tool.IsInstalled() {
		for _, inst := range tool.Instances {
			installedSources[string(inst.Source)] = true
		}
	}

	var b strings.Builder
	pmIdx := 0 // tracks index into m.toolMenuItems (only available PMs)

	for _, p := range pkgs {
		isAvailable, knownPM := avail[p.source]
		// Skip PMs not available on PATH.
		if knownPM && !isAvailable {
			continue
		}
		if !knownPM {
			continue // PM not applicable to this OS
		}

		// Bullet color: green = installed via this PM, orange = PM available but not used.
		bullet := upgradableStyle.Render("●") // orange — available but not installed via this PM
		if installedSources[p.source] {
			bullet = upToDateStyle.Render("●") // green — installed via this PM
		}

		cursor := "  "
		if interactive && pmIdx == m.toolMenu {
			cursor = "▸ "
		}

		pmName := sourceStyle.Render(fixedWidth(p.source, 8))
		pkgID := nameStyle.Render(p.id)

		// Action hints on the selected row.
		hint := ""
		if interactive && pmIdx == m.toolMenu {
			if tool.IsInstalled() && pmIdx < len(m.toolMenuItems) {
				item := m.toolMenuItems[pmIdx]
				var actions []string
				if item.picker != nil {
					if item.picker.action == actionUpgrade {
						actions = append(actions, dimVersion.Render("Enter")+" upgrade")
					} else {
						actions = append(actions, dimVersion.Render("Enter")+" install")
					}
				}
				if item.removePicker != nil {
					actions = append(actions, dimVersion.Render("x")+" remove")
				}
				if len(actions) > 0 {
					hint = "  " + strings.Join(actions, "  ")
				}
			} else {
				hint = "  " + dimVersion.Render("Enter") + " install"
			}
		}

		line := cursor + bullet + "  " + pmName + "  " + pkgID + hint

		if interactive && pmIdx == m.toolMenu {
			w := lipgloss.Width(line)
			if w < m.width {
				line += strings.Repeat(" ", m.width-w)
			}
			line = selectedRowStyle.Render(line)
		}

		b.WriteString(line + "\n")
		pmIdx++
	}
	b.WriteString("\n")

	return b.String()
}

// renderAboutSection renders consolidated metadata: binary names, platforms,
// and a deduped list of tags + GitHub topics.

func (m Model) renderAboutSection(tool registry.Tool) string {
	var b strings.Builder
	label := detailLabelStyle.Render
	dim := dimVersion.Render

	// Binaries.
	if len(tool.BinaryNames) > 0 {
		b.WriteString("  " + label(fixedWidth("Binaries", 14)) + dim(strings.Join(tool.BinaryNames, ", ")) + "\n")
	}

	// Platforms as colored pills (highlighted for the current OS).
	if platforms := derivePlatforms(tool.Packages); len(platforms) > 0 {
		current := currentOSLabel()
		var lineBuilder strings.Builder
		lineBuilder.WriteString("  " + label(fixedWidth("Platforms", 14)))
		for i, p := range platforms {
			pill := chipStyle.Render(p)
			if p == current {
				pill = chipAccentStyle.Render(p + " (this host)")
			}
			lineBuilder.WriteString(pill)
			if i < len(platforms)-1 {
				lineBuilder.WriteString(" ")
			}
		}
		lineBuilder.WriteString("\n")
		b.WriteString(lineBuilder.String())
	}

	// Tags + topics (deduped, case-insensitive).
	if labels := combineTagsAndTopics(tool); len(labels) > 0 {
		line := "  " + label(fixedWidth("Tags", 14))
		for _, t := range labels {
			pill := chipStyle.Render(t)
			pillW := lipgloss.Width(pill) + 1
			if lipgloss.Width(line)+pillW > m.width-4 {
				b.WriteString(line + "\n")
				line = "  " + strings.Repeat(" ", 14)
			}
			line += pill + " "
		}
		b.WriteString(line + "\n")
	}

	if b.Len() > 0 {
		b.WriteString("\n")
	}
	return b.String()
}

// renderCommunitySection renders GitHub repo URL + homepage. Counts, license
// and activity are surfaced in the hero quick-stats bar to avoid duplication.
// Returns "" when the tool has no GitHub slug.

func (m Model) renderCommunitySection(tool registry.Tool) string {
	if tool.GitHubSlug == "" && tool.GitHubInfo == nil {
		return ""
	}

	var b strings.Builder
	label := detailLabelStyle.Render
	dim := dimVersion.Render

	if url := githubRepoURL(tool.GitHubSlug); url != "" {
		b.WriteString("  " + label(fixedWidth("GitHub", 14)) + dim(url) + "\n")
	}
	if info := tool.GitHubInfo; info != nil && info.Homepage != "" {
		b.WriteString("  " + label(fixedWidth("Homepage", 14)) + dim(info.Homepage) + "\n")
	}

	if b.Len() == 0 {
		return ""
	}
	b.WriteString("\n")
	return b.String()
}

// combineTagsAndTopics, currentOSLabel, collectPackageEntries,
// derivePlatforms, wordWrap, and the packageEntry type now live in util.go.

// renderGitHubSection renders a multi-line block with GitHub repository
// metadata for the detail view. Returns "" when the tool has no GitHub slug
// and no fetched info.

func (m Model) renderGitHubSection(tool registry.Tool) string {
	if tool.GitHubSlug == "" && tool.GitHubInfo == nil {
		return ""
	}

	var b strings.Builder
	label := detailLabelStyle.Render
	dim := dimVersion.Render

	b.WriteString("  " + detailTitleStyle.Render("GitHub") + "\n")

	slug := tool.GitHubSlug
	if url := githubRepoURL(slug); url != "" {
		b.WriteString("  " + label("Repo:       ") + dim(url) + "\n")
	}

	info := tool.GitHubInfo
	if info == nil {
		// Slug present but no enriched info (e.g. catalog predates assembly,
		// or fetch failed) — nothing else to show.
		b.WriteString("\n")
		return b.String()
	}

	if info.Archived {
		b.WriteString("  " + upgradableStyle.Render("⚠ Repository is archived (no longer maintained)") + "\n")
	}

	// Stars / forks on a single line for compactness.
	var stats []string
	if info.Stars > 0 {
		stats = append(stats, "★ "+formatStars(info.Stars)+" stars")
	}
	if info.Forks > 0 {
		stats = append(stats, "⑂ "+formatStars(info.Forks)+" forks")
	}
	if len(stats) > 0 {
		b.WriteString("  " + label("Stats:      ") + strings.Join(stats, "   ") + "\n")
	}

	if info.License != "" {
		b.WriteString("  " + label("License:    ") + info.License + "\n")
	}

	if info.Homepage != "" {
		b.WriteString("  " + label("Homepage:   ") + dim(info.Homepage) + "\n")
	}

	if len(info.Topics) > 0 {
		b.WriteString("  " + label("Topics:     ") + dim(strings.Join(info.Topics, ", ")) + "\n")
	}

	if d := formatGitHubDate(info.PushedAt); d != "" {
		b.WriteString("  " + label("Last push:  ") + dim(d) + "\n")
	}

	b.WriteString("\n")
	return b.String()
}

// packageEntry / collectPackageEntries / derivePlatforms moved to util.go.

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
		srcNames := make([]string, 0, len(sources))
		for src := range sources {
			srcNames = append(srcNames, string(src))
		}
		// Sort for stable ordering across renders (map iteration is
		// non-deterministic and would otherwise cause visible flicker as
		// the detail view re-renders during background version resolution).
		sort.Strings(srcNames)
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
