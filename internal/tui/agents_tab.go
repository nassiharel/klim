package tui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
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
		st.setSubTab((st.subTab + 1) % agentsSubCount)
		return true, nil
	case "shift+tab", "left":
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
		for _, x := range st.snapshot.Marketplaces {
			rows = append(rows, agentRow{id: x.ID, name: x.Name, subtitle: x.Description, provider: x.Provider, source: x.Source})
		}
	case agentsSubPlugins:
		for _, x := range st.snapshot.Plugins {
			sub := x.Description
			if x.Marketplace != "" {
				sub = "[" + x.Marketplace + "] " + sub
			}
			rows = append(rows, agentRow{id: x.ID, name: x.Name, subtitle: sub, provider: x.Provider, source: x.Source, enabled: x.Enabled})
		}
	case agentsSubSkills:
		for _, x := range st.snapshot.Skills {
			rows = append(rows, agentRow{id: x.ID, name: x.Name, subtitle: x.Description, provider: x.Provider, source: x.Source, scope: x.Scope, enabled: x.Enabled})
		}
	case agentsSubMCPs:
		for _, x := range st.snapshot.MCPs {
			sub := x.Transport
			if x.URL != "" {
				sub += " " + x.URL
			} else if x.Command != "" {
				sub += " " + x.Command
			}
			rows = append(rows, agentRow{id: x.ID, name: x.Name, subtitle: sub, provider: x.Provider, source: x.Source, scope: x.Scope, enabled: x.Enabled})
		}
	case agentsSubSessions:
		for _, x := range st.snapshot.Sessions {
			label := x.Name
			if label == "" {
				label = x.ID
			}
			sub := x.ProjectPath
			if !x.LastModified.IsZero() {
				sub += " · " + x.LastModified.Format(time.RFC3339)
			}
			rows = append(rows, agentRow{id: x.ID, name: label, subtitle: sub, provider: x.Provider, source: x.Source})
		}
	}
	return filterAgentRows(rows, q)
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
	for i, r := range rows {
		if i >= maxRows {
			fmt.Fprintf(&b, "  … %d more (search to narrow)\n", len(rows)-maxRows)
			break
		}
		cursor := "  "
		if i == st.cursor {
			cursor = "▸ "
		}
		marker := ""
		if r.source == agents.SourceCatalogClaude || r.source == agents.SourceCatalogCopilot || r.source == agents.SourceCatalogMCP {
			marker = " (catalog)"
		}
		fmt.Fprintf(&b, "%s%-30s  %-12s  %s%s\n",
			cursor, truncAgentRow(r.name, 30), r.provider, truncAgentRow(r.subtitle, 60), marker)
	}

	if st.detailOpen && st.cursor < len(rows) {
		r := rows[st.cursor]
		b.WriteString("\n  ─── detail ───\n")
		fmt.Fprintf(&b, "  id        %s\n", r.id)
		fmt.Fprintf(&b, "  provider  %s\n", r.provider)
		fmt.Fprintf(&b, "  source    %s\n", r.source)
		if r.scope != "" {
			fmt.Fprintf(&b, "  scope     %s\n", r.scope)
		}
		if r.subtitle != "" {
			b.WriteString("  detail    " + r.subtitle + "\n")
		}
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
