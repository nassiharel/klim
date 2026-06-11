// Package sessionstui provides the standalone Bubbletea dashboard
// reachable via `klim agents sessions` (when stdout is a TTY) and
// `klim agents sessions list --watch`.
//
// The dashboard is intentionally narrow in scope — it does not
// reproduce the full Agents tab's marketplace / plugin / skill
// browsers, nor does it try to be a long-running daemon. It is a
// focused view over the snapshot's Sessions slice with:
//
//   - tabbed views (Active, Previous, Files, Stats),
//   - a stats row of running / waiting / turn / tool counters,
//   - a search input filtering by ID, title, project, branch, MCP,
//   - a detail pane for the selected session,
//   - keyboard actions (r=resume, c=copy resume cmd, s=star,
//     g=cycle group-by, /=search, ?=help, q=quit), and
//   - two-tier polling (fast state poll, slow full reload).
//
// The package is kept separate from internal/tui (the rich Agents
// tab) so its lifecycle and key map don't have to coexist with the
// larger model. It re-uses the agents Service / Snapshot / enrich
// helpers, so any improvement to the data foundation lands here for
// free.
package sessionstui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/agents/bookmarks"
	"github.com/nassiharel/klim/internal/agents/enrich"
)

// Tab indexes for the dashboard's tab bar.
const (
	tabActive = iota
	tabPrevious
	tabFiles
	tabStats
)

var tabNames = []string{"Active", "Previous", "Files", "Stats"}

// Poll cadences.
const (
	fastTick = 5 * time.Second  // re-derive live state per session
	slowTick = 30 * time.Second // full re-scan
)

// GroupBy modes cycled by the `g` key.
const (
	groupByProject  = "project"
	groupByProvider = "provider"
	groupByNone     = "none"
)

// Model is the Bubbletea dashboard state. Exported so callers in the
// CLI package can construct it (the constructor takes a *agents.Service
// so tests can pass a stub).
type Model struct {
	svc       *agents.Service
	loading   bool
	loadErr   error
	snapshot  *agents.Snapshot
	updatedAt time.Time
	now       func() time.Time

	tab          int
	groupBy      string
	search       string
	insertSearch bool

	cursor int
	flat   []agents.Session // current filtered+sorted view

	width  int
	height int

	help   bool
	status string // ephemeral status line (cleared on next interaction)
}

// New constructs a Model. `svc` must already be wired to the desired
// providers; the Model takes no ownership of its lifecycle.
func New(svc *agents.Service) *Model {
	return &Model{
		svc:     svc,
		now:     time.Now,
		tab:     tabActive,
		groupBy: groupByProject,
	}
}

// Init satisfies tea.Model. Kicks off the first scan immediately so
// the user sees something on the very first frame; subsequent loads
// are driven by the ticker messages.
func (m *Model) Init() tea.Cmd {
	m.loading = true
	return tea.Batch(loadCmd(m.svc, true), tea.Tick(fastTick, func(t time.Time) tea.Msg { return fastTickMsg(t) }), tea.Tick(slowTick, func(t time.Time) tea.Msg { return slowTickMsg(t) }))
}

// snapshotMsg is the result of a Service.LoadAll call.
type snapshotMsg struct {
	snap *agents.Snapshot
	err  error
}

// fastTickMsg / slowTickMsg drive the two-tier polling loop.
type fastTickMsg time.Time
type slowTickMsg time.Time

// loadCmd schedules a Service.LoadAll. `refresh=true` skips the cache,
// used for the initial load and the slow tick.
func loadCmd(svc *agents.Service, refresh bool) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		snap, err := svc.LoadAll(ctx, agents.LoadOpts{Refresh: refresh})
		return snapshotMsg{snap: snap, err: err}
	}
}

// Update satisfies tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case snapshotMsg:
		m.loading = false
		m.loadErr = msg.err
		if msg.snap != nil {
			m.snapshot = msg.snap
			m.updatedAt = m.now()
			// Surface hydration warnings (corrupt bookmarks YAML,
			// unreadable grouping file, …) via the status row.
			// hydrateSessionExtras can't write to stderr because
			// stderr writes from a tea.Cmd land on top of the
			// rendered screen — the snap carries the messages
			// instead and we splice them into the status line.
			// Multiple warnings join with " | " so the row stays
			// single-line.
			if len(msg.snap.Warnings) > 0 {
				m.status = strings.Join(msg.snap.Warnings, " | ")
			}
			m.rebuildView()
		}
	case fastTickMsg:
		// Re-hydrate only — uses cache where possible so the fast
		// tick stays cheap.
		return m, tea.Batch(
			loadCmd(m.svc, false),
			tea.Tick(fastTick, func(t time.Time) tea.Msg { return fastTickMsg(t) }),
		)
	case slowTickMsg:
		return m, tea.Batch(
			loadCmd(m.svc, true),
			tea.Tick(slowTick, func(t time.Time) tea.Msg { return slowTickMsg(t) }),
		)
	case statusMsg:
		// Set by runResumeCmd's tea.ExecProcess callback (or any other
		// command that wants to surface a one-line outcome to the user).
		m.status = msg.text
	case tea.KeyMsg:
		cmd := m.handleKey(msg)
		return m, cmd
	}
	return m, nil
}

// handleKey dispatches keyboard input.
//
// While the search input is active (`/`), printable keys append to
// the buffer and Backspace pops the last rune. Esc / Enter close it.
// Outside search mode the standard dashboard shortcuts apply.
func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	if m.insertSearch {
		switch msg.String() {
		case "esc":
			m.insertSearch = false
			m.search = ""
			m.rebuildView()
		case "enter":
			m.insertSearch = false
		case "backspace":
			if r := []rune(m.search); len(r) > 0 {
				m.search = string(r[:len(r)-1])
				m.rebuildView()
			}
		default:
			s := msg.String()
			if len(s) == 1 {
				m.search += s
				m.rebuildView()
			}
		}
		return nil
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return tea.Quit
	case "?":
		m.help = !m.help
	case "/":
		m.insertSearch = true
		m.search = ""
	case "tab":
		m.tab = (m.tab + 1) % len(tabNames)
		m.rebuildView()
		m.cursor = 0
	case "shift+tab":
		m.tab = (m.tab - 1 + len(tabNames)) % len(tabNames)
		m.rebuildView()
		m.cursor = 0
	case "g":
		m.groupBy = nextGroupBy(m.groupBy)
		m.status = "group by: " + m.groupBy
		m.rebuildView()
	case "r":
		if s, ok := m.selected(); ok {
			return m.runResumeCmd(s)
		}
	case "c":
		if s, ok := m.selected(); ok && s.RestartCommand != "" {
			if err := clipboard.WriteAll(s.RestartCommand); err == nil {
				m.status = "copied resume command"
			} else {
				m.status = "copy failed: " + err.Error()
			}
		}
	case "s":
		if s, ok := m.selected(); ok {
			if err := toggleStar(s.ID); err == nil {
				m.status = "toggled star: " + s.ID
				// Force a fast re-hydrate to flip the star.
				return loadCmd(m.svc, false)
			} else {
				m.status = "star failed: " + err.Error()
			}
		}
	case "R":
		m.loading = true
		return loadCmd(m.svc, true)
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.flat)-1 {
			m.cursor++
		}
	case "home":
		m.cursor = 0
	case "end":
		if len(m.flat) > 0 {
			m.cursor = len(m.flat) - 1
		}
	}
	return nil
}

// selected returns the currently highlighted session.
func (m *Model) selected() (agents.Session, bool) {
	if m.cursor < 0 || m.cursor >= len(m.flat) {
		return agents.Session{}, false
	}
	return m.flat[m.cursor], true
}

// runResumeCmd hands control to the underlying agent CLI. tea.ExecProcess
// suspends the bubbletea program for the duration so signals (Ctrl-C)
// reach the agent rather than klim. The error returned by ExecProcess
// (non-zero exit, spawn failure) is surfaced back to the user as a
// status line so the dashboard doesn't quietly report success on a
// failed resume.
//
// Security note: we DO NOT shell out the session's RestartCommand
// string here. That field is paste-ready ("cd ... && claude --resume
// <id>") and only safe via copy-to-clipboard; piping it through
// /bin/sh -c (or cmd.exe /c) makes any unescaped shell metacharacter
// in ProjectPath (e.g. `;`, `$(...)`, backtick, `&`) a command-
// injection vector. Instead, we ask the provider's BuildLaunch for a
// clean (bin, args, cwd) triple and exec the binary directly with
// Cmd.Dir set — no shell layer at all.
func (m *Model) runResumeCmd(s agents.Session) tea.Cmd {
	cmd, err := m.buildResumeExec(s)
	if err != nil {
		msg := "resume: " + err.Error()
		return func() tea.Msg { return statusMsg{text: msg} }
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return statusMsg{text: "resume failed: " + err.Error()}
		}
		return statusMsg{text: "resumed (exit ok)"}
	})
}

// buildResumeExec returns an *exec.Cmd that will resume the session
// without going through any shell. Extracted so tests can pin the
// no-shell contract (the cmd's Path must be the agent binary, not
// /bin/sh / cmd.exe, regardless of what shell metacharacters might
// appear in the session's ProjectPath).
func (m *Model) buildResumeExec(s agents.Session) (*exec.Cmd, error) {
	provID := providerForSessionID(s.ID)
	if provID == "" {
		return nil, fmt.Errorf("cannot infer provider from id %q", s.ID)
	}
	if m.svc == nil {
		return nil, errors.New("no service wired")
	}
	p := m.svc.ProviderFor(provID)
	if p == nil {
		return nil, fmt.Errorf("provider %q not registered", provID)
	}
	plan, err := p.BuildLaunch(agents.LaunchSpec{
		Provider:  provID,
		SessionID: s.ID,
		Cwd:       s.ProjectPath,
	})
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(plan.Bin, plan.Args...)
	if plan.Cwd != "" {
		cmd.Dir = plan.Cwd
	}
	return cmd, nil
}

type statusMsg struct{ text string }

func providerForSessionID(id string) agents.ProviderID {
	switch {
	case strings.HasPrefix(id, "claude:"):
		return agents.ProviderClaudeCode
	case strings.HasPrefix(id, "copilot:"):
		return agents.ProviderCopilotCLI
	}
	return ""
}

// toggleStar flips the star state in the bookmarks store.
func toggleStar(id string) error {
	st, err := bookmarks.Load()
	if err != nil {
		return err
	}
	st.Toggle(id)
	return st.Save()
}

// nextGroupBy cycles through the three group-by modes.
func nextGroupBy(cur string) string {
	switch cur {
	case groupByProject:
		return groupByProvider
	case groupByProvider:
		return groupByNone
	default:
		return groupByProject
	}
}

// rebuildView refilters and resorts m.flat based on the current
// tab + search + groupBy. The view's order is what `cursor` indexes
// into.
//
// Active tab = anything except status=completed/stopped.
// Previous = completed | stopped.
func (m *Model) rebuildView() {
	m.flat = m.flat[:0]
	if m.snapshot == nil {
		return
	}
	q := strings.ToLower(strings.TrimSpace(m.search))
	for _, s := range m.snapshot.Sessions {
		if !matchesTab(s, m.tab) {
			continue
		}
		if q != "" && !matchesSearch(s, q) {
			continue
		}
		m.flat = append(m.flat, s)
	}
	// Sort: starred first, then state (working > thinking > waiting >
	// idle > unknown), then LastModified desc.
	sort.SliceStable(m.flat, func(i, j int) bool {
		a, b := m.flat[i], m.flat[j]
		if a.Starred != b.Starred {
			return a.Starred
		}
		if stateOrder(a.LiveState) != stateOrder(b.LiveState) {
			return stateOrder(a.LiveState) < stateOrder(b.LiveState)
		}
		return a.LastModified.After(b.LastModified)
	})
	if m.cursor >= len(m.flat) {
		m.cursor = len(m.flat) - 1
		if m.cursor < 0 {
			m.cursor = 0
		}
	}
}

func matchesTab(s agents.Session, tab int) bool {
	switch tab {
	case tabActive:
		return s.Status != agents.SessionStatusCompleted && s.Status != agents.SessionStatusStopped
	case tabPrevious:
		return s.Status == agents.SessionStatusCompleted || s.Status == agents.SessionStatusStopped
	default:
		return true
	}
}

func matchesSearch(s agents.Session, q string) bool {
	if strings.Contains(strings.ToLower(s.ID), q) {
		return true
	}
	if strings.Contains(strings.ToLower(s.Title), q) {
		return true
	}
	if strings.Contains(strings.ToLower(s.ProjectPath), q) {
		return true
	}
	if strings.Contains(strings.ToLower(s.Branch), q) {
		return true
	}
	if strings.Contains(strings.ToLower(s.Group), q) {
		return true
	}
	for _, mcp := range s.MCPServers {
		if strings.Contains(strings.ToLower(mcp), q) {
			return true
		}
	}
	return false
}

func stateOrder(st agents.LiveState) int {
	switch st {
	case agents.StateWorking:
		return 0
	case agents.StateThinking:
		return 1
	case agents.StateWaiting:
		return 2
	case agents.StateIdle:
		return 3
	}
	return 4
}

// ----- View -----

var (
	headerStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#A5E844")).Padding(0, 1)
	tabActiveStyle = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color("#A5E844"))
	tabIdleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#8B8B8B"))
	dimStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("#7F7F7F"))
	mutedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#9D9D9D"))
	starStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD166"))
	stateWorking   = lipgloss.NewStyle().Foreground(lipgloss.Color("#5BE49B"))
	stateThinking  = lipgloss.NewStyle().Foreground(lipgloss.Color("#5DD3FF"))
	stateWaiting   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD166"))
	stateIdle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#7F7F7F"))
	highlightStyle = lipgloss.NewStyle().Background(lipgloss.Color("#2C2C2C"))
)

// View satisfies tea.Model.
func (m *Model) View() tea.View {
	body := m.renderBody()
	v := tea.NewView(body)
	v.AltScreen = true
	return v
}

func (m *Model) renderBody() string {
	if m.help {
		return m.renderHelp()
	}
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteByte('\n')
	b.WriteString(m.renderTabs())
	b.WriteByte('\n')
	b.WriteString(m.renderSearch())
	b.WriteByte('\n')
	switch m.tab {
	case tabFiles:
		b.WriteString(m.renderFiles())
	case tabStats:
		b.WriteString(m.renderStats())
	default:
		b.WriteString(m.renderList())
		if s, ok := m.selected(); ok {
			b.WriteByte('\n')
			b.WriteString(m.renderDetail(s))
		}
	}
	b.WriteByte('\n')
	b.WriteString(m.renderFooter())
	return b.String()
}

func (m *Model) renderHeader() string {
	parts := []string{headerStyle.Render("klim sessions")}
	waiting := m.countLiveState(agents.StateWaiting)
	if waiting > 0 {
		parts = append(parts, stateWaiting.Render(fmt.Sprintf("⏳ %d waiting", waiting)))
	}
	if m.loading {
		parts = append(parts, mutedStyle.Render("loading…"))
	} else if !m.updatedAt.IsZero() {
		parts = append(parts, dimStyle.Render("updated "+enrich.RelativeTime(m.updatedAt, m.now())))
	}
	if m.loadErr != nil {
		parts = append(parts, stateWaiting.Render("err: "+m.loadErr.Error()))
	}
	return strings.Join(parts, "  ")
}

func (m *Model) renderTabs() string {
	parts := make([]string, len(tabNames))
	for i, name := range tabNames {
		label := fmt.Sprintf("%s (%d)", name, m.countForTab(i))
		if i == m.tab {
			parts[i] = tabActiveStyle.Render(label)
		} else {
			parts[i] = tabIdleStyle.Render(label)
		}
	}
	return strings.Join(parts, "  ")
}

func (m *Model) renderSearch() string {
	cursor := ""
	if m.insertSearch {
		cursor = "_"
	}
	prefix := dimStyle.Render("/")
	if m.insertSearch {
		prefix = tabActiveStyle.Render("/")
	}
	return prefix + " " + m.search + cursor
}

func (m *Model) renderList() string {
	if len(m.flat) == 0 {
		return mutedStyle.Render("  no sessions match")
	}
	groups := groupBy(m.flat, m.groupBy)
	var b strings.Builder
	row := 0
	for _, g := range groups {
		if g.name != "" {
			b.WriteString(headerStyle.Render(fmt.Sprintf(" %s (%d) ", g.name, len(g.items))))
			b.WriteByte('\n')
		}
		for _, s := range g.items {
			line := m.renderRow(s)
			if row == m.cursor {
				line = highlightStyle.Render(line)
			}
			b.WriteString(line)
			b.WriteByte('\n')
			row++
		}
	}
	return b.String()
}

type listGroup struct {
	name  string
	items []agents.Session
}

func groupBy(in []agents.Session, mode string) []listGroup {
	if mode == groupByNone {
		return []listGroup{{name: "", items: in}}
	}
	key := func(s agents.Session) string {
		if mode == groupByProvider {
			switch s.Provider {
			case agents.ProviderClaudeCode:
				return "claude"
			case agents.ProviderCopilotCLI:
				return "copilot"
			}
			return string(s.Provider)
		}
		if s.Group != "" {
			return s.Group
		}
		return "Other"
	}
	idx := map[string]int{}
	var groups []listGroup
	for _, s := range in {
		k := key(s)
		if i, ok := idx[k]; ok {
			groups[i].items = append(groups[i].items, s)
			continue
		}
		idx[k] = len(groups)
		groups = append(groups, listGroup{name: k, items: []agents.Session{s}})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].name < groups[j].name })
	return groups
}

func (m *Model) renderRow(s agents.Session) string {
	star := " "
	if s.Starred {
		star = starStyle.Render("★")
	}
	state := stateGlyph(s.LiveState)
	title := truncate(displayTitle(s), 50)
	branch := truncate(dashOr(s.Branch), 18)
	turns := dashOr(strconv.Itoa(s.TurnCount))
	if s.TurnCount == 0 {
		turns = "—"
	}
	modified := enrich.RelativeTime(s.LastModified, m.now())
	recent := truncate(dashOr(s.RecentActivity), 60)
	return fmt.Sprintf("  %s%s  %-28s  %-18s  %-50s  %5s  %10s  %s",
		state, star, truncate(s.ID, 28), branch, title, turns, modified, mutedStyle.Render(recent))
}

func (m *Model) renderDetail(s agents.Session) string {
	if s.ID == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(headerStyle.Render(" detail "))
	b.WriteByte('\n')
	pairs := [][2]string{
		{"id", s.ID},
		{"project", s.ProjectPath},
		{"branch", dashOr(s.Branch)},
		{"state", dashOr(string(s.LiveState))},
		{"resume", dashOr(s.RestartCommand)},
		{"recent", dashOr(s.RecentActivity)},
	}
	if s.WaitingContext != "" {
		pairs = append(pairs, [2]string{"waiting", s.WaitingContext})
	}
	if len(s.MCPServers) > 0 {
		pairs = append(pairs, [2]string{"mcps", strings.Join(s.MCPServers, ", ")})
	}
	for _, p := range pairs {
		fmt.Fprintf(&b, "  %s %s\n", dimStyle.Render(fmt.Sprintf("%-9s", p[0])), p[1])
	}
	return b.String()
}

func (m *Model) renderFooter() string {
	keys := []string{
		"q quit",
		"? help",
		"/ search",
		"tab next-tab",
		"g group",
		"r resume",
		"c copy",
		"s star",
		"R refresh",
	}
	parts := []string{dimStyle.Render(strings.Join(keys, "  "))}
	if m.status != "" {
		parts = append(parts, starStyle.Render(m.status))
	}
	return strings.Join(parts, "  ")
}

func (m *Model) renderHelp() string {
	return strings.Join([]string{
		headerStyle.Render(" klim sessions — help "),
		"",
		"  q / Ctrl-C   quit",
		"  /            start search (Esc to clear, Enter to apply)",
		"  Tab / S-Tab  cycle tabs (Active / Previous / Files / Stats)",
		"  g            cycle group-by (project / provider / none)",
		"  r            resume the selected session (launches the agent CLI)",
		"  c            copy the resume command to the clipboard",
		"  s            toggle star (pins to top, re-saved to bookmarks)",
		"  R            force refresh (skip cache)",
		"  arrows/j-k   move cursor",
		"  ?            toggle this help",
		"",
		dimStyle.Render("press ? again to return"),
	}, "\n")
}

func (m *Model) renderFiles() string {
	if m.snapshot == nil {
		return mutedStyle.Render("  no data")
	}
	// Reuse the per-session ToolCounts to surface top-tool projects
	// without re-parsing every transcript — this keeps the TUI fast.
	type entry struct {
		name  string
		count int
	}
	bucket := map[string]int{}
	for _, s := range m.snapshot.Sessions {
		for tool, c := range s.ToolCounts {
			if tool == "Edit" || tool == "Write" || tool == "NotebookEdit" {
				bucket[s.ProjectPath] += c
			}
		}
	}
	if len(bucket) == 0 {
		return mutedStyle.Render("  no edit activity recorded yet")
	}
	var rows []entry
	for k, v := range bucket {
		rows = append(rows, entry{k, v})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].count > rows[j].count })
	peak := rows[0].count
	var b strings.Builder
	for i, r := range rows {
		if i >= 20 {
			break
		}
		bar := strings.Repeat("█", (r.count*30+peak-1)/peak)
		fmt.Fprintf(&b, "  %4d  %s  %s\n", r.count, stateWorking.Render(bar), truncate(r.name, 80))
	}
	return b.String()
}

func (m *Model) renderStats() string {
	if m.snapshot == nil {
		return mutedStyle.Render("  no data")
	}
	totalTurns := 0
	totalTools := 0
	totalSubagents := 0
	bg := 0
	live := map[agents.LiveState]int{}
	provider := map[agents.ProviderID]int{}
	for _, s := range m.snapshot.Sessions {
		totalTurns += s.TurnCount
		totalSubagents += s.SubagentRuns
		bg += s.BackgroundTasks
		live[s.LiveState]++
		provider[s.Provider]++
		for _, c := range s.ToolCounts {
			totalTools += c
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "  total sessions:   %d\n", len(m.snapshot.Sessions))
	fmt.Fprintf(&b, "  total turns:      %d\n", totalTurns)
	fmt.Fprintf(&b, "  total tool calls: %d\n", totalTools)
	fmt.Fprintf(&b, "  subagent runs:    %d\n", totalSubagents)
	fmt.Fprintf(&b, "  background tasks: %d\n", bg)
	b.WriteByte('\n')
	b.WriteString(headerStyle.Render(" by live state "))
	b.WriteByte('\n')
	for _, st := range []agents.LiveState{agents.StateWorking, agents.StateThinking, agents.StateWaiting, agents.StateIdle, agents.StateUnknown} {
		fmt.Fprintf(&b, "  %-9s %d\n", stateLabel(st), live[st])
	}
	b.WriteByte('\n')
	b.WriteString(headerStyle.Render(" by provider "))
	b.WriteByte('\n')
	keys := make([]agents.ProviderID, 0, len(provider))
	for k := range provider {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return string(keys[i]) < string(keys[j]) })
	for _, k := range keys {
		fmt.Fprintf(&b, "  %-12s %d\n", k, provider[k])
	}
	return b.String()
}

// ----- helpers -----

func (m *Model) countLiveState(st agents.LiveState) int {
	if m.snapshot == nil {
		return 0
	}
	n := 0
	for _, s := range m.snapshot.Sessions {
		if s.LiveState == st {
			n++
		}
	}
	return n
}

func (m *Model) countForTab(tab int) int {
	if m.snapshot == nil {
		return 0
	}
	n := 0
	for _, s := range m.snapshot.Sessions {
		if matchesTab(s, tab) {
			n++
		}
	}
	return n
}

func stateGlyph(st agents.LiveState) string {
	switch st {
	case agents.StateWorking:
		return stateWorking.Render("●")
	case agents.StateThinking:
		return stateThinking.Render("◐")
	case agents.StateWaiting:
		return stateWaiting.Render("▲")
	case agents.StateIdle:
		return stateIdle.Render("○")
	}
	return stateIdle.Render("·")
}

func stateLabel(st agents.LiveState) string {
	if st == "" {
		return "unknown"
	}
	return string(st)
}

func dashOr(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func truncate(s string, n int) string {
	if n <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n < 2 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

func displayTitle(s agents.Session) string {
	if s.Title != "" {
		return s.Title
	}
	if s.Repository != "" {
		return s.Repository
	}
	return s.ProjectPath
}
