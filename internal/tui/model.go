package tui

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/nassiharel/clim/internal/catalog"
	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/service"
)

const (
	tabInstalled = iota
	tabUpdates
	tabDiscover
	tabBackup
	tabConfig
	tabCount // total number of tabs, used for modular cycling
)

// Marketplace sub-tabs.
const (
	discoverTools       = 0
	discoverPacks       = 1
	discoverForYou      = 2
	discoverSubTabCount = 3
)

// Scan phases — the loading lifecycle.
const (
	phaseLoading   = 0 // loading catalog + scanning PATH
	phaseResolving = 1 // version resolution in progress
	phaseDone      = 2 // all tools resolved
)

// Backup tab menu indices.
const (
	backupMenuExport    = 0
	backupMenuImport    = 1
	backupMenuShare     = 2
	backupMenuOpenToken = 3
	backupMenuCount     = 4
)

// Sentinel values.
const (
	noDetail = -1 // detailIdx when no detail view is open
	noMenu   = -1 // toolMenu when no action menu is shown
)

// Backup mode states.
const (
	backupModeIdle   = ""
	backupModeExport = "export"
	backupModeImport = "import"
	backupModeShare  = "share"
)

// Tool action names.
const (
	actionInstall = "install"
	actionUpgrade = "upgrade"
	actionRemove  = "remove"
)

// Model is the Bubbletea model for the interactive TUI.
type Model struct {
	// Dependencies.
	svc  *service.ToolService
	clip Clipboard
	cfg  *config.Config

	tools   []registry.Tool
	cursor  int
	spinner spinner.Model

	// Tabs.
	activeTab int

	// Filter.
	filterInput    textinput.Model
	filtering      bool
	filterText     string
	filteredIndex  []int
	categoryFilter string   // "" = all categories; non-empty = only this category
	tagFilter      string   // "" = all tags; non-empty = only tools with this tag
	platformFilter string   // "" = all platforms; non-empty = only this platform
	categories     []string // sorted unique categories, computed once after scan
	tags           []string // sorted unique tags, computed once after scan
	platforms      []string // sorted unique platforms, computed once after scan

	// Sidebar filter panel.
	categoryPicker bool
	sidebarIdx     int
	sidebarItems   []sidebarItem

	// Marketplace sub-tabs (Tools / Packs / For You).
	discoverSubTab  int // 0=tools, 1=packs, 2=forYou
	packs           []registry.Pack
	recommendations []recommendation // tag-based, computed after scan
	showPackDetail  bool
	packDetailIdx   int // index into m.packs

	// Pack install/remove state (inline in pack detail view).
	packItems      []packItem // per-tool status during pack install/remove
	packInstalling bool       // true while a pack operation is in progress
	packDone       int        // count of completed items

	// Marketplace refresh diff — carried across rescans to apply badges.
	lastDiff *catalog.DiffResult

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

	// Tool action menu (Upgrade/Remove/Install).
	toolMenu      int              // noMenu = hidden; 0+ = selected action index
	toolMenuItems []toolMenuAction // resolved actions for the current tool

	// Import file path input.
	importInput   textinput.Model
	importingPath bool // true = import path input is active

	// Share token input.
	tokenInput    textinput.Model
	enteringToken bool   // true = token input is active
	sharedToken   string // last generated share token (for display)
	tokenCopied   bool   // true after token copied to clipboard

	// Backup tab state.
	backupItems     []backupItem   // items being exported/imported
	backupMode      string         // "" (idle), "export", "import"
	backupDone      int            // count of completed items
	backupBar       progress.Model // overall progress bar
	backupResult    string         // deferred status message shown after progress animation
	backupConfirm   bool           // true = import plan shown, waiting for user confirmation
	backupCancelled bool           // true = user pressed Esc during install

	// Batch update state (Updates tab).
	updateSelected map[int]bool      // tool index → selected for batch upgrade
	batchUpdating  bool              // true while batch upgrade is in progress
	batchQueue     batchUpgradeQueue // remaining tool indices to upgrade
}

// NewModel creates a new TUI model.
func NewModel() Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	ti := textinput.New()
	ti.Placeholder = "search tools..."
	ti.CharLimit = 30

	ii := textinput.New()
	ii.Placeholder = "path/to/manifest.yaml"
	ii.CharLimit = 200

	ti2 := textinput.New()
	ti2.Placeholder = "paste share token here..."
	ti2.CharLimit = 1000

	return Model{
		svc:            service.New(),
		clip:           systemClipboard{},
		spinner:        s,
		filterInput:    ti,
		importInput:    ii,
		tokenInput:     ti2,
		backupBar:      progress.New(progress.WithWidth(40)),
		updateSelected: make(map[int]bool),
		loading:        true,
		phase:          phaseLoading,
		activeTab:      tabInstalled,
		detailIdx:      noDetail,
		toolMenu:       noMenu,
		width:          80,
		height:         24,
	}
}

// NewModelWithConfig creates a new TUI model configured from the given Config.
func NewModelWithConfig(cfg *config.Config) Model {
	m := NewModel()
	m.svc = service.NewWithConfig(cfg)
	m.activeTab = tabFromName(cfg.UI.DefaultTab)
	m.cfg = cfg
	return m
}

// tabFromName maps a config tab name string to the tab constant.
func tabFromName(name string) int {
	switch name {
	case "installed":
		return tabInstalled
	case "updates":
		return tabUpdates
	case "marketplace":
		return tabDiscover
	case "backup":
		return tabBackup
	case "config":
		return tabConfig
	default:
		return tabInstalled
	}
}

// Init starts the initial tool discovery process.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg { return findToolsCmd(m.svc)() },
	)
}

// startScan prepares the model for a new scan, invalidating any in-flight
// version resolution from a previous scan so stale toolVersionMsg messages
// are discarded immediately. Every code path that fires findToolsCmd must
// call this first.
func (m *Model) startScan() tea.Cmd {
	m.loading = true
	m.phase = phaseLoading
	m.tools = nil
	m.filteredIndex = nil
	m.cursor = 0
	m.scanGen++
	m.pending = 0
	// Clear upgrade selection — indices are tied to the old tool ordering
	// and would select the wrong tools after a rescan reorders the list.
	m.updateSelected = make(map[int]bool)
	m.batchUpdating = false
	m.batchQueue = nil
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg { return findToolsCmd(m.svc)() },
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
		m.scanGen++

		// Set status based on how the catalog was loaded.
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("⚠ %v", msg.err)
		} else if info := msg.catalogInfo; info != nil {
			switch info.Source {
			case catalog.SourceCache:
				m.statusMsg = fmt.Sprintf("✓ Loaded %d tools from cache", info.Tools)
			case catalog.SourceRemote:
				m.statusMsg = fmt.Sprintf("✓ Fetched catalog (%d tools)", info.Tools)
			}
		}

		// If no tools loaded (e.g. catalog fetch failed), skip to done.
		if len(m.tools) == 0 {
			m.phase = phaseDone
			m.loading = false
			m.pending = 0
			m.applyFilter()
			return m, nil
		}

		m.phase = phaseResolving
		m.pending = 0

		// Reset filters — applyFilter() will rebuild sidebar items contextually.
		m.categoryFilter = ""
		m.tagFilter = ""
		m.platformFilter = ""

		// Load packs.
		if packs, err := m.svc.LoadPacks(context.Background()); err != nil {
			m.packs = nil
			slog.Warn("failed to load packs", "error", err)
			if m.statusMsg == "" {
				m.statusMsg = fmt.Sprintf("⚠ Marketplace unavailable: %v", err)
			}
		} else {
			m.packs = packs
		}

		// Clamp pack-related indices to the new list size.
		if m.showPackDetail && m.packDetailIdx >= len(m.packs) {
			m.showPackDetail = false
		}
		if m.discoverSubTab == discoverPacks && m.cursor >= len(m.packs) {
			m.cursor = max(0, len(m.packs)-1)
		}

		// Compute smart recommendations from tag overlap.
		m.recommendations = computeRecommendations(m.tools)
		if m.discoverSubTab == discoverForYou && m.cursor >= len(m.recommendations) {
			m.cursor = max(0, len(m.recommendations)-1)
		}

		// Apply marketplace refresh badges if a diff is pending.
		if m.lastDiff != nil {
			newSet := make(map[string]struct{}, len(m.lastDiff.NewTools))
			for _, name := range m.lastDiff.NewTools {
				newSet[name] = struct{}{}
			}
			for i := range m.tools {
				if _, ok := newSet[m.tools[i].Name]; ok {
					m.tools[i].MarketplaceStatus = registry.StatusNew
				} else if _, ok := m.lastDiff.ChangedTools[m.tools[i].Name]; ok {
					m.tools[i].MarketplaceStatus = registry.StatusChanged
				}
			}
			m.lastDiff = nil
		}

		m.applyFilter()

		// Fire per-tool version resolution commands.
		gen := m.scanGen
		var cmds []tea.Cmd
		for i, tool := range m.tools {
			if tool.IsInstalled() {
				m.pending++
				idx := i
				t := tool // capture
				cmds = append(cmds, func() tea.Msg { return resolveToolVersionCmd(m.svc, idx, gen, t)() })
			}
		}
		if len(cmds) == 0 {
			m.phase = phaseDone
			m.loading = false
			m.statusMsg = ""
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
			// Auto-select for batch upgrade if an update is available.
			if m.tools[msg.index].HasUpdate() {
				m.updateSelected[msg.index] = true
			}
		}
		m.pending--
		if m.pending <= 0 {
			m.phase = phaseDone
			m.loading = false
			m.statusMsg = ""
		}
		return m, nil

	case execFinishedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ %s failed: %s", msg.action, msg.err)
			toolName := "unknown"
			if msg.toolIdx < len(m.tools) {
				toolName = m.tools[msg.toolIdx].Name
			}
			slog.Warn("tool action failed", "action", msg.action, "tool", toolName, "error", msg.err)
			return m, nil
		}
		if msg.toolIdx >= len(m.tools) {
			return m, nil
		}
		slog.Info("tool action succeeded", "action", msg.action, "tool", m.tools[msg.toolIdx].Name)
		m.statusMsg = fmt.Sprintf("✓ %s succeeded — refreshing...", msg.action)
		// Re-scan the affected tool to pick up changes.
		tool := m.tools[msg.toolIdx]
		return m, refreshSingleToolCmd(m.svc, msg.toolIdx, tool)

	case refreshToolMsg:
		if msg.toolIdx < len(m.tools) {
			m.tools[msg.toolIdx] = msg.tool
		}
		m.statusMsg = fmt.Sprintf("✓ %s refreshed", msg.tool.DisplayName)
		m.applyFilter()
		// Rebuild the tool menu if the detail view is still showing.
		if m.showDetail {
			m.buildToolMenu()
		}
		return m, nil

	case toolInfoMsg:
		if msg.toolIdx < len(m.tools) {
			m.tools[msg.toolIdx].Info = msg.info
			m.tools[msg.toolIdx].InfoFetched = true
		}
		return m, nil

	case marketplaceRefreshMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("⚠ Refresh failed: %s", msg.err)
			return m, nil
		}
		diff := msg.result.Diff
		// Store diff so badges survive the subsequent rescan.
		m.lastDiff = &diff
		// Reload tools from the updated cache so new tools appear.
		if msg.result.Updated {
			cmd := m.startScan()
			m.statusMsg = fmt.Sprintf("✓ Marketplace updated: %d new, %d changed",
				len(diff.NewTools), len(diff.ChangedTools))
			return m, cmd
		}
		m.statusMsg = "✓ Marketplace is up to date"
		return m, nil

	case exportFinishedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ Export failed: %s", msg.err)
			for i := range m.backupItems {
				m.backupItems[i].status = backupFailed
				m.backupItems[i].errMsg = msg.err.Error()
			}
			m.backupDone = len(m.backupItems)
			return m, nil
		}
		// Store final message; animate progress item by item.
		m.backupResult = fmt.Sprintf("✓ Exported %d tools to %s", msg.count, msg.path)
		m.statusMsg = "Exporting..."
		return m, tea.Tick(40*time.Millisecond, func(time.Time) tea.Msg { return backupTickMsg{} })

	case backupTickMsg:
		// Mark the next pending item as done.
		for i := range m.backupItems {
			if m.backupItems[i].status == backupPending {
				m.backupItems[i].status = backupDone
				m.backupDone++
				// More items? schedule next tick.
				if m.backupDone < len(m.backupItems) {
					return m, tea.Tick(40*time.Millisecond, func(time.Time) tea.Msg { return backupTickMsg{} })
				}
				// All done — show final message.
				m.statusMsg = m.backupResult
				m.backupResult = ""
				return m, nil
			}
		}
		// No pending items left.
		m.statusMsg = m.backupResult
		m.backupResult = ""
		return m, nil

	case backupPlanMsg:
		if msg.err != nil {
			// Import failed entirely — show error and return to idle menu.
			m.backupMode = backupModeIdle
			m.backupItems = nil
			m.backupDone = 0
			m.backupConfirm = false
			m.cursor = backupMenuImport
			if msg.fromToken {
				m.cursor = backupMenuOpenToken
			}
			m.statusMsg = fmt.Sprintf("✗ Import failed: %s", msg.err)
			return m, nil
		}
		m.backupItems = msg.items
		m.backupDone = 0
		// Count already-skipped items as "done" for progress.
		for _, item := range m.backupItems {
			if item.status == backupSkipped || item.status == backupFailed {
				m.backupDone++
			}
		}
		// Pause for user confirmation before installing.
		m.backupConfirm = true
		m.statusMsg = ""
		return m, nil

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
		// If cancelled or no more items, finish up.
		if m.backupCancelled {
			m.statusMsg = m.importSummary()
			cmd := m.startScan()
			return m, cmd
		}
		if cmd := m.nextBackupInstall(); cmd != nil {
			return m, cmd
		}
		// All done — refresh tools to pick up newly installed ones.
		m.statusMsg = m.importSummary()
		cmd := m.startScan()
		return m, cmd

	case batchUpgradeItemMsg:
		if msg.toolIdx >= len(m.tools) {
			return m.fireNextBatchUpgrade()
		}
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ %s upgrade failed: %s", m.tools[msg.toolIdx].DisplayName, msg.err)
		} else {
			m.statusMsg = fmt.Sprintf("✓ %s upgraded", m.tools[msg.toolIdx].DisplayName)
		}
		// Deselect the completed tool.
		delete(m.updateSelected, msg.toolIdx)
		// Fire the next upgrade, or finish with a full refresh.
		return m.fireNextBatchUpgrade()

	case shareFinishedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ Share failed: %s", msg.err)
			return m, nil
		}
		m.sharedToken = msg.token
		m.backupMode = backupModeShare
		m.tokenCopied = false
		// Auto-copy to clipboard.
		if err := m.clip.WriteAll(msg.token); err != nil {
			m.statusMsg = fmt.Sprintf("✓ Token generated (%d tools)", msg.count)
		} else {
			m.tokenCopied = true
			m.statusMsg = fmt.Sprintf("✓ Copied to clipboard! (%d tools)", msg.count)
		}
		return m, nil

	case packItemDoneMsg:
		if msg.idx < len(m.packItems) {
			if msg.err != nil {
				m.packItems[msg.idx].status = packItemFailed
				m.packItems[msg.idx].errMsg = msg.err.Error()
			} else {
				m.packItems[msg.idx].status = packItemDone
			}
			m.packDone++
		}
		// Fire the next item, or finish.
		if cmd := m.nextPackItem(); cmd != nil {
			return m, cmd
		}
		// All done — refresh tools to pick up changes.
		m.packInstalling = false
		cmd := m.startScan()
		m.statusMsg = "✓ Pack operation complete — refreshing..."
		return m, cmd

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	default:
		// Forward non-key messages to text inputs (for paste support).
		if m.importingPath {
			var cmd tea.Cmd
			m.importInput, cmd = m.importInput.Update(msg)
			return m, cmd
		}
		if m.enteringToken {
			var cmd tea.Cmd
			m.tokenInput, cmd = m.tokenInput.Update(msg)
			return m, cmd
		}
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
	return RunWithConfig(config.Default())
}

// RunWithConfig launches the TUI with the given configuration.
func RunWithConfig(cfg *config.Config) error {
	model := NewModelWithConfig(cfg)
	p := tea.NewProgram(model)
	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	return nil
}

// --- Key handling ---

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
func (m Model) nextSelectableIdx(idx int) int {
	for i := idx + 1; i < len(m.sidebarItems); i++ {
		if !m.sidebarItems[i].isHeader {
			return i
		}
	}
	return idx
}

// prevSelectableIdx returns the previous non-header index before idx, or idx if at start.
func (m Model) prevSelectableIdx(idx int) int {
	for i := idx - 1; i >= 0; i-- {
		if !m.sidebarItems[i].isHeader {
			return i
		}
	}
	return idx
}

// handleKeyDetail handles navigation in the tool detail/action menu view.
func (m Model) handleKeyDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "backspace":
		m.showDetail = false
		m.toolMenu = noMenu
		m.toolMenuItems = nil
		return m, nil
	case "up", "k":
		if m.toolMenu > 0 {
			m.toolMenu--
		}
	case "down", "j":
		if m.toolMenu < len(m.toolMenuItems)-1 {
			m.toolMenu++
		}
	case "enter":
		if m.toolMenu >= 0 && m.toolMenu < len(m.toolMenuItems) {
			action := m.toolMenuItems[m.toolMenu]
			m.toolMenu = noMenu
			m.toolMenuItems = nil
			m.startAction(action.picker)
		}
	}
	return m, nil
}

// handleKeyPackDetail handles navigation in the pack detail view.
func (m Model) handleKeyPackDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// While a pack operation is running, allow dismissing the view
	// but keep the operation running in the background.
	if m.packInstalling {
		if msg.String() == "esc" || msg.String() == "q" {
			m.showPackDetail = false
			// packItems and packInstalling remain — the queue continues
			// and packItemDoneMsg will fire the next item when it arrives.
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
		return m, nil
	case "enter", "i":
		// Install missing tools.
		if m.packDetailIdx >= 0 && m.packDetailIdx < len(m.packs) {
			pack := m.packs[m.packDetailIdx]
			m.packItems = buildPackInstallItems(m.tools, pack)
			m.packDone = countPackSkipped(m.packItems)
			m.packInstalling = true
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
func (m *Model) nextPackItem() tea.Cmd {
	for i := range m.packItems {
		if m.packItems[i].status == packItemPending {
			m.packItems[i].status = packItemRunning
			return execPackItemCmd(i, m.packItems[i].cmdArgs)
		}
	}
	return nil
}

func countPackSkipped(items []packItem) int {
	n := 0
	for _, item := range items {
		if item.status == packItemSkipped {
			n++
		}
	}
	return n
}

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
	default:
		var cmd tea.Cmd
		m.filterInput, cmd = m.filterInput.Update(msg)
		m.filterText = m.filterInput.Value()
		m.applyFilter()
		return m, cmd
	}
}

// handleKeyDefault handles keys when no modal is active — tabs, navigation, actions.
func (m Model) handleKeyDefault(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Clear transient status on keypress — but preserve error messages
	// when no tools are loaded (e.g. catalog fetch failure).
	if len(m.tools) > 0 {
		m.statusMsg = ""
	}

	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "c":
		// Copy share token to clipboard (Backup tab only).
		if m.activeTab == tabBackup && m.backupMode == backupModeShare && m.sharedToken != "" {
			if err := m.clip.WriteAll(m.sharedToken); err != nil {
				m.statusMsg = "⚠ Clipboard unavailable"
			} else {
				m.tokenCopied = true
				m.statusMsg = "✓ Copied to clipboard!"
			}
			return m, nil
		}
	case "esc":
		// Cancel running import — mark remaining as cancelled, let current finish.
		if m.activeTab == tabBackup && m.backupMode != backupModeIdle && m.isImportRunning() {
			m.cancelImport()
			return m, nil
		}
		// Reset completed backup back to idle.
		if m.activeTab == tabBackup && m.backupMode != backupModeIdle {
			m.backupMode = ""
			m.backupItems = nil
			m.backupDone = 0
			m.backupConfirm = false
			m.statusMsg = ""
			return m, nil
		}
		// Clear active filters.
		if m.categoryFilter != "" || m.tagFilter != "" || m.platformFilter != "" {
			m.categoryFilter = ""
			m.tagFilter = ""
			m.platformFilter = ""
			m.cursor = 0
			m.applyFilter()
			return m, nil
		}
	case "right", "tab":
		// On Marketplace tab, cycle sub-tabs before switching main tabs.
		if m.activeTab == tabDiscover && m.discoverSubTab < discoverSubTabCount-1 {
			m.discoverSubTab++
			m.cursor = 0
			return m, nil
		}
		m.activeTab = (m.activeTab + 1) % tabCount
		m.cursor = 0
		m.discoverSubTab = discoverTools
		m.applyFilter()
		return m, nil
	case "left", "shift+tab":
		if m.activeTab == tabDiscover && m.discoverSubTab > discoverTools {
			m.discoverSubTab--
			m.cursor = 0
			if m.discoverSubTab == discoverTools {
				m.applyFilter()
			}
			return m, nil
		}
		m.activeTab = (m.activeTab + tabCount - 1) % tabCount
		m.cursor = 0
		m.discoverSubTab = discoverTools
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
		m.activeTab = tabBackup
		m.cursor = 0
		return m, nil
	case "5":
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
	case "f":
		// Focus the filter sidebar on tool-list tabs.
		if m.activeTab <= tabDiscover && len(m.sidebarItems) > 0 {
			m.categoryPicker = true
			// Position cursor on first selectable item.
			m.sidebarIdx = 0
			for i, item := range m.sidebarItems {
				if !item.isHeader {
					m.sidebarIdx = i
					break
				}
			}
			return m, nil
		}
	case "space":
		// Toggle selection for batch upgrade (Updates tab only).
		if m.activeTab == tabUpdates && m.cursor < len(m.filteredIndex) {
			idx := m.filteredIndex[m.cursor]
			m.updateSelected[idx] = !m.updateSelected[idx]
			// Advance cursor.
			if m.cursor < m.rowCount()-1 {
				m.cursor++
			}
		}
		return m, nil
	case "a":
		// Select/deselect all on Updates tab.
		if m.activeTab == tabUpdates {
			anySelected := false
			for _, idx := range m.filteredIndex {
				if m.updateSelected[idx] {
					anySelected = true
					break
				}
			}
			for _, idx := range m.filteredIndex {
				m.updateSelected[idx] = !anySelected
			}
		}
		return m, nil
	case "u":
		// Batch upgrade selected tools (Updates tab only).
		if m.activeTab == tabUpdates && !m.batchUpdating {
			return m.startBatchUpgrade()
		}
		return m, nil
	case "enter":
		// On Backup tab (idle), execute the selected menu item.
		if m.activeTab == tabBackup && m.backupMode == backupModeIdle {
			switch m.cursor {
			case backupMenuExport:
				// Export installed tools.
				if m.phase < phaseDone {
					m.statusMsg = "Still scanning — please wait..."
					return m, nil
				}
				if len(m.tools) == 0 {
					m.statusMsg = "No tools found to export."
					return m, nil
				}
				m.backupItems = nil
				m.backupDone = 0
				for _, tool := range m.tools {
					if tool.IsInstalled() {
						m.backupItems = append(m.backupItems, backupItem{
							name:    tool.Name,
							display: tool.DisplayName,
							source:  "—",
							status:  backupPending,
						})
					}
				}
				m.backupMode = backupModeExport
				m.cursor = 0
				m.statusMsg = "Exporting..."
				return m, exportToolsCmd(m.tools)
			case backupMenuImport:
				// Import from manifest — enter path input mode.
				if m.phase < phaseDone {
					m.statusMsg = "Still scanning — please wait..."
					return m, nil
				}
				m.importingPath = true
				return m, m.importInput.Focus()
			case backupMenuShare:
				// Share — generate a share token.
				if m.phase < phaseDone {
					m.statusMsg = "Still scanning — please wait..."
					return m, nil
				}
				m.statusMsg = "Generating share token..."
				return m, shareToolsCmd(m.tools)
			case backupMenuOpenToken:
				// Open Token — enter token input mode.
				if m.phase < phaseDone {
					m.statusMsg = "Still scanning — please wait..."
					return m, nil
				}
				m.enteringToken = true
				return m, m.tokenInput.Focus()
			}
			return m, nil
		}
		// On Marketplace Packs sub-tab, open pack detail.
		if m.activeTab == tabDiscover && m.discoverSubTab == discoverPacks {
			if m.packInstalling {
				m.statusMsg = "Pack operation in progress — please wait..."
				return m, nil
			}
			if m.cursor < len(m.packs) {
				m.packDetailIdx = m.cursor
				m.showPackDetail = true
				m.packItems = nil // clear any leftover progress from a previous pack
				m.packDone = 0
			}
			return m, nil
		}
		// On Marketplace For You sub-tab, open the recommended tool's detail.
		if m.activeTab == tabDiscover && m.discoverSubTab == discoverForYou {
			if m.cursor < len(m.recommendations) {
				m.detailIdx = m.recommendations[m.cursor].toolIdx
				m.showDetail = true
				m.buildToolMenu()
				tool := m.tools[m.detailIdx]
				if !tool.InfoFetched {
					return m, fetchToolInfoCmd(m.svc, m.detailIdx, tool)
				}
			}
			return m, nil
		}
		if m.cursor < len(m.filteredIndex) {
			// Open tool detail view.
			m.detailIdx = m.filteredIndex[m.cursor]
			m.showDetail = true
			m.buildToolMenu()
			// Fetch tool info lazily if not already fetched.
			tool := m.tools[m.detailIdx]
			if !tool.InfoFetched {
				return m, fetchToolInfoCmd(m.svc, m.detailIdx, tool)
			}
		}
		return m, nil
	case "r":
		// On the Marketplace tab, refresh the catalog from GitHub.
		if m.activeTab == tabDiscover {
			m.statusMsg = "Refreshing marketplace..."
			var marketplaceURL string
			if m.cfg != nil {
				marketplaceURL = m.cfg.Marketplace.URL
			}
			fetcher := &catalog.GitHubFetcher{URL: marketplaceURL}
			return m, refreshMarketplaceCmd(fetcher)
		}
		// On other tabs, do a full rescan.
		cmd := m.startScan()
		return m, cmd
	}
	return m, nil
}

// --- Filtering ---

func (m *Model) applyFilter() {
	m.filteredIndex = nil
	filter := strings.ToLower(m.filterText)

	// First pass: collect tools matching the current tab (for contextual sidebar).
	var tabTools []registry.Tool
	for _, tool := range m.tools {
		if m.matchesTab(tool) {
			tabTools = append(tabTools, tool)
		}
	}

	// Rebuild sidebar items from tab-scoped tools only.
	m.categories = collectCategories(tabTools)
	m.tags = collectTags(tabTools)
	m.platforms = collectPlatforms(tabTools)
	m.sidebarItems = buildSidebarItems(m.categories, m.tags, m.platforms, tabTools)

	// Second pass: apply all filters (sidebar + text search).
	for i, tool := range m.tools {
		if !m.matchesTab(tool) {
			continue
		}
		// Structured category filter.
		if m.categoryFilter != "" && !strings.EqualFold(tool.Category, m.categoryFilter) {
			continue
		}
		// Tag filter.
		if m.tagFilter != "" && !hasTag(tool.Tags, m.tagFilter) {
			continue
		}
		// Platform filter.
		if m.platformFilter != "" && !hasPlatform(tool.Packages, m.platformFilter) {
			continue
		}
		if filter != "" &&
			!strings.Contains(strings.ToLower(tool.DisplayName), filter) &&
			!strings.Contains(strings.ToLower(tool.Name), filter) &&
			!strings.Contains(strings.ToLower(tool.Category), filter) &&
			!matchesTags(tool.Tags, filter) {
			continue
		}
		m.filteredIndex = append(m.filteredIndex, i)
	}
	if m.cursor >= len(m.filteredIndex) {
		m.cursor = max(0, len(m.filteredIndex)-1)
	}
}

// matchesTags reports whether any tag contains the filter substring.
func matchesTags(tags []string, filter string) bool {
	for _, tag := range tags {
		if strings.Contains(strings.ToLower(tag), filter) {
			return true
		}
	}
	return false
}

// hasTag reports whether the tool has an exact tag match (case-insensitive).
func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// hasPlatform reports whether the tool supports the given platform.
func hasPlatform(pkgs registry.PackageIDs, platform string) bool {
	for _, p := range derivePlatforms(pkgs) {
		if strings.EqualFold(p, platform) {
			return true
		}
	}
	return false
}

func (m *Model) matchesTab(tool registry.Tool) bool {
	switch m.activeTab {
	case tabInstalled:
		return tool.IsInstalled()
	case tabUpdates:
		return tool.HasUpdate()
	case tabDiscover:
		return !tool.IsInstalled()
	case tabBackup:
		return false // Backup tab renders from backupItems, not tools
	case tabConfig:
		return false // Config tab renders static content, not tools
	}
	return false
}

// collectCategories returns sorted unique category names from the tool list.
func collectCategories(tools []registry.Tool) []string {
	seen := make(map[string]struct{})
	for _, t := range tools {
		if t.Category != "" {
			seen[t.Category] = struct{}{}
		}
	}
	cats := make([]string, 0, len(seen))
	for cat := range seen {
		cats = append(cats, cat)
	}
	sort.Strings(cats)
	return cats
}

// collectTags returns sorted unique tag names from the tool list.
func collectTags(tools []registry.Tool) []string {
	seen := make(map[string]struct{})
	for _, t := range tools {
		for _, tag := range t.Tags {
			if tag != "" {
				seen[tag] = struct{}{}
			}
		}
	}
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}

// collectPlatforms returns sorted unique platform names inferred from all tools.
func collectPlatforms(tools []registry.Tool) []string {
	seen := make(map[string]struct{})
	for _, t := range tools {
		for _, p := range derivePlatforms(t.Packages) {
			seen[p] = struct{}{}
		}
	}
	platforms := make([]string, 0, len(seen))
	for p := range seen {
		platforms = append(platforms, p)
	}
	sort.Strings(platforms)
	return platforms
}

// buildSidebarItems constructs the flat sidebar item list from categories, tags, and platforms.
// tabTools are the tools visible on the current tab, used for counting.
func buildSidebarItems(categories, tags, platforms []string, tabTools []registry.Tool) []sidebarItem {
	var items []sidebarItem

	// Count tools per category.
	catCount := make(map[string]int, len(categories))
	for _, t := range tabTools {
		if t.Category != "" {
			catCount[t.Category]++
		}
	}

	// Count tools per tag.
	tagCount := make(map[string]int, len(tags))
	for _, t := range tabTools {
		for _, tag := range t.Tags {
			tagCount[tag]++
		}
	}

	// Count tools per platform.
	platCount := make(map[string]int, len(platforms))
	for _, t := range tabTools {
		for _, p := range derivePlatforms(t.Packages) {
			platCount[p]++
		}
	}

	totalCount := len(tabTools)

	// Category section.
	items = append(items,
		sidebarItem{label: "CATEGORY", isHeader: true},
		sidebarItem{label: fmt.Sprintf("All (%d)", totalCount), section: "category", value: ""},
	)
	for _, cat := range categories {
		items = append(items, sidebarItem{label: fmt.Sprintf("%s (%d)", cat, catCount[cat]), section: "category", value: cat})
	}

	// Platform section.
	if len(platforms) > 0 {
		items = append(items,
			sidebarItem{label: "PLATFORM", isHeader: true},
			sidebarItem{label: fmt.Sprintf("All (%d)", totalCount), section: "platform", value: ""},
		)
		for _, p := range platforms {
			items = append(items, sidebarItem{label: fmt.Sprintf("%s (%d)", p, platCount[p]), section: "platform", value: p})
		}
	}

	// Tag section.
	if len(tags) > 0 {
		items = append(items,
			sidebarItem{label: "TAG", isHeader: true},
			sidebarItem{label: fmt.Sprintf("All (%d)", totalCount), section: "tag", value: ""},
		)
		for _, tag := range tags {
			items = append(items, sidebarItem{label: fmt.Sprintf("%s (%d)", tag, tagCount[tag]), section: "tag", value: tag})
		}
	}

	return items
}

// --- Stats ---

func (m Model) stats() (installed, updates, notInstalled int) {
	for _, tool := range m.tools {
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
	// Multiple sources — show as menu items in the detail view.
	m.toolMenuItems = nil
	for _, c := range picker.choices {
		m.toolMenuItems = append(m.toolMenuItems, toolMenuAction{
			label: string(c.source),
			picker: &sourcePicker{
				toolIdx: picker.toolIdx,
				action:  picker.action,
				choices: []sourceChoice{c},
			},
		})
	}
	m.toolMenu = 0
}

// buildToolMenu resolves available actions for the current tool and populates the tool menu.
// Returns true if the menu has any actions, false otherwise.
func (m *Model) buildToolMenu() bool {
	tool := m.currentTool()
	if tool == nil {
		return false
	}
	m.toolMenuItems = nil
	idx := m.currentToolIdx()
	if tool.IsInstalled() {
		if p := m.resolveUpgradeAction(); p != nil {
			m.toolMenuItems = append(m.toolMenuItems, toolMenuAction{label: "Upgrade", picker: p})
		}
		if p := m.resolveRemoveAction(); p != nil {
			m.toolMenuItems = append(m.toolMenuItems, toolMenuAction{label: "Remove", picker: p})
		}
	} else {
		// Install — show each source as a separate menu item.
		if p := m.resolveInstallAction(); p != nil {
			if len(p.choices) == 1 {
				m.toolMenuItems = append(m.toolMenuItems, toolMenuAction{label: "Install", picker: p})
			} else {
				for _, c := range p.choices {
					m.toolMenuItems = append(m.toolMenuItems, toolMenuAction{
						label: "Install via " + string(c.source),
						picker: &sourcePicker{
							toolIdx: idx,
							action:  actionInstall,
							choices: []sourceChoice{c},
						},
					})
				}
			}
		}
	}
	if len(m.toolMenuItems) == 0 {
		return false
	}
	m.toolMenu = 0
	return true
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
		action:  actionInstall,
		choices: choices,
	}
}

// resolveUpgradeAction builds a source picker for upgrading the current tool.
// Only offers upgrade when the detected source is a known package manager.
// Manual installs cannot be upgraded through clim.
func (m Model) resolveUpgradeAction() *sourcePicker {
	tool := m.currentTool()
	if tool == nil || !tool.IsInstalled() {
		return nil
	}

	detected := tool.PrimaryInstance().Source
	if detected == registry.SourceManual {
		return nil
	}

	// Prefer detected source first.
	var choices []sourceChoice
	if args := tool.Packages.UpgradeArgs(detected); args != nil {
		choices = append(choices, sourceChoice{source: detected, cmdArgs: args})
	}

	// Then other available sources.
	for _, src := range registry.SourcesForOS() {
		if src == detected {
			continue
		}
		if args := tool.Packages.UpgradeArgs(src); args != nil {
			choices = append(choices, sourceChoice{source: src, cmdArgs: args})
		}
	}
	if len(choices) == 0 {
		return nil
	}
	return &sourcePicker{
		toolIdx: m.currentToolIdx(),
		action:  actionUpgrade,
		choices: choices,
	}
}

// resolveRemoveAction builds a source picker for removing the current tool.
// Only offers remove when the detected source is a known package manager.
// Manual installs cannot be removed through clim.
func (m Model) resolveRemoveAction() *sourcePicker {
	tool := m.currentTool()
	if tool == nil || !tool.IsInstalled() {
		return nil
	}

	detected := tool.PrimaryInstance().Source
	if detected == registry.SourceManual {
		return nil
	}

	// Prefer detected source first.
	var choices []sourceChoice
	if args := tool.Packages.RemoveArgs(detected); args != nil {
		choices = append(choices, sourceChoice{source: detected, cmdArgs: args})
	}

	// Then other available sources.
	for _, src := range registry.SourcesForOS() {
		if src == detected {
			continue
		}
		if args := tool.Packages.RemoveArgs(src); args != nil {
			choices = append(choices, sourceChoice{source: src, cmdArgs: args})
		}
	}
	if len(choices) == 0 {
		return nil
	}
	return &sourcePicker{
		toolIdx: m.currentToolIdx(),
		action:  actionRemove,
		choices: choices,
	}
}

// --- Backup ---

// rowCount returns the number of navigable rows for the current tab.
func (m Model) rowCount() int {
	switch m.activeTab {
	case tabDiscover:
		if m.discoverSubTab == discoverPacks {
			return len(m.packs)
		}
		if m.discoverSubTab == discoverForYou {
			return len(m.recommendations)
		}
		return len(m.filteredIndex)
	case tabBackup:
		if m.backupMode == backupModeIdle {
			return backupMenuCount
		}
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
			m.cursor = i // auto-scroll to installing item
			m.statusMsg = fmt.Sprintf("Installing %s...", itemLabel(m.backupItems[i].name, m.backupItems[i].display))
			return execBackupInstallCmd(i, m.backupItems[i].cmdArgs)
		}
	}
	return nil
}

// isImportRunning returns true if an import install is in progress.
func (m Model) isImportRunning() bool {
	for _, item := range m.backupItems {
		if item.status == backupRunning {
			return true
		}
	}
	return false
}

// cancelImport marks all remaining pending items as skipped/cancelled.
func (m *Model) cancelImport() {
	m.backupCancelled = true
	for i := range m.backupItems {
		if m.backupItems[i].status == backupPending {
			m.backupItems[i].status = backupSkipped
			m.backupItems[i].errMsg = "cancelled"
		}
	}
	m.statusMsg = "⚠ Import cancelled — waiting for current install to finish..."
}

// importSummary builds a status string with installed/failed/skipped counts.
func (m Model) importSummary() string {
	installed, failed, skipped := 0, 0, 0
	for _, item := range m.backupItems {
		switch item.status {
		case backupDone:
			installed++
		case backupFailed:
			failed++
		case backupSkipped:
			skipped++
		}
	}
	parts := []string{fmt.Sprintf("%d installed", installed)}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	prefix := "✓"
	if failed > 0 {
		prefix = "⚠"
	}
	return fmt.Sprintf("%s Import complete — %s", prefix, strings.Join(parts, ", "))
}

// --- Batch upgrade (Updates tab) ---

// batchUpgradeQueue holds the ordered list of tool indices to upgrade.
// Items are shifted off the front as each upgrade completes.
type batchUpgradeQueue []int

// startBatchUpgrade kicks off sequential upgrades for all selected tools.
func (m Model) startBatchUpgrade() (tea.Model, tea.Cmd) {
	// Build the queue of selected tool indices that have upgrade args.
	var queue batchUpgradeQueue
	for _, idx := range m.filteredIndex {
		if !m.updateSelected[idx] {
			continue
		}
		tool := &m.tools[idx]
		if !tool.HasUpdate() {
			continue
		}
		// Find upgrade args (best available source).
		for _, src := range registry.SourcesForOS() {
			if args := tool.Packages.UpgradeArgs(src); args != nil {
				queue = append(queue, idx)
				break
			}
		}
	}
	if len(queue) == 0 {
		m.statusMsg = "No upgradable tools selected."
		return m, nil
	}
	m.batchUpdating = true
	m.batchQueue = queue
	return m.fireNextBatchUpgrade()
}

// fireNextBatchUpgrade pops the next tool off the queue and fires its upgrade.
// Returns (model, nil) when the queue is empty.
func (m Model) fireNextBatchUpgrade() (tea.Model, tea.Cmd) {
	if len(m.batchQueue) == 0 {
		cmd := m.startScan()
		m.statusMsg = "✓ Batch upgrade complete — refreshing..."
		return m, cmd
	}
	idx := m.batchQueue[0]
	m.batchQueue = m.batchQueue[1:]
	tool := &m.tools[idx]
	// Find best upgrade args.
	for _, src := range registry.SourcesForOS() {
		if args := tool.Packages.UpgradeArgs(src); args != nil {
			m.statusMsg = fmt.Sprintf("Upgrading %s...", tool.DisplayName)
			return m, execBatchUpgradeCmd(idx, args)
		}
	}
	// No upgrade args — skip and try next.
	return m.fireNextBatchUpgrade()
}
