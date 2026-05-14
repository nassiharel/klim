package tui

import (
	"bufio"
	"context"
	"fmt"
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
	"github.com/nassiharel/klim/internal/agents/providers/claudecode"
	"github.com/nassiharel/klim/internal/agents/providers/copilotcli"
	"github.com/nassiharel/klim/internal/agents/providers/mcpregistry"
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

	searchActive bool
	searchInput  string

	// sortMode is per sub-tab: see agentsSortMode* below.
	sortMode map[int]agentsSortMode

	// statusFilter is per sub-tab: see agentsFilter* below.
	statusFilter map[int]agentsFilter

	launchPrompt string // non-empty while the launch confirmation modal is up
	launchPlan   agents.ExecPlan

	// deleteTarget is non-empty while the delete confirmation modal is up.
	// The string is a human description; the actual action is held in
	// deleteAction so the model can fire it after the user confirms.
	deleteTarget string
	deleteAction tea.Cmd

	// helpOpen toggles the keymap overlay.
	helpOpen bool

	// viewerLines holds the first N lines of a session transcript so the
	// user can peek at it without leaving the TUI.
	viewerOpen  bool
	viewerTitle string
	viewerLines []string

	flash    string
	flashEnd time.Time
}

// agentsFilter narrows a sub-tab's rows beyond the search box.
type agentsFilter int

const (
	agentsFilterAll       agentsFilter = iota
	agentsFilterInstalled              // plugins: installed; MCPs: configured (scope ≠ remote)
	agentsFilterCatalog                // remote / not installed
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
	agentsSortCount
)

// agentsCacheTTL is how long a cached scan is considered fresh on tab
// entry. Older caches still render immediately but trigger a
// background refresh so the user sees latest state on the next tick.
const agentsCacheTTL = 10 * time.Minute

const (
	agentsSubMarketplaces = 0
	agentsSubPlugins      = 1
	agentsSubSkills       = 2
	agentsSubMCPs         = 3
	agentsSubSessions     = 4
	agentsSubCount        = 5
)

// agentsService factory. Swappable so tests can inject a fake.
var agentsService = func() *agents.Service {
	return agents.NewService(4,
		claudecode.New(),
		copilotcli.New(),
		mcpregistry.New(),
	)
}

func newAgentsState() *agentsState { return &agentsState{} }

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
	if len(plan.Env) > 0 {
		c.Env = append(c.Env, plan.Env...)
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
		// Already loaded — trigger a background refresh if the cache
		// is older than agentsCacheTTL. We keep showing the stale
		// snapshot while the new one is fetched.
		if time.Since(st.loadedAt) > agentsCacheTTL {
			return loadAgentsCmd(true)
		}
		return nil
	}
	st.loading = true
	return loadAgentsCmd(false)
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

	// Help overlay closes on any key.
	if st.helpOpen {
		st.helpOpen = false
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
	case "tab", "right":
		// On the rightmost sub-tab, let the global handler advance to
		// the next parent tab instead of wrapping inside Agents.
		if st.subTab == agentsSubCount-1 {
			return false, nil
		}
		st.setSubTab((st.subTab + 1) % agentsSubCount)
		return true, nil
	case "shift+tab", "left":
		if st.subTab == 0 {
			return false, nil
		}
		st.setSubTab((st.subTab + agentsSubCount - 1) % agentsSubCount)
		return true, nil
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
	case "f":
		// Cycle status filter for the current sub-tab. Only Plugins and
		// MCPs care; other sub-tabs treat the keystroke as a no-op flash.
		if !agentsSupportsFilter(st.subTab) {
			st.flash = "filter not applicable to this sub-tab"
			st.flashEnd = time.Now().Add(2 * time.Second)
			return true, nil
		}
		next := (st.statusFilter[st.subTab] + 1) % agentsFilterCount
		st.statusFilter[st.subTab] = next
		st.cursor = 0
		st.flash = "filter: " + filterName(next)
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
	case "?":
		st.helpOpen = true
		return true, nil
	case "r":
		st.loading = true
		st.flash = "refreshing…"
		st.flashEnd = time.Now().Add(2 * time.Second)
		return true, loadAgentsCmd(true)
	}
	return false, nil
}

func (st *agentsState) setSubTab(i int) {
	st.subTab = i
	st.cursor = 0
	st.detailOpen = false
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
			return true, nil
		}
		st.flash = fmt.Sprintf("deleted %s %s", v.typ, v.id)
		st.flashEnd = time.Now().Add(3 * time.Second)
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
		for i := range st.snapshot.Sessions {
			x := st.snapshot.Sessions[i]
			label := x.Name
			if label == "" {
				label = sessionShortID(x.ID)
			}
			rows = append(rows, agentRow{
				id: x.ID, name: label, subtitle: x.ProjectPath,
				provider: x.Provider, source: x.Source,
				session: &x,
			})
		}
	}
	return sortAgentRows(applyStatusFilter(filterAgentRows(rows, q), st.subTab, st.statusFilter[st.subTab]), st.subTab, st.sortMode[st.subTab])
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
	case agentsSubPlugins, agentsSubSkills, agentsSubMCPs, agentsSubMarketplaces:
		return []agentsSortMode{agentsSortDefault, agentsSortName, agentsSortModified}
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
	switch id {
	case agents.ProviderClaudeCode:
		return base.Foreground(cyberPrimary) // cyan
	case agents.ProviderCopilotCLI:
		return base.Foreground(cyberSecondary) // magenta
	case agents.ProviderMCPRegistry:
		return base.Foreground(cyberAccent) // amber
	}
	return base.Foreground(cyberFG)
}

// agentsStatusStyle colors a session status string.
func agentsStatusStyle(status agents.SessionStatus) lipgloss.Style {
	switch status {
	case agents.SessionStatusActive:
		return lipgloss.NewStyle().Foreground(cyberOK)
	case agents.SessionStatusCompleted:
		return lipgloss.NewStyle().Foreground(cyberFGDim)
	case agents.SessionStatusStopped:
		return lipgloss.NewStyle().Foreground(cyberAlert)
	}
	return lipgloss.NewStyle().Foreground(cyberFGDim)
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

	subs := []string{"Marketplaces", "Plugins", "Skills", "MCPs", "Sessions"}
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

	switch {
	case st.searchActive:
		b.WriteString("  search: " + st.searchInput + "▌\n")
	case st.searchInput != "":
		b.WriteString("  filter: " + st.searchInput + "  " + dimVersion.Render("/ edit · esc clear"))
		b.WriteString("\n")
	default:
		// Single, subtle hint — full keymap lives behind ?.
		b.WriteString("  " + dimVersion.Render("press ? for help") + "\n")
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

	const maxRows = 25
	visible := rows
	more := 0
	if len(visible) > maxRows {
		more = len(visible) - maxRows
		visible = visible[:maxRows]
	}

	b.WriteString(renderAgentsTable(st.subTab, visible, st.cursor))
	if more > 0 {
		fmt.Fprintf(&b, "  … %d more (search to narrow)\n", more)
	}

	if st.detailOpen && st.cursor < len(rows) {
		b.WriteString("\n  ─── detail ───\n")
		b.WriteString(renderAgentDetail(rows[st.cursor], st.snapshot))
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

	if st.viewerOpen {
		b.WriteString("\n  ╔ Transcript ══════════════════════════════════════════╗\n")
		b.WriteString("  ║ " + truncAgentRow(st.viewerTitle, 64) + "\n")
		b.WriteString("  ╟──────────────────────────────────────────────────────╢\n")
		for _, line := range st.viewerLines {
			b.WriteString("  ║ " + truncAgentRow(line, 80) + "\n")
		}
		b.WriteString("  ╟──────────────────────────────────────────────────────╢\n")
		b.WriteString("  ║ Esc / Enter / q = close\n")
		b.WriteString("  ╚══════════════════════════════════════════════════════╝\n")
	}

	if st.helpOpen {
		b.WriteString(agentsHelpOverlay())
	}

	return b.String()
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
	case agents.ProviderMCPRegistry:
		return "mcp-reg"
	default:
		return string(id)
	}
}

// agentsSnapshotCounts returns the per-sub-tab counts. Returns zeros
// when snapshot is nil (mid-load).
func agentsSnapshotCounts(s *agents.Snapshot) [5]int {
	if s == nil {
		return [5]int{}
	}
	return [5]int{
		len(s.Marketplaces),
		len(s.Plugins),
		len(s.Skills),
		len(s.MCPs),
		len(s.Sessions),
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
	order := []agents.ProviderID{agents.ProviderClaudeCode, agents.ProviderCopilotCLI, agents.ProviderMCPRegistry}
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

// renderProviderShort returns the colored short provider label.
func renderProviderShort(id agents.ProviderID) string {
	return agentsProviderStyle(id).Render(providerShort(id))
}

// renderScope colors the scope tag.
func renderScope(s agents.Scope) string {
	if s == "" {
		return "—"
	}
	return agentsScopeStyle(s).Render(string(s))
}

// renderStatusOrDash returns a colored status label or "—" for empty.
func renderStatusOrDash(s agents.SessionStatus) string {
	if s == "" {
		return "—"
	}
	return agentsStatusStyle(s).Render(string(s))
}

// column describes one column in a sub-tab's table.
type column struct {
	header string
	width  int
}

// renderRow concatenates cells into a row with each cell padded to the
// configured column width. lipgloss.NewStyle().Width is ANSI-aware so
// colored content stays aligned (unlike text/tabwriter).
func renderRow(cells []string, cols []column, lead string) string {
	var b strings.Builder
	b.WriteString(lead)
	for i, c := range cells {
		w := cols[i].width
		if w == 0 {
			b.WriteString(c)
		} else {
			b.WriteString(lipgloss.NewStyle().Width(w).Render(c))
		}
		if i < len(cells)-1 {
			b.WriteString("  ")
		}
	}
	b.WriteString("\n")
	return b.String()
}

func renderHeader(cols []column) string {
	cells := make([]string, len(cols))
	for i, c := range cols {
		cells[i] = headerStyle.Render(c.header)
	}
	return renderRow(cells, cols, "    ")
}

// renderAgentsTable produces an aligned table for the current sub-tab.
// Uses lipgloss fixed-width cells so colored content (SOURCE chips,
// STATUS pills, SCOPE tags) stays in line — text/tabwriter counted
// ANSI escapes as visible width and pushed columns out of alignment.
func renderAgentsTable(subTab int, rows []agentRow, cursor int) string {
	var b strings.Builder

	switch subTab {
	case agentsSubMarketplaces:
		cols := []column{
			{header: "SOURCE", width: 8},
			{header: "NAME", width: 28},
			{header: "OWNER", width: 14},
			{header: "PLUGINS", width: 8},
			{header: "URL", width: 48},
		}
		b.WriteString(renderHeader(cols))
		for i, r := range rows {
			mp := r.marketplace
			owner, url, count := "", "", 0
			if mp != nil {
				owner, url, count = mp.Owner, mp.URL, mp.PluginCount
			}
			cells := []string{
				renderProviderShort(r.provider),
				truncAgentRow(r.name, cols[1].width),
				truncAgentRow(owner, cols[2].width),
				dashOrInt(count),
				truncAgentRow(url, cols[4].width),
			}
			b.WriteString(renderRow(cells, cols, rowLead(i, cursor)))
		}
	case agentsSubPlugins:
		cols := []column{
			{header: "SOURCE", width: 8},
			{header: "NAME", width: 26},
			{header: "VERSION", width: 10},
			{header: "MARKETPLACE", width: 24},
			{header: "STATUS", width: 10},
			{header: "DESCRIPTION", width: 50},
		}
		b.WriteString(renderHeader(cols))
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
				renderProviderShort(r.provider),
				truncAgentRow(r.name, cols[1].width),
				truncAgentRow(version, cols[2].width),
				truncAgentRow(market, cols[3].width),
				status,
				truncAgentRow(desc, cols[5].width),
			}
			b.WriteString(renderRow(cells, cols, rowLead(i, cursor)))
		}
	case agentsSubSkills:
		cols := []column{
			{header: "SOURCE", width: 8},
			{header: "NAME", width: 32},
			{header: "SCOPE", width: 8},
			{header: "FROM", width: 18},
			{header: "MODEL", width: 8},
			{header: "DESCRIPTION", width: 50},
		}
		b.WriteString(renderHeader(cols))
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
				renderProviderShort(r.provider),
				truncAgentRow(r.name, cols[1].width),
				renderScope(r.scope),
				truncAgentRow(from, cols[3].width),
				truncAgentRow(model, cols[4].width),
				truncAgentRow(desc, cols[5].width),
			}
			b.WriteString(renderRow(cells, cols, rowLead(i, cursor)))
		}
	case agentsSubMCPs:
		cols := []column{
			{header: "SOURCE", width: 8},
			{header: "NAME", width: 34},
			{header: "TRANSPORT", width: 10},
			{header: "SCOPE", width: 8},
			{header: "TOOLS", width: 6},
			{header: "ENDPOINT", width: 48},
		}
		b.WriteString(renderHeader(cols))
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
				renderProviderShort(r.provider),
				truncAgentRow(r.name, cols[1].width),
				transport,
				renderScope(r.scope),
				dashOrInt(tools),
				truncAgentRow(endpoint, cols[5].width),
			}
			b.WriteString(renderRow(cells, cols, rowLead(i, cursor)))
		}
	case agentsSubSessions:
		cols := []column{
			{header: "SOURCE", width: 8},
			{header: "ID", width: 10},
			{header: "TYPE", width: 12},
			{header: "STATUS", width: 10},
			{header: "TURNS", width: 6},
			{header: "CREATED", width: 18},
			{header: "MODIFIED", width: 14},
			{header: "PROJECT", width: 36},
		}
		b.WriteString(renderHeader(cols))
		for i, r := range rows {
			s := r.session
			typ, status, project := "", "", r.subtitle
			created, modified := "", ""
			turns := 0
			if s != nil {
				typ, project, turns = s.Type, s.ProjectPath, s.TurnCount
				if typ == "" {
					typ = "interactive"
				}
				status = renderStatusOrDash(s.Status)
				created = humaniseTime(s.Created)
				modified = humaniseTime(s.LastModified)
			}
			cells := []string{
				renderProviderShort(r.provider),
				truncAgentRow(r.name, cols[1].width),
				typ,
				status,
				dashOrInt(turns),
				created,
				modified,
				truncAgentRow(project, cols[7].width),
			}
			b.WriteString(renderRow(cells, cols, rowLead(i, cursor)))
		}
	}
	return b.String()
}

// rowLead returns the two-character cursor mark prefix for the row.
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

// humaniseTime renders a time in a column-friendly form: empty for zero,
// a date for older than 24h, "HH:MM" for the same day, or a relative
// "<n>m ago" / "<n>h ago" string for very recent events.
func humaniseTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	now := time.Now()
	diff := now.Sub(t)
	switch {
	case diff < 0:
		return t.Format("2006-01-02 15:04")
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	case diff < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(diff.Hours()/24))
	default:
		return t.Format("2006-01-02 15:04")
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
			_, _ = fmt.Fprintf(tw, "  project\t%s\n", s.ProjectPath)
		}
		if s.Title != "" {
			_, _ = fmt.Fprintf(tw, "  title\t%s\n", s.Title)
		}
		if s.TranscriptPath != "" {
			_, _ = fmt.Fprintf(tw, "  transcript\t%s\n", s.TranscriptPath)
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

// agentsSupportsFilter reports whether `f` cycling applies to a sub-tab.
func agentsSupportsFilter(subTab int) bool {
	switch subTab {
	case agentsSubPlugins, agentsSubMCPs:
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
		return "catalog"
	}
	return "all"
}

// applyStatusFilter narrows rows for the current sub-tab/filter. Default
// is identity. Plugins: installed vs catalog (Installed bool). MCPs:
// installed (scope ≠ remote) vs catalog (scope == remote).
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
			if (f == agentsFilterInstalled) == r.plugin.Installed {
				keep = true
			}
		case agentsSubMCPs:
			if r.mcp == nil {
				continue
			}
			installed := r.mcp.Scope != agents.ScopeRemote
			if (f == agentsFilterInstalled) == installed {
				keep = true
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

// agentsHelpOverlay renders the keymap modal.
func agentsHelpOverlay() string {
	keymap := [][2]string{
		{"1-5", "jump to sub-tab"},
		{"Tab / Shift-Tab", "next / previous sub-tab (or parent at edge)"},
		{"j / k or ↓ / ↑", "move cursor"},
		{"Enter", "toggle detail pane"},
		{"/", "search (fuzzy)"},
		{"s", "cycle sort mode"},
		{"f", "cycle status filter (plugins, MCPs)"},
		{"l", "launch session (skill / plugin / session)"},
		{"y", "yank entity id to clipboard"},
		{"v", "view first 60 lines of session transcript"},
		{"d", "delete current row (sessions / MCPs) — confirms first"},
		{"r", "refresh (re-scan everything)"},
		{"?", "toggle this help overlay"},
		{"Esc", "close overlay / cancel modal"},
	}
	var b strings.Builder
	b.WriteString("\n  ╔ Agents — keymap ════════════════════════════════════════════╗\n")
	for _, row := range keymap {
		b.WriteString(fmt.Sprintf("  ║ %-18s  %-40s ║\n", row[0], row[1]))
	}
	b.WriteString("  ║                                                                 ║\n")
	b.WriteString("  ║ press any key to close                                          ║\n")
	b.WriteString("  ╚═════════════════════════════════════════════════════════════════╝\n")
	return b.String()
}

// readSessionTranscript reads at most `max` lines from a session
// transcript file. Returns the lines so the viewer modal can render them.
func readSessionTranscript(path string, limit int) ([]string, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.IsDir() {
		path = filepath.Join(path, "events.jsonl")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var lines []string
	br := bufio.NewReader(f)
	for i := 0; i < limit; i++ {
		line, err := br.ReadString('\n')
		if line != "" {
			lines = append(lines, strings.TrimRight(line, "\r\n"))
		}
		if err != nil {
			break
		}
	}
	return lines, nil
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
