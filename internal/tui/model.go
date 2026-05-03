package tui

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"log/slog"
	"path/filepath"

	"charm.land/bubbles/v2/progress"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/nassiharel/clim/internal/audit"
	"github.com/nassiharel/clim/internal/catalog"
	"github.com/nassiharel/clim/internal/compliance"
	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/custompacks"
	"github.com/nassiharel/clim/internal/doctor"
	"github.com/nassiharel/clim/internal/favorites"
	"github.com/nassiharel/clim/internal/onboard"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/score"
	"github.com/nassiharel/clim/internal/service"
	"github.com/nassiharel/clim/internal/teamfile"
)

const (
	tabInstalled = iota
	tabFavorites
	tabUpdates
	tabDiscover
	tabBackup
	tabProject
	tabDashboard
	tabConfig
	tabDoctor
	tabCount // total number of tabs, used for modular cycling
)

// Marketplace sub-tabs.
const (
	discoverTools       = 0
	discoverPacks       = 1
	discoverForYou      = 2
	discoverOnboard     = 3
	discoverSubTabCount = 4
)

// Scan phases — the loading lifecycle.
const (
	phaseLoading   = 0 // loading catalog + scanning PATH
	phaseResolving = 1 // version resolution in progress
	phaseDone      = 2 // all tools resolved
)

// Backup tab menu indices.
const (
	backupMenuExport     = 0
	backupMenuImport     = 1
	backupMenuShare      = 2
	backupMenuOpenToken  = 3
	backupMenuCreatePack = 4
	backupMenuMyPacks    = 5
	backupMenuMyBackups  = 6
	backupMenuCount      = 7
)

// Sentinel values.
const (
	noDetail = -1 // detailIdx when no detail view is open
	noMenu   = -1 // toolMenu when no action menu is shown
)

// Sort modes for tool list tabs.
const (
	sortByName    = 0
	sortByStars   = 1
	sortModeCount = 2
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
	sortMode  int // sortByName or sortByStars

	// Config warnings (unknown fields, invalid values).
	configWarnings []string

	// Filter.
	filterInput    textinput.Model
	filtering      bool
	filterText     string
	filteredIndex  []int
	categoryFilter string   // "" = all categories; non-empty = only this category
	tagFilter      string   // "" = all tags; non-empty = only tools with this tag
	platformFilter string   // "" = all platforms; non-empty = only this platform
	statusFilter   string   // "" = all; "installed" = installed; "available" = not installed
	categories     []string // sorted unique categories, computed once after scan
	tags           []string // sorted unique tags, computed once after scan
	platforms      []string // sorted unique platforms, computed once after scan

	// Sidebar filter panel.
	categoryPicker bool
	sidebarIdx     int
	sidebarItems   []sidebarItem

	// Marketplace sub-tabs (Tools / Packs / For You).
	discoverSubTab  int // 0=tools, 1=packs, 2=forYou
	packSortMode    int // 0=status (default), 1=name
	packs           []registry.Pack
	recommendations []recommendation // tag-based, computed after scan
	showPackDetail  bool
	packDetailIdx   int // index into m.packs
	packToolCursor  int // cursor within pack tool list (for navigating to tool detail)

	// Pack install/remove state (inline in pack detail view).
	packItems      []packItem // per-tool status during pack install/remove
	packInstalling bool       // true while a pack operation is in progress
	packDone       int        // count of completed items
	packCancelled  bool       // true if user cancelled pack operation
	packAction     string     // "Installing" or "Removing" — for progress display

	// Marketplace refresh diff — carried across rescans to apply badges.
	lastDiff *catalog.DiffResult

	// Detail view.
	detailIdx       int // index into m.tools, -1 = no detail
	showDetail      bool
	detailScroll    int              // vertical scroll offset (in rendered lines) for detail body
	detailMaxScroll int              // last computed max scroll (set by view, used by key handler)
	detailRelated   []recommendation // cached related tools for current detail view
	detailRelCursor int              // cursor in "You might also like" list (-1 = not focused)

	// Loading state.
	phase   int // 0=scanning, 1=resolving, 2=done
	loading bool
	pending int  // count of tools still resolving versions
	scanGen int  // incremented on each scan; used to discard stale toolVersionMsg
	scanOK  bool // true when the current scan completed without errors; gates cache writes

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
	updateSelected map[int]bool // tool index → selected for batch upgrade

	// Active batch upgrade operation (Updates tab).
	activeBatch *batchOp // nil = no active batch operation

	// Pack creation state (Backup tab → Create Pack).
	creatingPack       bool            // true = pack creation wizard is active
	packCreatePhase    int             // 0=name, 1=display_name, 2=description, 3=select tools, 4=output choice, 5=done
	packCreateName     textinput.Model // name input
	packCreateDispName textinput.Model // display_name input
	packCreateDesc     textinput.Model // description input
	packCreateSelected map[int]bool    // tool index → selected for pack
	packCreateCursor   int             // cursor for tool selection list
	packCreateFilter   string          // search filter for tool selection
	packCreateFiltered []int           // filtered tool indices

	// My Packs state (Backup tab → My Packs).
	customPacks         []registry.Pack // loaded from custom-packs.yaml
	viewingMyPacks      bool            // true = My Packs list is active
	myPacksCursor       int
	viewingMyPackDetail bool   // true = detail view for a custom pack
	myPackDetailIdx     int    // index into customPacks
	myPackMenuCursor    int    // cursor in detail action menu
	myPackToken         string // generated share token (for display in detail view)

	// My Backups state (Backup tab → My Backups).
	viewingMyBackups bool
	myBackupsCursor  int
	myBackupFiles    []backupFileInfo

	// Dashboard scroll offset.
	dashboardScroll int

	// Favorites state.
	favoriteNames   map[string]bool // in-memory lookup set, loaded at init
	favMode         string          // "" (list), "export", "share"
	favClearConfirm bool            // true = awaiting y/n to clear all favorites

	// Config editor state.
	configCursor    int
	configEditing   bool            // true = text input active for a setting
	configEditInput textinput.Model // text input for string/int/duration settings
	configScroll    int             // scroll offset for config tab

	// Doctor tab state (includes audit + compliance findings).
	doctorIssues     []doctor.Issue     // diagnostic results (nil = not yet checked)
	auditFindings    []audit.Finding    // security audit findings
	auditLicenses    map[string]int     // license counts
	complianceResult *compliance.Result // compliance check result (nil = no policy)
	complianceError  string             // non-empty when policy failed to load
	cachedScore      score.Result       // computed once in runDoctor
	doctorScroll     int                // scroll offset for doctor tab
	doctorChecked    bool               // true after first doctor check completed
	doctorSubTab     int                // 0=doctor, 1=audit, 2=compliance

	// Onboard state (Discover → Onboard sub-tab).
	onboardRole  int              // selected role index (0 = first role)
	onboardTools []recommendation // role-scored tools for current role

	// Team file (.clim.yaml) state.
	teamFilePath    string                 // path to detected .clim.yaml ("" = not found)
	teamFile        *teamfile.TeamFile     // parsed team file (nil = not found)
	teamCheckResult []teamfile.CheckResult // check results (nil = not checked yet)

	// Project tab state.
	projectView          int // projectViewList, projectViewDetail, projectViewAddTool
	projectCursor        int
	projectInitResult    *teamfile.DetectResult
	projectScroll        int
	projectEntries       []teamfile.ProjectEntry
	projectsLoaded       bool // true after first load
	projectAddCursor     int
	projectAddFilter     string
	projectAddFiltered   []int
	projectAddOptional   bool   // true = adding to optional, false = required
	projectConfirmReinit bool   // true = showing detection results, waiting for confirm
	projectReinitDir     string // directory for pending reinit

	// Generate confirmation state.
	projectGenConfirm bool   // true = file exists, waiting for y/n
	projectGenFormat  string // pending format
	projectGenPath    string // pending output path
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

	pcName := textinput.New()
	pcName.Placeholder = "my-custom-pack"
	pcName.CharLimit = 60
	pcName.SetWidth(40)

	pcDisp := textinput.New()
	pcDisp.Placeholder = "(defaults to name)"
	pcDisp.CharLimit = 80
	pcDisp.SetWidth(40)

	pcDesc := textinput.New()
	pcDesc.Placeholder = "A brief description of your pack"
	pcDesc.CharLimit = 200
	pcDesc.SetWidth(60)

	cfgEdit := textinput.New()
	cfgEdit.Placeholder = "enter value"
	cfgEdit.CharLimit = 200
	cfgEdit.SetWidth(40)

	return Model{
		svc:                service.New(),
		clip:               systemClipboard{},
		spinner:            s,
		filterInput:        ti,
		importInput:        ii,
		tokenInput:         ti2,
		packCreateName:     pcName,
		packCreateDispName: pcDisp,
		packCreateDesc:     pcDesc,
		packCreateSelected: make(map[int]bool),
		configEditInput:    cfgEdit,
		backupBar:          progress.New(progress.WithWidth(40)),
		updateSelected:     make(map[int]bool),
		favoriteNames:      loadFavoriteNames(),
		loading:            true,
		phase:              phaseLoading,
		activeTab:          tabInstalled,
		detailIdx:          noDetail,
		toolMenu:           noMenu,
		width:              80,
		height:             24,
	}
}

// NewModelWithConfig creates a new TUI model configured from the given Config.
func NewModelWithConfig(cfg *config.Config, warnings []string) Model {
	m := NewModel()
	m.svc = service.NewWithConfig(cfg)
	m.activeTab = tabFromName(cfg.UI.DefaultTab)
	m.cfg = cfg
	m.configWarnings = warnings
	return m
}

// tabFromName maps a config tab name string to the tab constant.
func tabFromName(name string) int {
	switch name {
	case "installed":
		return tabInstalled
	case "favorites":
		return tabFavorites
	case "updates":
		return tabUpdates
	case "marketplace":
		return tabDiscover
	case "backup":
		return tabBackup
	case "project":
		return tabProject
	case "dashboard":
		return tabDashboard
	case "config":
		return tabConfig
	case "doctor":
		return tabDoctor
	default:
		return tabInstalled
	}
}

// Init starts the initial tool discovery process.
func (m Model) Init() tea.Cmd {
	gen := m.scanGen
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg { return findToolsCmd(m.svc, false, gen)() },
	)
}

// startScan prepares the model for a new scan, invalidating any in-flight
// version resolution from a previous scan so stale toolVersionMsg messages
// are discarded immediately. Every code path that fires findToolsCmd must
// call this first. startScan always performs a fresh scan (no cache) since
// it's called after mutating actions and user-triggered rescans.
func (m *Model) startScan() tea.Cmd {
	// Bump the generation up front so any in-flight results from a prior
	// scan (scanResultMsg or toolVersionMsg) are discarded on arrival.
	m.scanGen++
	gen := m.scanGen

	m.loading = true
	m.phase = phaseLoading
	m.tools = nil
	m.filteredIndex = nil
	m.cursor = 0
	m.pending = 0
	m.scanOK = false
	// Clear upgrade selection — indices are tied to the old tool ordering
	// and would select the wrong tools after a rescan reorders the list.
	m.updateSelected = make(map[int]bool)
	if m.activeBatch == nil || !m.activeBatch.isRunning() {
		m.activeBatch = nil
	}
	m.doctorIssues = nil
	m.auditFindings = nil
	m.auditLicenses = nil
	m.complianceResult = nil
	m.complianceError = ""
	m.doctorChecked = false
	m.doctorScroll = 0
	m.doctorSubTab = doctorSubDoctor
	m.onboardTools = nil // indices tied to old tools slice
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg { return findToolsCmd(m.svc, true, gen)() },
	)
}

// Update handles all incoming messages and returns updated model and commands.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case scanResultMsg:
		// Discard stale scan results from an earlier scan generation. Without
		// this, pressing "r" rapidly (or receiving an import-triggered rescan
		// while an earlier one is still in flight) could overwrite the
		// model with stale tools from the first scan.
		if msg.gen != m.scanGen {
			return m, nil
		}
		m.tools = msg.tools
		registry.SortByName(m.tools)
		// Only writes from a clean scan are safe to persist — a partial
		// scan (PATH walk failed mid-flight) can't be distinguished from a
		// full one on load, and caching it would poison future runs.
		m.scanOK = msg.err == nil

		// Set status based on how the catalog was loaded.
		switch {
		case msg.err != nil:
			m.statusMsg = fmt.Sprintf("⚠ %v", msg.err)
		case msg.cacheWarning != "":
			// Non-fatal: cache was invalidated but the fresh scan succeeded.
			m.statusMsg = "⚠ " + msg.cacheWarning
		case msg.scanInfo != nil && msg.scanInfo.Source == service.ScanSourceCache:
			// Cache hit: scan results came from disk, no subprocess calls ran.
			ageStr := humaniseCacheAge(msg.scanInfo.CacheAt)
			if ageStr != "" {
				m.statusMsg = fmt.Sprintf("✓ Loaded %d tools from cache (scanned %s). Press 'r' to rescan.", len(m.tools), ageStr)
			} else {
				m.statusMsg = fmt.Sprintf("✓ Loaded %d tools from cache. Press 'r' to rescan.", len(m.tools))
			}
		default:
			if info := msg.catalogInfo; info != nil {
				switch info.Source {
				case catalog.SourceCache:
					m.statusMsg = fmt.Sprintf("✓ Loaded %d tools from cache", info.Tools)
				case catalog.SourceRemote:
					m.statusMsg = fmt.Sprintf("✓ Fetched catalog (%d tools)", info.Tools)
				}
				// Adopt any diff produced by an auto-refresh so badges + status
				// reflect what changed in the remote catalog.
				if info.Diff != nil && info.Diff.HasChanges() {
					d := *info.Diff
					m.lastDiff = &d
					m.statusMsg = fmt.Sprintf("✓ Marketplace updated: %d new, %d changed, %d removed",
						len(d.NewTools), len(d.ChangedTools), len(d.RemovedTools))
				}
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
		fromCache := msg.scanInfo != nil && msg.scanInfo.Source == service.ScanSourceCache

		// Reset filters — applyFilter() will rebuild sidebar items contextually.
		m.categoryFilter = ""
		m.tagFilter = ""
		m.platformFilter = ""
		m.statusFilter = ""

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

		// Load custom packs.
		if cp, err := custompacks.Load(); err != nil {
			slog.Warn("failed to load custom packs", "error", err)
		} else {
			m.customPacks = cp
		}

		// Cache backup file list for dashboard.
		m.myBackupFiles = scanBackupsDir()

		// Clear stale teamfile state — will be checked after version resolution.
		m.teamFilePath = ""
		m.teamFile = nil
		m.teamCheckResult = nil

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

		// Recompute onboard recommendations if on that sub-tab.
		if m.discoverSubTab == discoverOnboard {
			m.recomputeOnboardTools()
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

		// Cache hit: versions are already resolved, skip the per-tool
		// subprocess queries entirely. Auto-select tools with available
		// updates for batch upgrade, matching the resolve path.
		if fromCache {
			for i := range m.tools {
				if m.tools[i].HasUpdate() {
					m.updateSelected[i] = true
				}
			}
			m.phase = phaseDone
			m.loading = false
			m.detectTeamFile()
			m.runDoctor(doctor.ScanMeta{FromCache: true})
			return m, nil
		}

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
			// Persist cache even when nothing was installed so repeat runs
			// skip the PATH scan too — but only when the scan succeeded.
			if m.scanOK {
				_ = m.svc.SaveScanCache(m.tools)
			}
			m.detectTeamFile()
			m.runDoctor()
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
			// Persist fully-resolved scan so the next launch is instant.
			// Gated on scanOK: a partial PATH scan (msg.err on the initial
			// scanResultMsg) must not be cached, since an incomplete cache
			// would make future runs falsely treat the app as "up to date".
			if m.scanOK {
				_ = m.svc.SaveScanCache(m.tools)
			}
			// Check .clim.yaml now that versions are resolved.
			m.detectTeamFile()
			m.runDoctor()
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
		// Persist the updated state so future runs see the refreshed tool
		// — but only when the original scan produced a complete tools list.
		if m.scanOK {
			_ = m.svc.SaveScanCache(m.tools)
		}
		// Rebuild the tool menu if the detail view is still showing.
		if m.showDetail {
			m.buildToolMenu()
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
			m.statusMsg = fmt.Sprintf("✓ Marketplace updated: %d new, %d changed, %d removed",
				len(diff.NewTools), len(diff.ChangedTools), len(diff.RemovedTools))
			return m, cmd
		}
		m.statusMsg = "✓ Marketplace is up to date"
		return m, nil

	case exportFinishedMsg:
		// Favorites tab export — simple status, no progress animation.
		if m.activeTab == tabFavorites {
			if msg.err != nil {
				m.statusMsg = fmt.Sprintf("✗ Export failed: %s", msg.err)
			} else {
				m.statusMsg = fmt.Sprintf("✓ Exported %d favorites to %s", msg.count, msg.path)
			}
			m.favMode = ""
			return m, nil
		}
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
			// Only update if still running (not already skipped by user).
			if m.backupItems[msg.idx].status == backupRunning {
				if msg.err != nil {
					m.backupItems[msg.idx].status = backupFailed
					m.backupItems[msg.idx].errMsg = msg.err.Error()
				} else {
					m.backupItems[msg.idx].status = backupDone
				}
				m.backupDone++
			}
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

	case batchOpDoneMsg:
		if m.activeBatch != nil {
			m.activeBatch.complete(msg.idx, msg.err)
			m.statusMsg = m.activeBatch.statusLine()
			// If cancelled or no items left, finish immediately (no delay window).
			if m.activeBatch.cancelled || !m.activeBatch.hasPending() {
				m.activeBatch.finish()
				m.statusMsg = m.activeBatch.summary() + " — refreshing..."
				cmd := m.startScan()
				return m, cmd
			}
			// Defer starting the next item so the TUI can render
			// progress and accept skip/cancel input between items.
			return m, batchAdvanceCmd()
		}
		return m, nil

	case batchAdvanceMsg:
		if m.activeBatch != nil && m.activeBatch.isRunning() {
			if m.activeBatch.cancelled {
				m.activeBatch.finish()
				m.statusMsg = m.activeBatch.summary() + " — refreshing..."
				cmd := m.startScan()
				return m, cmd
			}
			if cmd := m.activeBatch.next(); cmd != nil {
				m.statusMsg = m.activeBatch.statusLine()
				return m, cmd
			}
			// All done.
			m.activeBatch.finish()
			m.statusMsg = m.activeBatch.summary() + " — refreshing..."
			cmd := m.startScan()
			return m, cmd
		}
		return m, nil

	case shareFinishedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ Share failed: %s", msg.err)
			if m.activeTab == tabFavorites {
				m.favMode = ""
			}
			return m, nil
		}
		m.sharedToken = msg.token
		m.tokenCopied = false
		if m.activeTab != tabFavorites {
			m.backupMode = backupModeShare
		}
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
			// Only update if not already marked (e.g., by skip/cancel).
			if m.packItems[msg.idx].status == packItemRunning {
				if msg.err != nil {
					m.packItems[msg.idx].status = packItemFailed
					m.packItems[msg.idx].errMsg = msg.err.Error()
				} else {
					m.packItems[msg.idx].status = packItemDone
				}
				m.packDone++
			}
		}
		// If no pending items left, finish immediately (no delay window).
		hasPending := false
		for _, item := range m.packItems {
			if item.status == packItemPending {
				hasPending = true
				break
			}
		}
		if !hasPending {
			m.packInstalling = false
			cmd := m.startScan()
			m.statusMsg = m.packSummary() + " — refreshing..."
			return m, cmd
		}
		// Defer starting the next item so the TUI can render and accept input.
		return m, packAdvanceCmd()

	case packAdvanceMsg:
		if m.packInstalling {
			if cmd := m.nextPackItem(); cmd != nil {
				return m, cmd
			}
			m.packInstalling = false
			cmd := m.startScan()
			m.statusMsg = m.packSummary() + " — refreshing..."
			return m, cmd
		}
		return m, nil

	case packSavedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ Save failed: %s", msg.err)
			return m, nil
		}
		// Reload custom packs and navigate to My Packs detail view.
		if cp, err := custompacks.Load(); err == nil {
			m.customPacks = cp
		}
		m.resetPackCreate()
		m.viewingMyPacks = true
		m.viewingMyPackDetail = true
		m.myPackMenuCursor = 0
		// Find the saved pack by name.
		m.myPackDetailIdx = 0
		for i, p := range m.customPacks {
			if p.Name == msg.name {
				m.myPackDetailIdx = i
				m.myPacksCursor = i
				break
			}
		}
		m.statusMsg = fmt.Sprintf("✓ Pack '%s' saved", msg.name)
		return m, nil

	case myPackActionMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ %s failed: %s", msg.action, msg.err)
			return m, nil
		}
		if msg.token != "" {
			// Share action — store token for display, copy to clipboard.
			m.myPackToken = msg.token
			if err := m.clip.WriteAll(msg.token); err == nil {
				m.statusMsg = "✓ Copied to clipboard!"
			} else {
				m.statusMsg = msg.result
			}
		} else {
			m.statusMsg = "✓ " + msg.result
		}
		return m, nil

	case myPackDeletedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ Delete failed: %s", msg.err)
			return m, nil
		}
		m.statusMsg = fmt.Sprintf("✓ Pack '%s' deleted", msg.name)
		m.viewingMyPackDetail = false
		// Reload custom packs.
		if cp, err := custompacks.Load(); err == nil {
			m.customPacks = cp
		}
		if m.myPacksCursor >= len(m.customPacks) {
			m.myPacksCursor = max(0, len(m.customPacks)-1)
		}
		return m, nil

	case backupDeletedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ Delete failed: %s", msg.err)
			return m, nil
		}
		m.statusMsg = fmt.Sprintf("✓ Backup '%s' deleted", msg.name)
		m.myBackupFiles = scanBackupsDir()
		if m.myBackupsCursor >= len(m.myBackupFiles) {
			m.myBackupsCursor = max(0, len(m.myBackupFiles)-1)
		}
		return m, nil

	case configSavedMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ Save failed: %s", msg.err)
		} else {
			m.statusMsg = "✓ Config saved"
		}
		return m, nil

	case projectCheckMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ %s", msg.err)
			// If re-check after editor, refresh teamfile state.
			m.detectTeamFile()
			return m, nil
		}
		m.teamFile = msg.tf
		m.teamFilePath = msg.path
		m.teamCheckResult = msg.results
		m.projectCursor = 0
		m.statusMsg = "✓ Check complete"
		return m, nil

	case projectInitMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ Detection failed: %s", msg.err)
			return m, nil
		}
		m.projectInitResult = msg.result
		if len(msg.result.Tools) == 0 {
			m.statusMsg = "No project files detected."
			return m, nil
		}
		// Show detection results and ask for confirmation.
		m.projectConfirmReinit = true
		m.statusMsg = fmt.Sprintf("Detected %d tools — Enter to confirm, Esc to cancel", len(msg.result.Tools))
		return m, nil

	case projectInitDoneMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ Init failed: %s", msg.err)
			return m, nil
		}
		m.statusMsg = fmt.Sprintf("✓ Generated .clim.yaml (%d tools)", msg.tools)
		// Check the newly written file directly (not CWD-based detection).
		m.projectView = projectViewDetail
		m.projectCursor = 0
		return m, projectCheckCmd(msg.path, m.tools)

	case projectListLoadedMsg:
		m.projectEntries = msg.entries
		m.projectsLoaded = true
		// Sort: current CWD project first.
		cwd, _ := os.Getwd()
		cwdAbs, _ := filepath.Abs(cwd)
		sort.SliceStable(m.projectEntries, func(i, j int) bool {
			iCurrent := m.projectEntries[i].Path == cwdAbs
			jCurrent := m.projectEntries[j].Path == cwdAbs
			if iCurrent != jCurrent {
				return iCurrent
			}
			return m.projectEntries[i].Name < m.projectEntries[j].Name
		})
		return m, nil

	case projectEditorDoneMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ Editor: %s", msg.err)
			return m, nil
		}
		// Re-check after editor closes.
		if msg.path != "" {
			m.statusMsg = "Re-checking..."
			return m, projectCheckCmd(msg.path, m.tools)
		}
		m.detectTeamFile()
		return m, nil

	case projectAddToolMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ %s", msg.err)
			return m, nil
		}
		label := "required"
		if msg.optional {
			label = "optional"
		}
		m.statusMsg = fmt.Sprintf("✓ Added %s to %s tools", msg.toolName, label)
		// Re-check to reflect changes.
		if m.teamFilePath != "" {
			return m, projectCheckCmd(m.teamFilePath, m.tools)
		}
		return m, nil

	case projectGenerateMsg:
		if msg.err != nil {
			m.statusMsg = fmt.Sprintf("✗ Generate failed: %s", msg.err)
			return m, nil
		}
		m.statusMsg = fmt.Sprintf("✓ Generated %s → %s (%d tools)", msg.format, msg.path, msg.tools)
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Clamp scroll offsets on resize.
		if m.dashboardScroll > 0 {
			m.dashboardScroll = max(0, min(m.dashboardScroll, m.height))
		}
		if m.configScroll > 0 {
			m.configScroll = max(0, min(m.configScroll, m.height))
		}
		if m.doctorScroll > 0 {
			m.doctorScroll = max(0, min(m.doctorScroll, m.height))
		}
		if m.showDetail {
			m.computeDetailMaxScroll()
			m.clampDetailScroll()
		}
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
		if m.creatingPack {
			switch m.packCreatePhase {
			case packCreatePhaseName:
				var cmd tea.Cmd
				m.packCreateName, cmd = m.packCreateName.Update(msg)
				return m, cmd
			case packCreatePhaseDispName:
				var cmd tea.Cmd
				m.packCreateDispName, cmd = m.packCreateDispName.Update(msg)
				return m, cmd
			case packCreatePhaseDesc:
				var cmd tea.Cmd
				m.packCreateDesc, cmd = m.packCreateDesc.Update(msg)
				return m, cmd
			}
		}
		if m.configEditing {
			var cmd tea.Cmd
			m.configEditInput, cmd = m.configEditInput.Update(msg)
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
	return RunWithConfig(config.Default(), nil)
}

// RunWithConfig launches the TUI with the given configuration.
func RunWithConfig(cfg *config.Config, warnings []string) error {
	model := NewModelWithConfig(cfg, warnings)
	p := tea.NewProgram(model)
	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	return nil
}

// --- Key handling ---

// prevSelectableIdx moved to keys.go.
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

// packSummary returns a completion summary for the current pack operation.
func (m Model) packSummary() string {
	var succeeded, failed, skipped int
	for _, item := range m.packItems {
		switch item.status {
		case packItemDone:
			succeeded++
		case packItemFailed:
			failed++
		case packItemSkipped:
			skipped++
		}
	}
	var parts []string
	if succeeded > 0 {
		parts = append(parts, fmt.Sprintf("%d succeeded", succeeded))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", failed))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	prefix := "✓"
	if m.packCancelled {
		prefix = "⚠ Cancelled —"
	} else if failed > 0 {
		prefix = "⚠"
	}
	if len(parts) == 0 {
		return prefix + " Nothing to do"
	}
	return prefix + " " + strings.Join(parts, ", ")
}

// handleKeyFilter moved to keys.go.
func (m Model) handleKeyDefault(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Clear transient status on keypress — but preserve error messages
	// when no tools are loaded (e.g. catalog fetch failure).
	if len(m.tools) > 0 {
		m.statusMsg = ""
	}

	// Project tab has its own key handler.
	if m.activeTab == tabProject {
		return m.handleKeyProject(msg)
	}

	// Dashboard and Doctor tabs use static/scroll-only key handling.
	if m.activeTab == tabDashboard || m.activeTab == tabDoctor {
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			if m.activeTab == tabDashboard && m.dashboardScroll > 0 {
				m.dashboardScroll--
			}
			if m.activeTab == tabDoctor && m.doctorScroll > 0 {
				m.doctorScroll--
			}
			return m, nil
		case "down", "j":
			if m.activeTab == tabDashboard {
				m.dashboardScroll++
			}
			if m.activeTab == tabDoctor {
				m.doctorScroll++
			}
			return m, nil
		case "home", "g":
			if m.activeTab == tabDashboard {
				m.dashboardScroll = 0
			}
			if m.activeTab == tabDoctor {
				m.doctorScroll = 0
			}
			return m, nil
		case "right", "tab":
			// On Doctor tab, cycle sub-tabs before switching main tabs.
			if m.activeTab == tabDoctor && m.doctorSubTab < doctorSubCompliance {
				m.doctorSubTab++
				m.doctorScroll = 0
				return m, nil
			}
			m.activeTab = (m.activeTab + 1) % tabCount
			m.cursor = 0
			m.dashboardScroll = 0
			m.doctorScroll = 0
			m.doctorSubTab = doctorSubDoctor
			m.discoverSubTab = discoverTools
			m.applyFilter()
			if m.activeTab == tabProject {
				return m, projectLoadListCmd(m.tools)
			}
			return m, nil
		case "left", "shift+tab":
			if m.activeTab == tabDoctor && m.doctorSubTab > doctorSubDoctor {
				m.doctorSubTab--
				m.doctorScroll = 0
				return m, nil
			}
			m.activeTab = (m.activeTab + tabCount - 1) % tabCount
			m.cursor = 0
			m.dashboardScroll = 0
			m.doctorScroll = 0
			m.doctorSubTab = doctorSubDoctor
			m.discoverSubTab = discoverTools
			m.applyFilter()
			if m.activeTab == tabProject {
				return m, projectLoadListCmd(m.tools)
			}
			return m, nil
		case "1":
			m.activeTab = tabInstalled
			m.cursor = 0
			m.applyFilter()
			return m, nil
		case "2":
			m.activeTab = tabFavorites
			m.cursor = 0
			m.applyFilter()
			return m, nil
		case "3":
			m.activeTab = tabUpdates
			m.cursor = 0
			m.applyFilter()
			return m, nil
		case "4":
			m.activeTab = tabDiscover
			m.cursor = 0
			m.applyFilter()
			return m, nil
		case "5":
			m.activeTab = tabBackup
			m.cursor = 0
			return m, nil
		case "6":
			m.activeTab = tabProject
			m.cursor = 0
			m.projectCursor = 0
			m.projectScroll = 0
			m.projectView = projectViewList
			return m, projectLoadListCmd(m.tools)
		case "7":
			m.activeTab = tabDashboard
			m.cursor = 0
			m.dashboardScroll = 0
			m.myBackupFiles = scanBackupsDir()
			return m, nil
		case "8":
			m.activeTab = tabConfig
			m.cursor = 0
			m.configScroll = 0
			return m, nil
		case "9":
			m.activeTab = tabDoctor
			m.cursor = 0
			m.doctorScroll = 0
			return m, nil
		case "r":
			if m.activeBatch != nil && m.activeBatch.isRunning() {
				return m, nil // block rescan during batch
			}
			cmd := m.startScan()
			return m, cmd
		}
		return m, nil
	}

	// Config tab uses the config editor.
	if m.activeTab == tabConfig {
		return m.handleKeyConfigEditor(msg)
	}

	// Favorites tab — handle export/share modes and favorites-specific keys.
	if m.activeTab == tabFavorites {
		return m.handleKeyFavorites(msg)
	}

	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	}

	// While a batch operation is running, only allow skip (s), cancel (Esc),
	// and quit. Block all other actions to prevent state corruption.
	if m.activeBatch != nil && m.activeBatch.isRunning() {
		switch msg.String() {
		case "esc":
			m.activeBatch.cancel()
			m.statusMsg = "⚠ Cancelling — waiting for current item..."
			return m, nil
		case "s":
			if m.activeBatch.skip() {
				m.statusMsg = "⏭ Next item skipped"
			}
			return m, nil
		}
		return m, nil
	}

	switch msg.String() {
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
		if m.categoryFilter != "" || m.tagFilter != "" || m.platformFilter != "" || m.statusFilter != "" {
			m.categoryFilter = ""
			m.tagFilter = ""
			m.platformFilter = ""
			m.statusFilter = ""
			m.cursor = 0
			m.applyFilter()
			return m, nil
		}
	case "right", "tab":
		// On Marketplace tab, cycle sub-tabs before switching main tabs.
		if m.activeTab == tabDiscover && m.discoverSubTab < discoverSubTabCount-1 {
			m.discoverSubTab++
			m.cursor = 0
			if m.discoverSubTab == discoverOnboard {
				m.recomputeOnboardTools()
			}
			return m, nil
		}
		m.activeTab = (m.activeTab + 1) % tabCount
		m.cursor = 0
		m.discoverSubTab = discoverTools
		m.applyFilter()
		if m.activeTab == tabProject {
			return m, projectLoadListCmd(m.tools)
		}
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
		if m.activeTab == tabProject {
			return m, projectLoadListCmd(m.tools)
		}
		return m, nil
	case "1":
		m.activeTab = tabInstalled
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "2":
		m.activeTab = tabFavorites
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "3":
		m.activeTab = tabUpdates
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "4":
		m.activeTab = tabDiscover
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "5":
		m.activeTab = tabBackup
		m.cursor = 0
		return m, nil
	case "6":
		m.activeTab = tabProject
		m.cursor = 0
		m.projectCursor = 0
		m.projectScroll = 0
		m.projectView = projectViewList
		return m, projectLoadListCmd(m.tools)
	case "7":
		m.activeTab = tabDashboard
		m.cursor = 0
		m.dashboardScroll = 0
		m.myBackupFiles = scanBackupsDir()
		return m, nil
	case "8":
		m.activeTab = tabConfig
		m.cursor = 0
		m.configScroll = 0
		return m, nil
	case "9":
		m.activeTab = tabDoctor
		m.cursor = 0
		m.doctorScroll = 0
		return m, nil
	case "s":
		// Skip current item during import.
		if m.activeTab == tabBackup && m.isImportRunning() {
			skipped := false
			for i := range m.backupItems {
				if m.backupItems[i].status == backupPending {
					m.backupItems[i].status = backupSkipped
					m.backupItems[i].errMsg = "skipped"
					m.backupDone++
					skipped = true
					break
				}
			}
			if skipped {
				m.statusMsg = "⏭ Next item skipped"
			}
			return m, nil
		}
		// Cycle sort mode on tool-list tabs (not Backup/Dashboard/Config).
		if m.activeTab <= tabDiscover && (m.activeTab != tabDiscover || m.discoverSubTab == discoverTools) {
			m.sortMode = (m.sortMode + 1) % sortModeCount
			m.cursor = 0
			m.applyFilter()
			switch m.sortMode {
			case sortByName:
				m.statusMsg = "Sort: A→Z name"
			case sortByStars:
				m.statusMsg = "Sort: ★ stars"
			}
		}
		// Toggle pack sort mode (Packs sub-tab).
		if m.activeTab == tabDiscover && m.discoverSubTab == discoverPacks {
			m.packSortMode = (m.packSortMode + 1) % 2
			m.cursor = 0
			if m.packSortMode == 0 {
				m.statusMsg = "Sort: by status"
			} else {
				m.statusMsg = "Sort: by name"
			}
		}
		return m, nil
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		} else if m.rowCount() > 0 {
			m.cursor = m.rowCount() - 1 // wrap to bottom
		}
	case "down", "j":
		if m.cursor < m.rowCount()-1 {
			m.cursor++
		} else {
			m.cursor = 0 // wrap to top
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
	case "*":
		// Toggle favorite — For You sub-tab uses recommendations, not filteredIndex.
		if m.activeTab == tabDiscover && m.discoverSubTab == discoverForYou {
			if m.cursor < len(m.recommendations) {
				idx := m.recommendations[m.cursor].toolIdx
				if idx < len(m.tools) {
					name := m.tools[idx].Name
					added, err := favorites.Toggle(name)
					switch {
					case err != nil:
						m.statusMsg = "⚠ " + err.Error()
					case added:
						m.favoriteNames[name] = true
						m.statusMsg = "★ Added to favorites"
					default:
						delete(m.favoriteNames, name)
						m.statusMsg = "☆ Removed from favorites"
					}
				}
			}
			return m, nil
		}
		// Toggle favorite — Onboard sub-tab uses onboardTools.
		if m.activeTab == tabDiscover && m.discoverSubTab == discoverOnboard {
			if m.cursor < len(m.onboardTools) {
				idx := m.onboardTools[m.cursor].toolIdx
				if idx < len(m.tools) {
					name := m.tools[idx].Name
					added, err := favorites.Toggle(name)
					switch {
					case err != nil:
						m.statusMsg = "⚠ " + err.Error()
					case added:
						m.favoriteNames[name] = true
						m.statusMsg = "★ Added to favorites"
					default:
						delete(m.favoriteNames, name)
						m.statusMsg = "☆ Removed from favorites"
					}
				}
			}
			return m, nil
		}
		// Toggle favorite on other tool-list tabs.
		if m.activeTab <= tabDiscover && m.cursor < len(m.filteredIndex) {
			idx := m.filteredIndex[m.cursor]
			if idx < len(m.tools) {
				name := m.tools[idx].Name
				added, err := favorites.Toggle(name)
				switch {
				case err != nil:
					m.statusMsg = "⚠ " + err.Error()
				case added:
					m.favoriteNames[name] = true
					m.statusMsg = "★ Added to favorites"
				default:
					delete(m.favoriteNames, name)
					m.statusMsg = "☆ Removed from favorites"
				}
			}
		}
		return m, nil
	case "[":
		// Cycle onboard role left.
		if m.activeTab == tabDiscover && m.discoverSubTab == discoverOnboard {
			if m.onboardRole > 0 {
				m.onboardRole--
			} else {
				m.onboardRole = len(onboard.Roles) - 1
			}
			m.recomputeOnboardTools()
		}
		return m, nil
	case "]":
		// Cycle onboard role right.
		if m.activeTab == tabDiscover && m.discoverSubTab == discoverOnboard {
			if m.onboardRole < len(onboard.Roles)-1 {
				m.onboardRole++
			} else {
				m.onboardRole = 0
			}
			m.recomputeOnboardTools()
		}
		return m, nil
	case "i":
		// Quick install from For You sub-tab.
		if m.activeTab == tabDiscover && m.discoverSubTab == discoverForYou {
			if m.cursor < len(m.recommendations) {
				rec := m.recommendations[m.cursor]
				if rec.toolIdx < len(m.tools) {
					tool := m.tools[rec.toolIdx]
					src := tool.Packages.BestInstallSource()
					if src == "" {
						m.statusMsg = "⚠ No package manager available for this tool"
						return m, nil
					}
					args := tool.Packages.InstallArgs(src)
					if args == nil {
						m.statusMsg = "⚠ No install command available"
						return m, nil
					}
					m.pendingAction = &pendingAction{
						toolIdx: rec.toolIdx,
						action:  actionInstall,
						cmdArgs: args,
					}
				}
			}
			return m, nil
		}
		// Quick install from Onboard sub-tab.
		if m.activeTab == tabDiscover && m.discoverSubTab == discoverOnboard {
			if m.cursor < len(m.onboardTools) {
				rec := m.onboardTools[m.cursor]
				if rec.toolIdx < len(m.tools) {
					tool := m.tools[rec.toolIdx]
					src := tool.Packages.BestInstallSource()
					if src == "" {
						m.statusMsg = "⚠ No package manager available for this tool"
						return m, nil
					}
					args := tool.Packages.InstallArgs(src)
					if args == nil {
						m.statusMsg = "⚠ No install command available"
						return m, nil
					}
					m.pendingAction = &pendingAction{
						toolIdx: rec.toolIdx,
						action:  actionInstall,
						cmdArgs: args,
					}
				}
			}
			return m, nil
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
		if m.activeTab == tabUpdates && (m.activeBatch == nil || !m.activeBatch.isRunning()) {
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
			case backupMenuCreatePack:
				// Create Pack — enter pack creation wizard.
				if m.phase < phaseDone {
					m.statusMsg = "Still scanning — please wait..."
					return m, nil
				}
				if len(m.tools) == 0 {
					m.statusMsg = "No tools available to create a pack."
					return m, nil
				}
				m.startPackCreate()
				return m, m.packCreateName.Focus()
			case backupMenuMyPacks:
				// My Packs — view saved custom packs.
				if cp, err := custompacks.Load(); err != nil {
					m.statusMsg = fmt.Sprintf("✗ %v", err)
				} else {
					m.customPacks = cp
					m.viewingMyPacks = true
					m.myPacksCursor = 0
					m.statusMsg = ""
				}
				return m, nil
			case backupMenuMyBackups:
				// My Backups — view saved backup files.
				files := scanBackupsDir()
				m.myBackupFiles = files
				m.viewingMyBackups = true
				m.myBackupsCursor = 0
				m.statusMsg = ""
				return m, nil
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
				m.packDetailIdx = m.packDisplayIndex(m.cursor)
				m.showPackDetail = true
				m.packToolCursor = 0
				m.packItems = nil // clear any leftover progress from a previous pack
				m.packDone = 0
				m.packCancelled = false
			}
			return m, nil
		}
		// On Marketplace For You sub-tab, open the recommended tool's detail.
		if m.activeTab == tabDiscover && m.discoverSubTab == discoverForYou {
			if m.cursor < len(m.recommendations) {
				m.openDetailView(m.recommendations[m.cursor].toolIdx)
			}
			return m, nil
		}
		// On Marketplace Onboard sub-tab, open the recommended tool's detail.
		if m.activeTab == tabDiscover && m.discoverSubTab == discoverOnboard {
			if m.cursor < len(m.onboardTools) {
				m.openDetailView(m.onboardTools[m.cursor].toolIdx)
			}
			return m, nil
		}
		if m.cursor < len(m.filteredIndex) {
			m.openDetailView(m.filteredIndex[m.cursor])
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

// applyFilter, hasTag, hasPlatform, matchesTab, collectCategories,
// collectTags, collectPlatforms, and buildSidebarItems have moved to
// sidebar.go to keep model.go focused on Update/View dispatch.

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

// detectTeamFile looks for .clim.yaml in CWD/parents and runs checks
// against the current tool list. Called after version resolution completes.
func (m *Model) detectTeamFile() {
	m.teamFilePath = ""
	m.teamFile = nil
	m.teamCheckResult = nil

	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	path := teamfile.Find(cwd)
	if path == "" {
		return
	}
	tf, err := teamfile.Parse(path)
	if err != nil {
		slog.Warn("failed to parse .clim.yaml", "path", path, "error", err)
		m.statusMsg = fmt.Sprintf("⚠ .clim.yaml parse error: %s", err)
		// Don't set teamFilePath — leave state clean.
		return
	}
	m.teamFilePath = path
	m.teamFile = tf
	m.teamCheckResult = teamfile.Check(tf, m.tools)
}

// runDoctor computes environment diagnostics and stores results.
// Callers may optionally provide scan metadata (e.g. cache/fresh state).
func (m *Model) runDoctor(meta ...doctor.ScanMeta) {
	scanMeta := doctor.ScanMeta{}
	if len(meta) > 0 {
		scanMeta = meta[0]
	}
	m.doctorIssues = doctor.Diagnose(m.tools, scanMeta)
	m.auditFindings, m.auditLicenses = audit.Analyze(m.tools)

	// Run compliance check if a policy is configured.
	policyPath := ""
	if m.cfg != nil {
		policyPath = m.cfg.Compliance.Policy
	}
	m.complianceResult, m.complianceError = runComplianceForTUI(m.tools, policyPath)

	// Compute environment health score using precomputed data.
	auditW, auditI := audit.CountBySeverity(m.auditFindings)
	m.cachedScore = score.Compute(score.Input{
		Tools:         m.tools,
		DoctorIssues:  m.doctorIssues,
		AuditWarnings: auditW,
		AuditInfos:    auditI,
		CompResult:    m.complianceResult,
		ComplianceErr: m.complianceError,
	})

	m.doctorChecked = true
	m.doctorScroll = 0
}

// --- Actions ---

// buildToolMenu resolves available actions for the current tool and populates the tool menu.
// Returns true if the menu has any actions, false otherwise.
func (m *Model) buildToolMenu() bool {
	tool := m.currentTool()
	if tool == nil {
		return false
	}
	m.toolMenuItems = nil
	idx := m.currentToolIdx()

	// Only show PMs that are available on PATH.
	pmAvail := make(map[registry.InstallSource]bool)
	for _, pm := range registry.AllPMStatusForOS() {
		if pm.Available {
			pmAvail[pm.Source] = true
		}
	}

	if tool.IsInstalled() {
		primary := tool.PrimaryInstance()
		installedSources := make(map[registry.InstallSource]bool)
		if primary != nil {
			installedSources[primary.Source] = true
		}
		for _, inst := range tool.Instances {
			installedSources[inst.Source] = true
		}

		allPMs := []registry.InstallSource{
			registry.SourceWinget, registry.SourceChoco, registry.SourceScoop,
			registry.SourceBrew, registry.SourceApt, registry.SourceSnap, registry.SourceNPM,
		}
		for _, src := range allPMs {
			if tool.Packages.InstallArgs(src) == nil {
				continue
			}
			if !pmAvail[src] {
				continue // PM not on PATH — hide row
			}
			item := toolMenuAction{}
			if installedSources[src] {
				if args := tool.Packages.UpgradeArgs(src); args != nil {
					item.picker = &sourcePicker{
						toolIdx: idx, action: actionUpgrade,
						choices: []sourceChoice{{source: src, cmdArgs: args}},
					}
				}
				if args := tool.Packages.RemoveArgs(src); args != nil {
					item.removePicker = &sourcePicker{
						toolIdx: idx, action: actionRemove,
						choices: []sourceChoice{{source: src, cmdArgs: args}},
					}
				}
			} else {
				// Not installed via this PM — offer install.
				if args := tool.Packages.InstallArgs(src); args != nil {
					item.picker = &sourcePicker{
						toolIdx: idx, action: actionInstall,
						choices: []sourceChoice{{source: src, cmdArgs: args}},
					}
				}
			}
			m.toolMenuItems = append(m.toolMenuItems, item)
		}
	} else {
		// Install — one menu item per declared package manager, ordered to
		// match renderPackageManagers (collectPackageEntries order).
		// This allows toolMenu index to map directly to PM row in the view.
		allPMs := []registry.InstallSource{
			registry.SourceWinget, registry.SourceChoco, registry.SourceScoop,
			registry.SourceBrew, registry.SourceApt, registry.SourceSnap, registry.SourceNPM,
		}
		for _, src := range allPMs {
			args := tool.Packages.InstallArgs(src)
			if args == nil {
				continue
			}
			if !pmAvail[src] {
				continue // PM not on PATH — hide row
			}
			m.toolMenuItems = append(m.toolMenuItems, toolMenuAction{
				label: "Install via " + string(src),
				picker: &sourcePicker{
					toolIdx: idx,
					action:  actionInstall,
					choices: []sourceChoice{{source: src, cmdArgs: args}},
				},
			})
		}
	}
	if len(m.toolMenuItems) == 0 {
		return false
	}
	m.toolMenu = 0
	return true
}

// openDetailView sets up the detail view for the tool at the given index.
func (m *Model) openDetailView(toolIdx int) {
	m.detailIdx = toolIdx
	m.showDetail = true
	m.detailScroll = 0
	m.detailRelCursor = -1
	if toolIdx >= 0 && toolIdx < len(m.tools) {
		m.detailRelated = m.relatedTools(m.tools[toolIdx])
	} else {
		m.detailRelated = nil
	}
	m.buildToolMenu()
	m.computeDetailMaxScroll()
}

// computeDetailMaxScroll computes the max scroll (in logical lines) from the
// actual rendered detail body. Computes footer height the same way as
// renderDetailView to avoid mismatch.
func (m *Model) computeDetailMaxScroll() {
	if !m.showDetail || m.detailIdx < 0 || m.detailIdx >= len(m.tools) {
		m.detailMaxScroll = 0
		return
	}
	body := m.renderDetailBody(m.tools[m.detailIdx])
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")

	// Build the same footer as renderDetailView to measure its height.
	footerRows := m.detailFooterHeight()

	const minGap = 1
	visibleRows := m.height - footerRows - minGap
	if visibleRows < 5 {
		visibleRows = 5
	}
	m.detailMaxScroll = len(lines) - visibleRows
	if m.detailMaxScroll < 0 {
		m.detailMaxScroll = 0
	}
}

// detailFooterHeight returns the visual row count of the detail view footer,
// matching what renderDetailView builds.
func (m Model) detailFooterHeight() int {
	dim := dimVersion.Render
	var footer strings.Builder
	switch {
	case m.pendingAction != nil:
		prompt := confirmStyle.Render(fmt.Sprintf("  Run %s?", strings.Join(m.pendingAction.cmdArgs, " ")))
		keys := dim("y") + " confirm   " + dim("Esc") + " cancel"
		footer.WriteString(prompt + "  " + keys)
	default:
		hints := []string{
			dim("↑↓") + " navigate",
			dim("PgUp/PgDn") + " scroll",
			dim("Enter") + " select",
			dim("Esc") + " back",
		}
		footer.WriteString("  " + helpStyle.Render(strings.Join(hints, "   ")))
	}
	if m.statusMsg != "" {
		footer.WriteString("\n  " + upgradableStyle.Render(m.statusMsg))
	}
	return visualRows(footer.String(), m.width)
}

// clampDetailScroll ensures detailScroll stays within [0, detailMaxScroll].
func (m *Model) clampDetailScroll() {
	if m.detailScroll > m.detailMaxScroll {
		m.detailScroll = m.detailMaxScroll
	}
	if m.detailScroll < 0 {
		m.detailScroll = 0
	}
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

// recomputeOnboardTools recomputes role-based tool recommendations
// for the currently selected onboard role.
func (m *Model) recomputeOnboardTools() {
	if m.onboardRole < 0 || m.onboardRole >= len(onboard.Roles) {
		m.onboardTools = nil
		return
	}
	role := &onboard.Roles[m.onboardRole]
	scored := onboard.Recommend(role, m.tools, 20)
	recs := make([]recommendation, 0, len(scored))
	maxScore := 0
	for _, s := range scored {
		if s.Score > maxScore {
			maxScore = s.Score
		}
	}
	for _, s := range scored {
		desc := ""
		stars := 0
		if s.Tool.GitHubInfo != nil {
			desc = s.Tool.GitHubInfo.Description
			stars = s.Tool.GitHubInfo.Stars
		}
		pct := 0
		if maxScore > 0 {
			pct = s.Score * 100 / maxScore
			if pct < 1 {
				pct = 1
			}
		}
		recs = append(recs, recommendation{
			toolIdx:     s.Index,
			score:       s.Score,
			reason:      "", // role description shown in header, not per-card
			category:    s.Tool.Category,
			description: desc,
			stars:       stars,
			matchPct:    pct,
		})
	}
	m.onboardTools = recs
	m.cursor = 0
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
		if m.discoverSubTab == discoverOnboard {
			return len(m.onboardTools)
		}
		return len(m.filteredIndex)
	case tabBackup:
		if m.backupMode == backupModeIdle {
			return backupMenuCount
		}
		return len(m.backupItems)
	case tabProject:
		return 0
	case tabDashboard:
		return 0
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
			m.backupDone++
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
	verb := "complete"
	if m.backupCancelled {
		prefix = "⚠"
		verb = "cancelled"
	} else if failed > 0 {
		prefix = "⚠"
	}
	return fmt.Sprintf("%s Import %s — %s", prefix, verb, strings.Join(parts, ", "))
}

// --- Batch upgrade (Updates tab) ---

// startBatchUpgrade kicks off sequential upgrades for all selected tools
// using the unified batch operation engine.
func (m Model) startBatchUpgrade() (tea.Model, tea.Cmd) {
	var items []batchItem
	for _, idx := range m.filteredIndex {
		if !m.updateSelected[idx] {
			continue
		}
		tool := &m.tools[idx]
		if !tool.HasUpdate() {
			continue
		}
		// Find upgrade args (best available source).
		var args []string
		var src string
		for _, s := range registry.SourcesForOS() {
			if a := tool.Packages.UpgradeArgs(s); a != nil {
				args = a
				src = string(s)
				break
			}
		}
		if args == nil {
			items = append(items, batchItem{
				name:    tool.Name,
				display: tool.DisplayName,
				source:  "",
				status:  batchSkipped,
				errMsg:  "no upgrade command available",
			})
			continue
		}
		items = append(items, batchItem{
			name:    tool.Name,
			display: tool.DisplayName,
			cmdArgs: args,
			source:  src,
			status:  batchPending,
		})
	}
	if len(items) == 0 {
		m.statusMsg = "No upgradable tools selected."
		return m, nil
	}
	m.activeBatch = newBatchOp("Upgrading", items)
	if cmd := m.activeBatch.next(); cmd != nil {
		m.statusMsg = m.activeBatch.statusLine()
		return m, cmd
	}
	// All items were pre-skipped.
	m.activeBatch.finish()
	m.statusMsg = m.activeBatch.summary()
	return m, nil
}
