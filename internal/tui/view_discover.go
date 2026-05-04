package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/clim/internal/onboard"
)

func (m Model) renderForYouList() string {
	var b strings.Builder

	if len(m.recommendations) == 0 {
		b.WriteString("\n\n")
		b.WriteString("  " + dimVersion.Render("No recommendations yet.") + "\n\n")
		b.WriteString("  " + dimVersion.Render("Install a few tools and clim will suggest related ones") + "\n")
		b.WriteString("  " + dimVersion.Render("based on what you use.") + "\n")
		return b.String()
	}

	// Section header.
	b.WriteString("  " + dashSection.Render("Recommended for you") +
		"  " + dimVersion.Render(fmt.Sprintf("(%d suggestions)", len(m.recommendations))) + "\n\n")

	// Overhead: title(1) + tabs(1) + rule(1) + blank(1) + search(1) + sub-tabs(1) + section header(1) + blank(1) + gap(1) + footer.
	visibleLines := m.height - 9 - m.footerHeight()
	if visibleLines < 6 {
		visibleLines = 6
	}
	itemsPerPage := visibleLines / 3
	if itemsPerPage < 2 {
		itemsPerPage = 2
	}

	start := 0
	if m.cursor >= itemsPerPage {
		start = m.cursor - itemsPerPage + 1
	}

	end := start + itemsPerPage
	if end > len(m.recommendations) {
		end = len(m.recommendations)
	}

	for vi := start; vi < end; vi++ {
		rec := m.recommendations[vi]
		selected := vi == m.cursor
		b.WriteString(m.renderRecCard(rec, selected, false) + "\n")

		// Blank separator between cards (not after last).
		if vi < end-1 {
			b.WriteString("\n")
		}
	}

	// Pad remaining lines to prevent layout jitter.
	renderedItems := end - start
	renderedLines := renderedItems*2 + max(renderedItems-1, 0)
	for i := renderedLines; i < visibleLines; i++ {
		b.WriteString("\n")
	}

	return b.String()
}

// renderOnboardList renders the role-based onboard sub-tab.

func (m Model) renderOnboardList() string {
	var b strings.Builder

	// Role selector.
	b.WriteString("  " + dashSection.Render("Select your role") + "  ")
	for i, r := range onboard.Roles {
		label := r.Name
		if i == m.onboardRole {
			b.WriteString(activeTabStyle.Render("▸ " + label))
		} else {
			b.WriteString(inactiveTabStyle.Render("  " + label))
		}
		b.WriteString(" ")
	}
	b.WriteString("\n")

	if m.onboardRole < 0 || m.onboardRole >= len(onboard.Roles) {
		b.WriteString("\n  " + dimVersion.Render("Use [ ] to pick a role, then browse recommended tools.") + "\n")
		return b.String()
	}

	role := &onboard.Roles[m.onboardRole]
	b.WriteString("  " + dimVersion.Render(role.Description) + "\n\n")

	if len(m.onboardTools) == 0 {
		b.WriteString("  " + dimVersion.Render("No additional tools found — you may already have everything for this role!") + "\n")
		return b.String()
	}

	b.WriteString("  " + dimVersion.Render(fmt.Sprintf("%d tools recommended", len(m.onboardTools))) + "\n\n")

	// Paginated list using recommendation cards.
	// Overhead: title(1) + tabs(1) + rule(1) + blank(1) + search(1) + sub-tabs(1) + role selector(1)
	//           + role desc(1) + count(1) + blank(1) + footer.
	visibleLines := m.height - 10 - m.footerHeight()
	if visibleLines < 6 {
		visibleLines = 6
	}
	itemsPerPage := visibleLines / 3
	if itemsPerPage < 2 {
		itemsPerPage = 2
	}

	start := 0
	if m.cursor >= itemsPerPage {
		start = m.cursor - itemsPerPage + 1
	}
	end := start + itemsPerPage
	if end > len(m.onboardTools) {
		end = len(m.onboardTools)
	}

	for vi := start; vi < end; vi++ {
		rec := m.onboardTools[vi]
		selected := vi == m.cursor
		b.WriteString(m.renderRecCard(rec, selected, false) + "\n")
		if vi < end-1 {
			b.WriteString("\n")
		}
	}

	// Pad remaining lines.
	renderedItems := end - start
	renderedLines := renderedItems*2 + max(renderedItems-1, 0)
	for i := renderedLines; i < visibleLines; i++ {
		b.WriteString("\n")
	}

	return b.String()
}

// Fixed column widths for recommendation card alignment.
const (
	colCatFY    = 16 // category column (chip style adds padding)
	colStarsFY  = 11 // stars badge column
	colGaugeFY  = 12 // match gauge width
	colPctFY    = 5  // percentage column
	colReasonFY = 34 // "You use: ..." column
)

// renderRecCard renders a single 2-line recommendation card.
// selected highlights both lines. compact omits row 2 (for inline use in detail views).

func (m Model) renderRecCard(rec recommendation, selected, compact bool) string {
	if rec.ToolIdx >= len(m.tools) {
		return ""
	}
	tool := m.tools[rec.ToolIdx]

	// --- Row 1: cursor + name + category + stars + gauge + pct ---
	cursor := "  "
	if selected {
		cursor = "▸ "
	}

	displayName := tool.DisplayName
	if displayName == "" {
		displayName = tool.Name
	}
	nameCell := nameStyle.Render(fixedWidth(displayName, colNameWide))

	catText := ""
	if rec.Category != "" {
		catText = chipStyle.Render(rec.Category)
	}
	catCell := fixedWidthANSI(catText, colCatFY)

	starsText := ""
	if rec.Stars > 0 {
		starsText = dimVersion.Render("★ " + formatStars(rec.Stars))
	}
	starsCell := fixedWidthANSI(starsText, colStarsFY)

	pct := rec.MatchPct
	filled := pct * colGaugeFY / 100
	if filled < 1 && pct > 0 {
		filled = 1
	}
	bar := gauge(filled, colGaugeFY, colGaugeFY, dashGaugeFill, dashGaugeEmpty)
	pctCell := fixedWidthANSI(upgradableStyle.Render(fmt.Sprintf("%d%%", pct)), colPctFY)

	line1 := cursor + nameCell + " " + catCell + " " + starsCell + " " + bar + " " + pctCell

	if selected {
		w := lipgloss.Width(line1)
		if w < m.width {
			line1 += strings.Repeat(" ", m.width-w)
		}
		line1 = selectedRowStyle.Render(line1)
	}

	if compact {
		return line1
	}

	// --- Row 2: indent + description + reason ---
	desc := rec.Description
	if desc == "" {
		desc = "No description"
	}
	desc = fixedWidth(desc, colNameWide)
	descCell := dimVersion.Render(desc)

	reasonText := ""
	if rec.Reason != "" {
		reasonText = "You use: " + rec.Reason
	}
	reasonCell := dimVersion.Render(fixedWidth(reasonText, colReasonFY))

	line2 := "  " + descCell + " " + reasonCell

	if selected {
		w := lipgloss.Width(line2)
		if w < m.width {
			line2 += strings.Repeat(" ", m.width-w)
		}
		line2 = selectedRowStyle.Render(line2)
	}

	return line1 + "\n" + line2
}

// renderDiscoverSubTabs renders the [Tools] [Packs] [For You] sub-tab bar.
func (m Model) renderDiscoverSubTabs() string {
	labels := []struct {
		name string
		idx  int
	}{
		{"Tools", discoverTools},
		{"Packs", discoverPacks},
		{"For You", discoverForYou},
		{"Onboard", discoverOnboard},
	}
	var parts []string
	for _, l := range labels {
		if l.idx == m.discoverSubTab {
			parts = append(parts, activeTabStyle.Render(l.name))
		} else {
			parts = append(parts, inactiveTabStyle.Render(l.name))
		}
	}
	return "  " + strings.Join(parts, " ")
}
