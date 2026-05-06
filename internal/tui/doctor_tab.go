package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/audit"
	"github.com/nassiharel/klim/internal/compliance"
	"github.com/nassiharel/klim/internal/config"
	"github.com/nassiharel/klim/internal/doctor"
	"github.com/nassiharel/klim/internal/paths"
	"github.com/nassiharel/klim/internal/registry"
	"github.com/nassiharel/klim/internal/vuln"
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
	doctorWarning = lipgloss.NewStyle().Foreground(warningColor)                     // warm gold
	doctorInfo    = lipgloss.NewStyle().Foreground(primaryColor)                     // teal
	doctorFix     = lipgloss.NewStyle().Foreground(subtleColor)                      // gray
	doctorSection = lipgloss.NewStyle().Bold(true).Foreground(highlightColor)        // white bold
	doctorOK      = lipgloss.NewStyle().Foreground(successColor).Bold(true)          // mint green
)

// renderDoctorView renders the Doctor tab content with sub-tabs.
func (m Model) renderDoctorView() string {
	var b strings.Builder

	// Sub-tab bar.
	labels := []struct {
		text string
		idx  int
	}{
		{"Health", doctorSubDoctor},
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
// showing where a tool is referenced (projects, packs) — the TUI equivalent of `klim why`.
func (m Model) renderReferencesSection(tool registry.Tool) string {
	var refs []string

	// Check .klim.yaml projects.
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
		b.WriteString("  " + dashDim.Render("Create one with: klim compliance init") + "\n")
		policyHint := "Policy is stored in the klim config directory"
		if p, pathErr := paths.CompliancePolicy(); pathErr == nil {
			policyHint = "Policy location: " + p
		}
		b.WriteString("  " + dashDim.Render(policyHint) + "\n")
		return b.String()
	}

	// Refresh failed but a previous cache is loaded — warn so the user
	// knows the doctor verdict is from the stale cache, not a fresh
	// fetch. Without this banner the tab silently shows the old policy.
	if m.complianceError != "" {
		b.WriteString("  " + doctorError.Render("⚠ Policy refresh failed: "+m.complianceError) + "\n")
		b.WriteString("  " + dashDim.Render("Showing previously cached policy below.") + "\n\n")
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

// runComplianceForTUI loads the compliance policy from local sources
// only (synchronous, no network) and builds an evaluation result + per-
// tool index. Used inside the Bubble Tea Update loop where blocking on
// HTTP would freeze the UI for up to the fetch timeout. For remote
// policies, callers also dispatch loadComplianceURLCmd which posts
// complianceLoadedMsg when the fetch completes.
//
// Resolution (local-only):
//   - Explicit policyPath argument → load it.
//   - Cached remote policy at paths.ComplianceCachePath() → load.
//   - cfg.Compliance.Policy → load.
//   - paths.CompliancePolicy() default global file → load.
//
// Returns (result, index, errMsg). errMsg is non-empty only when a
// configured source failed to load; missing optional sources are not
// errors. policy may be nil with empty errMsg when nothing is configured.
func runComplianceForTUI(tools []registry.Tool, cfg *config.Config, policyPath string) (*compliance.Result, *compliance.Index, string) {
	policy, errMsg := loadPolicyForTUISync(cfg, policyPath)
	if policy == nil {
		return nil, nil, errMsg
	}
	result := compliance.Check(policy, tools, loadVulnSeveritiesFromConfig(cfg))
	idx := compliance.BuildIndex(policy, tools)
	idx.ApplyVulnSeverities(loadVulnSeveritiesFromConfig(cfg))
	return &result, idx, ""
}

// loadVulnSeveritiesFromConfig reads the vuln cache that
// `klim security vuln` wrote, keyed by the configured OSV URL.
// Returns nil when no cache exists; compliance silently skips the
// vuln gate in that case.
func loadVulnSeveritiesFromConfig(cfg *config.Config) map[string]string {
	url := vuln.DefaultOSVURL
	if cfg != nil {
		if u := strings.TrimSpace(cfg.Vuln.URL); u != "" {
			url = u
		}
	}
	if rep, ok := vuln.ReadCache(url); ok {
		return rep.SeverityByTool()
	}
	return nil
}

// loadPolicyForTUISync is the non-blocking half of policy loading:
// disk reads only, never an HTTP fetch. Mirrors the CLI's resolution
// order with one twist — when compliance.url is set we serve the
// previously cached payload (if any) so the user gets immediate
// feedback while the async fetcher refreshes it.
func loadPolicyForTUISync(cfg *config.Config, policyPath string) (*compliance.Policy, string) {
	// 1. Explicit policy path always wins.
	if policyPath != "" {
		p, err := compliance.LoadPolicy(policyPath)
		if err != nil {
			return nil, fmt.Sprintf("Failed to load policy %s: %v", policyPath, err)
		}
		return p, ""
	}

	// 2. compliance.url — serve the cached copy synchronously; the
	//    async fetcher will refresh it shortly. A malformed cache is
	//    surfaced as an error rather than silently treated as "no
	//    policy" — otherwise startup with a broken cache would let
	//    blocked actions through until the async fetch completed.
	if cfg != nil && cfg.Compliance.URL != "" {
		if cachePath, err := paths.ComplianceCachePath(); err == nil {
			if info, statErr := os.Stat(cachePath); statErr == nil && info.Size() > 0 {
				p, perr := compliance.LoadPolicy(cachePath)
				if perr != nil {
					return nil, fmt.Sprintf("Cached policy %s is malformed: %v", cachePath, perr)
				}
				return p, ""
			}
		}
		// No cache yet — UI will show "no policy" until the async
		// load completes; that's acceptable for the first launch.
		return nil, ""
	}

	// 3. cfg.Compliance.Policy — explicit local file.
	if cfg != nil && cfg.Compliance.Policy != "" {
		p, err := compliance.LoadPolicy(cfg.Compliance.Policy)
		if err != nil {
			return nil, fmt.Sprintf("Failed to load policy %s: %v", cfg.Compliance.Policy, err)
		}
		return p, ""
	}

	// 4. Default global location.
	if p, err := paths.CompliancePolicy(); err == nil {
		if _, statErr := os.Stat(p); statErr == nil {
			policy, err := compliance.LoadPolicy(p)
			if err != nil {
				return nil, fmt.Sprintf("Failed to load policy %s: %v", p, err)
			}
			return policy, ""
		}
	}
	return nil, ""
}
