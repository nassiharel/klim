package tui

import (
	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/compliance"
)

// Compliance badge palette. Distinct soft red for hard violations vs
// the existing warm gold for warnings — without this both states render
// in upgradableStyle and the eye can't tell "blocked" from "license not
// preferred". Color 167 is a muted rose: clearly red, but not the
// "fire alarm" 196+bold that washes out the rest of the row. Mint
// green is reserved for explicit "compliant" chips (used in the detail
// header), not for list rows where silence is already the compliant
// signal.
var (
	complianceErrorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("167")) // soft rose-red
	complianceWarnStyle  = lipgloss.NewStyle().Foreground(warningColor)          // gold
	complianceOKStyle    = lipgloss.NewStyle().Foreground(successColor).Bold(true)
)

// complianceBadge returns a short colored badge for the tool's compliance status.
// Returns "" when no policy is configured or the tool is compliant.
func (m Model) complianceBadge(toolName string) string {
	if m.complianceIndex == nil || !m.complianceIndex.HasPolicy() {
		return ""
	}
	switch m.complianceIndex.Status(toolName) {
	case compliance.StatusViolation:
		return complianceErrorStyle.Render("✗ blocked")
	case compliance.StatusWarning:
		return complianceWarnStyle.Render("⚠ policy")
	default:
		return ""
	}
}

// complianceVerdictChip returns a hero-header chip for the tool's
// compliance state. Compliant tools and the "no policy" case both
// return "" so the page stays uncluttered when there's nothing to flag.
// The chip uses padded styling consistent with the existing
// INSTALLED/ARCHIVED chips so it slots into the header without
// re-aligning anything else.
func (m Model) complianceVerdictChip(toolName string) string {
	if m.complianceIndex == nil || !m.complianceIndex.HasPolicy() {
		return ""
	}
	switch m.complianceIndex.Status(toolName) {
	case compliance.StatusViolation:
		return complianceErrorStyle.Render(" ✗ POLICY VIOLATION ")
	case compliance.StatusWarning:
		return complianceWarnStyle.Render(" ⚠ POLICY WARNING ")
	default:
		return ""
	}
}

// complianceBlocksInstall returns (true, reason) when policy forbids installing the tool
// AND the user has block_installs=true configured.
func (m Model) complianceBlocksInstall(toolName string) (bool, string) {
	if m.complianceIndex == nil || !m.complianceIndex.HasPolicy() {
		return false, ""
	}
	if m.cfg == nil || !m.cfg.Compliance.BlockInstalls {
		return false, ""
	}
	ok, reason := m.complianceIndex.CanInstall(toolName)
	if ok {
		return false, ""
	}
	return true, "compliance blocked: " + reason
}
