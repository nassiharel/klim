package tui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/audit"
	"github.com/nassiharel/klim/internal/compliance"
	"github.com/nassiharel/klim/internal/registry"
	"github.com/nassiharel/klim/internal/security"
	"github.com/nassiharel/klim/internal/vuln"
)

// renderSecuritySection renders the per-tool Security panel for the
// detail view: a 4-state badge (clean/watch/risk/unknown), a list of
// reasons, and any cached vulnerability rows. Returns "" when the
// tool isn't installed (a verdict only makes sense for installed
// tools).
//
// Vuln data is read from cache only — opening a tool detail page must
// never block on a 30s OSV.dev round-trip. The user populates the
// cache via `klim security vuln` (or the "v" key on the Security tab).
func (m Model) renderSecuritySection(tool registry.Tool) string {
	if !tool.IsInstalled() {
		return ""
	}

	findings, _ := audit.Analyze(m.tools)

	var match *vuln.Match
	cacheLoaded := false
	if rep, ok := vuln.ReadCache(m.vulnSourceKey()); ok {
		cacheLoaded = true
		for i := range rep.Matches {
			if rep.Matches[i].Tool == tool.Name {
				match = &rep.Matches[i]
				break
			}
		}
	}

	v := security.Score(tool, findings, match)

	var b strings.Builder
	b.WriteString("  " + securityBadge(v.Status) + "\n")
	for _, reason := range v.Reasons {
		b.WriteString("    · " + reason + "\n")
	}
	if match != nil && len(match.Vulnerabilities) > 0 {
		b.WriteString("\n")
		for _, vu := range match.Vulnerabilities {
			fix := vu.FixedIn
			if fix == "" {
				fix = "—"
			}
			b.WriteString("    " + severityChip(vu.Severity) + " " + vu.ID + "  fixed in: " + fix + "\n")
			if vu.Summary != "" {
				summary := vu.Summary
				if len(summary) > 80 {
					summary = summary[:77] + "…"
				}
				b.WriteString("      " + dashDim.Render(summary) + "\n")
			}
		}
	} else if !cacheLoaded {
		b.WriteString("\n    " + dashDim.Render("vulnerability cache empty — run `klim security vuln` to populate") + "\n")
	}
	b.WriteString("\n")
	return b.String()
}

// vulnSourceKey returns the OSV cache key matching whatever
// `klim security vuln` writes. Reads from the loaded config; falls
// back to the default OSV endpoint when unconfigured.
func (m Model) vulnSourceKey() string {
	if m.cfg != nil {
		if u := strings.TrimSpace(m.cfg.Vuln.URL); u != "" {
			return u
		}
	}
	return vuln.DefaultOSVURL
}

// securityBadge formats the 4-state Verdict.Status as a colored chip.
func securityBadge(status security.Status) string {
	switch status {
	case security.StatusRisk:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true).Render("⛔ at risk")
	case security.StatusWatch:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true).Render("⚠ watch")
	case security.StatusClean:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true).Render("🛡 clean")
	}
	return dashDim.Render("? unknown")
}

// severityChip is a single-letter severity marker for the per-CVE rows.
func severityChip(s vuln.Severity) string {
	switch s {
	case vuln.SeverityCritical:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true).Render("[C]")
	case vuln.SeverityHigh:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render("[H]")
	case vuln.SeverityMedium:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("[M]")
	case vuln.SeverityLow:
		return dashDim.Render("[L]")
	}
	return dashDim.Render("[?]")
}

// renderToolComplianceSection renders the per-tool Compliance panel for
// the detail view: policy name, status, reasons, and (when applicable)
// a notice that the user's `compliance.block_installs` setting will
// stop install/upgrade actions on this tool.
//
// Returns "" when no policy is loaded — the divider stays absent and
// the page is uncluttered for users who haven't opted in to compliance
// yet.
func (m Model) renderToolComplianceSection(tool registry.Tool) string {
	if m.complianceIndex == nil || !m.complianceIndex.HasPolicy() {
		return ""
	}

	var b strings.Builder
	policyName := m.complianceIndex.PolicyName()
	if policyName == "" {
		policyName = "compliance policy"
	}

	status := m.complianceIndex.Status(tool.Name)

	// Header line: status icon + policy name. Always rendered so users
	// can see "compliant under policy X" without hunting for it.
	switch status {
	case compliance.StatusViolation:
		b.WriteString("  " + complianceErrorStyle.Render("✗ Violation") + "  " + dashDim.Render("under "+policyName) + "\n")
	case compliance.StatusWarning:
		b.WriteString("  " + complianceWarnStyle.Render("⚠ Warning") + "  " + dashDim.Render("under "+policyName) + "\n")
	case compliance.StatusCompliant:
		b.WriteString("  " + complianceOKStyle.Render("✓ Compliant") + "  " + dashDim.Render("under "+policyName) + "\n")
	default:
		// StatusUnknown: tool is not in the policy's purview (e.g. not
		// installed, no rules apply). Make that explicit instead of
		// staying silent — readers shouldn't have to infer "no chip
		// means OK".
		b.WriteString("  " + dashDim.Render("○ No rules apply") + "  " + dashDim.Render("under "+policyName) + "\n")
	}

	// Per-tool reasons. The Index returns reasons for both warning and
	// violation states; for compliant tools the slice is empty.
	for _, reason := range m.complianceIndex.Reasons(tool.Name) {
		b.WriteString("    · " + reason + "\n")
	}

	// Install gating: if compliance.block_installs is on and this tool
	// can't be installed under the policy, surface that explicitly so
	// users understand why the install action is rejected before they
	// attempt it.
	if blocked, reason := m.complianceBlocksInstall(tool.Name); blocked {
		b.WriteString("\n    " + complianceErrorStyle.Render("⛔ install/upgrade blocked") +
			"  " + dashDim.Render(reason) + "\n")
		b.WriteString("    " + dashDim.Render("Disable in config.yaml: compliance.block_installs=false") + "\n")
	}

	b.WriteString("\n")
	return b.String()
}
