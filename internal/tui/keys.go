package tui

import (
	"fmt"
	"strings"

	"log/slog"

	tea "charm.land/bubbletea/v2"
)

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Modal key handlers — each intercepts all keys when active.
	if m.pendingAction != nil {
		return m.handleKeyConfirmation(msg)
	}
	if m.importingPath {
		return m.handleKeyImportPath(msg)
	}
	if m.enteringToken {
		return m.handleKeyTokenInput(msg)
	}
	if m.backupConfirm {
		return m.handleKeyBackupConfirm(msg)
	}
	if m.creatingPack {
		return m.handleKeyPackCreate(msg)
	}
	if m.viewingMyPackDetail {
		return m.handleKeyMyPackDetail(msg)
	}
	if m.viewingMyPacks {
		return m.handleKeyMyPacks(msg)
	}
	if m.viewingMyBackups {
		return m.handleKeyMyBackups(msg)
	}
	if m.categoryPicker {
		return m.handleKeySidebar(msg)
	}
	if m.showDetail {
		return m.handleKeyDetail(msg)
	}
	if m.showPackDetail {
		return m.handleKeyPackDetail(msg)
	}
	if m.filtering {
		return m.handleKeyFilter(msg)
	}
	return m.handleKeyDefault(msg)
}

// handleKeyConfirmation handles y/n confirmation for tool actions.

// handleKey moved to keys.go.
func (m Model) handleKeyConfirmation(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		action := *m.pendingAction
		m.pendingAction = nil
		slog.Info("executing tool action", "action", action.action, "cmd", strings.Join(action.cmdArgs, " "))
		m.statusMsg = fmt.Sprintf("Running %s...", action.action)
		return m, execToolActionCmd(action)
	case "n", "N", "esc":
		m.pendingAction = nil
		m.statusMsg = ""
		return m, nil
	}
	return m, nil // swallow all other keys
}

// handleKeyImportPath handles text input for the import file path.

// handleKeyConfirmation moved to keys.go.
func (m Model) handleKeyImportPath(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.importingPath = false
		m.importInput.SetValue("")
		return m, nil
	case "enter":
		path := strings.TrimSpace(m.importInput.Value())
		m.importingPath = false
		m.importInput.SetValue("")
		if path == "" {
			return m, nil
		}
		m.backupItems = nil
		m.backupDone = 0
		m.backupMode = backupModeImport
		m.activeTab = tabBackup
		m.cursor = 0
		m.statusMsg = "Building import plan..."
		return m, buildImportPlanCmd(m.svc, path)
	default:
		var cmd tea.Cmd
		m.importInput, cmd = m.importInput.Update(msg)
		return m, cmd
	}
}

// handleKeyTokenInput handles text input for the share token.

// handleKeyImportPath moved to keys.go.
func (m Model) handleKeyTokenInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.enteringToken = false
		m.tokenInput.SetValue("")
		return m, nil
	case "enter":
		token := strings.TrimSpace(m.tokenInput.Value())
		m.enteringToken = false
		m.tokenInput.SetValue("")
		if token == "" {
			return m, nil
		}
		m.backupItems = nil
		m.backupDone = 0
		m.backupMode = backupModeImport
		m.activeTab = tabBackup
		m.cursor = 0
		m.statusMsg = "Decoding share token..."
		return m, buildTokenImportPlanCmd(m.svc, token)
	default:
		var cmd tea.Cmd
		m.tokenInput, cmd = m.tokenInput.Update(msg)
		return m, cmd
	}
}

// handleKeyBackupConfirm handles the import plan review and item selection.

// handleKeyTokenInput moved to keys.go.
func (m Model) handleKeyBackupConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "y", "Y":
		m.backupConfirm = false
		for i := range m.backupItems {
			if m.backupItems[i].status == backupPending && !m.backupItems[i].selected {
				m.backupItems[i].status = backupSkipped
				m.backupItems[i].errMsg = "deselected"
				m.backupDone++
			}
		}
		cmd := m.nextBackupInstall()
		if cmd == nil {
			m.statusMsg = "Nothing to install — all tools skipped."
			return m, nil
		}
		m.statusMsg = "Installing..."
		return m, cmd
	case "esc", "n", "N":
		m.backupConfirm = false
		m.backupMode = ""
		m.backupItems = nil
		m.backupDone = 0
		m.statusMsg = ""
		return m, nil
	case "space":
		if m.cursor < len(m.backupItems) && m.backupItems[m.cursor].status == backupPending {
			m.backupItems[m.cursor].selected = !m.backupItems[m.cursor].selected
			if m.cursor < len(m.backupItems)-1 {
				m.cursor++
			}
		}
		return m, nil
	case "a":
		anySelected := false
		for _, item := range m.backupItems {
			if item.status == backupPending && item.selected {
				anySelected = true
				break
			}
		}
		for i := range m.backupItems {
			if m.backupItems[i].status == backupPending {
				m.backupItems[i].selected = !anySelected
			}
		}
		return m, nil
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.backupItems)-1 {
			m.cursor++
		}
	}
	return m, nil
}

// handleKeySidebar handles navigation in the filter sidebar panel.

// handleKeyBackupConfirm moved to keys.go.
func (m Model) handleKeySidebar(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "f":
		m.categoryPicker = false
		return m, nil
	case "enter":
		// Apply the selected filter.
		if m.sidebarIdx >= 0 && m.sidebarIdx < len(m.sidebarItems) {
			item := m.sidebarItems[m.sidebarIdx]
			if !item.isHeader {
				switch item.section {
				case "category":
					m.categoryFilter = item.value
				case "tag":
					m.tagFilter = item.value
				case "platform":
					m.platformFilter = item.value
				case "status":
					m.statusFilter = item.value
				}
				m.cursor = 0
				m.applyFilter()
			}
		}
		m.categoryPicker = false
		return m, nil
	case "up", "k":
		m.sidebarIdx = m.prevSelectableIdx(m.sidebarIdx)
	case "down", "j":
		m.sidebarIdx = m.nextSelectableIdx(m.sidebarIdx)
	}
	return m, nil
}

// nextSelectableIdx returns the next non-header index after idx, or idx if at end.

// handleKeyFilter handles the search/filter text input mode.
// The search box is focused — typing goes into it. Tab cycles categories.
func (m Model) handleKeyFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.filtering = false
		m.filterText = ""
		m.filterInput.SetValue("")
		m.categoryFilter = ""
		m.applyFilter()
		return m, nil
	case "enter":
		m.filtering = false
		return m, nil
	case "up":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "down":
		if m.cursor < m.rowCount()-1 {
			m.cursor++
		}
		return m, nil
	default:
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		m.filterText = m.filterInput.Value()
		m.cursor = 0
		m.applyFilter()
		return m, cmd
	}
}

// handleKeyDefault handles keys when no modal is active — tabs, navigation, actions.

// handleKeySidebar moved to keys.go.
func (m Model) nextSelectableIdx(idx int) int {
	for i := idx + 1; i < len(m.sidebarItems); i++ {
		if !m.sidebarItems[i].isHeader {
			return i
		}
	}
	return idx
}

// prevSelectableIdx returns the previous non-header index before idx, or idx if at start.

// nextSelectableIdx moved to keys.go.
func (m Model) prevSelectableIdx(idx int) int {
	for i := idx - 1; i >= 0; i-- {
		if !m.sidebarItems[i].isHeader {
			return i
		}
	}
	return idx
}

// handleKeyPackDetail moved to keys_detail.go.
