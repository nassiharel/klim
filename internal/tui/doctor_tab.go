package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/clim/internal/doctor"
	"github.com/nassiharel/clim/internal/registry"
)

// Doctor sub-tab indices.
const (
	doctorSubDoctor = 0
	doctorSubAudit  = 1
)

// auditFinding represents a single audit issue for TUI display.
type auditFinding struct {
	severity string // "warning", "info"
	tool     string
	category string
	message  string
}

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

	// Count by severity.
	var warns, infos int
	for _, f := range m.auditFindings {
		switch f.severity {
		case "warning":
			warns++
		case "info":
			infos++
		}
	}

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
			if f.severity == "warning" {
				icon = "⚠"
				style = doctorWarning
			}
			b.WriteString("    " + style.Render(icon) + " " + nameStyle.Render(f.tool) + "  " + dashDim.Render(f.category) + "\n")
			b.WriteString("      " + dashDim.Render(f.message) + "\n")
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

// computeAuditFindings computes security audit findings from the tool list.
func computeAuditFindings(tools []registry.Tool) ([]auditFinding, map[string]int) {
	var findings []auditFinding
	licenses := make(map[string]int)

	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		primary := t.PrimaryInstance()
		if primary == nil {
			continue
		}

		if primary.Source == registry.SourceManual {
			findings = append(findings, auditFinding{
				severity: "warning",
				tool:     t.Name,
				category: "Unmanaged",
				message:  fmt.Sprintf("Installed from unknown source at %s", primary.Path),
			})
		}

		if primary.Version == "" && primary.Source != registry.SourceManual {
			findings = append(findings, auditFinding{
				severity: "warning",
				tool:     t.Name,
				category: "No Version",
				message:  "Version could not be determined",
			})
		}

		if t.GitHubInfo != nil && t.GitHubInfo.Archived {
			findings = append(findings, auditFinding{
				severity: "warning",
				tool:     t.Name,
				category: "Archived",
				message:  "Upstream repository is archived",
			})
		}

		if t.GitHubInfo != nil && t.GitHubInfo.PushedAt != "" {
			if pushed, err := time.Parse(time.RFC3339, t.GitHubInfo.PushedAt); err == nil {
				age := time.Since(pushed)
				if age > 365*24*time.Hour {
					months := int(age.Hours() / 24 / 30)
					findings = append(findings, auditFinding{
						severity: "info",
						tool:     t.Name,
						category: "Stale",
						message:  fmt.Sprintf("Last upstream activity %d months ago", months),
					})
				}
			}
		}

		if t.HasUpdate() {
			findings = append(findings, auditFinding{
				severity: "info",
				tool:     t.Name,
				category: "Outdated",
				message:  fmt.Sprintf("Update available: %s → %s", primary.Version, t.Latest),
			})
		}

		if t.GitHubInfo != nil && t.GitHubInfo.License != "" {
			licenses[t.GitHubInfo.License]++
		} else {
			licenses["Unknown"]++
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].severity != findings[j].severity {
			return findings[i].severity < findings[j].severity
		}
		return findings[i].tool < findings[j].tool
	})

	return findings, licenses
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
