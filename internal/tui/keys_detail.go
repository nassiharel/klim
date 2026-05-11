package tui

import (
	"fmt"
	"strings"

	"log/slog"

	tea "charm.land/bubbletea/v2"
)

// handleKeyDetail handles navigation in the tool detail/action menu view.
//
// Key bindings:
//   - up/k, down/j   — scroll body first; once at the top/bottom of the
//     body they move the action-menu selection, then the related-tools
//     cursor. This lets ↑/↓ behave like ordinary reading keys on long
//     tool pages instead of silently moving a menu cursor that's off
//     screen below the fold.
//   - PgUp/PgDn      — scroll the detail body by one page
//   - Home/End       — jump to top/bottom of the detail body
//   - Enter          — run the selected action
//   - Esc/q/Backspace — close the detail view
func (m Model) handleKeyDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "backspace":
		m.showDetail = false
		m.toolMenu = noMenu
		m.toolMenuItems = nil
		m.detailScroll = 0
		m.detailRelCursor = -1
		m.detailRelated = nil
		return m, nil
	case "up", "k":
		// Priority: scroll body up → move action menu up → exit related list.
		// We scroll first when the user is partway down the page so ↑/↓ feel
		// like ordinary reading keys; only at the top of the body do they
		// re-take responsibility for the menu / related-tools cursors.
		switch {
		case m.detailScroll > 0:
			m.detailScroll--
		case m.detailRelCursor > 0:
			m.detailRelCursor--
		case m.detailRelCursor == 0:
			// Move from related list back to action menu (last item).
			m.detailRelCursor = -1
			if len(m.toolMenuItems) > 0 {
				m.toolMenu = len(m.toolMenuItems) - 1
			}
		case m.toolMenu > 0:
			m.toolMenu--
		}
	case "down", "j":
		// Priority: scroll body down → action menu → related tools. ↓ on a
		// freshly-opened long page now actually scrolls the page instead of
		// silently moving a cursor that's off-screen below the fold.
		switch {
		case m.detailScroll < m.detailMaxScroll:
			m.detailScroll++
		case m.toolMenu < len(m.toolMenuItems)-1:
			m.toolMenu++
		case len(m.detailRelated) > 0 && m.detailRelCursor == -1:
			// Move from action menu to related list.
			m.detailRelCursor = 0
			if len(m.toolMenuItems) > 0 {
				m.toolMenu = len(m.toolMenuItems) - 1
			}
		case m.detailRelCursor >= 0 && m.detailRelCursor < len(m.detailRelated)-1:
			m.detailRelCursor++
		}
		m.clampDetailScroll()
	case "pgup":
		page := m.height - 6
		if page < 1 {
			page = 1
		}
		m.detailScroll -= page
		if m.detailScroll < 0 {
			m.detailScroll = 0
		}
	case "pgdown", " ":
		page := m.height - 6
		if page < 1 {
			page = 1
		}
		m.detailScroll += page
		m.clampDetailScroll()
	case "home", "g":
		m.detailScroll = 0
	case "end", "G":
		m.detailScroll = m.detailMaxScroll
	case "enter":
		// Enter on related tool → open its detail view.
		if m.detailRelCursor >= 0 && m.detailRelCursor < len(m.detailRelated) {
			m.openDetailView(m.detailRelated[m.detailRelCursor].ToolIdx)
			return m, nil
		}
		// Enter on PM row → execute primary action (install or upgrade) directly.
		if m.toolMenu >= 0 && m.toolMenu < len(m.toolMenuItems) {
			action := m.toolMenuItems[m.toolMenu]
			if action.picker != nil && len(action.picker.choices) > 0 {
				pa := pendingAction{
					toolIdx: action.picker.toolIdx,
					action:  action.picker.action,
					cmdArgs: action.picker.choices[0].cmdArgs,
				}
				// Honour compliance.block_installs here too — without
				// this the detail view's PM-row Enter bypasses the
				// confirmation-based gate that handleKeys enforces.
				if pa.toolIdx >= 0 && pa.toolIdx < len(m.tools) {
					if blocked, reason := m.complianceBlocksInstall(m.tools[pa.toolIdx].Name); blocked {
						m.statusMsg = reason
						return m, nil
					}
				}
				slog.Info("executing tool action", "action", pa.action, "cmd", strings.Join(pa.cmdArgs, " "))
				m.statusMsg = fmt.Sprintf("Running %s...", pa.action)
				return m, execToolActionCmd(pa)
			}
		}
		return m, nil
	case "x":
		// Remove via selected PM — execute directly.
		if m.toolMenu >= 0 && m.toolMenu < len(m.toolMenuItems) {
			action := m.toolMenuItems[m.toolMenu]
			if action.removePicker != nil && len(action.removePicker.choices) > 0 {
				pa := pendingAction{
					toolIdx: action.removePicker.toolIdx,
					action:  action.removePicker.action,
					cmdArgs: action.removePicker.choices[0].cmdArgs,
				}
				slog.Info("executing tool action", "action", pa.action, "cmd", strings.Join(pa.cmdArgs, " "))
				m.statusMsg = fmt.Sprintf("Running %s...", pa.action)
				return m, execToolActionCmd(pa)
			}
		}
		return m, nil
	}
	return m, nil
}

// handleKeyPackDetail handles navigation in the pack detail view.

func (m Model) handleKeyPackDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While a pack operation is running: Esc cancels, s skips, q dismisses view.
	if m.packInstalling {
		switch msg.String() {
		case "esc":
			// Cancel pack operation — only if items are still pending/running.
			hasActive := false
			for _, item := range m.packItems {
				if item.status == packItemPending || item.status == packItemRunning {
					hasActive = true
					break
				}
			}
			if !hasActive {
				return m, nil
			}
			m.packCancelled = true
			for i := range m.packItems {
				if m.packItems[i].status == packItemPending {
					m.packItems[i].status = packItemSkipped
					m.packItems[i].errMsg = "cancelled"
					m.packDone++
				}
			}
			m.statusMsg = "⚠ Cancelled — waiting for current item..."
			return m, nil
		case "s":
			// Skip the next pending item so it won't be executed.
			skipped := false
			for i := range m.packItems {
				if m.packItems[i].status == packItemPending {
					m.packItems[i].status = packItemSkipped
					m.packItems[i].errMsg = "skipped"
					m.packDone++
					skipped = true
					break
				}
			}
			if skipped {
				m.statusMsg = "⏭ Next item skipped"
			}
			return m, nil
		case "q":
			m.showPackDetail = false
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
	case "esc", "q", "backspace":
		m.showPackDetail = false
		m.packItems = nil
		m.packInstalling = false
		m.packDone = 0
		m.packCancelled = false
		return m, nil
	case "up", "k":
		if m.packToolCursor > 0 {
			m.packToolCursor--
		} else if m.packDetailIdx >= 0 && m.packDetailIdx < len(m.packs) && len(m.packs[m.packDetailIdx].ToolNames) > 0 {
			m.packToolCursor = len(m.packs[m.packDetailIdx].ToolNames) - 1
		}
	case "down", "j":
		if m.packDetailIdx >= 0 && m.packDetailIdx < len(m.packs) {
			maxIdx := len(m.packs[m.packDetailIdx].ToolNames) - 1
			if m.packToolCursor < maxIdx {
				m.packToolCursor++
			} else {
				m.packToolCursor = 0
			}
		}
	case "enter":
		// Open tool detail for selected tool in pack.
		if m.packDetailIdx >= 0 && m.packDetailIdx < len(m.packs) {
			pack := m.packs[m.packDetailIdx]
			if m.packToolCursor < len(pack.ToolNames) {
				toolName := pack.ToolNames[m.packToolCursor]
				for i, t := range m.tools {
					if t.Name == toolName {
						m.detailIdx = i
						m.showDetail = true
						m.showPackDetail = false
						m.buildToolMenu()
						return m, nil
					}
				}
			}
		}
	case "i":
		// Install missing tools.
		if m.packDetailIdx >= 0 && m.packDetailIdx < len(m.packs) {
			pack := m.packs[m.packDetailIdx]
			m.packItems = buildPackInstallItems(m.tools, pack)
			m.packDone = countPackSkipped(m.packItems)
			m.packInstalling = true
			m.packCancelled = false
			m.packAction = "Installing"
			if cmd := m.nextPackItem(); cmd != nil {
				return m, cmd
			}
			m.packInstalling = false
			m.statusMsg = "Nothing to install — all tools skipped."
		}
		return m, nil
	case "x":
		// Remove installed tools.
		if m.packDetailIdx >= 0 && m.packDetailIdx < len(m.packs) {
			pack := m.packs[m.packDetailIdx]
			m.packItems = buildPackRemoveItems(m.tools, pack)
			m.packDone = countPackSkipped(m.packItems)
			m.packInstalling = true
			m.packCancelled = false
			m.packAction = "Removing"
			if cmd := m.nextPackItem(); cmd != nil {
				return m, cmd
			}
			m.packInstalling = false
			m.statusMsg = "Nothing to remove — all tools skipped."
		}
		return m, nil
	}
	return m, nil
}

// nextPackItem finds the next pending pack item and fires its command.
