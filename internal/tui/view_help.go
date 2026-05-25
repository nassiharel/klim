package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// renderHelpOverlay produces a full-screen, centred help modal with
// tab-aware keybinding sections. Called from renderView when
// m.helpOverlay is true; dismissed by ? or any other key.
func (m Model) renderHelpOverlay() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	// Collect sections: global keys + tab-specific keys.
	type section struct {
		title string
		keys  [][2]string
	}

	global := section{
		title: "Global",
		keys: [][2]string{
			{"1-0", "jump to tab"},
			{"Tab/Shift-Tab", "next / prev sub-tab"},
			{"P", "open Plan modal"},
			{"q / Ctrl-C", "quit"},
			{"?", "toggle this help"},
		},
	}

	sections := []section{global}

	switch m.activeTab {
	case tabInstalled, tabUpdates, tabFavorites:
		sections = append(sections, section{
			title: "My Tools",
			keys: [][2]string{
				{"↑/↓", "navigate"},
				{"Enter", "open detail"},
				{"u", "upgrade"},
				{"i", "install"},
				{"r", "remove"},
				{"*", "toggle favorite"},
				{"s", "share"},
				{"/", "search"},
				{"Esc", "cancel"},
			},
		})
	case tabDiscover:
		sections = append(sections, section{
			title: "Marketplace",
			keys: [][2]string{
				{"↑/↓", "navigate"},
				{"Enter", "open detail"},
				{"i", "install"},
				{"*", "toggle favorite"},
				{"/", "search"},
				{"Esc", "cancel"},
			},
		})
	case tabProject:
		sections = append(sections, section{
			title: "Project",
			keys: [][2]string{
				{"↑/↓", "navigate"},
				{"Enter", "open detail"},
				{"r", "refresh"},
			},
		})
	case tabDashboard:
		sections = append(sections, section{
			title: "Dashboard",
			keys: [][2]string{
				{"↑/↓", "scroll"},
			},
		})
	case tabAgents:
		sections = append(sections, section{
			title: "Agents",
			keys: [][2]string{
				{"1-7", "sub-tab"},
				{"↑/↓", "cursor"},
				{"Enter", "detail page"},
				{"/", "fuzzy filter"},
				{"S", "search transcripts"},
				{"s", "sort"},
				{"f", "filter sidebar"},
				{"i", "installed filter"},
				{"Space", "bulk select"},
				{"b", "bookmark"},
				{"l", "launch"},
				{"o", "open URL"},
				{"r", "refresh"},
			},
		}, section{
			title: "Detail Page",
			keys: [][2]string{
				{"←/→", "action focus"},
				{"↑/↓", "scroll body"},
				{"Enter", "run action"},
				{"o", "open URL"},
				{"c", "copy command"},
				{"Esc/q", "back"},
			},
		})
	case tabProfile:
		sections = append(sections, section{
			title: "My Profile",
			keys: [][2]string{
				{"Enter", "generate profile"},
				{"↑/↓", "scroll"},
			},
		})
	case tabHealth:
		sections = append(sections, section{
			title: "Health",
			keys: [][2]string{
				{"↑/↓", "navigate"},
				{"Enter", "fix wizard"},
				{"r", "re-scan"},
				{"t", "toggle view"},
				{"u", "uninstall shadowed"},
			},
		})
	case tabDoctor:
		sections = append(sections, section{
			title: "Security",
			keys: [][2]string{
				{"↑/↓", "scroll"},
			},
		})
	case tabBackup:
		sections = append(sections, section{
			title: "Backup",
			keys: [][2]string{
				{"↑/↓", "navigate"},
				{"Enter", "select"},
				{"e", "export"},
				{"i", "import"},
				{"s", "share"},
				{"Esc", "cancel"},
			},
		})
	case tabConfig:
		sections = append(sections, section{
			title: "Config",
			keys: [][2]string{
				{"↑/↓", "scroll"},
				{"Enter", "edit value"},
				{"r", "reset default"},
			},
		})
	}

	// Build two-column layout: if we have 2+ sections, put them
	// side by side; otherwise single column.
	keyStyle := lipgloss.NewStyle().
		Foreground(cyberAccent).
		Bold(true).
		PaddingRight(2)
	descStyle := lipgloss.NewStyle().
		Foreground(cyberFG)
	sectionTitleStyle := lipgloss.NewStyle().
		Foreground(cyberPrimary).
		Bold(true).
		PaddingBottom(1)
	dimStyle := lipgloss.NewStyle().
		Foreground(cyberFGDim)

	renderSection := func(sec section) string {
		var sb strings.Builder
		sb.WriteString(sectionTitleStyle.Render(sec.title) + "\n")
		for _, kv := range sec.keys {
			sb.WriteString(keyStyle.Render(fmt.Sprintf("%-14s", kv[0])) + descStyle.Render(kv[1]) + "\n")
		}
		return sb.String()
	}

	// Arrange sections in two columns.
	var leftParts, rightParts []string
	for i, sec := range sections {
		if i%2 == 0 {
			leftParts = append(leftParts, renderSection(sec))
		} else {
			rightParts = append(rightParts, renderSection(sec))
		}
	}

	leftCol := strings.Join(leftParts, "\n")
	rightCol := strings.Join(rightParts, "\n")

	leftBlock := lipgloss.NewStyle().Width(30).Render(leftCol)
	rightBlock := lipgloss.NewStyle().Width(30).Render(rightCol)

	columns := lipgloss.JoinHorizontal(lipgloss.Top, leftBlock, "   ", rightBlock)

	// Title + dismiss hint.
	title := lipgloss.NewStyle().
		Foreground(cyberPrimary).
		Bold(true).
		PaddingBottom(1).
		Render("⌨  Keyboard Shortcuts")
	dismiss := dimStyle.Render("press ? or any key to close")

	inner := title + "\n\n" + columns + "\n\n" + dismiss

	// Wrap in a styled box.
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cyberPrimary).
		Padding(1, 3).
		Render(inner)

	// Centre horizontally.
	centred := lipgloss.NewStyle().
		Width(m.width).
		Align(lipgloss.Center).
		Render(box)

	// Centre vertically.
	contentLines := strings.Count(centred, "\n") + 1
	topPad := (m.height - contentLines) / 2
	if topPad < 1 {
		topPad = 1
	}

	return strings.Repeat("\n", topPad) + centred
}

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
				dimVersion.Render("P") + " preview plan",
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

	// Append ? help hint to every footer (unless in a modal/prompt
	// state that already overrides parts).
	if len(parts) > 0 && m.pendingAction == nil {
		parts = append(parts, dimVersion.Render("?")+" help")
	}

	help := helpStyle.Render("  " + strings.Join(parts, "   "))
	if m.statusMsg != "" {
		// Status bar on its own line above help keys.
		status := "  " + upgradableStyle.Render(m.statusMsg)
		return status + "\n" + help
	}
	return help
}
