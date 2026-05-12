package tui

import (
	"fmt"
	"strings"
)

// renderHelp builds the help / status footer line shown beneath each tab.
// All keys come from m.activeTab / m.* mode flags; nothing in this file
// touches IO or Model state.
func (m Model) renderHelp() string {
	// Confirmation mode — show prompt instead of normal help.
	if m.pendingAction != nil {
		prompt := confirmStyle.Render(fmt.Sprintf("  Run %s?", strings.Join(m.pendingAction.cmdArgs, " ")))
		keys := dimVersion.Render("y") + " confirm   " + dimVersion.Render("Esc") + " cancel"
		return prompt + "  " + keys
	}

	var parts []string

	switch m.activeTab {
	case tabBackup:
		switch {
		case m.viewingTrail:
			switch m.trailState {
			case trailViewLabelInput:
				parts = []string{
					dimVersion.Render("Enter") + " capture",
					dimVersion.Render("Esc") + " cancel",
				}
			default:
				parts = []string{
					dimVersion.Render("↑↓") + " navigate",
					dimVersion.Render("c") + " capture",
					dimVersion.Render("r") + " reload",
					dimVersion.Render("Esc") + " back",
				}
			}
		case m.backupMode == backupModeIdle:
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("←→") + " tab",
				dimVersion.Render("Enter") + " select",
				dimVersion.Render("q") + " quit",
			}
		case m.backupConfirm:
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("Space") + " toggle",
				dimVersion.Render("a") + " select all",
				dimVersion.Render("Enter") + " confirm",
				dimVersion.Render("Esc") + " cancel",
			}
		case m.backupMode == backupModeShare:
			parts = []string{
				dimVersion.Render("c") + " copy to clipboard",
				dimVersion.Render("Esc") + " back",
				dimVersion.Render("q") + " quit",
			}
		default:
			if m.isImportRunning() {
				parts = []string{
					dimVersion.Render("s") + " skip",
					dimVersion.Render("Esc") + " cancel",
					dimVersion.Render("q") + " quit",
				}
			} else {
				parts = []string{
					dimVersion.Render("↑↓") + " navigate",
					dimVersion.Render("←→") + " tab",
					dimVersion.Render("Esc") + " back",
					dimVersion.Render("q") + " quit",
				}
			}
		}
	case tabConfig:
		if m.configEditing {
			parts = []string{
				dimVersion.Render("Enter") + " confirm",
				dimVersion.Render("Esc") + " cancel",
			}
		} else {
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("Enter") + " edit",
				dimVersion.Render("S") + " save",
				dimVersion.Render("r") + " reset",
				dimVersion.Render("u") + " check update",
				dimVersion.Render("←→") + " tab",
				dimVersion.Render("q") + " quit",
			}
		}
	case tabProject:
		switch m.projectView {
		case projectViewList:
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("Enter") + " open",
				dimVersion.Render("n") + " new",
				dimVersion.Render("d") + " delete",
				dimVersion.Render("←→") + " tab",
				dimVersion.Render("q") + " quit",
			}
		case projectViewAddTool:
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("Enter") + " add",
				dimVersion.Render("Esc") + " cancel",
				dimVersion.Render("type") + " filter",
			}
		default:
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("Enter") + " select",
				dimVersion.Render("Esc") + " back",
				dimVersion.Render("q") + " quit",
			}
		}
	case tabDashboard:
		parts = []string{
			dimVersion.Render("↑↓") + " scroll",
			dimVersion.Render("Home") + " top",
			dimVersion.Render("←→") + " tab",
			dimVersion.Render("r") + " refresh",
			dimVersion.Render("q") + " quit",
		}
	case tabProfile:
		// My Profile tab — env sub-view keybindings.
		switch m.envState {
		case envViewIdle:
			parts = []string{
				dimVersion.Render("↑↓") + " scroll",
				dimVersion.Render("c") + " copy",
				dimVersion.Render("o") + " open",
				dimVersion.Render("d") + " diff",
				dimVersion.Render("a") + " apply",
				dimVersion.Render("r") + " refresh",
				dimVersion.Render("←→") + " tab",
				dimVersion.Render("q") + " quit",
			}
		case envViewInputOpen, envViewInputDiff, envViewInputApply:
			parts = []string{
				dimVersion.Render("Enter") + " submit",
				dimVersion.Render("Esc") + " cancel",
			}
		default:
			// Show / Diff / ApplyReport states: Esc returns to
			// idle (NOT to a parent menu, since Profile owns
			// the env sub-view directly). q falls through to
			// the same back-out, so don't advertise it as quit
			// here — that's misleading. ctrl+c always quits.
			parts = []string{
				dimVersion.Render("Esc") + " back",
				dimVersion.Render("←→") + " tab",
				dimVersion.Render("ctrl+c") + " quit",
			}
		}
	case tabDoctor:
		parts = []string{
			dimVersion.Render("↑↓") + " scroll",
			dimVersion.Render("Home") + " top",
			dimVersion.Render("←→") + " sub-tab / tab",
			dimVersion.Render("r") + " refresh",
		}
		// Sub-tab-specific hints. Keep the doctor footer compact
		// — only show keys that are actually live for the current
		// view.
		switch m.doctorSubTab {
		case doctorSubAudit:
			parts = append(parts, dimVersion.Render("v")+" scan vulns")
		case doctorSubCompliance:
			if m.complianceResult == nil && m.complianceError == "" {
				parts = append(parts, dimVersion.Render("i")+" init policy")
			}
			if m.cfg != nil && m.cfg.Compliance.URL != "" {
				parts = append(parts, dimVersion.Render("R")+" refresh policy")
			}
		}
		parts = append(parts, dimVersion.Render("q")+" quit")
	case tabHealth:
		switch m.healthSubTab {
		case healthSubPath:
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("t") + " switch view",
				dimVersion.Render("c") + " copy path",
				dimVersion.Render("o") + " open location",
				dimVersion.Render("u") + " uninstall shadowed",
				dimVersion.Render("←→") + " sub-tab / tab",
				dimVersion.Render("r") + " refresh",
				dimVersion.Render("q") + " quit",
			}
		default:
			parts = []string{
				dimVersion.Render("↑↓") + " select issue",
				dimVersion.Render("f/Enter") + " fix",
				dimVersion.Render("←→") + " sub-tab / tab",
				dimVersion.Render("r") + " refresh",
				dimVersion.Render("q") + " quit",
			}
		}
	case tabFavorites:
		switch {
		case m.favClearConfirm:
			parts = []string{
				dimVersion.Render("y") + " confirm",
				dimVersion.Render("n/Esc") + " cancel",
			}
		case m.favMode == "share" && m.sharedToken != "":
			parts = []string{
				dimVersion.Render("c") + " copy to clipboard",
				dimVersion.Render("Esc") + " back",
				dimVersion.Render("q") + " quit",
			}
		default:
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("*") + " unfavorite",
				dimVersion.Render("e") + " export",
				dimVersion.Render("s") + " share",
				dimVersion.Render("x") + " clear all",
				dimVersion.Render("q") + " quit",
			}
		}
	default:
		sortLabel := "s sort:name"
		if m.sortMode == sortByStars {
			sortLabel = "s sort:★"
		}
		parts = []string{
			dimVersion.Render("↑↓") + " navigate",
			dimVersion.Render("←→") + " tab",
			dimVersion.Render("*") + " favorite",
			dimVersion.Render(sortLabel),
			dimVersion.Render("Enter") + " detail",
			dimVersion.Render("f") + " filter",
			dimVersion.Render("r") + " refresh",
			dimVersion.Render("q") + " quit",
		}
		if m.activeTab == tabUpdates {
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("Space") + " toggle",
				dimVersion.Render("a") + " select all",
				dimVersion.Render("u") + " upgrade",
				dimVersion.Render("f") + " category",
				dimVersion.Render("Enter") + " detail",
				dimVersion.Render("q") + " quit",
			}
		}
		if m.activeTab == tabDiscover && m.discoverSubTab == discoverForYou {
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("Enter") + " detail",
				dimVersion.Render("i") + " install",
				dimVersion.Render("*") + " favorite",
				dimVersion.Render("←→") + " tab",
				dimVersion.Render("q") + " quit",
			}
		}
		if m.activeTab == tabDiscover && m.discoverSubTab == discoverOnboard {
			parts = []string{
				dimVersion.Render("↑↓") + " navigate",
				dimVersion.Render("[]") + " role",
				dimVersion.Render("Enter") + " detail",
				dimVersion.Render("i") + " install",
				dimVersion.Render("*") + " favorite",
				dimVersion.Render("←→") + " tab",
				dimVersion.Render("q") + " quit",
			}
		}
		// Override footer during active batch operation (applied last).
		if m.activeBatch != nil && m.activeBatch.isRunning() {
			parts = []string{
				dimVersion.Render(m.activeBatch.progress()),
				dimVersion.Render("s") + " skip",
				dimVersion.Render("Esc") + " cancel",
				dimVersion.Render("q") + " quit",
			}
		}
	}

	help := helpStyle.Render("  " + strings.Join(parts, "   "))
	if m.statusMsg != "" {
		// Status bar on its own line above help keys.
		status := "  " + upgradableStyle.Render(m.statusMsg)
		return status + "\n" + help
	}
	return help
}
