package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m Model) renderBackupView() string {
	var b strings.Builder

	// Pack creation wizard.
	if m.creatingPack {
		return m.renderPackCreateView()
	}

	// My Packs view.
	if m.viewingMyPacks {
		return m.renderMyPacksView()
	}

	// My Backups view.
	if m.viewingMyBackups {
		return m.renderMyBackupsView()
	}

	if m.backupMode == backupModeIdle {
		b.WriteString("\n")

		type menuItem struct {
			label string
			desc  string
		}
		items := []menuItem{
			{"Export", "Save installed tools to a manifest file"},
			{"Import", "Reinstall tools from a manifest file"},
			{"Share", "Generate a share token for chat/messaging"},
			{"Open Token", "Install tools from a share token"},
			{"Create Pack", "Build a custom pack from marketplace tools"},
			{"My Packs", "View and manage your custom packs"},
			{"My Backups", "View and restore saved backups"},
		}

		for i, item := range items {
			cursor := "  "
			if i == m.cursor {
				cursor = "▸ "
			}
			line := cursor + nameStyle.Render(fixedWidth(item.label, 12)) + "  " + dimVersion.Render(item.desc)
			if i == m.cursor {
				w := lipgloss.Width(line)
				if w < m.width {
					line += strings.Repeat(" ", m.width-w)
				}
				line = selectedRowStyle.Render(line)
			}
			b.WriteString(line + "\n")
		}

		// Pad remaining space.
		visibleRows := m.height - 9 - m.footerHeight()
		for range max(visibleRows, 0) {
			b.WriteString("\n")
		}
		return b.String()
	}

	// Share token display mode.
	if m.backupMode == backupModeShare {
		b.WriteString("\n")
		b.WriteString("  " + detailTitleStyle.Render("Share Token") + "\n\n")
		b.WriteString("  " + dimVersion.Render("Send this token via Slack, Teams, or any chat:") + "\n\n")

		// Word-wrap the token to fit the terminal width.
		maxW := m.width - 6
		if maxW < 40 {
			maxW = 40
		}
		for _, line := range wordWrap(m.sharedToken, maxW) {
			b.WriteString("  " + dimVersion.Render(line) + "\n")
		}

		b.WriteString("\n")

		// Copy button.
		if m.tokenCopied {
			b.WriteString("  " + buttonDoneStyle.Render("✓ Copied to clipboard") + "\n")
		} else {
			b.WriteString("  " + buttonStyle.Render("⎘ Copy to clipboard (c)") + "\n")
		}

		b.WriteString("\n")
		b.WriteString("  " + dimVersion.Render("Recipients can install with:") + "  " + detailCmdStyle.Render("clim open <token>") + "\n")

		// Pad remaining space.
		visibleRows := m.height - 13 - m.footerHeight()
		for range max(visibleRows, 0) {
			b.WriteString("\n")
		}
		return b.String()
	}

	// Confirm mode — show review header instead of progress bar.
	if m.backupConfirm {
		pending := 0
		selected := 0
		skipped := 0
		for _, item := range m.backupItems {
			switch item.status {
			case backupPending:
				pending++
				if item.selected {
					selected++
				}
			case backupSkipped, backupFailed:
				skipped++
			}
		}
		b.WriteString("\n")
		b.WriteString(confirmStyle.Render("  Review import plan") + "  " +
			dimVersion.Render(fmt.Sprintf("%d selected of %d to install, %d skipped", selected, pending, skipped)) + "\n\n")
	} else {
		// Show currently installing tool + progress bar.
		total := len(m.backupItems)
		if total > 0 {
			// Find running item.
			for _, item := range m.backupItems {
				if item.status == backupRunning {
					fmt.Fprintf(&b, "  %s %s (%d/%d)\n",
						upgradableStyle.Render("Installing:"),
						itemLabel(item.name, item.display),
						m.backupDone+1, total,
					)
					break
				}
			}

			frac := float64(m.backupDone) / float64(total)
			barWidth := m.width - 30
			if barWidth < 20 {
				barWidth = 20
			}
			m.backupBar.SetWidth(barWidth)
			fmt.Fprintf(&b, "  %s  %s  %d/%d\n\n",
				detailLabelStyle.Render("Progress:"),
				m.backupBar.ViewAs(frac),
				m.backupDone, total,
			)
		}
	}

	// Header.
	b.WriteString(m.renderHeader() + "\n")

	// Backup rows.
	visibleRows := m.height - 8 - m.footerHeight()
	if visibleRows < 3 {
		visibleRows = 3
	}

	start := 0
	if m.cursor >= visibleRows {
		start = m.cursor - visibleRows + 1
	}

	for vi := start; vi < len(m.backupItems) && vi < start+visibleRows; vi++ {
		item := m.backupItems[vi]
		selected := vi == m.cursor
		b.WriteString(m.renderBackupRow(item, selected, m.backupConfirm) + "\n")
	}

	// Pad.
	rendered := min(len(m.backupItems)-start, visibleRows)
	for range max(visibleRows-rendered, 0) {
		b.WriteString("\n")
	}

	return b.String()
}

func (m Model) renderBackupRow(item backupItem, selected bool, confirmMode bool) string {
	cursor := "  "
	if selected {
		cursor = "▸ "
	}

	// Status icon (fixed 2-char width: icon + space).
	var icon string
	var statusLabel string
	var statusStyle lipgloss.Style
	switch item.status {
	case backupPending:
		if confirmMode {
			if item.selected {
				icon = upToDateStyle.Render("✓ ")
			} else {
				icon = dimVersion.Render("· ")
			}
		} else {
			icon = dimVersion.Render("○ ")
		}
		statusLabel = "pending"
		statusStyle = dimVersion
	case backupRunning:
		icon = upgradableStyle.Render("◉ ")
		statusLabel = "installing"
		statusStyle = upgradableStyle
	case backupDone:
		icon = upToDateStyle.Render("✓ ")
		statusLabel = "done"
		statusStyle = upToDateStyle
	case backupFailed:
		icon = upgradableStyle.Render("✗ ")
		if item.errMsg != "" {
			statusLabel = item.errMsg
		} else {
			statusLabel = "failed"
		}
		statusStyle = upgradableStyle
	case backupSkipped:
		icon = dimVersion.Render("– ")
		if item.errMsg != "" {
			statusLabel = item.errMsg
		} else {
			statusLabel = "skipped"
		}
		statusStyle = dimVersion
	}

	nameCell := nameStyle.Render(fixedWidth(itemLabel(item.name, item.display), colName))
	statusCell := statusStyle.Render(fixedWidth(statusLabel, colStatus))
	sourceCell := sourceStyle.Render(fixedWidth(item.source, colSource))

	line := cursor + icon + nameCell + "  " + statusCell + "  " + sourceCell

	if selected {
		// Pad to full width for selection highlight.
		w := lipgloss.Width(line)
		if w < m.width {
			line += strings.Repeat(" ", m.width-w)
		}
		line = selectedRowStyle.Render(line)
	}

	// Show attempted command for failed items when selected.
	if selected && item.status == backupFailed && len(item.cmdArgs) > 0 {
		line += "\n    " + dimVersion.Render("→ "+strings.Join(item.cmdArgs, " "))
	}

	return line
}

// --- Config tab ---
