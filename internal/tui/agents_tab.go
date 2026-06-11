package tui

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/agents/bookmarks"
	"github.com/nassiharel/klim/internal/agents/catalog"
	"github.com/nassiharel/klim/internal/agents/providers/claudecode"
	"github.com/nassiharel/klim/internal/agents/providers/copilotcli"
)

// agentsState holds all in-memory state for the Agents tab.
type agentsState struct {
	subTab int // 0=marketplaces, 1=plugins, 2=skills, 3=mcps, 4=sessions

	loading  bool
	loadedAt time.Time
	loadErr  error
	snapshot *agents.Snapshot

	cursor     int
	detailOpen bool

	// detailPage is the full-screen detail view, layered above the list.
	// When true, renderView() short-circuits to renderAgentsDetailPage()
	// and key dispatch routes through handleAgentsDetailKey. The
	// detailStack lets users drill from marketplace -> plugin -> back
	// without losing the original entry point.
	detailPage  bool
	detailStack []agentDetailFrame

	searchActive bool
	searchInput  string

	// sortMode is per sub-tab: see agentsSortMode* below.
	sortMode map[int]agentsSortMode

	// subTabViewMode toggles between dense table (default) and tile
	// grid per Agents sub-tab. Lazily initialised; a nil map reads
	// back as sessionsViewList (zero value). Cycled with the `t`
	// key. Originally session-only — now per-sub-tab so plugins,
	// MCPs, etc. can flip independently.
	subTabViewMode map[int]sessionsViewMode

	// statusFilter is per sub-tab: see agentsFilter* below.
	statusFilter map[int]agentsFilter

	launchPrompt string // non-empty while the launch confirmation modal is up
	launchPlan   agents.ExecPlan

	// deleteTarget is non-empty while the delete confirmation modal is up.
	// The string is a human description; the actual action is held in
	// deleteAction so the model can fire it after the user confirms.
	deleteTarget string
	deleteAction tea.Cmd

	// viewerLines holds the first N lines of a session transcript so the
	// user can peek at it without leaving the TUI.
	viewerOpen  bool
	viewerTitle string
	viewerLines []string

	flash    string
	flashEnd time.Time

	// actionRunning is the human label of an in-flight action (e.g.
	// "updating plugin foo…"). Cleared by handleAgentsMsg on result.
	actionRunning string

	// statusFilterValue carries a sub-tab-specific status string when
	// the discrete agentsFilter enum can't express it (marketplace
	// builtin/local, session active/completed/stopped). Empty means
	// "no extra status restriction beyond statusFilter[subTab]".
	statusFilterValue string

	// providerFilter narrows rows to a single provider when set
	// (claude-code / copilot-cli). Empty string = all.
	// Applies to every sub-tab.
	providerFilter agents.ProviderID

	// marketplaceFilter narrows the Plugins sub-tab to a single
	// marketplace. Empty string = all.
	marketplaceFilter string

	// pluginFilter narrows the Skills sub-tab to skills whose
	// SourcePlugin matches. Empty string = all. Set when the user
	// uses "View skills →" from a plugin detail page.
	pluginFilter string

	// scopeFilter narrows Skills / MCPs by scope (user/project/plugin/remote).
	scopeFilter agents.Scope

	// transportFilter narrows MCPs by transport (stdio/http/sse).
	transportFilter string

	// Sidebar (Tools-style picker): per-sub-tab item list + cursor,
	// only consulted when sidebarOpen is true.
	sidebarOpen  bool
	sidebarIdx   int
	sidebarItems []agentSidebarItem

	// costs is the Agents → Costs sub-tab state.
	costs agentsCostsState

	// healthSub is the Agents → Health sub-tab state.
	healthSub agentsHealthState

	// searchOverlay is the full-text search modal (key `?` on any
	// Agents sub-tab). When Open=true it takes input precedence over
	// the rest of the tab.
	searchOverlay agentsSearchState

	// promotePicker is the inline target-picker that opens from the
	// detail-page Promote ▸ action.
	promotePicker agentsPromoteState

	// bookmarks is the persistent session-bookmarks store. Lazy-
	// loaded on first access; ★ shown next to bookmarked rows and
	// they pin to the top of the Sessions table.
	bookmarks *bookmarks.Store

	// noteInput drives the inline "edit note" prompt opened with `N`
	// on a bookmarked session row.
	noteOpen   bool
	noteTarget string // session id being edited
	noteBuffer string

	// selected holds the row ids checked via Space for bulk operations.
	// Keyed per sub-tab so each tab's selection is independent and
	// cleared when the user switches tabs. The string keys mirror
	// agentRow.id so the set survives re-scans (ids are stable across
	// snapshot rebuilds even when the row order shifts).
	selected map[int]map[string]bool

	// bulkPrompt drives the inline "Apply X to N items?" confirmation.
	// When non-empty the bulk-action keys (Shift+U/I/X etc.) have
	// already been pressed and we're waiting for y/n.
	bulkPrompt string
	bulkAction func() tea.Cmd
}

// agentSidebarItem is one row of the Agents filter sidebar.
//
// Headers (`isHeader=true`) are styled but not selectable; the cursor
// skips them. `section` names the filter dimension this item drives;
// `value` is the filter value to set (empty string = "all of this
// dimension"). `count` is the number of matching rows in the current
// sub-tab snapshot.
type agentSidebarItem struct {
	label    string
	section  string
	value    string
	count    int
	isHeader bool
}

// agentDetailFrame is one entry in the detail-page navigation stack.
// Identifies the displayed entity by sub-tab + stable id so that
// after a re-scan we can resolve it against the fresh snapshot.
type agentDetailFrame struct {
	subTab     int    // which sub-tab the entity belongs to
	entityID   string // agentRow.id at push time
	listCursor int    // cursor to restore on pop (only set on the bottom frame)
	actionIdx  int    // selected action button
	bodyCursor int    // cursor inside the contextual body (e.g. plugin list)
	scroll     int    // body scroll offset
}

// agentsFilter narrows a sub-tab's rows beyond the search box.
type agentsFilter int

const (
	agentsFilterAll       agentsFilter = iota
	agentsFilterInstalled              // plugins: installed; MCPs: configured (scope ≠ remote)
	agentsFilterCatalog                // remote / not installed
	agentsFilterEnabled                // plugins/MCPs: enabled
	agentsFilterDisabled               // plugins/MCPs: installed but disabled
	agentsFilterCount
)

// agentsSortMode names the supported sort orders.
type agentsSortMode int

const (
	agentsSortDefault  agentsSortMode = iota // per-sub-tab default
	agentsSortName                           // alphabetical
	agentsSortModified                       // recent-first (mtime / last event)
	agentsSortCreated                        // recent-first (created)
	agentsSortTurns                          // sessions: most turns first
	agentsSortProvider                       // group by provider
	agentsSortCount
)

// agentsCacheTTL is how long a cached scan is considered fresh on tab
// entry. Older caches still render immediately but trigger a
// background refresh so the user sees latest state on the next tick.
const agentsCacheTTL = 10 * time.Minute

// sessionsViewMode controls how an Agents sub-tab renders its rows
// (dense per-row table vs bordered tile grid). The name is historical
// — sessions were the first sub-tab to ship a tile view — but the
// type is reused for every sub-tab's tile toggle, keyed by sub-tab
// id on agentsState.subTabViewMode.
type sessionsViewMode int

// Session view modes.
const (
	sessionsViewList  sessionsViewMode = 0 // default — dense table
	sessionsViewTiles sessionsViewMode = 1 // bordered tile grid
)

// next returns the view mode that follows v in the toggle cycle.
func (v sessionsViewMode) next() sessionsViewMode {
	if v == sessionsViewList {
		return sessionsViewTiles
	}
	return sessionsViewList
}

// label returns a short human label for the current view mode.
func (v sessionsViewMode) label() string {
	if v == sessionsViewTiles {
		return "tiles"
	}
	return "list"
}

const (
	agentsSubMarketplaces = 0
	agentsSubPlugins      = 1
	agentsSubSkills       = 2
	agentsSubMCPs         = 3
	agentsSubSessions     = 4
	agentsSubCosts        = 5
	agentsSubHealth       = 6
	agentsSubCount        = 7
)

// agentsService factory. Swappable so tests can inject a fake.
var agentsService = func() *agents.Service {
	svc := agents.NewService(4,
		claudecode.New(),
		copilotcli.New(),
	)
	svc.RemoteCatalog = tuiCatalogAdapter{f: catalog.New()}
	return svc
}

// tuiCatalogAdapter bridges catalog.Fetcher to agents.RemoteCatalog.
type tuiCatalogAdapter struct{ f *catalog.Fetcher }

// FetchAll fetches every configured marketplace and adapts the result
// into the agents.RemoteCatalogResult shape the service expects.
func (a tuiCatalogAdapter) FetchAll(ctx context.Context) []agents.RemoteCatalogResult {
	in := a.f.FetchAll(ctx)
	out := make([]agents.RemoteCatalogResult, 0, len(in))
	for _, r := range in {
		out = append(out, agents.RemoteCatalogResult{
			SourceName:   r.Source.Name,
			Plugins:      r.Plugins,
			Marketplaces: r.Marketplaces,
			Err:          r.Err,
		})
	}
	return out
}

func newAgentsState() *agentsState { return &agentsState{} }

// agentsBookmarks lazy-loads the persistent bookmarks store the
// first time it's accessed. Cached on the state so subsequent
// reads/writes don't hit the disk twice per render.
func agentsBookmarks(st *agentsState) *bookmarks.Store {
	if st == nil {
		return bookmarks.New()
	}
	if st.bookmarks == nil {
		s, _ := bookmarks.Load()
		st.bookmarks = s
	}
	return st.bookmarks
}

// ---------------- messages & commands ----------------

type agentsLoadedMsg struct {
	snap *agents.Snapshot
	err  error
}

type agentsLaunchedMsg struct{ err error }

func loadAgentsCmd(refresh bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		svc := agentsService()
		snap, err := svc.LoadAll(ctx, agents.LoadOpts{Refresh: refresh})
		return agentsLoadedMsg{snap: snap, err: err}
	}
}

func launchAgentsCmd(plan agents.ExecPlan) tea.Cmd {
	c := exec.Command(plan.Bin, plan.Args...)
	if plan.Cwd != "" {
		c.Dir = plan.Cwd
	}
	// PR #77 review fix: never assign a non-nil c.Env unless we're
	// adding to the inherited environment. Setting c.Env strips
	// os.Environ() (PATH/HOME/etc.) and breaks the launched agent
	// CLI; only override when the plan actually contributes extras.
	if len(plan.Env) > 0 {
		c.Env = append(os.Environ(), plan.Env...)
	}
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return agentsLaunchedMsg{err: err}
	})
}

// ---------------- entry points called from Model ----------------

func (m *Model) agentsInit() tea.Cmd {
	if m.agents == nil {
		m.agents = newAgentsState()
	}
	st := m.agents
	if st.sortMode == nil {
		st.sortMode = make(map[int]agentsSortMode)
	}
	if st.snapshot != nil && !st.loadedAt.IsZero() {
		// A stale on-disk cache from before this feature shipped will
		// have zero marketplaces; treat that as a forced refresh so
		// the user doesn't have to press `r` themselves on first
		// open. Same for plugins / sessions — if all three are empty
		// it's almost certainly a stale or empty cache.
		if len(st.snapshot.Marketplaces) == 0 && len(st.snapshot.Plugins) == 0 && len(st.snapshot.Sessions) == 0 {
			st.loading = true
			return loadAgentsCmd(true)
		}
		// Otherwise: refresh in the background once the TTL elapses,
		// keep showing the stale snapshot in the meantime.
		if time.Since(st.loadedAt) > agentsCacheTTL {
			return loadAgentsCmd(true)
		}
		return nil
	}
	st.loading = true
	// Skip the on-disk cache on first open of a session. The cache is
	// helpful for warm-loads but tends to bite users with stale data
	// after upgrades (e.g. a marketplace was added but the cache
	// pre-dates the change). Fresh scans are still fast on Windows
	// (~1s) so the latency hit is acceptable.
	return loadAgentsCmd(true)
}

// handleAgentsKey processes keystrokes when the Agents tab is active.
// Returns (handled, cmd). When handled is true, caller should skip
// other key dispatch.
func (m *Model) handleAgentsKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.agents == nil {
		m.agents = newAgentsState()
	}
	st := m.agents
	if st.sortMode == nil {
		st.sortMode = make(map[int]agentsSortMode)
	}
	if st.statusFilter == nil {
		st.statusFilter = make(map[int]agentsFilter)
	}

	// Detail-page mode owns input — its own dispatcher handles every key.
	if st.detailPage {
		return m.handleAgentsDetailKey(msg)
	}

	// Full-text search overlay owns input when open.
	if st.searchOverlay.Open {
		return m.handleAgentsSearchKey(msg)
	}

	// Note prompt owns input while open (inline single-line editor).
	if st.noteOpen {
		switch msg.String() {
		case "esc":
			st.noteOpen = false
			st.noteTarget = ""
			st.noteBuffer = ""
		case "enter":
			id := st.noteTarget
			note := strings.TrimSpace(st.noteBuffer)
			bm := agentsBookmarks(st)
			// Ensure the session is bookmarked so the note has somewhere
			// to live; Add() preserves Created on existing entries.
			bm.Add(id, note)
			bm.SetNote(id, note)
			_ = bm.Save()
			st.noteOpen = false
			st.noteTarget = ""
			st.noteBuffer = ""
			st.flash = "note saved"
			st.flashEnd = time.Now().Add(2 * time.Second)
		case "backspace":
			if len(st.noteBuffer) > 0 {
				st.noteBuffer = st.noteBuffer[:len(st.noteBuffer)-1]
			}
		default:
			k := msg.String()
			if len(k) == 1 {
				st.noteBuffer += k
			} else if k == "space" {
				st.noteBuffer += " "
			}
		}
		return true, nil
	}

	// Costs sub-tab uses its own key handler. The handler returns
	// false for keys it doesn't claim (e.g. number keys, tab) so the
	// normal sub-tab routing below stays intact.
	if st.subTab == agentsSubCosts {
		if handled, cmd := m.handleAgentsCostsKey(msg); handled {
			return true, cmd
		}
	}
	// Health sub-tab — same pattern.
	if st.subTab == agentsSubHealth {
		if handled, cmd := m.handleAgentsHealthKey(msg); handled {
			return true, cmd
		}
	}

	// Sidebar (filter picker) owns input while open.
	if st.sidebarOpen {
		switch msg.String() {
		case "esc", "q", "f":
			st.sidebarOpen = false
			return true, nil
		case "down", "j":
			agentsSidebarMove(st, 1)
			return true, nil
		case "up", "k":
			agentsSidebarMove(st, -1)
			return true, nil
		case "enter", " ":
			return true, agentsSidebarSelect(m)
		case "X":
			st.providerFilter = ""
			st.marketplaceFilter = ""
			st.pluginFilter = ""
			st.scopeFilter = ""
			st.transportFilter = ""
			st.statusFilterValue = ""
			st.statusFilter[st.subTab] = agentsFilterAll
			st.cursor = 0
			st.sidebarItems = buildAgentsSidebarItems(st)
			st.flash = "filters cleared"
			st.flashEnd = time.Now().Add(2 * time.Second)
			return true, nil
		}
		return true, nil
	}

	// Viewer modal closes on Esc/Enter/q.
	if st.viewerOpen {
		switch msg.String() {
		case "esc", "enter", "q":
			st.viewerOpen = false
		}
		return true, nil
	}
	// Bulk-confirmation prompt owns input while open.
	if st.bulkPrompt != "" {
		switch msg.String() {
		case "y", "Y", "enter":
			cmd := st.bulkAction
			st.bulkPrompt = ""
			st.bulkAction = nil
			if cmd != nil {
				return true, cmd()
			}
			return true, nil
		case "esc", "n", "N":
			st.bulkPrompt = ""
			st.bulkAction = nil
		}
		return true, nil
	}

	// Delete confirmation owns input while open.
	if st.deleteTarget != "" {
		switch msg.String() {
		case "y", "Y", "enter":
			cmd := st.deleteAction
			st.deleteTarget = ""
			st.deleteAction = nil
			st.flash = "deleting…"
			st.flashEnd = time.Now().Add(2 * time.Second)
			return true, cmd
		case "esc", "n", "N":
			st.deleteTarget = ""
			st.deleteAction = nil
		}
		return true, nil
	}

	// Launch confirmation modal owns input while open.
	if st.launchPrompt != "" {
		switch msg.String() {
		case "y", "Y", "enter":
			plan := st.launchPlan
			st.launchPrompt = ""
			st.flash = "launching: " + plan.Bin
			st.flashEnd = time.Now().Add(3 * time.Second)
			return true, launchAgentsCmd(plan)
		case "esc", "n", "N":
			st.launchPrompt = ""
		}
		return true, nil
	}

	// Search input owns printable keys while active.
	if st.searchActive {
		switch msg.String() {
		case "esc":
			st.searchActive = false
			st.searchInput = ""
		case "enter":
			st.searchActive = false
		case "backspace":
			if len(st.searchInput) > 0 {
				st.searchInput = st.searchInput[:len(st.searchInput)-1]
			}
		default:
			ks := msg.String()
			if len(ks) == 1 {
				st.searchInput += ks
				st.cursor = 0
			}
		}
		return true, nil
	}

	// Bulk-action shortcuts win when there's an active selection on
	// the current sub-tab. This lets us reuse single-letter keys
	// (D, X, B) for both single-row and bulk modes without forcing
	// the user to remember a different keystroke set.
	if agentsBulkCapable(st.subTab) && agentsSelectionCount(st, st.subTab) > 0 {
		if label, action, ok := agentsBulkActionForKey(m, msg.String()); ok {
			n := agentsSelectionCount(st, st.subTab)
			st.bulkPrompt = fmt.Sprintf("Apply %s to %d items?", label, n)
			st.bulkAction = action
			return true, nil
		}
		if msg.String() == "esc" {
			agentsClearSelection(st, st.subTab)
			return true, nil
		}
	}

	switch msg.String() {
	case "1":
		st.setSubTab(agentsSubMarketplaces)
		return true, nil
	case "2":
		st.setSubTab(agentsSubPlugins)
		return true, nil
	case "3":
		st.setSubTab(agentsSubSkills)
		return true, nil
	case "4":
		st.setSubTab(agentsSubMCPs)
		return true, nil
	case "5":
		st.setSubTab(agentsSubSessions)
		return true, nil
	case "6":
		st.setSubTab(agentsSubCosts)
		return true, m.agentsSubTabEnterCmd(agentsSubCosts)
	case "7":
		st.setSubTab(agentsSubHealth)
		return true, m.agentsSubTabEnterCmd(agentsSubHealth)
	case "tab", "right":
		// On the rightmost sub-tab, let the global handler advance to
		// the next parent tab instead of wrapping inside Agents.
		if st.subTab == agentsSubCount-1 {
			return false, nil
		}
		next := (st.subTab + 1) % agentsSubCount
		st.setSubTab(next)
		return true, m.agentsSubTabEnterCmd(next)
	case "shift+tab", "left":
		if st.subTab == 0 {
			return false, nil
		}
		next := (st.subTab + agentsSubCount - 1) % agentsSubCount
		st.setSubTab(next)
		return true, m.agentsSubTabEnterCmd(next)
	case "down", "j":
		st.cursor++
		if n := len(m.agentsVisibleRows()); st.cursor >= n && n > 0 {
			st.cursor = n - 1
		}
		if st.cursor < 0 {
			st.cursor = 0
		}
		return true, nil
	case "up", "k":
		st.cursor--
		if st.cursor < 0 {
			st.cursor = 0
		}
		return true, nil
	case "enter":
		// Push a full-screen detail page for the selected row. The old
		// inline detail pane is kept available via Shift+D so users
		// who relied on the quick-peek can still get it.
		rows := m.agentsVisibleRows()
		if st.cursor < 0 || st.cursor >= len(rows) {
			return true, nil
		}
		st.detailPage = true
		st.detailStack = []agentDetailFrame{{
			subTab:     st.subTab,
			entityID:   rows[st.cursor].id,
			listCursor: st.cursor,
		}}
		return true, nil
	case "D":
		// Legacy inline-detail toggle (Shift+D).
		st.detailOpen = !st.detailOpen
		return true, nil
	case "/":
		st.searchActive = true
		st.searchInput = ""
		st.cursor = 0
		return true, nil
	case "l":
		plan, ok := m.agentsBuildLaunchPlan()
		if !ok {
			st.flash = "no launch action for this row"
			st.flashEnd = time.Now().Add(2 * time.Second)
			return true, nil
		}
		st.launchPlan = plan
		st.launchPrompt = "Launch this command?"
		return true, nil
	case "s":
		// Cycle sort mode for the current sub-tab.
		modes := agentsSortModesFor(st.subTab)
		cur := st.sortMode[st.subTab]
		next := nextSortMode(modes, cur)
		st.sortMode[st.subTab] = next
		st.cursor = 0
		st.flash = "sort: " + sortModeName(next)
		st.flashEnd = time.Now().Add(2 * time.Second)
		return true, nil
	case "y":
		// Yank the current row's id (or command, if the launch modal is up).
		rows := m.agentsVisibleRows()
		if st.cursor >= 0 && st.cursor < len(rows) {
			cb := systemClipboard{}
			if err := cb.WriteAll(rows[st.cursor].id); err == nil {
				st.flash = "copied id: " + rows[st.cursor].id
			} else {
				st.flash = "clipboard error: " + err.Error()
			}
			st.flashEnd = time.Now().Add(2 * time.Second)
		}
		return true, nil
	case "b":
		// Toggle a session bookmark. Only sessions are bookmarkable;
		// other entities already have favorites or aren't worth pinning.
		rows := m.agentsVisibleRows()
		if st.cursor < 0 || st.cursor >= len(rows) || rows[st.cursor].session == nil {
			st.flash = "bookmarks only apply to sessions"
			st.flashEnd = time.Now().Add(2 * time.Second)
			return true, nil
		}
		bm := agentsBookmarks(st)
		on := bm.Toggle(rows[st.cursor].id)
		_ = bm.Save()
		if on {
			st.flash = "★ bookmarked"
		} else {
			st.flash = "☆ removed"
		}
		st.flashEnd = time.Now().Add(2 * time.Second)
		return true, nil
	case "N":
		// Edit (or create) the note attached to the focused session
		// bookmark. The inline note-prompt grabs input until Enter / Esc.
		rows := m.agentsVisibleRows()
		if st.cursor < 0 || st.cursor >= len(rows) || rows[st.cursor].session == nil {
			st.flash = "notes only apply to sessions"
			st.flashEnd = time.Now().Add(2 * time.Second)
			return true, nil
		}
		id := rows[st.cursor].id
		bm := agentsBookmarks(st)
		// Prefill the buffer with the existing note (if any) so users
		// can edit instead of retype.
		st.noteOpen = true
		st.noteTarget = id
		if e, ok := bm.Get(id); ok {
			st.noteBuffer = e.Note
		} else {
			st.noteBuffer = ""
		}
		return true, nil
	case " ", "space":
		// Toggle row selection for bulk operations. Only relevant on
		// sub-tabs that support bulk actions (plugins / mcps /
		// sessions); a no-op flash elsewhere.
		if !agentsBulkCapable(st.subTab) {
			st.flash = "bulk ops not available on this sub-tab"
			st.flashEnd = time.Now().Add(2 * time.Second)
			return true, nil
		}
		rows := m.agentsVisibleRows()
		if st.cursor < 0 || st.cursor >= len(rows) {
			return true, nil
		}
		agentsToggleSelection(st, st.subTab, rows[st.cursor].id)
		// Advance the cursor like Updates-tab's space toggle so power
		// users can hold space-down without re-aiming.
		if st.cursor < len(rows)-1 {
			st.cursor++
		}
		return true, nil
	case "f":
		// Open the filter sidebar (Tools-style picker). Builds the
		// item list for the current sub-tab so counts are fresh.
		st.sidebarItems = buildAgentsSidebarItems(st)
		if len(st.sidebarItems) == 0 {
			st.flash = "no filters for this sub-tab"
			st.flashEnd = time.Now().Add(2 * time.Second)
			return true, nil
		}
		st.sidebarOpen = true
		// Park cursor on the first selectable (non-header) item.
		st.sidebarIdx = 0
		for i, it := range st.sidebarItems {
			if !it.isHeader {
				st.sidebarIdx = i
				break
			}
		}
		return true, nil
	case "M", "P":
		// Legacy quick-cycle keys still work but jump straight into
		// the sidebar focused on the relevant section so users get a
		// fuller picker without losing muscle memory.
		st.sidebarItems = buildAgentsSidebarItems(st)
		if len(st.sidebarItems) == 0 {
			return true, nil
		}
		st.sidebarOpen = true
		want := "provider"
		if msg.String() == "M" {
			want = "marketplace"
		}
		st.sidebarIdx = 0
		for i, it := range st.sidebarItems {
			if !it.isHeader && it.section == want {
				st.sidebarIdx = i
				break
			}
		}
		return true, nil
	case "X":
		// Clear every active filter on the current sub-tab.
		st.providerFilter = ""
		st.marketplaceFilter = ""
		st.pluginFilter = ""
		st.scopeFilter = ""
		st.transportFilter = ""
		st.statusFilterValue = ""
		st.statusFilter[st.subTab] = agentsFilterAll
		st.searchInput = ""
		st.cursor = 0
		st.sidebarItems = buildAgentsSidebarItems(st)
		st.flash = "filters cleared"
		st.flashEnd = time.Now().Add(2 * time.Second)
		return true, nil
	case "i":
		// Toggle the Installed-only filter on Plugins or Marketplaces
		// sub-tabs. Quick keyboard shortcut alternative to the sidebar
		// STATUS section.
		if st.subTab != agentsSubPlugins && st.subTab != agentsSubMarketplaces {
			return true, nil
		}
		entity := "plugins"
		if st.subTab == agentsSubMarketplaces {
			entity = "marketplaces"
		}
		if st.statusFilter[st.subTab] == agentsFilterInstalled {
			st.statusFilter[st.subTab] = agentsFilterAll
			st.flash = "installed-only filter cleared"
		} else {
			st.statusFilter[st.subTab] = agentsFilterInstalled
			st.statusFilterValue = ""
			st.flash = "showing installed " + entity + " only"
		}
		st.cursor = 0
		st.sidebarItems = buildAgentsSidebarItems(st)
		st.flashEnd = time.Now().Add(2 * time.Second)
		return true, nil
	case "d":
		// Delete current row — only sessions and MCPs are deletable here.
		rows := m.agentsVisibleRows()
		if st.cursor < 0 || st.cursor >= len(rows) {
			return true, nil
		}
		row := rows[st.cursor]
		switch {
		case row.session != nil:
			st.deleteTarget = "session " + row.id
			st.deleteAction = deleteAgentEntityCmd(row.provider, agents.EntitySession, row.id)
		case row.mcp != nil:
			st.deleteTarget = "MCP " + row.mcp.Name
			st.deleteAction = deleteAgentEntityCmd(row.provider, agents.EntityMCP, row.mcp.Name)
		default:
			st.flash = "delete not supported for this row type"
			st.flashEnd = time.Now().Add(2 * time.Second)
		}
		return true, nil
	case "v":
		// View transcript — sessions only.
		rows := m.agentsVisibleRows()
		if st.cursor < 0 || st.cursor >= len(rows) || rows[st.cursor].session == nil {
			st.flash = "view: not a session row"
			st.flashEnd = time.Now().Add(2 * time.Second)
			return true, nil
		}
		s := rows[st.cursor].session
		path := s.TranscriptPath
		if path == "" {
			st.flash = "no transcript path recorded"
			st.flashEnd = time.Now().Add(2 * time.Second)
			return true, nil
		}
		lines, err := readSessionTranscript(path, 60)
		if err != nil {
			st.flash = "view error: " + err.Error()
			st.flashEnd = time.Now().Add(3 * time.Second)
			return true, nil
		}
		st.viewerOpen = true
		st.viewerTitle = path
		st.viewerLines = lines
		return true, nil
	case "o":
		// Open the current row's primary URL in the user's browser.
		rows := m.agentsVisibleRows()
		if st.cursor < 0 || st.cursor >= len(rows) {
			return true, nil
		}
		url := rowOpenURL(rows[st.cursor])
		if url == "" {
			st.flash = "no URL to open for this row"
			st.flashEnd = time.Now().Add(2 * time.Second)
			return true, nil
		}
		if err := openBrowser(url); err != nil {
			st.flash = "open error: " + err.Error()
		} else {
			st.flash = "opened: " + url
		}
		st.flashEnd = time.Now().Add(3 * time.Second)
		return true, nil
	case "c":
		// Copy the row's most useful command/URL/path to clipboard.
		rows := m.agentsVisibleRows()
		if st.cursor < 0 || st.cursor >= len(rows) {
			return true, nil
		}
		text, label := rowCopyText(rows[st.cursor])
		if text == "" {
			st.flash = "nothing to copy for this row"
			st.flashEnd = time.Now().Add(2 * time.Second)
			return true, nil
		}
		cb := systemClipboard{}
		if err := cb.WriteAll(text); err != nil {
			st.flash = "clipboard error: " + err.Error()
		} else {
			st.flash = "copied " + label + ": " + truncAgentRow(text, 60)
		}
		st.flashEnd = time.Now().Add(2 * time.Second)
		return true, nil
	case "?":
		// Handled globally by model.go — let it fall through.
		return false, nil
	case "S":
		// Full-text search overlay across every indexed transcript
		// (Shift+S — `/` stays as the per-tab fuzzy filter).
		return true, m.agentsSearchOpenCmd()
	case "t":
		// View mode toggle — list ↔ tiles. Works on every Agents
		// list sub-tab (marketplaces / plugins / skills / MCPs /
		// sessions). Costs and Health have custom layouts and
		// ignore the binding silently.
		//
		// `v` is already taken by "View transcript", so the toggle
		// lives on `t` for "tiles / table".
		if st.subTab == agentsSubCosts || st.subTab == agentsSubHealth {
			return true, nil
		}
		if st.subTabViewMode == nil {
			st.subTabViewMode = make(map[int]sessionsViewMode)
		}
		st.subTabViewMode[st.subTab] = st.subTabViewMode[st.subTab].next()
		st.flash = "view: " + st.subTabViewMode[st.subTab].label()
		st.flashEnd = time.Now().Add(2 * time.Second)
		return true, nil
	case "r":
		st.loading = true
		st.flash = "refreshing…"
		st.flashEnd = time.Now().Add(2 * time.Second)
		return true, loadAgentsCmd(true)
	}
	return false, nil
}

// rowOpenURL picks the most-useful URL for `o`-to-open. Each entity
// type has a canonical "primary" link.
func rowOpenURL(r agentRow) string {
	switch {
	case r.marketplace != nil:
		return r.marketplace.URL
	case r.plugin != nil:
		if r.plugin.Homepage != "" {
			return r.plugin.Homepage
		}
		return r.plugin.Repository
	case r.mcp != nil:
		return r.mcp.URL
	}
	return ""
}

// rowCopyText returns the text that `c` should yank, along with a
// short label that goes into the flash message. Sessions/skills get
// a ready-to-run resume/invoke command; plugins+MCPs get an install
// or config snippet.
func rowCopyText(r agentRow) (text, label string) {
	switch {
	case r.session != nil:
		id := strings.TrimPrefix(r.session.ID, "claude:")
		id = strings.TrimPrefix(id, "copilot:")
		switch r.session.Provider {
		case agents.ProviderClaudeCode:
			return "claude -r " + id, "resume command"
		case agents.ProviderCopilotCLI:
			return "copilot --resume=" + id, "resume command"
		}
		return r.session.TranscriptPath, "transcript path"
	case r.skill != nil:
		return "/" + r.skill.Name, "skill invocation"
	case r.plugin != nil:
		ref := r.plugin.Name
		if r.plugin.Marketplace != "" {
			ref = r.plugin.Name + "@" + r.plugin.Marketplace
		}
		switch r.plugin.Provider {
		case agents.ProviderClaudeCode:
			return "claude plugin install " + ref, "install command"
		case agents.ProviderCopilotCLI:
			return "copilot plugin install " + ref, "install command"
		}
		return ref, "plugin ref"
	case r.mcp != nil:
		if r.mcp.URL != "" {
			return r.mcp.URL, "MCP URL"
		}
		if r.mcp.Command != "" {
			return r.mcp.Command + " " + strings.Join(r.mcp.Args, " "), "MCP command"
		}
		return r.mcp.Name, "MCP name"
	case r.marketplace != nil:
		return r.marketplace.URL, "marketplace URL"
	}
	return "", ""
}

// openBrowser opens the given URL in the user's default browser. Uses
// platform-appropriate tooling (rundll32 / open / xdg-open).
func openBrowser(url string) error {
	if url == "" {
		return errors.New("empty url")
	}
	var cmd *exec.Cmd
	switch runtimeGOOS() {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func runtimeGOOS() string { return goos }

// agentsSubTabEnterCmd returns the background loader cmd appropriate
// for entering a given Agents sub-tab. The Costs and Health sub-tabs
// have async loaders that previously only fired when the user hit
// the numeric shortcut (6 / 7); Tab navigation skipped them and left
// the page showing "press r to scan" — visually indistinguishable
// from "stuck". This helper makes Tab entry behave the same as the
// numeric shortcut. Returns nil for sub-tabs with no loader (or when
// the data is already loaded and isn't stale).
func (m *Model) agentsSubTabEnterCmd(sub int) tea.Cmd {
	st := m.agents
	if st == nil {
		return nil
	}
	switch sub {
	case agentsSubCosts:
		if st.costs.loaded || st.costs.loading {
			return nil
		}
		return m.agentsCostsLoadCmd()
	case agentsSubHealth:
		if st.healthSub.loaded || st.healthSub.loading {
			return nil
		}
		return m.agentsHealthLoadCmd()
	}
	return nil
}

func (st *agentsState) setSubTab(i int) {
	if st.subTab != i {
		// Tab change clears the previous tab's selection set so checks
		// don't leak across views.
		agentsClearSelection(st, st.subTab)
	}
	st.subTab = i
	st.cursor = 0
	st.detailOpen = false
	// Sidebar is sub-tab-specific — recompute items when switching.
	if st.sidebarOpen {
		st.sidebarItems = buildAgentsSidebarItems(st)
		st.sidebarIdx = 0
		for k, it := range st.sidebarItems {
			if !it.isHeader {
				st.sidebarIdx = k
				break
			}
		}
	}
}

func (m *Model) agentsBuildLaunchPlan() (agents.ExecPlan, bool) {
	st := m.agents
	if st == nil || st.snapshot == nil {
		return agents.ExecPlan{}, false
	}
	rows := m.agentsVisibleRows()
	if st.cursor < 0 || st.cursor >= len(rows) {
		return agents.ExecPlan{}, false
	}
	row := rows[st.cursor]
	svc := agentsService()
	prov := svc.ProviderFor(row.provider)
	if prov == nil {
		return agents.ExecPlan{}, false
	}
	spec := agents.LaunchSpec{Provider: row.provider}
	switch st.subTab {
	case agentsSubSkills:
		spec.SkillName = row.name
	case agentsSubSessions:
		spec.SessionID = row.id
	case agentsSubPlugins:
		spec.PluginName = row.name
	default:
		return agents.ExecPlan{}, false
	}
	plan, err := prov.BuildLaunch(spec)
	if err != nil {
		return agents.ExecPlan{}, false
	}
	return plan, true
}

func (m *Model) handleAgentsMsg(msg tea.Msg) (handled bool, cmd tea.Cmd) {
	if m.agents == nil {
		m.agents = newAgentsState()
	}
	st := m.agents
	switch v := msg.(type) {
	case agentsLoadedMsg:
		st.loading = false
		st.loadedAt = time.Now()
		st.snapshot = v.snap
		st.loadErr = v.err
		// Drop any lingering "refreshing…" / spinner flash and the
		// detail-page action spinner so the user sees the fresh data
		// without a stale status line on top.
		st.actionRunning = ""
		if strings.HasPrefix(st.flash, "refreshing") || strings.HasPrefix(st.flash, "scanning") {
			st.flash = ""
			st.flashEnd = time.Time{}
		}
		// Rebuild sidebar items so counts reflect the new snapshot.
		if st.sidebarOpen || len(st.sidebarItems) > 0 {
			st.sidebarItems = buildAgentsSidebarItems(st)
		}
		// Prune any detail-stack frames whose entity disappeared from
		// the refreshed snapshot (e.g. the user just deleted the
		// session that was open in detail view). Without this the
		// detail page renders "entity no longer present — press Esc
		// to return" until the user manually dismisses it, which is
		// noise: the action they took succeeded, and the screen
		// should follow them back to the list automatically.
		if st.detailPage && len(st.detailStack) > 0 {
			pruned := st.detailStack[:0]
			for _, frame := range st.detailStack {
				if _, ok := m.resolveDetailRow(frame); ok {
					pruned = append(pruned, frame)
				}
			}
			st.detailStack = pruned
			if len(st.detailStack) == 0 {
				st.detailPage = false
			}
		}
		return true, nil
	case agentsLaunchedMsg:
		if v.err != nil {
			st.flash = "launch error: " + v.err.Error()
		} else {
			st.flash = "session ended — back in klim"
		}
		st.flashEnd = time.Now().Add(4 * time.Second)
		return true, loadAgentsCmd(true)
	case agentsDeletedMsg:
		if v.err != nil {
			st.flash = "delete failed: " + v.err.Error()
			st.flashEnd = time.Now().Add(4 * time.Second)
			// Refresh anyway: a "not found" / "already deleted"
			// error often means our snapshot was stale (or the
			// user already removed the dir out of band). Without
			// the reload the failing row stays visibly on screen
			// forever and the user can't tell whether the action
			// took. The error toast still surfaces the underlying
			// failure for cases where the delete genuinely failed
			// (permissions, disk full, etc.).
			return true, loadAgentsCmd(true)
		}
		st.flash = fmt.Sprintf("deleted %s %s", v.typ, v.id)
		st.flashEnd = time.Now().Add(3 * time.Second)
		return true, loadAgentsCmd(true)
	case agentActionResultMsg:
		st.actionRunning = ""
		if v.err != nil {
			st.flash = "✗ " + v.label + ": " + v.err.Error()
			st.flashEnd = time.Now().Add(5 * time.Second)
			// Same recovery pattern as agentsDeletedMsg: refresh on
			// error so a stale row whose backing state already
			// changed (e.g. plugin uninstalled out of band) doesn't
			// linger on screen.
			return true, loadAgentsCmd(true)
		}
		st.flash = "✓ " + v.label
		st.flashEnd = time.Now().Add(3 * time.Second)
		return true, loadAgentsCmd(true)
	case agentTranscriptMsg:
		st.actionRunning = ""
		if v.err != nil {
			st.flash = "view error: " + v.err.Error()
			st.flashEnd = time.Now().Add(3 * time.Second)
			return true, nil
		}
		st.viewerOpen = true
		st.viewerTitle = v.path
		st.viewerLines = v.lines
		return true, nil
	case agentLaunchPlanMsg:
		st.actionRunning = ""
		st.launchPlan = v.plan
		st.launchPrompt = "Launch this command?"
		return true, nil
	case agentViewMarketplacePluginsMsg:
		// Pop the detail page, jump to the Plugins sub-tab, and
		// pre-filter by the marketplace the user was viewing so the
		// duplicated "plugins inside this marketplace" body list is
		// replaced by the canonical Plugins list scoped to it.
		st.actionRunning = ""
		if len(st.detailStack) == 0 {
			return true, nil
		}
		top := st.detailStack[len(st.detailStack)-1]
		row, ok := m.resolveDetailRow(top)
		if !ok || row.marketplace == nil {
			return true, nil
		}
		mpName := row.marketplace.Name
		st.detailPage = false
		st.detailStack = nil
		st.subTab = agentsSubPlugins
		st.marketplaceFilter = mpName
		st.cursor = 0
		st.sidebarItems = buildAgentsSidebarItems(st)
		st.flash = "showing plugins from " + mpName
		st.flashEnd = time.Now().Add(2 * time.Second)
		return true, nil
	case agentViewPluginSkillsMsg:
		// Pop the detail page, jump to the Skills sub-tab, and
		// pre-filter by the plugin the user was viewing so the body
		// "Contained skills" preview is replaced by the canonical
		// Skills list scoped to that plugin.
		st.actionRunning = ""
		if len(st.detailStack) == 0 {
			return true, nil
		}
		top := st.detailStack[len(st.detailStack)-1]
		row, ok := m.resolveDetailRow(top)
		if !ok || row.plugin == nil {
			return true, nil
		}
		plName := row.plugin.Name
		st.detailPage = false
		st.detailStack = nil
		st.subTab = agentsSubSkills
		st.pluginFilter = plName
		st.cursor = 0
		st.sidebarItems = buildAgentsSidebarItems(st)
		st.flash = "showing skills from " + plName
		st.flashEnd = time.Now().Add(2 * time.Second)
		return true, nil
	case agentsCostsLoadedMsg:
		st.costs.loading = false
		st.costs.loaded = true
		st.costs.loadedAt = time.Now()
		st.costs.loadErr = v.err
		st.costs.samples = v.samples
		if strings.HasPrefix(st.flash, "refreshing token") {
			st.flash = ""
			st.flashEnd = time.Time{}
		}
		return true, nil
	case agentsSearchIndexLoadedMsg:
		st.searchOverlay.Indexing = false
		st.searchOverlay.IndexErr = v.err
		st.searchOverlay.Index = v.idx
		st.searchOverlay.refreshHits()
		return true, nil
	case agentsHealthLoadedMsg:
		st.healthSub.loading = false
		st.healthSub.loaded = true
		st.healthSub.loadedAt = time.Now()
		st.healthSub.issues = v.issues
		if st.healthSub.cursor >= len(v.issues) {
			st.healthSub.cursor = 0
		}
		if strings.HasPrefix(st.flash, "running diagnostics") {
			st.flash = ""
			st.flashEnd = time.Time{}
		}
		return true, nil
	case agentsPromoteResultMsg:
		st.promotePicker.Open = false
		if v.err != nil {
			st.flash = "✗ promote: " + v.err.Error()
			st.flashEnd = time.Now().Add(5 * time.Second)
			return true, nil
		}
		st.flash = "✓ promoted → " + v.summary
		st.flashEnd = time.Now().Add(3 * time.Second)
		// Re-scan so the new entry shows up.
		return true, loadAgentsCmd(true)
	case agentsBulkResultMsg:
		// Bulk action finished; show "verb: ok/total (failed)" flash
		// and clear the selection. A re-scan picks up any state
		// changes (installed plugins, deleted sessions, etc.).
		agentsClearSelection(st, st.subTab)
		total := v.ok + v.failed
		if v.failed == 0 {
			st.flash = fmt.Sprintf("✓ %s: %d / %d", v.verb, v.ok, total)
		} else {
			extra := ""
			if v.firstErr != nil {
				extra = " — " + truncAgentRow(v.firstErr.Error(), 60)
			}
			st.flash = fmt.Sprintf("⚠ %s: %d ok, %d failed%s", v.verb, v.ok, v.failed, extra)
		}
		st.flashEnd = time.Now().Add(5 * time.Second)
		return true, loadAgentsCmd(true)
	}
	return false, nil
}

// ---------------- render ----------------

type agentRow struct {
	id       string
	name     string
	subtitle string
	provider agents.ProviderID
	source   agents.Source
	scope    agents.Scope
	enabled  bool

	// bookmarked is set on Session rows that the user has marked. The
	// renderer prefixes the title with a ★, and sortAgentRows pins
	// bookmarked sessions to the top of the list before any other
	// ordering kicks in.
	bookmarked bool
	note       string

	// Per-entity payloads — only one of these is set, matching the
	// current sub-tab. The render layer pulls richer columns from
	// these when present.
	marketplace *agents.Marketplace
	plugin      *agents.Plugin
	skill       *agents.Skill
	mcp         *agents.MCP
	session     *agents.Session
}

func (m *Model) agentsVisibleRows() []agentRow {
	st := m.agents
	if st == nil || st.snapshot == nil {
		return nil
	}
	q := strings.TrimSpace(st.searchInput)
	var rows []agentRow
	switch st.subTab {
	case agentsSubMarketplaces:
		for i := range st.snapshot.Marketplaces {
			x := st.snapshot.Marketplaces[i]
			rows = append(rows, agentRow{
				id: x.ID, name: x.Name, subtitle: x.Description,
				provider: x.Provider, source: x.Source,
				marketplace: &x,
			})
		}
	case agentsSubPlugins:
		for i := range st.snapshot.Plugins {
			x := st.snapshot.Plugins[i]
			rows = append(rows, agentRow{
				id: x.ID, name: x.Name, subtitle: x.Description,
				provider: x.Provider, source: x.Source, enabled: x.Enabled,
				plugin: &x,
			})
		}
	case agentsSubSkills:
		for i := range st.snapshot.Skills {
			x := st.snapshot.Skills[i]
			rows = append(rows, agentRow{
				id: x.ID, name: x.Name, subtitle: x.Description,
				provider: x.Provider, source: x.Source, scope: x.Scope, enabled: x.Enabled,
				skill: &x,
			})
		}
	case agentsSubMCPs:
		for i := range st.snapshot.MCPs {
			x := st.snapshot.MCPs[i]
			sub := x.URL
			if sub == "" {
				sub = x.Command
			}
			rows = append(rows, agentRow{
				id: x.ID, name: x.Name, subtitle: sub,
				provider: x.Provider, source: x.Source, scope: x.Scope, enabled: x.Enabled,
				mcp: &x,
			})
		}
	case agentsSubSessions:
		bm := agentsBookmarks(st)
		for i := range st.snapshot.Sessions {
			x := st.snapshot.Sessions[i]
			label := x.Name
			if label == "" {
				label = sessionShortID(x.ID)
			}
			row := agentRow{
				id: x.ID, name: label, subtitle: x.ProjectPath,
				provider: x.Provider, source: x.Source,
				session: &x,
			}
			if e, ok := bm.Get(x.ID); ok {
				row.bookmarked = true
				row.note = e.Note
			}
			rows = append(rows, row)
		}
	}
	rows = filterAgentRows(rows, q)
	rows = applyProviderFilter(rows, st.providerFilter)
	rows = applyMarketplaceFilter(rows, st.subTab, st.marketplaceFilter)
	rows = applyPluginFilter(rows, st.subTab, st.pluginFilter)
	rows = applyScopeFilter(rows, st.scopeFilter)
	rows = applyTransportFilter(rows, st.transportFilter)
	rows = applyStatusFilter(rows, st.subTab, st.statusFilter[st.subTab])
	rows = applyStatusValueFilter(rows, st.subTab, st.statusFilterValue)
	rows = applyBookmarkedFilter(rows, st.subTab, st.statusFilterValue)
	rows = sortAgentRows(rows, st.subTab, st.sortMode[st.subTab])
	if st.subTab == agentsSubSessions {
		rows = pinBookmarkedFirst(rows)
	}
	if st.subTab == agentsSubPlugins {
		rows = pinInstalledFirst(rows)
	}
	return rows
}

// applyBookmarkedFilter narrows session rows to bookmarked-only when
// the user picked "Bookmarked" in the sidebar STATUS section. A
// no-op on every other sub-tab.
func applyBookmarkedFilter(rows []agentRow, subTab int, statusValue string) []agentRow {
	if subTab != agentsSubSessions || statusValue != "bookmarked" {
		return rows
	}
	out := make([]agentRow, 0, len(rows))
	for _, r := range rows {
		if r.bookmarked {
			out = append(out, r)
		}
	}
	return out
}

// pinBookmarkedFirst moves bookmarked session rows to the top while
// preserving relative order within both groups.
func pinBookmarkedFirst(rows []agentRow) []agentRow {
	if len(rows) < 2 {
		return rows
	}
	var head, tail []agentRow
	for _, r := range rows {
		if r.bookmarked {
			head = append(head, r)
		} else {
			tail = append(tail, r)
		}
	}
	return append(head, tail...)
}

// pinInstalledFirst moves installed plugin rows to the top while
// preserving relative order within both groups.
func pinInstalledFirst(rows []agentRow) []agentRow {
	if len(rows) < 2 {
		return rows
	}
	var head, tail []agentRow
	for _, r := range rows {
		if r.plugin != nil && r.plugin.Installed {
			head = append(head, r)
		} else {
			tail = append(tail, r)
		}
	}
	return append(head, tail...)
}

func sessionShortID(id string) string {
	// Strip "<provider>:" prefix and keep the first 8 chars of the uuid
	// for a denser column.
	if i := strings.IndexByte(id, ':'); i >= 0 {
		id = id[i+1:]
	}
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func filterAgentRows(rows []agentRow, q string) []agentRow {
	if q == "" {
		return rows
	}
	out := make([]agentRow, 0, len(rows))
	for _, r := range rows {
		nameScore, _ := agents.FuzzyMatch(q, r.name)
		subScore, _ := agents.FuzzyMatch(q, r.subtitle)
		if nameScore > 0 || subScore > 0 {
			out = append(out, r)
		}
	}
	return out
}

// agentsSortModesFor returns the sort modes available for a sub-tab.
// Default is always included as the first cycle stop so the user can
// return to the natural order with one extra `s`.
func agentsSortModesFor(subTab int) []agentsSortMode {
	switch subTab {
	case agentsSubSessions:
		return []agentsSortMode{agentsSortDefault, agentsSortName, agentsSortCreated, agentsSortTurns}
	case agentsSubPlugins, agentsSubSkills, agentsSubMCPs:
		return []agentsSortMode{agentsSortDefault, agentsSortProvider}
	case agentsSubMarketplaces:
		return []agentsSortMode{agentsSortDefault, agentsSortModified, agentsSortProvider}
	}
	return []agentsSortMode{agentsSortDefault}
}

func nextSortMode(modes []agentsSortMode, cur agentsSortMode) agentsSortMode {
	for i, m := range modes {
		if m == cur {
			return modes[(i+1)%len(modes)]
		}
	}
	return modes[0]
}

func sortModeName(m agentsSortMode) string {
	switch m {
	case agentsSortName:
		return "name"
	case agentsSortModified:
		return "modified"
	case agentsSortCreated:
		return "created"
	case agentsSortTurns:
		return "turns"
	case agentsSortProvider:
		return "provider"
	default:
		return "default"
	}
}

// sortAgentRows reorders rows in place per the given mode. Default
// preserves the original order (matches snapshot sort: name asc for
// most entities, modified desc for sessions).
func sortAgentRows(rows []agentRow, subTab int, mode agentsSortMode) []agentRow {
	if mode == agentsSortDefault || len(rows) < 2 {
		return rows
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		switch mode {
		case agentsSortName:
			return strings.ToLower(a.name) < strings.ToLower(b.name)
		case agentsSortModified:
			ai, bi := sortRowMTime(a), sortRowMTime(b)
			return ai.After(bi)
		case agentsSortCreated:
			ai, bi := sortRowCTime(a), sortRowCTime(b)
			return ai.After(bi)
		case agentsSortTurns:
			at, bt := 0, 0
			if a.session != nil {
				at = a.session.TurnCount
			}
			if b.session != nil {
				bt = b.session.TurnCount
			}
			return at > bt
		case agentsSortProvider:
			return string(a.provider) < string(b.provider)
		}
		return false
	})
	return rows
}

func sortRowMTime(r agentRow) time.Time {
	switch {
	case r.session != nil:
		return r.session.LastModified
	case r.marketplace != nil:
		return r.marketplace.LastSynced
	}
	return time.Time{}
}

func sortRowCTime(r agentRow) time.Time {
	if r.session != nil {
		return r.session.Created
	}
	return time.Time{}
}

// agentsProviderStyle returns a lipgloss style for the short provider
// label. Same palette across the TUI so users can map color→provider
// at a glance.
func agentsProviderStyle(id agents.ProviderID) lipgloss.Style {
	base := lipgloss.NewStyle().Bold(true)
	return base.Foreground(providerBrandFG(id))
}

// providerBrandFG returns the brand foreground for a provider —
// Anthropic orange for Claude, Copilot purple for Copilot.
func providerBrandFG(id agents.ProviderID) color.Color {
	switch id {
	case agents.ProviderClaudeCode:
		return claudeBrand
	case agents.ProviderCopilotCLI:
		return copilotBrand
	}
	return cyberFG
}

// agentsScopeStyle dims user scope and bolds project scope so a
// project entity stands out in a list dominated by user-wide entries.
func agentsScopeStyle(s agents.Scope) lipgloss.Style {
	base := lipgloss.NewStyle()
	switch s {
	case agents.ScopeProject:
		return base.Foreground(cyberAccent).Bold(true)
	case agents.ScopeRemote:
		return base.Foreground(cyberInfo)
	}
	return base.Foreground(cyberFGDim)
}

func (m *Model) renderAgentsView() string {
	if m.agents == nil {
		m.agents = newAgentsState()
	}
	st := m.agents
	var b strings.Builder

	subs := []string{"Marketplaces", "Plugins", "Skills", "MCPs", "Sessions", "Costs", "Health"}
	counts := agentsSnapshotCounts(st.snapshot)
	var parts []string
	for i, label := range subs {
		label := fmt.Sprintf("%s (%d)", label, counts[i])
		if i == st.subTab {
			parts = append(parts, cyberSubtabActive(label))
		} else {
			parts = append(parts, cyberSubtabInactive(label))
		}
	}
	b.WriteString("  " + strings.Join(parts, "  ") + "\n")

	// Cache age + provider status line.
	if st.snapshot != nil && !st.loadedAt.IsZero() {
		age := dimVersion.Render("scanned " + humaniseTime(st.loadedAt))
		providersLine := agentsProviderHealth(st.snapshot)
		b.WriteString("  " + providersLine + "  " + age + "\n")
	}
	b.WriteString("\n")

	// Lazily build the sidebar so it's visible from the first render.
	// `f` opens the picker; otherwise it stays inert with the current
	// selection highlighted.
	if st.snapshot != nil && len(st.sidebarItems) == 0 {
		st.sidebarItems = buildAgentsSidebarItems(st)
	}

	switch {
	case st.searchActive:
		b.WriteString("  search: " + st.searchInput + "▌\n")
	case st.searchInput != "":
		b.WriteString("  filter: " + st.searchInput + "  " + dimVersion.Render("/ edit · esc clear"))
		b.WriteString("\n")
	default:
		// Single, subtle hint — full keymap lives behind ?. Every
		// list sub-tab gets the `t` toggle hint with its current
		// label so users see at a glance what mode they're in.
		hint := "/ filter · s sort · S search transcripts · ? help"
		if st.subTab != agentsSubCosts && st.subTab != agentsSubHealth {
			label := st.subTabViewMode[st.subTab].label()
			hint = "/ filter · s sort · t " + label + " view · S search · ? help"
		}
		b.WriteString("  " + dimVersion.Render(hint) + "\n")
	}

	// Active-filter chip row — only rendered when at least one filter
	// is set. Keeps the UI clean when filters are off.
	if chips := agentsFilterChips(st); chips != "" {
		b.WriteString("  " + chips + "\n")
	}
	// Bulk-selection summary row — only rendered when something is selected.
	if summary := agentsBulkRenderSummary(st); summary != "" {
		b.WriteString("  " + summary + "\n")
	}
	b.WriteString("\n")

	if st.loading && st.snapshot == nil {
		b.WriteString("  scanning agent ecosystem…\n")
		return b.String()
	}
	if st.loadErr != nil {
		b.WriteString("  load error: " + st.loadErr.Error() + "\n")
		return b.String()
	}

	// Costs sub-tab renders its own body — skip the row/table layout.
	if st.subTab == agentsSubCosts {
		b.WriteString(m.renderAgentsCostsView())
		if st.flash != "" && time.Now().Before(st.flashEnd) {
			b.WriteString("\n  " + st.flash + "\n")
		}
		return b.String()
	}
	// Health sub-tab renders its own body too.
	if st.subTab == agentsSubHealth {
		b.WriteString(m.renderAgentsHealthView())
		if st.flash != "" && time.Now().Before(st.flashEnd) {
			b.WriteString("\n  " + st.flash + "\n")
		}
		return b.String()
	}

	// Full-text search overlay replaces the table body so it appears
	// inline (not pushed to the bottom of the view).
	if st.searchOverlay.Open {
		b.WriteString(m.renderAgentsSearchOverlay())
		return b.String()
	}

	// Transcript viewer modal replaces the table body. The previous
	// implementation appended the modal after the table at the very
	// bottom — but the outer view wrapper clips the rendered body
	// from the bottom to fit `visibleRows`, which dropped the modal
	// off-screen. Render it as the body instead so it always sits
	// inside the visible viewport.
	if st.viewerOpen {
		b.WriteString(renderTranscriptViewer(st.viewerTitle, st.viewerLines, m.width))
		if st.flash != "" && time.Now().Before(st.flashEnd) {
			b.WriteString("\n  " + st.flash + "\n")
		}
		return b.String()
	}

	rows := m.agentsVisibleRows()
	if len(rows) == 0 {
		switch st.subTab {
		case agentsSubMarketplaces:
			b.WriteString("  no marketplaces detected — `klim agents marketplaces add <url>`\n")
		case agentsSubPlugins:
			b.WriteString("  no plugins installed — `klim agents plugins install <ref>`\n")
		case agentsSubSkills:
			b.WriteString("  no skills found in ~/.claude/skills or ~/.copilot/skills\n")
		case agentsSubMCPs:
			b.WriteString("  no MCP servers configured\n")
		case agentsSubSessions:
			b.WriteString("  no saved sessions detected yet\n")
		}
	}

	// Budget the table window from actual terminal height so the
	// bottom of the body isn't silently clipped by the outer view
	// wrapper. The header / sub-tab strip / filter chips / footer
	// together consume ~12 rows; everything else is data the user
	// is here to read. Floor at 6 rows so very small terminals still
	// show something usable.
	maxRows := m.height - 12
	if maxRows > 25 {
		maxRows = 25
	}
	if maxRows < 6 {
		maxRows = 6
	}
	// Tile mode budgets differently: each card is `tileHeight` rows
	// tall and we lay them out in 1–3 columns. Translate the visual
	// row budget into an entity-count budget so windowAgentRows
	// returns enough rows to fill the available tile rows without
	// overflowing them. Reserve 2 lines for the up/down indicator
	// rows ("↑ N above" / "↓ N below") so a near-full grid with
	// both indicators visible doesn't push past maxRows and get
	// clipped mid-card.
	tileCols := 1
	if st.subTabViewMode[st.subTab] == sessionsViewTiles {
		tileTotalW := m.width - agentsSidebarColWidth - 3
		_, cols := chooseTileLayout(tileTotalW)
		tileCols = cols
		const tileIndicatorBudget = 2
		tileRows := (maxRows - tileIndicatorBudget) / tileHeight
		if tileRows < 1 {
			tileRows = 1
		}
		maxRows = tileRows * cols
	}
	// In tile mode we snap the window start to a column boundary
	// (tileCols) so navigating doesn't reshuffle the visible tiles
	// every keystroke. For the dense table tileCols==1 makes this
	// a no-op vs. the original centred-cursor behaviour.
	visible, windowStart, displayCursor, hiddenAbove, hiddenBelow := windowAgentRowsAligned(rows, st.cursor, maxRows, tileCols)
	_ = windowStart

	// Build the table area into its own buffer so we can lay it out
	// side-by-side with the sidebar when present.
	var tbl strings.Builder
	if hiddenAbove > 0 {
		fmt.Fprintf(&tbl, "  %s\n", dimVersion.Render(fmt.Sprintf("↑ %d above", hiddenAbove)))
	}
	// On every Agents list sub-tab, the user can toggle between the
	// dense table and a tile grid via the `t` keybinding. Sessions
	// keeps its own session-specific tile renderer; other sub-tabs
	// use the generic agents-entity tile renderer (marketplaces,
	// plugins, skills, MCPs).
	if st.subTabViewMode[st.subTab] == sessionsViewTiles {
		if st.subTab == agentsSubSessions {
			tbl.WriteString(renderSessionTiles(visible, displayCursor, m.width-agentsSidebarColWidth-3))
		} else {
			tbl.WriteString(renderAgentsTiles(visible, displayCursor, m.width-agentsSidebarColWidth-3))
		}
	} else {
		tbl.WriteString(renderAgentsTable(st.subTab, visible, displayCursor, st.sortMode[st.subTab], m.width-agentsSidebarColWidth-3))
	}
	if hiddenBelow > 0 {
		fmt.Fprintf(&tbl, "  %s\n", dimVersion.Render(fmt.Sprintf("↓ %d below", hiddenBelow)))
	}
	if st.detailOpen && st.cursor < len(rows) {
		tbl.WriteString("\n  ─── detail ───\n")
		tbl.WriteString(renderAgentDetail(rows[st.cursor], st.snapshot))
	}

	// Lay out sidebar | table when the sidebar has items.
	if len(st.sidebarItems) > 0 {
		tableLines := strings.Split(strings.TrimRight(tbl.String(), "\n"), "\n")
		sideLines := buildAgentsSidebarLines(st, len(tableLines)+4)
		total := len(tableLines)
		if len(sideLines) > total {
			total = len(sideLines)
		}
		for i := 0; i < total; i++ {
			left := ""
			if i < len(sideLines) {
				left = sideLines[i]
			}
			right := ""
			if i < len(tableLines) {
				right = tableLines[i]
			}
			b.WriteString(fixedWidthANSI(left, agentsSidebarColWidth) + " │ " + right + "\n")
		}
	} else {
		b.WriteString(tbl.String())
	}

	if st.flash != "" && time.Now().Before(st.flashEnd) {
		b.WriteString("\n  " + st.flash + "\n")
	}

	if st.launchPrompt != "" {
		b.WriteString("\n  ╔ Launch ════════════════════════════════════════════╗\n")
		b.WriteString("  ║ " + st.launchPrompt + "\n")
		b.WriteString("  ║ $ " + st.launchPlan.CommandLine() + "\n")
		if st.launchPlan.Note != "" {
			b.WriteString("  ║ " + st.launchPlan.Note + "\n")
		}
		b.WriteString("  ║ y/Enter = run · n/Esc = cancel\n")
		b.WriteString("  ╚════════════════════════════════════════════════════╝\n")
	}

	if st.deleteTarget != "" {
		b.WriteString("\n  ╔ Confirm delete ══════════════════════════════════════╗\n")
		b.WriteString("  ║ Delete " + st.deleteTarget + "?\n")
		b.WriteString("  ║ y/Enter = delete · n/Esc = cancel\n")
		b.WriteString("  ╚══════════════════════════════════════════════════════╝\n")
	}

	if st.bulkPrompt != "" {
		b.WriteString(renderBulkConfirmPrompt(st))
	}

	// Note: when st.viewerOpen is true we render the viewer earlier
	// (above the rows table) and return immediately, so we don't
	// append it again here — the previous trailing block was clipped
	// off-screen by the outer view's bottom-truncating fitter.

	if st.noteOpen {
		b.WriteString(renderAgentNotePrompt(st))
	}

	return b.String()
}

// renderAgentNotePrompt renders the inline single-line editor used to
// attach (or edit) a note on a bookmarked session.
func renderAgentNotePrompt(st *agentsState) string {
	var b strings.Builder
	header := lipgloss.NewStyle().Foreground(cyberAccent).Bold(true).Render("📝 note")
	id := truncAgentRow(st.noteTarget, 40)
	b.WriteString("\n  ╔ " + header + "  " + dimVersion.Render(id) + " ═════════════════╗\n")
	b.WriteString("  ║ " + st.noteBuffer + lipgloss.NewStyle().Foreground(cyberAccent).Render("▌") + "\n")
	b.WriteString("  ║ " + dimVersion.Render("Enter = save · Esc = cancel · Backspace = edit") + "\n")
	b.WriteString("  ╚════════════════════════════════════════════════════════╝\n")
	return b.String()
}

// renderTranscriptViewer renders the session-transcript viewer modal.
// Both the list view (renderAgentsView) and the detail page
// (renderAgentsDetailPage) call this when st.viewerOpen is true.
//
// History: the original implementation drew a hand-rolled box with
// hard-coded 56-char top/bottom borders and 80-char content
// truncation, then was APPENDED at the very bottom of the body —
// past the height-padding in renderAgentsDetailPage and past
// fitToVisibleRows' bottom-clip in renderAgentsView. Both paths
// pushed the modal off-screen entirely.
//
// The current implementation uses lipgloss' `RoundedBorder()` and
// `Width(totalWidth - 4)` so the borders always align with the
// terminal width, drops the hard-coded truncation in favour of
// truncating to the actual available content width, and colours
// each row by its role chip (`user` / `assistant` / `tool`) for
// the same hierarchy the search-overlay viewer provides. Callers
// render the box in place of the body content (like the existing
// searchOverlay pattern) so it always lands inside the visible
// viewport.
func renderTranscriptViewer(title string, lines []string, totalWidth int) string {
	if totalWidth <= 0 {
		totalWidth = 80
	}
	const minWidth = 40
	if totalWidth < minWidth {
		totalWidth = minWidth
	}
	boxWidth := totalWidth - 4
	// Inner content width: lipgloss Padding(0, 1) eats 1 cell each
	// side, the rounded border eats 1 cell each side. Used to size
	// the per-row text so a "[user] …" line fits without wrapping.
	innerW := boxWidth - 4
	if innerW < 20 {
		innerW = 20
	}

	box := lipgloss.NewStyle().
		Foreground(cyberFG).
		Background(cyberSelectedBg).
		BorderForeground(cyberAccent).
		BorderStyle(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(boxWidth)

	header := lipgloss.NewStyle().Bold(true).Foreground(cyberPrimary).Render("📜 Transcript") +
		"  " + dimVersion.Render(truncAgentRow(title, innerW-16))

	out := []string{header, ""}

	if len(lines) == 0 {
		out = append(out, dimVersion.Render("(empty transcript — no events recorded yet)"))
	}
	for _, raw := range lines {
		out = append(out, renderTranscriptRow(raw, innerW))
	}

	out = append(out, "", dimVersion.Render(fmt.Sprintf("%d lines · Esc / Enter / q = close", len(lines))))
	return box.Render(strings.Join(out, "\n"))
}

// renderTranscriptRow colours a single transcript line based on the
// role prefix that renderTranscriptLine emits (`[user]`, `[assistant]`,
// `[tool]`). Lines without a recognised prefix fall through with
// the dim foreground so JSONL noise (rare in practice) is still
// readable but visually demoted.
func renderTranscriptRow(line string, innerW int) string {
	role, rest := splitTranscriptRolePrefix(line)
	if role == "" {
		// No recognised prefix — render as dim raw text.
		return lipgloss.NewStyle().Foreground(cyberFGDim).
			Render(truncAgentRow(line, innerW))
	}
	chip := transcriptRoleChip(role)
	// innerW budget: chip + 1-space gap + text.
	textW := innerW - lipgloss.Width(chip) - 1
	if textW < 8 {
		textW = 8
	}
	textStyle := lipgloss.NewStyle().Foreground(cyberFG)
	if role == "tool" {
		textStyle = textStyle.Foreground(cyberFGDim)
	}
	return chip + " " + textStyle.Render(truncAgentRow(rest, textW))
}

// splitTranscriptRolePrefix splits a "[role] body" line produced by
// renderTranscriptLine into (role, body). Returns ("", line) when
// the line doesn't start with a recognised role prefix.
func splitTranscriptRolePrefix(line string) (role, rest string) {
	switch {
	case strings.HasPrefix(line, "[user] "):
		return "user", strings.TrimPrefix(line, "[user] ")
	case strings.HasPrefix(line, "[assistant] "):
		return "assistant", strings.TrimPrefix(line, "[assistant] ")
	case strings.HasPrefix(line, "[tool]"):
		// renderTranscriptLine emits "[tool]      Name(...)" with
		// extra padding so a column aligns in plain text — strip it.
		rest := strings.TrimPrefix(line, "[tool]")
		return "tool", strings.TrimLeft(rest, " ")
	}
	return "", line
}

func truncAgentRow(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n < 4 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

// providerShort renders a compact, recognisable label for a ProviderID.
// Used as the leading SOURCE column in every sub-tab so the user can see
// at a glance whether an entity came from Claude Code, Copilot CLI, the
// MCP registry, or a future provider.
func providerShort(id agents.ProviderID) string {
	switch id {
	case agents.ProviderClaudeCode:
		return "claude"
	case agents.ProviderCopilotCLI:
		return "copilot"
	default:
		return string(id)
	}
}

// agentsSnapshotCounts returns the per-sub-tab counts. Returns zeros
// when snapshot is nil (mid-load). The last (Health) slot is filled
// in by the Health sub-tab itself with `errors+warnings`.
func agentsSnapshotCounts(s *agents.Snapshot) [7]int {
	if s == nil {
		return [7]int{}
	}
	return [7]int{
		len(s.Marketplaces),
		len(s.Plugins),
		len(s.Skills),
		len(s.MCPs),
		len(s.Sessions),
		0,
		0,
	}
}

// agentsProviderHealth renders a compact provider-status pill row:
//
//	claude ✓1.2.0   copilot ✓1.0.48   mcp-reg ✓
//
// Each pill is colored by provider; failing providers are dimmed.
func agentsProviderHealth(s *agents.Snapshot) string {
	if s == nil {
		return ""
	}
	order := []agents.ProviderID{agents.ProviderClaudeCode, agents.ProviderCopilotCLI}
	var pills []string
	for _, id := range order {
		st, ok := s.ProviderStatus[id]
		if !ok {
			continue
		}
		label := providerShort(id)
		if st.Installed {
			label += " ✓"
			pills = append(pills, agentsProviderStyle(id).Render(label))
		} else {
			label += " ✗"
			pills = append(pills, lipgloss.NewStyle().Foreground(cyberFGDim).Render(label))
		}
	}
	return strings.Join(pills, "  ")
}

// renderScope colors the scope tag.
func renderScope(s agents.Scope) string {
	if s == "" {
		return "—"
	}
	return agentsScopeStyle(s).Render(string(s))
}

// column describes one column in a sub-tab's table.
type column struct {
	header string
	width  int  // fixed width; ignored when grow=true
	grow   bool // the column expands to fill remaining terminal width
}

// computeColumnWidths returns a copy of the cols slice with widths
// resolved against the terminal's totalWidth. A column with grow=true
// receives `totalWidth - sum(fixed) - gaps - leadPad`, clamped to a
// reasonable minimum so descriptions don't collapse to zero.
func computeColumnWidths(cols []column, totalWidth int) []column {
	const leadPad = 4
	const gap = 2
	const minGrow = 16

	out := make([]column, len(cols))
	copy(out, cols)

	if totalWidth <= 0 {
		// No width info — fall back to a sensible default (older
		// behaviour for tests / pre-WindowSizeMsg renders).
		for i := range out {
			if out[i].grow {
				out[i].width = 50
			}
		}
		return out
	}

	fixed := 0
	growIdx := -1
	for i, c := range out {
		if c.grow {
			growIdx = i
			continue
		}
		fixed += c.width
	}
	if growIdx < 0 {
		return out
	}
	gaps := gap * (len(out) - 1)
	available := totalWidth - leadPad - fixed - gaps
	if available < minGrow {
		// CRITICAL: if minGrow would push the row past totalWidth,
		// every row wraps to 2 visual rows and the footer alignment
		// math (which budgets visibleRows based on line count, not
		// visual rows) falls apart. The Sessions sub-tab hit this
		// at any terminal width up to ~145 cols because it has
		// 8 columns / 7 gaps + an em-dash heavy table. Better to
		// squeeze the grow column hard than to wrap the whole row.
		//
		// Floor at 0 (not 1) so totalWidth that exactly equals the
		// fixed+gaps total still doesn't overflow — the grow column
		// just disappears.
		if available < 0 {
			available = 0
		}
	}
	out[growIdx].width = available
	return out
}

// renderRow concatenates cells into a row with each cell padded to the
// configured column width. lipgloss.NewStyle().Width is ANSI-aware so
// colored content stays aligned (unlike text/tabwriter).
//
// When selected is true the entire row is wrapped in
// cyberSelectedRowStyle, with the row first padded out to totalWidth
// so the background fill covers the whole horizontal strip — the same
// pattern as renderPackageManagers in view_detail.go.
func renderRow(cells []string, cols []column, lead string, selected bool, totalWidth int) string {
	var inner strings.Builder
	inner.WriteString(lead)
	for i, c := range cells {
		w := cols[i].width
		if w == 0 {
			inner.WriteString(c)
		} else {
			inner.WriteString(lipgloss.NewStyle().Width(w).Render(c))
		}
		if i < len(cells)-1 {
			inner.WriteString("  ")
		}
	}
	line := inner.String()

	if selected {
		w := lipgloss.Width(line)
		if totalWidth > w {
			line += strings.Repeat(" ", totalWidth-w)
		}
		line = cyberSelectedRowStyle.Render(line)
	}
	return line + "\n"
}

// renderHeader builds the table header row. When sortColumn is >= 0
// it appends a tiny ▼ arrow to that column's title so users can see
// at a glance which column drives the current order.
func renderHeader(cols []column, sortColumn int) string {
	cells := make([]string, len(cols))
	for i, c := range cols {
		title := c.header
		if i == sortColumn {
			title += " ▼"
		}
		cells[i] = headerStyle.Render(title)
	}
	return renderRow(cells, cols, "    ", false, 0)
}

// agentsProviderChip renders the provider identifier as a coloured
// circle followed by the short name. The dot uses each provider's
// brand colour (Anthropic orange for Claude, GitHub Copilot purple
// for Copilot, amber for the MCP registry); the label is bolded to
// stay legible against the table's dim text.
func agentsProviderChip(id agents.ProviderID) string {
	label := providerShort(id)
	dot := lipgloss.NewStyle().Foreground(providerBrandFG(id)).Render("●")
	name := lipgloss.NewStyle().Foreground(cyberFG).Bold(true).Render(label)
	return dot + " " + name
}

// agentsStatusChip is a colored chip for session status. Empty status
// renders as a single dim em-dash without a background fill.
func agentsStatusChip(s agents.SessionStatus) string {
	if s == "" {
		return lipgloss.NewStyle().Foreground(cyberFGDim).Render("—")
	}
	fg := statusChipFG(s)
	return lipgloss.NewStyle().
		Foreground(fg).
		Background(cyberChipBg).
		Padding(0, 1).
		Render(string(s))
}

func statusChipFG(s agents.SessionStatus) color.Color {
	switch s {
	case agents.SessionStatusActive:
		return cyberOK
	case agents.SessionStatusCompleted:
		return cyberFGDim
	case agents.SessionStatusStopped:
		return cyberAlert
	}
	return cyberFGDim
}

// sortColumnFor returns the column index that the given sort mode acts
// on for the given sub-tab, or -1 when the sort is the default order
// (no specific column to mark).
func sortColumnFor(subTab int, mode agentsSortMode) int {
	if mode == agentsSortDefault {
		return -1
	}
	switch subTab {
	case agentsSubMarketplaces, agentsSubPlugins, agentsSubSkills, agentsSubMCPs:
		if mode == agentsSortName {
			return 1
		}
	case agentsSubSessions:
		// Sessions columns: SOURCE(0) ID(1) TITLE(2) TYPE(3) STATUS(4)
		// TURNS(5) MODIFIED(6) PROJECT(7). Created column was dropped
		// in favour of TITLE; map agentsSortCreated to MODIFIED.
		switch mode {
		case agentsSortName:
			return 1
		case agentsSortCreated:
			return 6
		case agentsSortTurns:
			return 5
		case agentsSortModified:
			return 6
		}
	}
	return -1
}

// windowAgentRows centers a visible window of `maxRows` around the cursor
// and returns the slice plus the cursor position within the slice and
// the counts hidden above/below for the scroll indicators.
func windowAgentRows(rows []agentRow, cursor, maxRows int) (visible []agentRow, start, displayCursor, hiddenAbove, hiddenBelow int) {
	return windowAgentRowsAligned(rows, cursor, maxRows, 1)
}

// windowAgentRowsAligned is the tile-aware variant of windowAgentRows.
// When `colAlign` > 1 (used by the tile grid renderers), `start` is
// snapped to a multiple of colAlign so the visible window scrolls
// by full rows of tiles rather than one tile at a time.
//
// Bug history: with the old centred-cursor windowing, moving the
// cursor by one tile in a 3-column grid shifted every visible tile
// diagonally — A,B,C / D,E,F / G,H,I became B,C,D / E,F,G / H,I,J
// on a single ↓ keypress. Users couldn't track the cursor because
// every tile reshuffled position underneath them. Snapping start
// to a column boundary keeps the grid stationary until the cursor
// actually leaves the visible window, then pages by a full row of
// tiles.
func windowAgentRowsAligned(rows []agentRow, cursor, maxRows, colAlign int) (visible []agentRow, start, displayCursor, hiddenAbove, hiddenBelow int) {
	n := len(rows)
	if n == 0 {
		return nil, 0, 0, 0, 0
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= n {
		cursor = n - 1
	}
	if n <= maxRows {
		return rows, 0, cursor, 0, 0
	}
	if colAlign < 1 {
		colAlign = 1
	}

	// Center the cursor in the window when possible.
	start = cursor - maxRows/2
	if start < 0 {
		start = 0
	}
	if start+maxRows > n {
		start = n - maxRows
	}
	// Snap start to a row boundary so a multi-column tile grid
	// shifts by whole rows. For 1-column callers (the dense table)
	// colAlign==1 makes this a no-op.
	if colAlign > 1 {
		start = (start / colAlign) * colAlign
		if start+maxRows > n {
			// After snapping, the window might extend past the
			// end. Pull start back to the last aligned position
			// that still fits, but never go past 0.
			lastAligned := ((n - maxRows) / colAlign) * colAlign
			if lastAligned < 0 {
				lastAligned = 0
			}
			start = lastAligned
		}
	}
	end := start + maxRows
	if end > n {
		end = n
	}
	visible = rows[start:end]
	displayCursor = cursor - start
	hiddenAbove = start
	hiddenBelow = n - end
	return
}

// renderAgentsTable produces an aligned table for the current sub-tab.
// Uses lipgloss fixed-width cells so colored content (SOURCE chips,
// STATUS pills, SCOPE tags) stays in line — text/tabwriter counted
// ANSI escapes as visible width and pushed columns out of alignment.
//
// The activeSort argument drives a small ↑/↓ indicator on the column
// that's currently sorted so users can see the order at a glance.
func renderAgentsTable(subTab int, rows []agentRow, cursor int, activeSort agentsSortMode, totalWidth int) string {
	var b strings.Builder

	switch subTab {
	case agentsSubMarketplaces:
		cols := computeColumnWidths([]column{
			{header: "SOURCE", width: 10},
			{header: "NAME", width: 28},
			{header: "STATUS", width: 11},
			{header: "OWNER", width: 14},
			{header: "PLUGINS", width: 8},
			{header: "URL", grow: true},
		}, totalWidth)
		b.WriteString(renderHeader(cols, sortColumnFor(subTab, activeSort)))
		for i, r := range rows {
			mp := r.marketplace
			owner, url, count, status := "", "", 0, "available"
			if mp != nil {
				owner, url, count = mp.Owner, mp.URL, mp.PluginCount
				if mp.Installed {
					status = "installed"
				}
			}
			cells := []string{
				agentsProviderChip(r.provider),
				truncAgentRow(r.name, cols[1].width),
				agentsPluginStatusChip(status),
				truncAgentRow(owner, cols[3].width),
				dashOrInt(count),
				truncAgentRow(url, cols[5].width),
			}
			b.WriteString(renderRow(cells, cols, rowLead(i, cursor), i == cursor, totalWidth))
		}
	case agentsSubPlugins:
		cols := computeColumnWidths([]column{
			{header: "SOURCE", width: 10},
			{header: "NAME", width: 26},
			{header: "VERSION", width: 10},
			{header: "MARKETPLACE", width: 24},
			{header: "STATUS", width: 12},
			{header: "DESCRIPTION", grow: true},
		}, totalWidth)
		b.WriteString(renderHeader(cols, sortColumnFor(subTab, activeSort)))
		for i, r := range rows {
			pl := r.plugin
			version, market, status, desc := "", "", "available", ""
			if pl != nil {
				version, market, desc = pl.Version, pl.Marketplace, pl.Description
				switch {
				case pl.Installed && pl.Enabled:
					status = "installed"
				case pl.Installed:
					status = "disabled"
				}
			}
			cells := []string{
				agentsProviderChip(r.provider),
				truncAgentRow(r.name, cols[1].width),
				truncAgentRow(version, cols[2].width),
				truncAgentRow(market, cols[3].width),
				agentsPluginStatusChip(status),
				truncAgentRow(desc, cols[5].width),
			}
			b.WriteString(renderRow(cells, cols, rowLead(i, cursor), i == cursor, totalWidth))
		}
	case agentsSubSkills:
		cols := computeColumnWidths([]column{
			{header: "SOURCE", width: 10},
			{header: "NAME", width: 32},
			{header: "SCOPE", width: 10},
			{header: "FROM", width: 18},
			{header: "MODEL", width: 10},
			{header: "DESCRIPTION", grow: true},
		}, totalWidth)
		b.WriteString(renderHeader(cols, sortColumnFor(subTab, activeSort)))
		for i, r := range rows {
			sk := r.skill
			model, from, desc := "", "", ""
			if sk != nil {
				model, from, desc = sk.Model, sk.SourcePlugin, sk.Description
				if from == "" {
					from = string(sk.Scope)
				}
			}
			cells := []string{
				agentsProviderChip(r.provider),
				truncAgentRow(r.name, cols[1].width),
				renderScope(r.scope),
				truncAgentRow(from, cols[3].width),
				truncAgentRow(model, cols[4].width),
				truncAgentRow(desc, cols[5].width),
			}
			b.WriteString(renderRow(cells, cols, rowLead(i, cursor), i == cursor, totalWidth))
		}
	case agentsSubMCPs:
		cols := computeColumnWidths([]column{
			{header: "SOURCE", width: 10},
			{header: "NAME", width: 34},
			{header: "TRANSPORT", width: 10},
			{header: "SCOPE", width: 10},
			{header: "TOOLS", width: 6},
			{header: "ENDPOINT", grow: true},
		}, totalWidth)
		b.WriteString(renderHeader(cols, sortColumnFor(subTab, activeSort)))
		for i, r := range rows {
			mcp := r.mcp
			transport, endpoint, tools := "", "", 0
			if mcp != nil {
				transport = mcp.Transport
				endpoint = mcp.URL
				if endpoint == "" {
					endpoint = mcp.Command
				}
				tools = len(mcp.Tools)
			}
			cells := []string{
				agentsProviderChip(r.provider),
				truncAgentRow(r.name, cols[1].width),
				transport,
				renderScope(r.scope),
				dashOrInt(tools),
				truncAgentRow(endpoint, cols[5].width),
			}
			b.WriteString(renderRow(cells, cols, rowLead(i, cursor), i == cursor, totalWidth))
		}
	case agentsSubSessions:
		cols := computeColumnWidths([]column{
			{header: "SOURCE", width: 10},
			{header: "ID", width: 10},
			{header: "TITLE", grow: true},
			{header: "TYPE", width: 11},
			{header: "STATUS", width: 9},
			{header: "TURNS", width: 5},
			{header: "MODIFIED", width: 11},
			{header: "PROJECT", width: 26},
		}, totalWidth)
		b.WriteString(renderHeader(cols, sortColumnFor(subTab, activeSort)))
		for i, r := range rows {
			s := r.session
			typ, project := "", r.subtitle
			modified := ""
			title := ""
			var status agents.SessionStatus
			turns := 0
			if s != nil {
				typ, project, turns = s.Type, s.ProjectPath, s.TurnCount
				if typ == "" {
					typ = "interactive"
				}
				status = s.Status
				modified = humaniseTime(s.LastModified)
				title = s.Title
				if title == "" {
					title = s.Name
				}
				if title == "" {
					title = dimVersion.Render("(untitled)")
				}
			}
			cells := []string{
				agentsProviderChip(r.provider),
				truncAgentRow(r.name, cols[1].width),
				truncAgentRow(formatSessionTitle(r, title), cols[2].width),
				typ,
				agentsStatusChip(status),
				dashOrInt(turns),
				modified,
				truncAgentRow(project, cols[7].width),
			}
			b.WriteString(renderRow(cells, cols, rowLead(i, cursor), i == cursor, totalWidth))
		}
	}
	return b.String()
}

// agentsPluginStatusChip returns a colored chip for the plugin status
// label (installed / disabled / available). Same chip vocabulary as
// agentsStatusChip for visual consistency.
func agentsPluginStatusChip(status string) string {
	fg := pluginStatusChipFG(status)
	return lipgloss.NewStyle().
		Foreground(fg).
		Background(cyberChipBg).
		Padding(0, 1).
		Render(status)
}

func pluginStatusChipFG(status string) color.Color {
	switch status {
	case "installed":
		return cyberOK
	case "disabled":
		return cyberFGDim
	}
	return cyberAccent // "available"
}

// rowLead returns the lead string for the row. The cursor row gets a
// "▸" marker; non-cursor rows are blank-padded to the same width so
// the columns line up.
func rowLead(i, cursor int) string {
	if i == cursor {
		return "  ▸ "
	}
	return "    "
}

func dashOrInt(n int) string {
	if n <= 0 {
		return "—"
	}
	return strconv.Itoa(n)
}

// formatSessionTitle prefixes the title with a ★ when the session is
// bookmarked and appends a small "📝" hint when a note is present.
func formatSessionTitle(r agentRow, title string) string {
	prefix := ""
	if r.bookmarked {
		prefix = lipgloss.NewStyle().Foreground(cyberAccent).Bold(true).Render("★ ")
	}
	suffix := ""
	if r.note != "" {
		suffix = " " + lipgloss.NewStyle().Foreground(cyberInfo).Render("📝")
	}
	return prefix + title + suffix
}

// humaniseTime renders a time in a column-friendly form. The output is
// kept ≤11 chars so it fits in narrow CREATED/MODIFIED columns without
// wrapping (which previously corrupted the Sessions table).
func humaniseTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	now := time.Now()
	diff := now.Sub(t)
	switch {
	case diff < 0:
		return t.Format("2006-01-02")
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	case diff < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(diff.Hours()/24))
	default:
		return t.Format("2006-01-02")
	}
}

// renderAgentDetail builds a rich detail block per entity type.
func renderAgentDetail(r agentRow, snap *agents.Snapshot) string {
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)

	switch {
	case r.marketplace != nil:
		m := r.marketplace
		_, _ = fmt.Fprintf(tw, "  name\t%s\n", m.Name)
		if m.DisplayName != "" && m.DisplayName != m.Name {
			_, _ = fmt.Fprintf(tw, "  display\t%s\n", m.DisplayName)
		}
		_, _ = fmt.Fprintf(tw, "  provider\t%s\n", m.Provider)
		if m.Owner != "" {
			_, _ = fmt.Fprintf(tw, "  owner\t%s\n", m.Owner)
		}
		if m.URL != "" {
			_, _ = fmt.Fprintf(tw, "  url\t%s\n", m.URL)
		}
		if m.PluginCount > 0 {
			_, _ = fmt.Fprintf(tw, "  plugins\t%d\n", m.PluginCount)
		}
		if m.Description != "" {
			_, _ = fmt.Fprintf(tw, "  description\t%s\n", m.Description)
		}
		_, _ = fmt.Fprintf(tw, "  source\t%s\n", m.Source)
	case r.plugin != nil:
		p := r.plugin
		_, _ = fmt.Fprintf(tw, "  name\t%s\n", p.Name)
		if p.Version != "" {
			_, _ = fmt.Fprintf(tw, "  version\t%s\n", p.Version)
		}
		_, _ = fmt.Fprintf(tw, "  provider\t%s\n", p.Provider)
		if p.Marketplace != "" {
			_, _ = fmt.Fprintf(tw, "  marketplace\t%s\n", p.Marketplace)
		}
		_, _ = fmt.Fprintf(tw, "  installed\t%v   enabled  %v\n", p.Installed, p.Enabled)
		if p.Author != "" {
			_, _ = fmt.Fprintf(tw, "  author\t%s\n", p.Author)
		}
		if p.License != "" {
			_, _ = fmt.Fprintf(tw, "  license\t%s\n", p.License)
		}
		if p.Homepage != "" {
			_, _ = fmt.Fprintf(tw, "  homepage\t%s\n", p.Homepage)
		}
		if p.Repository != "" {
			_, _ = fmt.Fprintf(tw, "  repository\t%s\n", p.Repository)
		}
		if len(p.Keywords) > 0 {
			_, _ = fmt.Fprintf(tw, "  keywords\t%s\n", strings.Join(p.Keywords, ", "))
		}
		if p.InstallPath != "" {
			_, _ = fmt.Fprintf(tw, "  path\t%s\n", p.InstallPath)
		}
		if p.Description != "" {
			_, _ = fmt.Fprintf(tw, "  description\t%s\n", p.Description)
		}
		// Enumerate contained skills + MCPs by walking the snapshot
		// and matching SourcePlugin == p.Name.
		if snap != nil {
			var skillNames []string
			for _, s := range snap.Skills {
				if s.Provider == p.Provider && s.SourcePlugin == p.Name {
					skillNames = append(skillNames, s.Name)
				}
			}
			if len(skillNames) > 0 {
				_, _ = fmt.Fprintf(tw, "  skills (%d)\t%s\n", len(skillNames),
					strings.Join(truncList(skillNames, 8), ", "))
			}
		}
	case r.skill != nil:
		s := r.skill
		_, _ = fmt.Fprintf(tw, "  name\t%s\n", s.Name)
		_, _ = fmt.Fprintf(tw, "  provider\t%s\n", s.Provider)
		_, _ = fmt.Fprintf(tw, "  scope\t%s\n", s.Scope)
		if s.SourcePlugin != "" {
			_, _ = fmt.Fprintf(tw, "  source plugin\t%s\n", s.SourcePlugin)
		}
		if s.Model != "" {
			_, _ = fmt.Fprintf(tw, "  model\t%s\n", s.Model)
		}
		if s.AllowedTools != "" {
			_, _ = fmt.Fprintf(tw, "  allowed-tools\t%s\n", s.AllowedTools)
		}
		if s.ArgumentHint != "" {
			_, _ = fmt.Fprintf(tw, "  arguments\t%s\n", s.ArgumentHint)
		}
		_, _ = fmt.Fprintf(tw, "  user-invocable\t%v   model-invocable  %v\n", s.UserInvocable, !s.DisableModelInvoke)
		if s.Path != "" {
			_, _ = fmt.Fprintf(tw, "  path\t%s\n", s.Path)
		}
		if s.Description != "" {
			_, _ = fmt.Fprintf(tw, "  description\t%s\n", s.Description)
		}
		if s.WhenToUse != "" {
			_, _ = fmt.Fprintf(tw, "  when to use\t%s\n", s.WhenToUse)
		}
	case r.mcp != nil:
		m := r.mcp
		_, _ = fmt.Fprintf(tw, "  name\t%s\n", m.Name)
		_, _ = fmt.Fprintf(tw, "  provider\t%s\n", m.Provider)
		_, _ = fmt.Fprintf(tw, "  transport\t%s\n", m.Transport)
		_, _ = fmt.Fprintf(tw, "  scope\t%s\n", m.Scope)
		_, _ = fmt.Fprintf(tw, "  enabled\t%v\n", m.Enabled)
		if m.URL != "" {
			_, _ = fmt.Fprintf(tw, "  url\t%s\n", m.URL)
		}
		if m.Command != "" {
			_, _ = fmt.Fprintf(tw, "  command\t%s %s\n", m.Command, strings.Join(m.Args, " "))
		}
		if len(m.Tools) > 0 {
			_, _ = fmt.Fprintf(tw, "  tools\t%d (%s)\n", len(m.Tools), strings.Join(truncList(m.Tools, 6), ", "))
		}
		if len(m.EnvKeys) > 0 {
			_, _ = fmt.Fprintf(tw, "  env keys\t%s\n", strings.Join(m.EnvKeys, ", "))
		}
	case r.session != nil:
		s := r.session
		_, _ = fmt.Fprintf(tw, "  id\t%s\n", s.ID)
		if s.Name != "" {
			_, _ = fmt.Fprintf(tw, "  name\t%s\n", s.Name)
		}
		_, _ = fmt.Fprintf(tw, "  provider\t%s\n", s.Provider)
		if s.Type != "" {
			_, _ = fmt.Fprintf(tw, "  type\t%s\n", s.Type)
		}
		if s.Status != "" {
			_, _ = fmt.Fprintf(tw, "  status\t%s\n", s.Status)
		}
		if !s.Created.IsZero() {
			_, _ = fmt.Fprintf(tw, "  created\t%s\n", s.Created.Format(time.RFC3339))
		}
		if !s.LastModified.IsZero() {
			_, _ = fmt.Fprintf(tw, "  modified\t%s\n", s.LastModified.Format(time.RFC3339))
		}
		if s.TurnCount > 0 {
			_, _ = fmt.Fprintf(tw, "  turns\t%d\n", s.TurnCount)
		}
		if s.ProjectPath != "" {
			_, _ = fmt.Fprintf(tw, "  project\t%s\n", truncAgentRow(s.ProjectPath, 80))
		}
		if s.Title != "" {
			// Cap the title so a long first-user-message doesn't wrap
			// across two visual rows and throw off footer alignment.
			// 80 chars is a generous cap that still reads as the first
			// sentence of an intent on every reasonable terminal width.
			_, _ = fmt.Fprintf(tw, "  title\t%s\n", truncAgentRow(s.Title, 80))
		}
		if s.TranscriptPath != "" {
			_, _ = fmt.Fprintf(tw, "  transcript\t%s\n", truncAgentRow(s.TranscriptPath, 80))
		}
	default:
		_, _ = fmt.Fprintf(tw, "  id\t%s\n", r.id)
		_, _ = fmt.Fprintf(tw, "  provider\t%s\n", r.provider)
		_, _ = fmt.Fprintf(tw, "  source\t%s\n", r.source)
	}
	_ = tw.Flush()
	return b.String()
}

func truncList(items []string, n int) []string {
	if len(items) <= n {
		return items
	}
	out := make([]string, n, n+1)
	copy(out, items[:n])
	return append(out, fmt.Sprintf("+%d more", len(items)-n))
}

// agentsFilterChips returns a one-line summary of every active filter
// on the current sub-tab — provider, marketplace, status — colored as
// chips. Returns "" when nothing is filtered so the row collapses.
func agentsFilterChips(st *agentsState) string {
	if st == nil {
		return ""
	}
	var chips []string
	chipStyle := lipgloss.NewStyle().Background(cyberChipBg).Padding(0, 1)
	if st.providerFilter != "" {
		chips = append(chips, chipStyle.Foreground(cyberPrimary).Render("provider:"+providerShort(st.providerFilter)))
	}
	if st.subTab == agentsSubPlugins && st.marketplaceFilter != "" {
		chips = append(chips, chipStyle.Foreground(cyberAccent).Render("marketplace:"+st.marketplaceFilter))
	}
	if st.subTab == agentsSubSkills && st.pluginFilter != "" {
		chips = append(chips, chipStyle.Foreground(cyberAccent).Render("plugin:"+st.pluginFilter))
	}
	if agentsSupportsFilter(st.subTab) {
		if f := st.statusFilter[st.subTab]; f != agentsFilterAll {
			chips = append(chips, chipStyle.Foreground(cyberOK).Render("status:"+filterName(f)))
		}
	}
	if len(chips) == 0 {
		return ""
	}
	return "filters: " + strings.Join(chips, " ") + "  " + dimVersion.Render("X clears all")
}

// agentsSupportsFilter reports whether `f` cycling applies to a sub-tab.
func agentsSupportsFilter(subTab int) bool {
	switch subTab {
	case agentsSubMarketplaces, agentsSubPlugins, agentsSubMCPs:
		return true
	}
	return false
}

// filterName returns a display label for a status filter.
func filterName(f agentsFilter) string {
	switch f {
	case agentsFilterInstalled:
		return "installed"
	case agentsFilterCatalog:
		return "available"
	case agentsFilterEnabled:
		return "enabled"
	case agentsFilterDisabled:
		return "disabled"
	}
	return "all"
}

// applyStatusFilter narrows rows for the current sub-tab/filter. Default
// is identity. Plugins: installed vs catalog vs enabled vs disabled.
// MCPs: same axis (installed = scope ≠ remote, enabled/disabled use
// the Enabled bool).
func applyStatusFilter(rows []agentRow, subTab int, f agentsFilter) []agentRow {
	if f == agentsFilterAll {
		return rows
	}
	out := make([]agentRow, 0, len(rows))
	for _, r := range rows {
		keep := false
		switch subTab {
		case agentsSubPlugins:
			if r.plugin == nil {
				continue
			}
			switch f {
			case agentsFilterInstalled:
				keep = r.plugin.Installed
			case agentsFilterCatalog:
				keep = !r.plugin.Installed
			case agentsFilterEnabled:
				keep = r.plugin.Installed && r.plugin.Enabled
			case agentsFilterDisabled:
				keep = r.plugin.Installed && !r.plugin.Enabled
			}
		case agentsSubMCPs:
			if r.mcp == nil {
				continue
			}
			installed := r.mcp.Scope != agents.ScopeRemote
			switch f {
			case agentsFilterInstalled:
				keep = installed
			case agentsFilterCatalog:
				keep = !installed
			case agentsFilterEnabled:
				keep = installed && r.mcp.Enabled
			case agentsFilterDisabled:
				keep = installed && !r.mcp.Enabled
			}
		case agentsSubMarketplaces:
			if r.marketplace == nil {
				continue
			}
			switch f {
			case agentsFilterInstalled:
				keep = r.marketplace.Installed
			case agentsFilterCatalog:
				keep = !r.marketplace.Installed
			}
		default:
			keep = true
		}
		if keep {
			out = append(out, r)
		}
	}
	return out
}

// applyScopeFilter narrows rows whose scope doesn't match. Empty = all.
func applyScopeFilter(rows []agentRow, scope agents.Scope) []agentRow {
	if scope == "" {
		return rows
	}
	out := make([]agentRow, 0, len(rows))
	for _, r := range rows {
		if r.scope == scope {
			out = append(out, r)
		}
	}
	return out
}

// applyTransportFilter narrows MCPs by transport. No-op on other rows.
func applyTransportFilter(rows []agentRow, transport string) []agentRow {
	if transport == "" {
		return rows
	}
	out := make([]agentRow, 0, len(rows))
	for _, r := range rows {
		if r.mcp == nil {
			continue
		}
		if r.mcp.Transport == transport {
			out = append(out, r)
		}
	}
	return out
}

// applyStatusValueFilter handles the string-valued status filter — the
// catch-all for marketplace/session statuses that don't fit the
// agentsFilter enum.
func applyStatusValueFilter(rows []agentRow, subTab int, value string) []agentRow {
	if value == "" {
		return rows
	}
	out := make([]agentRow, 0, len(rows))
	for _, r := range rows {
		keep := false
		switch subTab {
		case agentsSubMarketplaces:
			if r.marketplace == nil {
				continue
			}
			builtin := false
			switch r.marketplace.Source {
			case agents.SourceCatalogClaude, agents.SourceCatalogCopilot:
				builtin = true
			}
			switch value {
			case "builtin":
				keep = builtin
			case "local":
				keep = !builtin
			}
		case agentsSubSessions:
			if r.session == nil {
				continue
			}
			keep = string(r.session.Status) == value
		}
		if keep {
			out = append(out, r)
		}
	}
	return out
}

// applyProviderFilter narrows rows to a single provider when set.
func applyProviderFilter(rows []agentRow, provider agents.ProviderID) []agentRow {
	if provider == "" {
		return rows
	}
	out := make([]agentRow, 0, len(rows))
	for _, r := range rows {
		if r.provider == provider {
			out = append(out, r)
		}
	}
	return out
}

// applyMarketplaceFilter narrows the Plugins sub-tab to a marketplace.
// No-op on other sub-tabs.
func applyMarketplaceFilter(rows []agentRow, subTab int, marketplace string) []agentRow {
	if marketplace == "" || subTab != agentsSubPlugins {
		return rows
	}
	out := make([]agentRow, 0, len(rows))
	for _, r := range rows {
		if r.plugin != nil && r.plugin.Marketplace == marketplace {
			out = append(out, r)
		}
	}
	return out
}

// applyPluginFilter narrows Skills rows to those whose SourcePlugin
// matches. Only applies on the Skills sub-tab; a no-op everywhere
// else. Set when the user clicks "View skills →" from a plugin detail
// page so the Skills tab opens scoped to the originating plugin.
func applyPluginFilter(rows []agentRow, subTab int, plugin string) []agentRow {
	if plugin == "" || subTab != agentsSubSkills {
		return rows
	}
	out := make([]agentRow, 0, len(rows))
	for _, r := range rows {
		if r.skill != nil && r.skill.SourcePlugin == plugin {
			out = append(out, r)
		}
	}
	return out
}

// agentsAvailableMarketplaces returns the list of marketplace names
// present in the current snapshot. Used to cycle the marketplace
// filter through known values only.
func agentsAvailableMarketplaces(s *agents.Snapshot) []string {
	if s == nil {
		return nil
	}
	seen := make(map[string]bool)
	for _, mp := range s.Marketplaces {
		if mp.Name != "" && !seen[mp.Name] {
			seen[mp.Name] = true
		}
	}
	// Also include marketplaces referenced by installed plugins even if
	// not in the Marketplaces slice (defensive — partial-scan recovery).
	for _, p := range s.Plugins {
		if p.Marketplace != "" && !seen[p.Marketplace] {
			seen[p.Marketplace] = true
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// cycleProviderFilter advances the providerFilter through (empty → p1
// → p2 → … → empty), restricted to providers present in the snapshot.
func cycleProviderFilter(current agents.ProviderID, available []agents.ProviderID) agents.ProviderID {
	if len(available) == 0 {
		return ""
	}
	if current == "" {
		return available[0]
	}
	for i, p := range available {
		if p == current {
			if i == len(available)-1 {
				return ""
			}
			return available[i+1]
		}
	}
	return available[0]
}

// cycleMarketplaceFilter advances the marketplaceFilter through
// (empty → mp1 → mp2 → … → empty), restricted to known marketplaces.
func cycleMarketplaceFilter(current string, available []string) string {
	if len(available) == 0 {
		return ""
	}
	if current == "" {
		return available[0]
	}
	for i, name := range available {
		if name == current {
			if i == len(available)-1 {
				return ""
			}
			return available[i+1]
		}
	}
	return available[0]
}

// readSessionTranscript reads at most `max` rendered lines from a
// session transcript and returns them ready for the viewer modal.
//
// When given a directory, looks for the canonical Copilot
// `events.jsonl` first; if that's missing, falls back to the most
// recently modified `*.jsonl` in the directory (Claude project
// dirs follow this layout — one file per session UUID).
//
// Each entry in the transcript is a JSON event. The viewer doesn't
// need the raw envelope — it needs the conversation. We parse each
// line and extract just the user-friendly bits:
//
//   - Claude `user` / `assistant` messages → `[role] <first text>`
//   - Claude `tool_use` blocks → `[tool] <name>(<arg-summary>)`
//   - Copilot `user.message` / `assistant.message` → same shape
//   - Other event types (queue-operation, hook_success, telemetry)
//     → skipped, since they're noise to the human reader
//
// Lines that don't parse as JSON fall through unchanged (capped at
// 4 KiB) so non-JSONL transcripts (legacy formats) still show up.
// Each rendered line is capped at 200 chars so a single event with
// a multi-megabyte payload can't blow up the viewer's layout.
func readSessionTranscript(path string, limit int) ([]string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		if eventsPath := filepath.Join(path, "events.jsonl"); fileExists(eventsPath) {
			path = eventsPath
		} else if newest := newestJSONL(path); newest != "" {
			path = newest
		} else {
			return nil, fmt.Errorf("no transcript file found in %s", path)
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	const maxRenderedLine = 200
	var lines []string
	for scanner.Scan() && len(lines) < limit {
		raw := scanner.Bytes()
		rendered := renderTranscriptLine(raw)
		if rendered == "" {
			continue
		}
		if len(rendered) > maxRenderedLine {
			rendered = rendered[:maxRenderedLine-1] + "…"
		}
		lines = append(lines, rendered)
	}
	return lines, nil
}

// renderTranscriptLine turns one raw JSONL transcript line into a
// human-friendly string. Returns "" for lines the viewer should skip
// (telemetry, hooks, mode changes — anything that isn't part of the
// conversation).
//
// Format:
//
//	[user]      <text>
//	[assistant] <text>
//	[tool]      <name>(<first-arg>)
//
// When the JSON has neither a recognised user/assistant message nor
// a tool_use, returns "" to skip the line.
func renderTranscriptLine(raw []byte) string {
	// Skip non-JSON noise (rare but possible for legacy transcripts).
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		if len(raw) > 200 {
			return string(raw[:199]) + "…"
		}
		return string(raw)
	}

	// Single struct that fits both Claude and Copilot shapes — we use
	// only the fields that are present for the role we're looking at.
	//
	// `message.content` is intentionally json.RawMessage: in real
	// Claude transcripts it appears in TWO shapes — a plain string
	// (`"content":"hi there"`, the dominant case for user-typed
	// messages) AND an array of typed blocks (`"content":[{"type":
	// "text",...}]`, used for assistant turns and tool calls).
	// Declaring it as `[]struct{...}` made json.Unmarshal fail with
	// a type mismatch on every string-form line, which was returned
	// as "" and silently dropped — so the viewer rendered an empty
	// box for sessions where the user-typed messages dominated. We
	// now branch on the first byte of the raw value to handle both.
	var ev struct {
		Type    string `json:"type"`
		Message struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"message"`
		// Copilot wraps everything in data.message.{text,content}.
		Data struct {
			Message struct {
				Role    string `json:"role"`
				Text    string `json:"text"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil {
		// Malformed JSON (typically a partially-written tail line
		// from an in-progress session). The doc contract for this
		// function is "lines that don't parse fall through unchanged
		// at 4 KiB" — so don't silently drop, surface the raw text
		// (capped) so the user still sees that something happened.
		const maxRaw = 4096
		if len(raw) > maxRaw {
			return string(raw[:maxRaw-1]) + "…"
		}
		return string(raw)
	}

	switch ev.Type {
	case "user", "assistant":
		role := ev.Type
		text, tool := extractClaudeContent(ev.Message.Content)
		if tool != "" {
			return tool
		}
		if text != "" {
			return "[" + role + "] " + collapseWhitespace(text)
		}
		return ""
	case "user.message", "assistant.message":
		role := "user"
		if ev.Type == "assistant.message" {
			role = "assistant"
		}
		text := ev.Data.Message.Text
		if text == "" {
			text = ev.Data.Message.Content
		}
		if text == "" {
			return ""
		}
		return "[" + role + "] " + collapseWhitespace(text)
	}
	return ""
}

// extractClaudeContent unpacks the polymorphic `message.content`
// field from a Claude transcript event. Returns (text, toolLine):
//   - text:     non-empty when the content was a plain string OR an
//     array containing a text block. The caller wraps it
//     in the role prefix.
//   - toolLine: a fully-rendered "[tool] name(arg)" string when the
//     content was an array containing a tool_use block.
//     When non-empty the caller uses it verbatim and
//     ignores `text`.
//
// Returns ("", "") when the content is empty, neither shape, or
// only contains blocks the viewer should skip (e.g. tool_result
// payloads belong to the tool, not the conversation).
func extractClaudeContent(raw json.RawMessage) (text, toolLine string) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "", ""
	}
	// String form: `"content":"plain text from the user"`. This is
	// the dominant case for user-typed messages and was the source
	// of the viewer-is-blank bug before the json.RawMessage switch.
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err == nil {
			return s, ""
		}
		return "", ""
	}
	// Array form: walk blocks for the first text or tool_use.
	if trimmed[0] == '[' {
		var blocks []struct {
			Type  string                 `json:"type"`
			Text  string                 `json:"text"`
			Name  string                 `json:"name"`
			Input map[string]interface{} `json:"input"`
		}
		if err := json.Unmarshal(trimmed, &blocks); err != nil {
			return "", ""
		}
		for _, c := range blocks {
			switch c.Type {
			case "text":
				if c.Text != "" {
					return c.Text, ""
				}
			case "tool_use":
				return "", "[tool]      " + c.Name + "(" + summariseInput(c.Input) + ")"
			}
		}
	}
	return "", ""
}

// summariseInput renders a tool input map as a short summary like
// `path="src/foo.go"`. Picks the first value-bearing key from a fixed
// short list of common ones; falls back to "..." when nothing useful
// is present.
func summariseInput(in map[string]interface{}) string {
	for _, key := range []string{"file_path", "path", "notebook_path", "command", "query", "pattern", "url"} {
		if v, ok := in[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				if len(s) > 60 {
					s = s[:59] + "…"
				}
				return key + "=" + strconv.Quote(s)
			}
		}
	}
	return "..."
}

// collapseWhitespace flattens \r\n\t into single spaces so a single
// transcript line stays on a single visual row.
func collapseWhitespace(s string) string {
	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		if r == '\r' || r == '\n' || r == '\t' || r == ' ' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	return strings.TrimSpace(b.String())
}

// fileExists is a tiny stat-only helper so readSessionTranscript can
// distinguish "events.jsonl missing" from "events.jsonl unreadable".
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// newestJSONL returns the path to the most recently modified `.jsonl`
// file in `dir`, or the empty string when none exist. Used by the
// transcript viewer to surface the most recent Claude project
// session when the project dir doesn't carry the canonical
// `events.jsonl` filename.
func newestJSONL(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var newest string
	var newestTime time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		if newest == "" || fi.ModTime().After(newestTime) {
			newest = filepath.Join(dir, e.Name())
			newestTime = fi.ModTime()
		}
	}
	return newest
}

// deleteAgentEntityCmd builds the Bubbletea command that deletes a
// session or MCP through the right provider. The result triggers
// agentsLoadedMsg via a re-scan when complete.
func deleteAgentEntityCmd(provider agents.ProviderID, typ agents.EntityType, id string) tea.Cmd {
	return func() tea.Msg {
		svc := agentsService()
		p := svc.ProviderFor(provider)
		if p == nil {
			return agentsDeletedMsg{err: fmt.Errorf("provider %q not registered", provider)}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var err error
		switch typ {
		case agents.EntitySession:
			err = p.DeleteSession(ctx, id)
		case agents.EntityMCP:
			err = p.RemoveMCP(ctx, id)
		default:
			err = fmt.Errorf("delete not supported for %s", typ)
		}
		return agentsDeletedMsg{err: err, typ: typ, id: id}
	}
}

// agentsDeletedMsg lands in handleAgentsMsg when a delete completes.
type agentsDeletedMsg struct {
	err error
	typ agents.EntityType
	id  string
}
