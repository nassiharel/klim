package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	tea "charm.land/bubbletea/v2"

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

	launchPrompt string // non-empty while the launch confirmation modal is up
	launchPlan   agents.ExecPlan

	flash    string
	flashEnd time.Time
}

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
	if m.agents.snapshot != nil && !m.agents.loadedAt.IsZero() {
		return nil
	}
	m.agents.loading = true
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
	return filterAgentRows(rows, q)
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

func (m *Model) renderAgentsView() string {
	if m.agents == nil {
		m.agents = newAgentsState()
	}
	st := m.agents
	var b strings.Builder

	subs := []string{"Marketplaces", "Plugins", "Skills", "MCPs", "Sessions"}
	var parts []string
	for i, label := range subs {
		if i == st.subTab {
			parts = append(parts, cyberSubtabActive(label))
		} else {
			parts = append(parts, cyberSubtabInactive(label))
		}
	}
	b.WriteString("  " + strings.Join(parts, "  ") + "\n\n")

	switch {
	case st.searchActive:
		b.WriteString("  search: " + st.searchInput + "▌\n")
	case st.searchInput != "":
		b.WriteString("  filter: " + st.searchInput + "  (/  edit · esc clear)\n")
	default:
		b.WriteString("  /  search       l  launch       r  refresh       enter  detail       1-5  sub-tab\n")
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
		b.WriteString(renderAgentDetail(rows[st.cursor]))
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

// renderAgentsTable produces an aligned table for the current sub-tab.
// Every sub-tab leads with a SOURCE column showing the short provider
// label (claude / copilot / mcp-reg / …) so provenance is always
// visible without expanding the detail pane.
func renderAgentsTable(subTab int, rows []agentRow, cursor int) string {
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	switch subTab {
	case agentsSubMarketplaces:
		_, _ = fmt.Fprintln(tw, "  \tSOURCE\tNAME\tOWNER\tPLUGINS\tURL")
		for i, r := range rows {
			mp := r.marketplace
			owner, url, count := "", "", 0
			if mp != nil {
				owner, url, count = mp.Owner, mp.URL, mp.PluginCount
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				cursorMark(i, cursor),
				providerShort(r.provider),
				truncAgentRow(r.name, 32),
				truncAgentRow(owner, 16),
				dashOrInt(count),
				truncAgentRow(url, 48),
			)
		}
	case agentsSubPlugins:
		_, _ = fmt.Fprintln(tw, "  \tSOURCE\tNAME\tVERSION\tMARKETPLACE\tSTATUS\tDESCRIPTION")
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
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				cursorMark(i, cursor),
				providerShort(r.provider),
				truncAgentRow(r.name, 32),
				truncAgentRow(version, 10),
				truncAgentRow(market, 22),
				status,
				truncAgentRow(desc, 50),
			)
		}
	case agentsSubSkills:
		_, _ = fmt.Fprintln(tw, "  \tSOURCE\tNAME\tSCOPE\tFROM\tMODEL\tDESCRIPTION")
		for i, r := range rows {
			sk := r.skill
			model, from, desc := "", "", ""
			if sk != nil {
				model, from, desc = sk.Model, sk.SourcePlugin, sk.Description
				if from == "" {
					from = string(sk.Scope)
				}
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				cursorMark(i, cursor),
				providerShort(r.provider),
				truncAgentRow(r.name, 32),
				string(r.scope),
				truncAgentRow(from, 20),
				truncAgentRow(model, 10),
				truncAgentRow(desc, 50),
			)
		}
	case agentsSubMCPs:
		_, _ = fmt.Fprintln(tw, "  \tSOURCE\tNAME\tTRANSPORT\tSCOPE\tTOOLS\tENDPOINT")
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
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				cursorMark(i, cursor),
				providerShort(r.provider),
				truncAgentRow(r.name, 32),
				transport,
				string(r.scope),
				dashOrInt(tools),
				truncAgentRow(endpoint, 48),
			)
		}
	case agentsSubSessions:
		_, _ = fmt.Fprintln(tw, "  \tSOURCE\tID\tTYPE\tSTATUS\tTURNS\tCREATED\tMODIFIED\tPROJECT")
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
				status = string(s.Status)
				if status == "" {
					status = "—"
				}
				created = humaniseTime(s.Created)
				modified = humaniseTime(s.LastModified)
			}
			_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				cursorMark(i, cursor),
				providerShort(r.provider),
				truncAgentRow(r.name, 10),
				typ,
				status,
				dashOrInt(turns),
				created,
				modified,
				truncAgentRow(project, 36),
			)
		}
	}
	_ = tw.Flush()
	return b.String()
}

func cursorMark(i, cursor int) string {
	if i == cursor {
		return "▸"
	}
	return " "
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
func renderAgentDetail(r agentRow) string {
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
