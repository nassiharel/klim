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
			{"1-0", "jump to tab (1=My Tools … 0=Config)"},
			{"Tab / Shift-Tab", "next / previous sub-tab"},
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
				{"↑ / ↓", "navigate tools"},
				{"Enter", "open detail view"},
				{"u", "upgrade selected tool"},
				{"i", "install selected tool"},
				{"r", "remove / uninstall"},
				{"*", "toggle favorite"},
				{"s", "share favorites (token)"},
				{"/", "search tools"},
				{"Esc", "close detail / cancel"},
			},
		})
	case tabDiscover:
		sections = append(sections, section{
			title: "Marketplace",
			keys: [][2]string{
				{"↑ / ↓", "navigate tools / packs"},
				{"Enter", "open detail view"},
				{"i", "install selected tool"},
				{"*", "toggle favorite"},
				{"/", "search marketplace"},
				{"Esc", "close detail / cancel"},
			},
		})
	case tabProject:
		sections = append(sections, section{
			title: "Project",
			keys: [][2]string{
				{"↑ / ↓", "navigate items"},
				{"Enter", "open detail"},
				{"r", "refresh scan"},
				{"Esc", "close"},
			},
		})
	case tabDashboard:
		sections = append(sections, section{
			title: "Dashboard",
			keys: [][2]string{
				{"↑ / ↓", "scroll view"},
			},
		})
	case tabAgents:
		sections = append(sections, section{
			title: "Agents — List",
			keys: [][2]string{
				{"1-7", "jump to sub-tab"},
				{"↑ / ↓", "move cursor"},
				{"Enter", "open detail page"},
				{"/", "fuzzy search"},
				{"S", "search transcripts"},
				{"s", "cycle sort mode"},
				{"f", "filter sidebar"},
				{"i", "toggle installed filter"},
				{"Space", "select for bulk ops"},
				{"b", "toggle bookmark"},
				{"l", "launch"},
				{"o", "open URL"},
				{"r", "refresh"},
				{"Esc", "close / cancel"},
			},
		}, section{
			title: "Agents — Detail",
			keys: [][2]string{
				{"← / →", "move action focus"},
				{"↑ / ↓", "scroll body"},
				{"Enter", "execute action"},
				{"o", "open URL"},
				{"c", "copy command"},
				{"r", "refresh"},
				{"Esc / q", "back"},
			},
		})
	case tabProfile:
		sections = append(sections, section{
			title: "My Profile",
			keys: [][2]string{
				{"Enter", "generate / refresh profile"},
				{"↑ / ↓", "scroll view"},
			},
		})
	case tabHealth:
		sections = append(sections, section{
			title: "Health",
			keys: [][2]string{
				{"↑ / ↓", "navigate issues"},
				{"Enter", "open fix wizard"},
				{"r", "re-scan"},
				{"t", "toggle view"},
				{"u", "uninstall shadowed copy"},
			},
		})
	case tabDoctor:
		sections = append(sections, section{
			title: "Security",
			keys: [][2]string{
				{"↑ / ↓", "scroll view"},
			},
		})
	case tabBackup:
		sections = append(sections, section{
			title: "Backup",
			keys: [][2]string{
				{"↑ / ↓", "navigate"},
				{"Enter", "select / confirm"},
				{"e", "export manifest"},
				{"i", "import manifest"},
				{"s", "share (token)"},
				{"Esc", "cancel"},
			},
		})
	case tabConfig:
		sections = append(sections, section{
			title: "Config",
			keys: [][2]string{
				{"↑ / ↓", "scroll"},
				{"Enter", "edit value"},
				{"r", "reset to default"},
			},
		})
	}

	// Build the inner content (no box chars — lipgloss adds the border).
	keyCol := 16
	descCol := 34
	innerWidth := keyCol + descCol + 3 // key + gap + desc

	keyStyle := lipgloss.NewStyle().Foreground(cyberAccent).Bold(true).Width(keyCol)
	descStyle := lipgloss.NewStyle().Foreground(cyberFGDim).Width(descCol)
	titleStyle := lipgloss.NewStyle().Foreground(cyberPrimary).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(cyberFGDim)

	var lines []string
	for si, sec := range sections {
		if si > 0 {
			lines = append(lines, dimStyle.Render(strings.Repeat("─", innerWidth)))
		}
		lines = append(lines, titleStyle.Render(sec.title))
		for _, kv := range sec.keys {
			line := keyStyle.Render(kv[0]) + "   " + descStyle.Render(kv[1])
			lines = append(lines, line)
		}
	}
	lines = append(lines, "", dimStyle.Render("press ? or any key to close"))

	content := strings.Join(lines, "\n")

	// Wrap in a lipgloss box with rounded border.
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cyberPrimary).
		Padding(1, 2).
		Width(innerWidth + 6). // padding + border
		Render(content)

	// Add title to the top border.
	boxLines := strings.Split(box, "\n")
	if len(boxLines) > 0 {
		topBorder := boxLines[0]
		titleText := " Keyboard Shortcuts "
		borderRunes := []rune(topBorder)
		titleRunes := []rune(titleText)
		// Insert title at position 4 (after "╭──").
		insertAt := 4
		if insertAt+len(titleRunes) < len(borderRunes) {
			for i, r := range titleRunes {
				borderRunes[insertAt+i] = r
			}
			boxLines[0] = string(borderRunes)
		}
		box = strings.Join(boxLines, "\n")
	}

	// Centre the box horizontally.
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
