package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/agents/health"
)

// agentsHealthState holds Agents → Health sub-tab state.
type agentsHealthState struct {
	loaded   bool
	loading  bool
	loadedAt time.Time
	issues   []health.Issue
	cursor   int
}

// agentsHealthLoadedMsg arrives after the background check run.
type agentsHealthLoadedMsg struct {
	issues []health.Issue
}

// agentsHealthLoadCmd kicks off a background diagnostic run over the
// current snapshot.
func (m *Model) agentsHealthLoadCmd() tea.Cmd {
	if m.agents == nil {
		return nil
	}
	st := m.agents
	st.healthSub.loading = true
	snap := st.snapshot
	return func() tea.Msg {
		issues := runAgentsHealthChecks(snap)
		return agentsHealthLoadedMsg{issues: issues}
	}
}

// runAgentsHealthChecks adapts the agents.Snapshot into a
// health.Snapshot and executes the default check set.
func runAgentsHealthChecks(snap *agents.Snapshot) []health.Issue {
	b := health.NewSnapshotBuilder()
	if snap != nil {
		for _, mp := range snap.Marketplaces {
			b.AddMarketplace(mp.Name, string(mp.Provider), mp.URL, string(mp.Source), mp.LastSynced)
		}
		for _, p := range snap.Plugins {
			b.AddPlugin(p.Name, string(p.Provider), p.Marketplace, p.Installed, p.Enabled, p.InstallPath, p.Version)
		}
		for _, s := range snap.Skills {
			b.AddSkill(s.Name, string(s.Provider), string(s.Scope), s.SourcePlugin, s.Path)
		}
		for _, mc := range snap.MCPs {
			b.AddMCP(mc.Name, string(mc.Provider), string(mc.Scope), mc.Transport, mc.Command, mc.Args, mc.URL)
		}
		for id, status := range snap.ProviderStatus {
			b.AddProvider(string(id), status.Installed, status.Version)
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		b.AddConfigFile(filepath.Join(home, ".claude.json"), "claude-code")
		b.AddConfigFile(filepath.Join(home, ".copilot", "mcp-config.json"), "copilot-cli")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return health.Run(ctx, b.Build(), health.DefaultChecks(health.DefaultHTTPProbe))
}

// handleAgentsHealthKey routes input while the Health sub-tab is active.
func (m *Model) handleAgentsHealthKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	st := m.agents
	hs := &st.healthSub
	switch msg.String() {
	case "down", "j":
		if hs.cursor < len(hs.issues)-1 {
			hs.cursor++
		}
		return true, nil
	case "up", "k":
		if hs.cursor > 0 {
			hs.cursor--
		}
		return true, nil
	case "r":
		st.flash = "running diagnostics…"
		st.flashEnd = time.Now().Add(2 * time.Second)
		return true, m.agentsHealthLoadCmd()
	case "enter":
		if hs.cursor < 0 || hs.cursor >= len(hs.issues) {
			return true, nil
		}
		issue := hs.issues[hs.cursor]
		if id, sub, ok := healthIssueDetailTarget(issue, st.snapshot); ok {
			st.detailPage = true
			st.detailStack = []agentDetailFrame{{
				subTab:   sub,
				entityID: id,
			}}
		}
		return true, nil
	}
	return false, nil
}

// healthIssueDetailTarget maps an Issue to a (sub-tab, entity-id)
// pair so Enter drills into the existing detail-page machinery.
func healthIssueDetailTarget(issue health.Issue, snap *agents.Snapshot) (string, int, bool) {
	if snap == nil {
		return "", 0, false
	}
	switch issue.Kind {
	case health.KindPlugin:
		for _, p := range snap.Plugins {
			if p.Name == issue.Subject && string(p.Provider) == issue.Provider {
				return p.ID, agentsSubPlugins, true
			}
		}
	case health.KindMCP:
		for _, m := range snap.MCPs {
			if m.Name == issue.Subject && string(m.Provider) == issue.Provider {
				return m.ID, agentsSubMCPs, true
			}
		}
	case health.KindSkill:
		for _, s := range snap.Skills {
			if s.Name == issue.Subject && string(s.Provider) == issue.Provider {
				return s.ID, agentsSubSkills, true
			}
		}
	case health.KindMarketplace:
		for _, mp := range snap.Marketplaces {
			if mp.Name == issue.Subject {
				return mp.ID, agentsSubMarketplaces, true
			}
		}
	}
	return "", 0, false
}

// renderAgentsHealthView produces the Health sub-tab body.
func (m *Model) renderAgentsHealthView() string {
	st := m.agents
	hs := &st.healthSub
	var b strings.Builder

	if hs.loading && !hs.loaded {
		b.WriteString("  running diagnostic checks…\n")
		return b.String()
	}
	if !hs.loaded {
		// PR #77 review: `7` now switches to the Health *parent* tab,
		// not a scan inside Agents → Health. The actual refresh key
		// handled in handleAgentsHealthKey is `r`.
		b.WriteString("  press r to scan the agent ecosystem for problems\n")
		return b.String()
	}

	counts := health.CountIssues(hs.issues)
	pillErr := lipgloss.NewStyle().Foreground(cyberAlert).Bold(true).Render(fmt.Sprintf("✗ %d errors", counts.Error))
	pillWarn := lipgloss.NewStyle().Foreground(cyberAccent).Bold(true).Render(fmt.Sprintf("⚠ %d warnings", counts.Warn))
	pillInfo := lipgloss.NewStyle().Foreground(cyberInfo).Render(fmt.Sprintf("• %d info", counts.Info))
	b.WriteString(fmt.Sprintf("  %s   %s   %s   %s\n",
		pillErr, pillWarn, pillInfo,
		dimVersion.Render("r refresh · ↑/↓ navigate · Enter drill into entity"),
	))
	b.WriteString("\n")

	if len(hs.issues) == 0 {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(cyberOK).Bold(true).Render("✓ no problems detected") + "\n")
		b.WriteString("  " + dimVersion.Render("scanned "+humaniseTime(hs.loadedAt)) + "\n")
		return b.String()
	}

	cols := computeColumnWidths([]column{
		{header: "SEV", width: 7},
		{header: "PROVIDER", width: 10},
		{header: "KIND", width: 12},
		{header: "SUBJECT", width: 24},
		{header: "ISSUE", grow: true},
	}, m.width)
	b.WriteString(renderHeader(cols, -1))

	// Windowed issue list — reserve rows for the header (already
	// emitted), the detail pane below, and the "scanned …" footer.
	// The detail pane takes ~4 lines; summary + header + footer ~5.
	dataRows := m.height - 12
	if dataRows < 3 {
		dataRows = 3
	}
	total := len(hs.issues)
	start, hiddenAbove, hiddenBelow, windowSize := windowWithIndicators(total, hs.cursor, dataRows)

	if hiddenAbove > 0 {
		b.WriteString("  " + dimVersion.Render(fmt.Sprintf("↑ %d above", hiddenAbove)) + "\n")
	}

	for vi := start; vi < total && vi < start+windowSize; vi++ {
		is := hs.issues[vi]
		sev := healthSeverityChip(is.Severity)
		cells := []string{
			sev,
			truncAgentRow(is.Provider, cols[1].width),
			truncAgentRow(string(is.Kind), cols[2].width),
			truncAgentRow(is.Subject, cols[3].width),
			truncAgentRow(is.Title, cols[4].width),
		}
		b.WriteString(renderRow(cells, cols, rowLead(vi, hs.cursor), vi == hs.cursor, m.width))
	}

	if hiddenBelow > 0 {
		b.WriteString("  " + dimVersion.Render(fmt.Sprintf("↓ %d below", hiddenBelow)) + "\n")
	}

	if hs.cursor >= 0 && hs.cursor < len(hs.issues) {
		focus := hs.issues[hs.cursor]
		b.WriteString("\n  ─── detail ───\n")
		if focus.Detail != "" {
			b.WriteString("  " + focus.Detail + "\n")
		}
		if focus.Hint != "" {
			b.WriteString("  " + lipgloss.NewStyle().Foreground(cyberAccent).Render("hint: ") + focus.Hint + "\n")
		}
		b.WriteString("  " + dimVersion.Render(fmt.Sprintf("check %s · %s · %s", focus.CheckID, focus.Provider, focus.Kind)) + "\n")
	}

	b.WriteString("\n  " + dimVersion.Render("scanned "+humaniseTime(hs.loadedAt)) + "\n")
	return b.String()
}

// healthSeverityChip renders a coloured severity chip.
func healthSeverityChip(s health.Severity) string {
	fg := cyberInfo
	switch s {
	case health.SeverityError:
		fg = cyberAlert
	case health.SeverityWarn:
		fg = cyberAccent
	}
	return lipgloss.NewStyle().
		Foreground(fg).
		Background(cyberChipBg).
		Padding(0, 1).
		Bold(true).
		Render(s.String())
}
