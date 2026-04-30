package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/clim/internal/doctor"
)

// Doctor view color palette.
var (
	doctorError   = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true) // red
	doctorWarning = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))            // orange
	doctorInfo    = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))             // cyan
	doctorFix     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))            // gray
	doctorSection = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))  // white bold
	doctorOK      = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)  // green
)

// renderDoctorView renders the Doctor tab content.
func (m Model) renderDoctorView() string {
	var b strings.Builder

	if !m.doctorChecked {
		b.WriteString("  " + loadingStyle.Render("Waiting for scan to complete..."))
		return b.String()
	}

	if len(m.doctorIssues) == 0 {
		b.WriteString("\n")
		b.WriteString("  " + doctorOK.Render("✓ No issues found — your environment looks healthy!") + "\n\n")
		b.WriteString("  " + dashDim.Render("All PATH entries are valid, no version conflicts detected,") + "\n")
		b.WriteString("  " + dashDim.Render("and your package managers are working correctly.") + "\n")
		return b.String()
	}

	errs, warns, infos := doctor.CountBySeverity(m.doctorIssues)

	// Summary header.
	b.WriteString("\n")
	var summaryParts []string
	if errs > 0 {
		summaryParts = append(summaryParts, doctorError.Render(fmt.Sprintf("%d error(s)", errs)))
	}
	if warns > 0 {
		summaryParts = append(summaryParts, doctorWarning.Render(fmt.Sprintf("%d warning(s)", warns)))
	}
	if infos > 0 {
		summaryParts = append(summaryParts, doctorInfo.Render(fmt.Sprintf("%d info(s)", infos)))
	}
	b.WriteString("  " + strings.Join(summaryParts, "  ") + "\n\n")

	// Group issues by category.
	grouped := make(map[string][]doctor.Issue)
	var categoryOrder []string
	for _, issue := range m.doctorIssues {
		if _, ok := grouped[issue.Category]; !ok {
			categoryOrder = append(categoryOrder, issue.Category)
		}
		grouped[issue.Category] = append(grouped[issue.Category], issue)
	}

	for _, cat := range categoryOrder {
		b.WriteString("  " + doctorSection.Render(cat) + "\n")
		for _, issue := range grouped[cat] {
			icon := severityStyle(issue.Severity)
			b.WriteString("    " + icon + " " + issue.Title + "\n")
			if issue.Detail != "" {
				for _, line := range strings.Split(issue.Detail, "\n") {
					if line != "" {
						b.WriteString("      " + dashDim.Render(line) + "\n")
					}
				}
			}
			if issue.Fix != "" {
				b.WriteString("      " + doctorFix.Render("→ "+issue.Fix) + "\n")
			}
		}
		b.WriteString("\n")
	}

	return b.String()
}

// severityStyle returns a styled icon for the given severity.
func severityStyle(s doctor.Severity) string {
	switch s {
	case doctor.SeverityError:
		return doctorError.Render("✗")
	case doctor.SeverityWarning:
		return doctorWarning.Render("⚠")
	case doctor.SeverityInfo:
		return doctorInfo.Render("ℹ")
	}
	return "?"
}
