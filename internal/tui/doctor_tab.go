package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/clim/internal/audit"
	"github.com/nassiharel/clim/internal/compliance"
	"github.com/nassiharel/clim/internal/doctor"
	"github.com/nassiharel/clim/internal/paths"
	"github.com/nassiharel/clim/internal/registry"
)

// Doctor sub-tab indices.
const (
	doctorSubDoctor     = 0
	doctorSubAudit      = 1
	doctorSubCompliance = 2
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
	labels := []struct {
		text string
		idx  int
	}{
		{"Doctor", doctorSubDoctor},
		{"Audit", doctorSubAudit},
		{"Compliance", doctorSubCompliance},
	}
	var tabs []string
	for _, l := range labels {
		if m.doctorSubTab == l.idx {
			tabs = append(tabs, activeTabStyle.Render(l.text))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(l.text))
		}
	}
	b.WriteString("  " + strings.Join(tabs, " ") + "\n\n")

	if !m.doctorChecked {
		b.WriteString("  " + loadingStyle.Render("Waiting for scan to complete..."))
		return b.String()
	}

	switch m.doctorSubTab {
	case doctorSubAudit:
		b.WriteString(m.renderAuditView())
	case doctorSubCompliance:
		b.WriteString(m.renderComplianceView())
	default:
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
			fmt.Fprintf(&b, "    %-20s %d tool(s)\n", lc.name, lc.count)
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

// renderComplianceView renders the compliance sub-view.
func (m Model) renderComplianceView() string {
	var b strings.Builder

	if m.complianceResult == nil {
		if m.complianceError != "" {
			b.WriteString("  " + doctorError.Render("✗ "+m.complianceError) + "\n")
			return b.String()
		}
		b.WriteString("  " + dashDim.Render("No compliance policy configured.") + "\n\n")
		b.WriteString("  " + dashDim.Render("Create one with: clim compliance init") + "\n")
		policyHint := "Policy is stored in the clim config directory"
		if p, pathErr := paths.CompliancePolicy(); pathErr == nil {
			policyHint = "Policy location: " + p
		}
		b.WriteString("  " + dashDim.Render(policyHint) + "\n")
		return b.String()
	}

	result := m.complianceResult
	b.WriteString("  " + doctorSection.Render("Policy: "+result.PolicyName) + "\n\n")

	if result.Compliant && len(result.Violations) == 0 {
		b.WriteString("  " + doctorOK.Render("✓ All tools comply with policy!") + "\n")
		return b.String()
	}

	var errors, warnings int
	for _, v := range result.Violations {
		if v.Severity == "error" {
			errors++
		} else {
			warnings++
		}
	}

	var summaryParts []string
	if errors > 0 {
		summaryParts = append(summaryParts, doctorError.Render(fmt.Sprintf("%d error(s)", errors)))
	}
	if warnings > 0 {
		summaryParts = append(summaryParts, doctorWarning.Render(fmt.Sprintf("%d warning(s)", warnings)))
	}
	b.WriteString("  " + strings.Join(summaryParts, "  ") + "\n\n")

	for _, v := range result.Violations {
		icon := doctorWarning.Render("⚠")
		if v.Severity == "error" {
			icon = doctorError.Render("✗")
		}
		b.WriteString("    " + icon + " " + nameStyle.Render(v.Tool) + "  " + dashDim.Render(v.Rule) + "\n")
		b.WriteString("      " + dashDim.Render(v.Message) + "\n")
	}
	b.WriteString("\n")

	return b.String()
}

// runComplianceForTUI loads the compliance policy and checks tools against it.
// Returns (result, error string). Error is non-empty when policy exists but can't be loaded.
func runComplianceForTUI(tools []registry.Tool, policyPath string) (*compliance.Result, string) {
	if policyPath == "" {
		// Check default global location.
		if p, err := paths.CompliancePolicy(); err == nil {
			if _, statErr := os.Stat(p); statErr == nil {
				policyPath = p
			}
		}
	}
	if policyPath == "" {
		return nil, ""
	}
	policy, err := compliance.LoadPolicy(policyPath)
	if err != nil {
		return nil, fmt.Sprintf("Failed to load policy %s: %v", policyPath, err)
	}
	result := compliance.Check(policy, tools)
	return &result, ""
}
