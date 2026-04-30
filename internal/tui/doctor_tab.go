package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/clim/internal/audit"
	"github.com/nassiharel/clim/internal/doctor"
	"github.com/nassiharel/clim/internal/registry"
)

// Doctor sub-tab indices.
const (
	doctorSubDoctor = 0
	doctorSubAudit  = 1
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

// renderDoctorView renders the Doctor tab content with sub-tabs.
func (m Model) renderDoctorView() string {
	var b strings.Builder

	// Sub-tab bar.
	doctorLabel := "Doctor"
	auditLabel := "Audit"
	if m.doctorSubTab == doctorSubDoctor {
		doctorLabel = activeTabStyle.Render(doctorLabel)
		auditLabel = inactiveTabStyle.Render(auditLabel)
	} else {
		doctorLabel = inactiveTabStyle.Render(doctorLabel)
		auditLabel = activeTabStyle.Render(auditLabel)
	}
	b.WriteString("  " + doctorLabel + " " + auditLabel + "\n\n")

	if !m.doctorChecked {
		b.WriteString("  " + loadingStyle.Render("Waiting for scan to complete..."))
		return b.String()
	}

	if m.doctorSubTab == doctorSubAudit {
		b.WriteString(m.renderAuditView())
	} else {
		b.WriteString(m.renderDoctorIssuesView())
	}

	return b.String()
}

// renderDoctorIssuesView renders the doctor diagnostics sub-view.
func (m Model) renderDoctorIssuesView() string {
	var b strings.Builder

	if len(m.doctorIssues) == 0 {
		b.WriteString("  " + doctorOK.Render("✓ No issues found — your environment looks healthy!") + "\n\n")
		b.WriteString("  " + dashDim.Render("All PATH entries are valid, no version conflicts detected,") + "\n")
		b.WriteString("  " + dashDim.Render("and your package managers are working correctly.") + "\n")
		return b.String()
	}

	errs, warns, infos := doctor.CountBySeverity(m.doctorIssues)

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

// renderAuditView renders the security audit sub-view.
func (m Model) renderAuditView() string {
	var b strings.Builder

	if len(m.auditFindings) == 0 && len(m.auditLicenses) == 0 {
		b.WriteString("  " + doctorOK.Render("✓ No audit findings — your toolchain looks clean!") + "\n")
		return b.String()
	}

	warns, infos := audit.CountBySeverity(m.auditFindings)

	if len(m.auditFindings) > 0 {
		var summaryParts []string
		if warns > 0 {
			summaryParts = append(summaryParts, doctorWarning.Render(fmt.Sprintf("%d warning(s)", warns)))
		}
		if infos > 0 {
			summaryParts = append(summaryParts, doctorInfo.Render(fmt.Sprintf("%d info(s)", infos)))
		}
		b.WriteString("  " + strings.Join(summaryParts, "  ") + "\n\n")

		for _, f := range m.auditFindings {
			icon := "ℹ"
			style := doctorInfo
			if f.Severity == "warning" {
				icon = "⚠"
				style = doctorWarning
			}
			b.WriteString("    " + style.Render(icon) + " " + nameStyle.Render(f.Tool) + "  " + dashDim.Render(f.Category) + "\n")
			b.WriteString("      " + dashDim.Render(f.Message) + "\n")
		}
		b.WriteString("\n")
	}

	// License summary.
	if len(m.auditLicenses) > 0 {
		b.WriteString("  " + doctorSection.Render("Licenses") + "\n")
		sorted := make([]struct {
			name  string
			count int
		}, 0, len(m.auditLicenses))
		for k, v := range m.auditLicenses {
			sorted = append(sorted, struct {
				name  string
				count int
			}{k, v})
		}
		sort.Slice(sorted, func(i, j int) bool {
			if sorted[i].count != sorted[j].count {
				return sorted[i].count > sorted[j].count
			}
			return sorted[i].name < sorted[j].name
		})
		for _, lc := range sorted {
			b.WriteString(fmt.Sprintf("    %-20s %d tool(s)\n", lc.name, lc.count))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// renderReferencesSection renders the "References" section in tool detail view,
// showing where a tool is referenced (projects, packs) — the TUI equivalent of `clim why`.
func (m Model) renderReferencesSection(tool registry.Tool) string {
	var refs []string

	// Check .clim.yaml projects.
	if m.teamFile != nil {
		for _, req := range m.teamFile.Tools {
			if req.Name == tool.Name {
				constraint := ""
				if req.Version != "" {
					constraint = " " + req.Version
				}
				refs = append(refs, fmt.Sprintf("This project (required%s)", constraint))
			}
		}
		for _, opt := range m.teamFile.Optional {
			if opt.Name == tool.Name {
				refs = append(refs, "This project (optional)")
			}
		}
	}

	// Check packs.
	for _, pack := range m.packs {
		for _, pToolName := range pack.ToolNames {
			if pToolName == tool.Name {
				refs = append(refs, fmt.Sprintf("Pack %q", pack.DisplayName))
			}
		}
	}

	// Check custom packs.
	for _, pack := range m.customPacks {
		for _, pToolName := range pack.ToolNames {
			if pToolName == tool.Name {
				refs = append(refs, fmt.Sprintf("Custom pack %q", pack.DisplayName))
			}
		}
	}

	if len(refs) == 0 {
		return ""
	}

	var b strings.Builder
	for _, ref := range refs {
		b.WriteString("    • " + ref + "\n")
	}
	b.WriteString("\n")
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
