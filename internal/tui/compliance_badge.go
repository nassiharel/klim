package tui

import (
	"fmt"
	"strings"

	"github.com/nassiharel/clim/internal/compliance"
)

// complianceBadge returns a short colored badge for the tool's compliance status.
// Returns "" when no policy is configured or the tool is compliant.
func (m Model) complianceBadge(toolName string) string {
	if m.complianceIndex == nil || !m.complianceIndex.HasPolicy() {
		return ""
	}
	switch m.complianceIndex.Status(toolName) {
	case compliance.StatusViolation:
		return upgradableStyle.Render("✗ blocked")
	case compliance.StatusWarning:
		return upgradableStyle.Render("⚠ policy")
	default:
		return ""
	}
}

// complianceShortBadge returns a single-character badge for compact rows.
func (m Model) complianceShortBadge(toolName string) string {
	if m.complianceIndex == nil || !m.complianceIndex.HasPolicy() {
		return ""
	}
	switch m.complianceIndex.Status(toolName) {
	case compliance.StatusViolation:
		return upgradableStyle.Render("✗")
	case compliance.StatusWarning:
		return upgradableStyle.Render("⚠")
	default:
		return ""
	}
}

// complianceReasons returns human-readable reasons for status, joined by "; ".
func (m Model) complianceReasons(toolName string) string {
	if m.complianceIndex == nil {
		return ""
	}
	reasons := m.complianceIndex.Reasons(toolName)
	if len(reasons) == 0 {
		return ""
	}
	return strings.Join(reasons, "; ")
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
	return true, fmt.Sprintf("compliance blocked: %s", reason)
}