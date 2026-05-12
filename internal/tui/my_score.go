package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/score"
)

// renderMyScoreSection renders the "My Score" panel shown on the
// My Profile tab. It exposes the full breakdown produced by
// internal/score so the user can see exactly which signal cost them
// how many points — no hidden math.
//
// Layout (terminal width permitting):
//
//	My Score
//	  73 / 100  Grade B   [█████████████░░░░░░░░░░░░░░░░░░░░░░░░░] 73%
//
//	  How it's calculated:
//	    ✓ Tools up to date   30 / 30  3/3 tools current
//	    ⚠ Doctor health      19 / 25  3 warning(s)
//	    ✓ Audit clean        20 / 20  No findings
//	    ✓ Compliance         15 / 15  No policy configured
//	    ⚠ Managed sources     7 / 10  1 unmanaged tool(s)
//
// Returns the empty string when the score hasn't been computed yet
// so the caller can decide whether to render a placeholder.
func renderMyScoreSection(result score.Result, width int) string {
	if result.MaxTotal == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("  " + detailTitleStyle.Render("My Score") + "  " +
		dimVersion.Render("how healthy this developer environment is") + "\n\n")

	// Headline row: total / max, grade, gauge, percentage.
	pct := result.Total * 100 / result.MaxTotal
	gaugeW := width - 40
	if gaugeW < 15 {
		gaugeW = 15
	}
	if gaugeW > 40 {
		gaugeW = 40
	}
	scoreColor := dashGaugeFill
	switch {
	case pct < 50:
		scoreColor = lipgloss.NewStyle().Foreground(lipgloss.Color("167"))
	case pct < 70:
		scoreColor = dashGaugeWarn
	}
	b.WriteString(fmt.Sprintf("  %s / %s  %s  %s  %s\n",
		dashNumber.Render(strconv.Itoa(result.Total)),
		dashDim.Render(strconv.Itoa(result.MaxTotal)),
		dashNumber.Render("Grade "+result.Grade),
		gauge(result.Total, result.MaxTotal, gaugeW, scoreColor, dashGaugeEmpty),
		dashNumber.Render(fmt.Sprintf("%d%%", pct)),
	))
	b.WriteString("\n")

	// Breakdown by category. The fixed-width formatting keeps the
	// columns aligned for any sensible terminal width.
	b.WriteString("  " + detailLabelStyle.Render("How it's calculated") + "\n")
	for _, cat := range result.Categories {
		icon := scoreIcon(cat.Status)
		name := fixedWidth(cat.Name, 20)
		points := fmt.Sprintf("%2d / %-2d", cat.Points, cat.MaxPts)
		// Mini per-category gauge — much smaller than the headline
		// so the breakdown stays compact.
		miniW := 14
		var miniStyle lipgloss.Style
		switch cat.Status {
		case "error":
			miniStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("167"))
		case "warning":
			miniStyle = dashGaugeWarn
		default:
			miniStyle = dashGaugeFill
		}
		miniBar := gauge(cat.Points, cat.MaxPts, miniW, miniStyle, dashGaugeEmpty)
		detail := dashDim.Render(cat.Details)
		b.WriteString(fmt.Sprintf("    %s  %s  %s  %s  %s\n",
			icon, name, dashNumber.Render(points), miniBar, detail))
	}

	// Footer explainer so the user knows the score is deterministic
	// and where they can dig deeper. Important for trust — the user
	// asked "show how it's calculated", so we name the inputs.
	b.WriteString("\n  " + dashDim.Render("Inputs: tool freshness, Health diagnostics, audit findings, compliance, source management.") + "\n")
	b.WriteString("  " + dashDim.Render("Updated automatically whenever the toolchain changes.") + "\n")
	return b.String()
}

func scoreIcon(status string) string {
	switch status {
	case "error":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("167")).Render("✗")
	case "warning":
		return dashGaugeWarn.Render("⚠")
	default:
		return dashGaugeFill.Render("✓")
	}
}
