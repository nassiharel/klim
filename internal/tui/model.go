package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/nassiharel/clim/internal/registry"
)

const (
	tabInstalled = iota
	tabUpdates
	tabDiscover
	tabDisabled
	tabBackup
	tabConfig
	tabCount // total number of tabs, used for modular cycling
)

// Model is the Bubbletea model for the interactive TUI.
type Model struct {
	tools   []registry.Tool
	cursor  int
	spinner spinner.Model

	// Tabs.
	activeTab int

	// Filter.
	filterInput   textinput.Model
	filtering     bool
	filterText    string
	filteredIndex []int

	// Detail view.
	detailIdx  int // index into m.tools, -1 = no detail
	showDetail bool

	// Loading state.
	phase   int // 0=scanning, 1=resolving, 2=done
	loading bool
	pending int // count of tools still resolving versions
	scanGen int // incremented on each scan; used to discard stale toolVersionMsg

	// Layout.
	width  int
	height int

	// Quitting.
	quitting bool

	// Status message (transient, e.g. error feedback).
	statusMsg string

	// Action confirmation state.
	pendingAction *pendingAction // nil = no pending confirmation

	// Source picker state (choose which package manager to use).
	sourcePicker *sourcePicker // nil = no picker active

	// Import file path input.
	importInput   textinput.Model
	importingPath bool // true = import path input is active

	// Backup tab state.
	backupItems []backupItem   // items being exported/imported
	backupMode  string         // "" (idle), "export", "import"
	backupDone  int            // count of completed items
	backupBar   progress.Model // overall progress bar
}

// NewModel creates a new TUI model.
func NewModel() Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	ti := textinput.New()
	ti.Placeholder = "filter..."
	ti.CharLimit = 30

	ii := textinput.New()
	ii.Placeholder = "path/to/manifest.yaml"
	ii.CharLimit = 200

	return Model{
		spinner:     s,
		filterInput: ti,
		importInput: ii,
		backupBar:   progress.New(progress.WithWidth(40)),
		loading:     true,
		phase:       0,
		activeTab:   tabInstalled,
		detailIdx:   -1,
		width:       80,
		height:      24,
	}
}

// Init starts the initial tool discovery process.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg { return findToolsCmd()() },
	)
}

// Update handles all incoming messages and returns updated model and commands.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case scanResultMsg:
		m.tools = msg.tools
		sort.Slice(m.tools, func(i, j int) bool {
			return strings.ToLower(m.tools[i].Name) < strings.ToLower(m.tools[j].Name)
		})
		m.phase = 1
		m.pending = 0
		m.scanGen++
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("⚠ %v", msg.err)
		}
		m.applyFilter()

		// Fire per-tool version resolution commands.
		gen := m.scanGen
		var cmds []tea.Cmd
		for i, tool := range m.tools {
			if tool.IsInstalled() && !tool.Disabled {
				m.pending++
				idx := i
				t := tool // capture
				cmds = append(cmds, func() tea.Msg { return resolveToolVersionCmd(idx, gen, t)() })
			}
		}
		if len(cmds) == 0 {
			m.phase = 2
			m.loading = false
		}
		return m, tea.Batch(cmds...)

	case toolVersionMsg:
		// Discard stale messages from a previous scan generation.
		if msg.gen != m.scanGen {
			return m, nil
		}
		// Update the tool in place with resolved version data.
		if msg.index < len(m.tools) {
			m.tools[msg.index].Instances = msg.tool.Instances
			m.tools[msg.index].Latest = msg.tool.Latest
			m.tools[msg.index].LatestFrom = msg.tool.LatestFrom
		}
		m.pending--
		if m.pending <= 0 {
			m.phase = 2
			m.loading = false
		}
		return m, nil

	case execFinishedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ %s failed: %s", msg.action, msg.err)
			return m, nil
		}
		m.statusMsg = fmt.Sprintf("✓ %s succeeded — refreshing...", msg.action)
		// Re-scan the affected tool to pick up changes.
		tool := m.tools[msg.toolIdx]
		return m, refreshSingleToolCmd(msg.toolIdx, tool)

	case refreshToolMsg:
		if msg.toolIdx < len(m.tools) {
			m.tools[msg.toolIdx] = msg.tool
		}
		m.statusMsg = fmt.Sprintf("✓ %s refreshed", msg.tool.DisplayName)
		m.applyFilter()
		return m, nil

	case toolInfoMsg:
		if msg.toolIdx < len(m.tools) {
			m.tools[msg.toolIdx].Info = msg.info
			m.tools[msg.toolIdx].InfoFetched = true
		}
		return m, nil

	case exportFinishedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ Export failed: %s", msg.err)
			// Mark all items as failed.
			for i := range m.backupItems {
				m.backupItems[i].status = backupFailed
			}
		} else {
			m.statusMsg = fmt.Sprintf("✓ Exported %d tools to %s", msg.count, msg.path)
			// Mark all items as done.
			for i := range m.backupItems {
				m.backupItems[i].status = backupDone
			}
			m.backupDone = len(m.backupItems)
		}
		return m, nil

	case backupPlanMsg:
		m.backupItems = msg.items
		m.backupDone = 0
		// Count already-skipped items as "done" for progress.
		for _, item := range m.backupItems {
			if item.status == backupSkipped || item.status == backupFailed {
				m.backupDone++
			}
		}
		// Start installing the first pending item.
		cmd := m.nextBackupInstall()
		return m, cmd

	case backupItemDoneMsg:
		if msg.idx < len(m.backupItems) {
			if msg.err != nil {
				m.backupItems[msg.idx].status = backupFailed
				m.backupItems[msg.idx].errMsg = msg.err.Error()
			} else {
				m.backupItems[msg.idx].status = backupDone
			}
			m.backupDone++
		}
		// Fire the next install, or finish.
		if cmd := m.nextBackupInstall(); cmd != nil {
			return m, cmd
		}
		// All done — refresh tools to pick up newly installed ones.
		m.statusMsg = "✓ Import complete — refreshing..."
		m.loading = true
		m.phase = 0
		m.tools = nil
		m.filteredIndex = nil
		return m, tea.Batch(
			m.spinner.Tick,
			func() tea.Msg { return findToolsCmd()() },
		)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	default:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}
	return m, nil
}

// View renders the current UI state.
func (m Model) View() tea.View {
	v := tea.NewView(m.renderView())
	v.AltScreen = true
	return v
}

// Run starts the interactive TUI.
func Run() error {
	model := NewModel()
	p := tea.NewProgram(model)
	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	return nil
}

// --- Key handling ---

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Confirmation mode — intercept all keys.
	if m.pendingAction != nil {
		switch msg.String() {
		case "y", "Y":
			action := *m.pendingAction
			m.pendingAction = nil
			m.statusMsg = fmt.Sprintf("Running %s...", action.action)
			return m, execToolActionCmd(action)
		case "n", "N", "esc":
			m.pendingAction = nil
			m.statusMsg = ""
			return m, nil
		}
		return m, nil // swallow all other keys
	}

	// Source picker mode — user chooses which package manager to use.
	if m.sourcePicker != nil {
		switch msg.String() {
		case "1", "2", "3", "4", "5", "6":
			idx := int(msg.String()[0]-'0') - 1
			if idx < len(m.sourcePicker.choices) {
				choice := m.sourcePicker.choices[idx]
				m.pendingAction = &pendingAction{
					toolIdx: m.sourcePicker.toolIdx,
					action:  m.sourcePicker.action,
					cmdArgs: choice.cmdArgs,
				}
				m.sourcePicker = nil
			}
			return m, nil
		case "esc", "n", "N":
			m.sourcePicker = nil
			return m, nil
		}
		return m, nil // swallow all other keys
	}

	// Import path input mode.
	if m.importingPath {
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
			m.backupMode = "import"
			m.activeTab = tabBackup
			m.cursor = 0
			m.statusMsg = "Building import plan..."
			return m, buildImportPlanCmd(path)
		default:
			var cmd tea.Cmd
			m.importInput, cmd = m.importInput.Update(msg)
			return m, cmd
		}
	}

	// Detail view — Esc goes back, action keys available.
	if m.showDetail {
		switch msg.String() {
		case "esc", "q", "backspace":
			m.showDetail = false
			return m, nil
		case "i":
			m.startAction(m.resolveInstallAction())
			return m, nil
		case "u":
			m.startAction(m.resolveUpgradeAction())
			return m, nil
		case "d":
			m.startAction(m.resolveRemoveAction())
			return m, nil
		}
		return m, nil
	}

	// Filter mode.
	if m.filtering {
		switch msg.String() {
		case "esc":
			m.filtering = false
			m.filterText = ""
			m.filterInput.SetValue("")
			m.applyFilter()
			return m, nil
		case "enter":
			m.filtering = false
			return m, nil
		default:
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			m.filterText = m.filterInput.Value()
			m.applyFilter()
			return m, cmd
		}
	}

	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "right", "tab":
		m.activeTab = (m.activeTab + 1) % tabCount
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "left", "shift+tab":
		m.activeTab = (m.activeTab + tabCount - 1) % tabCount
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "1":
		m.activeTab = tabInstalled
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "2":
		m.activeTab = tabUpdates
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "3":
		m.activeTab = tabDiscover
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "4":
		m.activeTab = tabDisabled
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "5":
		m.activeTab = tabBackup
		m.cursor = 0
		return m, nil
	case "6":
		m.activeTab = tabConfig
		m.cursor = 0
		return m, nil
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < m.rowCount()-1 {
			m.cursor++
		}
	case "home", "g":
		m.cursor = 0
	case "end", "G":
		m.cursor = max(0, m.rowCount()-1)
	case "/":
		m.filtering = true
		return m, m.filterInput.Focus()
	case "enter":
		if m.cursor < len(m.filteredIndex) {
			// On Updates tab, Enter triggers upgrade directly.
			if m.activeTab == tabUpdates {
				if picker := m.resolveUpgradeAction(); picker != nil {
					m.startAction(picker)
					return m, nil
				}
			}
			// Otherwise open detail view (including Discover tab).
			m.detailIdx = m.filteredIndex[m.cursor]
			m.showDetail = true
			// Fetch tool info lazily if not already fetched.
			tool := m.tools[m.detailIdx]
			if !tool.InfoFetched {
				return m, fetchToolInfoCmd(m.detailIdx, tool)
			}
		}
		return m, nil
	case "i":
		m.startAction(m.resolveInstallAction())
		return m, nil
	case "u":
		m.startAction(m.resolveUpgradeAction())
		return m, nil
	case "d":
		m.startAction(m.resolveRemoveAction())
		return m, nil
	case "x":
		if m.cursor < len(m.filteredIndex) {
			idx := m.filteredIndex[m.cursor]
			tool := &m.tools[idx]
			if m.activeTab == tabDisabled {
				// Re-enable.
				if err := registry.SetToolEnabled(tool.Name, true); err != nil {
					m.statusMsg = fmt.Sprintf("⚠ Could not save config: %v", err)
				} else {
					m.statusMsg = ""
				}
				tool.Disabled = false
			} else {
				// Disable.
				if err := registry.SetToolEnabled(tool.Name, false); err != nil {
					m.statusMsg = fmt.Sprintf("⚠ Could not save config: %v", err)
				} else {
					m.statusMsg = ""
				}
				tool.Disabled = true
			}
			m.applyFilter()
		}
		return m, nil
	case "r":
		m.loading = true
		m.phase = 0
		m.tools = nil
		m.filteredIndex = nil
		m.cursor = 0
		return m, tea.Batch(
			m.spinner.Tick,
			func() tea.Msg { return findToolsCmd()() },
		)
	case "e":
		if m.phase >= 2 && len(m.tools) > 0 {
			// Build backup items from installed tools.
			m.backupItems = nil
			m.backupDone = 0
			for _, tool := range m.tools {
				if tool.IsInstalled() && !tool.Disabled {
					m.backupItems = append(m.backupItems, backupItem{
						name:    tool.Name,
						display: tool.DisplayName,
						source:  "—",
						status:  backupPending,
					})
				}
			}
			m.backupMode = "export"
			m.activeTab = tabBackup
			m.cursor = 0
			m.statusMsg = "Exporting..."
			return m, exportToolsCmd(m.tools)
		}
		return m, nil
	case "I":
		m.importingPath = true
		return m, m.importInput.Focus()
	}
	return m, nil
}

// --- Filtering ---

func (m *Model) applyFilter() {
	m.filteredIndex = nil
	filter := strings.ToLower(m.filterText)

	for i, tool := range m.tools {
		if !m.matchesTab(tool) {
			continue
		}
		if filter != "" &&
			!strings.Contains(strings.ToLower(tool.DisplayName), filter) &&
			!strings.Contains(strings.ToLower(tool.Name), filter) &&
			!strings.Contains(strings.ToLower(tool.Category), filter) {
			continue
		}
		m.filteredIndex = append(m.filteredIndex, i)
	}
	if m.cursor >= len(m.filteredIndex) {
		m.cursor = max(0, len(m.filteredIndex)-1)
	}
}

func (m *Model) matchesTab(tool registry.Tool) bool {
	switch m.activeTab {
	case tabInstalled:
		return !tool.Disabled && tool.IsInstalled()
	case tabUpdates:
		return !tool.Disabled && tool.HasUpdate()
	case tabDiscover:
		return !tool.Disabled && !tool.IsInstalled()
	case tabDisabled:
		return tool.Disabled
	case tabBackup:
		return false // Backup tab renders from backupItems, not tools
	case tabConfig:
		return false // Config tab renders static content, not tools
	}
	return false
}

// --- Stats ---

func (m Model) stats() (installed, updates, notInstalled, disabled int) {
	for _, tool := range m.tools {
		if tool.Disabled {
			disabled++
			continue
		}
		if tool.IsInstalled() {
			installed++
			if tool.HasUpdate() {
				updates++
			}
		} else {
			notInstalled++
		}
	}
	return
}

// --- Actions ---

// startAction takes a source picker and either shows it (multiple choices)
// or skips straight to confirmation (single choice). Does nothing if picker is nil.
func (m *Model) startAction(picker *sourcePicker) {
	if picker == nil {
		return
	}
	if len(picker.choices) == 1 {
		// Only one source available — skip picker, go straight to confirmation.
		m.pendingAction = &pendingAction{
			toolIdx: picker.toolIdx,
			action:  picker.action,
			cmdArgs: picker.choices[0].cmdArgs,
		}
		return
	}
	m.sourcePicker = picker
}

// currentTool returns the tool at the current cursor position, or nil.
func (m Model) currentTool() *registry.Tool {
	if m.showDetail && m.detailIdx >= 0 && m.detailIdx < len(m.tools) {
		return &m.tools[m.detailIdx]
	}
	if m.cursor < len(m.filteredIndex) {
		return &m.tools[m.filteredIndex[m.cursor]]
	}
	return nil
}

// currentToolIdx returns the index into m.tools for the current selection.
func (m Model) currentToolIdx() int {
	if m.showDetail && m.detailIdx >= 0 {
		return m.detailIdx
	}
	if m.cursor < len(m.filteredIndex) {
		return m.filteredIndex[m.cursor]
	}
	return -1
}

// resolveInstallAction builds a source picker for installing the current tool.
// Returns nil if the tool is already installed or has no install sources.
func (m Model) resolveInstallAction() *sourcePicker {
	tool := m.currentTool()
	if tool == nil || tool.IsInstalled() {
		return nil
	}
	var choices []sourceChoice
	for _, src := range registry.SourcesForOS() {
		if args := tool.Packages.InstallArgs(src); args != nil {
			choices = append(choices, sourceChoice{source: src, cmdArgs: args})
		}
	}
	if len(choices) == 0 {
		return nil
	}
	return &sourcePicker{
		toolIdx: m.currentToolIdx(),
		action:  "install",
		choices: choices,
	}
}

// resolveUpgradeAction builds a source picker for upgrading the current tool.
func (m Model) resolveUpgradeAction() *sourcePicker {
	tool := m.currentTool()
	if tool == nil || !tool.IsInstalled() {
		return nil
	}
	var choices []sourceChoice
	for _, src := range registry.SourcesForOS() {
		if args := tool.Packages.UpgradeArgs(src); args != nil {
			choices = append(choices, sourceChoice{source: src, cmdArgs: args})
		}
	}
	if len(choices) == 0 {
		return nil
	}
	return &sourcePicker{
		toolIdx: m.currentToolIdx(),
		action:  "upgrade",
		choices: choices,
	}
}

// resolveRemoveAction builds a source picker for removing the current tool.
func (m Model) resolveRemoveAction() *sourcePicker {
	tool := m.currentTool()
	if tool == nil || !tool.IsInstalled() {
		return nil
	}
	var choices []sourceChoice
	for _, src := range registry.SourcesForOS() {
		if args := tool.Packages.RemoveArgs(src); args != nil {
			choices = append(choices, sourceChoice{source: src, cmdArgs: args})
		}
	}
	if len(choices) == 0 {
		return nil
	}
	return &sourcePicker{
		toolIdx: m.currentToolIdx(),
		action:  "remove",
		choices: choices,
	}
}

// --- Backup ---

// rowCount returns the number of navigable rows for the current tab.
func (m Model) rowCount() int {
	switch m.activeTab {
	case tabBackup:
		return len(m.backupItems)
	case tabConfig:
		return 0
	default:
		return len(m.filteredIndex)
	}
}

// nextBackupInstall finds the next pending transfer item and fires its install command.
// Returns nil if no pending items remain (all done/failed/skipped).
func (m *Model) nextBackupInstall() tea.Cmd {
	for i := range m.backupItems {
		if m.backupItems[i].status == backupPending {
			m.backupItems[i].status = backupRunning
			m.statusMsg = fmt.Sprintf("Installing %s...", m.backupItems[i].display)
			return execBackupInstallCmd(i, m.backupItems[i].cmdArgs)
		}
	}
	return nil
}
