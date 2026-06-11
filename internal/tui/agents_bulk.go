package tui

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/agents"
)

func agentsBulkCapable(subTab int) bool {
	switch subTab {
	case agentsSubPlugins, agentsSubMCPs, agentsSubSessions:
		return true
	}
	return false
}

func agentsSelected(st *agentsState, subTab int) map[string]bool {
	if st == nil {
		return map[string]bool{}
	}
	if st.selected == nil {
		st.selected = map[int]map[string]bool{}
	}
	s, ok := st.selected[subTab]
	if !ok {
		s = map[string]bool{}
		st.selected[subTab] = s
	}
	return s
}

func agentsToggleSelection(st *agentsState, subTab int, id string) {
	if id == "" {
		return
	}
	set := agentsSelected(st, subTab)
	if set[id] {
		delete(set, id)
		return
	}
	set[id] = true
}

func agentsClearSelection(st *agentsState, subTab int) {
	if st == nil || st.selected == nil {
		return
	}
	delete(st.selected, subTab)
}

func agentsSelectionCount(st *agentsState, subTab int) int {
	if st == nil || st.selected == nil {
		return 0
	}
	return len(st.selected[subTab])
}

func agentsBulkSummary(st *agentsState) string {
	n := agentsSelectionCount(st, st.subTab)
	if n == 0 {
		return ""
	}
	style := lipgloss.NewStyle().Foreground(cyberAccent).Bold(true)
	hint := dimVersion.Render(bulkHintFor(st.subTab))
	return style.Render(fmt.Sprintf("☑ %d selected", n)) + "  " + hint
}

func bulkHintFor(subTab int) string {
	switch subTab {
	case agentsSubPlugins:
		return "Shift+A all · Shift+I install · Shift+U update · Shift+X uninstall · Esc clear"
	case agentsSubMCPs:
		return "Shift+A all · Shift+E enable · Shift+D disable · Shift+R remove · Esc clear"
	case agentsSubSessions:
		return "Shift+A all · Shift+B bookmark · Shift+X delete · Esc clear"
	}
	return ""
}

func agentsBulkActionForKey(m *Model, key string) (label string, action func() tea.Cmd, ok bool) {
	st := m.agents
	if st == nil || !agentsBulkCapable(st.subTab) {
		return "", nil, false
	}
	rows := m.agentsVisibleRows()
	selected := agentsSelected(st, st.subTab)
	if len(selected) == 0 {
		return "", nil, false
	}
	var targets []agentRow
	for _, r := range rows {
		if selected[r.id] {
			targets = append(targets, r)
		}
	}
	if len(targets) == 0 {
		return "", nil, false
	}

	switch st.subTab {
	case agentsSubPlugins:
		switch key {
		case "I":
			return "Install", bulkProviderCmd("installed", targets, func(p agents.Provider, ctx context.Context, row agentRow) error {
				return p.InstallPlugin(ctx, agents.PluginRef{Name: row.plugin.Name, Marketplace: row.plugin.Marketplace})
			}), true
		case "U":
			return "Update", bulkProviderCmd("updated", targets, func(p agents.Provider, ctx context.Context, row agentRow) error {
				return p.UpdatePlugin(ctx, row.plugin.Name)
			}), true
		case "X":
			return "Uninstall", bulkProviderCmd("uninstalled", targets, func(p agents.Provider, ctx context.Context, row agentRow) error {
				return p.UninstallPlugin(ctx, row.plugin.Name)
			}), true
		}
	case agentsSubMCPs:
		switch key {
		case "E":
			return "Enable", bulkProviderCmd("enabled", targets, func(p agents.Provider, ctx context.Context, row agentRow) error {
				return p.EnableMCP(ctx, row.mcp.Name, true)
			}), true
		case "D":
			return "Disable", bulkProviderCmd("disabled", targets, func(p agents.Provider, ctx context.Context, row agentRow) error {
				return p.EnableMCP(ctx, row.mcp.Name, false)
			}), true
		case "R":
			return "Remove", bulkProviderCmd("removed", targets, func(p agents.Provider, ctx context.Context, row agentRow) error {
				return p.RemoveMCP(ctx, row.mcp.Name)
			}), true
		}
	case agentsSubSessions:
		switch key {
		case "B":
			return "Bookmark", bulkLocalCmd("bookmarked", targets, func(row agentRow) error {
				bm := agentsBookmarks(m.agents)
				bm.Add(row.id, "")
				return bm.Save()
			}), true
		case "X":
			return "Delete", bulkProviderCmd("deleted", targets, func(p agents.Provider, ctx context.Context, row agentRow) error {
				return p.DeleteSession(ctx, row.id)
			}), true
		}
	}
	return "", nil, false
}

type agentsBulkResultMsg struct {
	verb     string
	ok       int
	failed   int
	firstErr error
}

func bulkProviderCmd(verb string, targets []agentRow, do func(p agents.Provider, ctx context.Context, row agentRow) error) func() tea.Cmd {
	return func() tea.Cmd {
		return func() tea.Msg {
			svc := agentsService()
			ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
			defer cancel()
			res := agentsBulkResultMsg{verb: verb}
			for _, row := range targets {
				p := svc.ProviderFor(row.provider)
				if p == nil {
					res.failed++
					if res.firstErr == nil {
						res.firstErr = fmt.Errorf("provider %q not registered", row.provider)
					}
					continue
				}
				if err := do(p, ctx, row); err != nil {
					res.failed++
					if res.firstErr == nil {
						res.firstErr = err
					}
					continue
				}
				res.ok++
			}
			return res
		}
	}
}

func bulkLocalCmd(verb string, targets []agentRow, do func(row agentRow) error) func() tea.Cmd {
	return func() tea.Cmd {
		return func() tea.Msg {
			res := agentsBulkResultMsg{verb: verb}
			for _, row := range targets {
				if err := do(row); err != nil {
					res.failed++
					if res.firstErr == nil {
						res.firstErr = err
					}
					continue
				}
				res.ok++
			}
			return res
		}
	}
}

func agentsBulkRenderSummary(st *agentsState) string {
	if !agentsBulkCapable(st.subTab) {
		return ""
	}
	return agentsBulkSummary(st)
}

func renderBulkConfirmPrompt(st *agentsState, totalWidth int) string {
	if st.bulkPrompt == "" {
		return ""
	}
	return "\n" + renderConfirmModal(
		"⚠ Confirm bulk action",
		st.bulkPrompt,
		"y/Enter = apply · n/Esc = cancel",
		totalWidth,
	) + "\n"
}
