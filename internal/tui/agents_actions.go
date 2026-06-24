package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/agents/promote"
)

// agentActionResultMsg lands in handleAgentsMsg when an action
// completes. The result is shown as a flash and triggers a re-scan
// so the detail page picks up the new state.
type agentActionResultMsg struct {
	label string
	err   error
}

// actionFailedFlash builds the flash shown when a provider action
// fails. The label is a past-tense SUCCESS phrase ("deleted session
// X", "installed Y"); on failure we must not render it as if it
// happened — the old "✗ deleted session X: ..." read as a
// contradiction (done, but also an error). We frame it as a failure
// and translate the known sentinel errors into an actionable hint.
func actionFailedFlash(label string, err error) string {
	msg := "✗ failed — " + label
	switch {
	case errors.Is(err, agents.ErrProviderNotInstalled):
		return msg + ": the provider CLI isn't installed or isn't on your PATH"
	case errors.Is(err, agents.ErrNotSupported):
		return msg + ": not supported by this provider"
	default:
		return msg + ": " + err.Error()
	}
}

// agentAction describes one button on the detail-page action bar.
type agentAction struct {
	label     string // short verb shown on the bar
	disabled  bool
	reason    string // why disabled — shown as flash on Enter
	run       func() tea.Cmd
	highlight bool // primary action gets a brighter render
}

// agentsBuildActions returns the action list for the entity in the
// current detail frame. Order matters: the first enabled action is
// the default focus on first open.
func (m *Model) agentsBuildActions(frame agentDetailFrame, row agentRow) []agentAction {
	switch frame.subTab {
	case agentsSubMarketplaces:
		return m.actionsForMarketplace(frame, row)
	case agentsSubPlugins:
		return m.actionsForPlugin(frame, row)
	case agentsSubSkills:
		return m.actionsForSkill(frame, row)
	case agentsSubMCPs:
		return m.actionsForMCP(frame, row)
	case agentsSubSessions:
		return m.actionsForSession(frame, row)
	}
	return nil
}

// ---------- per-entity action sets ----------

func (m *Model) actionsForMarketplace(frame agentDetailFrame, row agentRow) []agentAction {
	mp := row.marketplace
	if mp == nil {
		return nil
	}
	url := mp.URL
	spec := mp.InstallSpec
	if spec == "" {
		spec = url
	}
	if !mp.Installed {
		// Discoverable (not yet installed) marketplace: surface
		// "Add to library" as the primary action and hide
		// install-only actions like Remove.
		return []agentAction{
			{label: "Add to library", highlight: true, disabled: spec == "",
				reason: "no install spec recorded for this marketplace",
				run: func() tea.Cmd {
					return providerActionCmd("added marketplace "+mp.Name, func(ctx context.Context, p agents.Provider) error {
						return p.AddMarketplace(ctx, spec)
					}, mp.Provider)
				}},
			{label: "Open URL", disabled: url == "", reason: "no URL recorded", run: func() tea.Cmd {
				return openURLCmd(url)
			}},
			{label: "Copy URL", disabled: url == "", reason: "no URL recorded", run: func() tea.Cmd {
				return copyTextCmd(url, "marketplace URL")
			}},
			{label: "Refresh", run: refreshAgentsCmd},
		}
	}
	return []agentAction{
		{label: "View all plugins →", highlight: true, disabled: m.marketplacePluginCount(mp) == 0,
			reason: "no plugins from this marketplace in the current snapshot",
			run:    viewMarketplacePluginsCmd},
		{label: "Refresh", run: refreshAgentsCmd},
		{label: "Remove", disabled: mp.Source == agents.SourceCatalogClaude || mp.Source == agents.SourceCatalogCopilot, reason: "built-in marketplace cannot be removed",
			run: func() tea.Cmd {
				return providerActionCmd("removed marketplace "+mp.Name, func(ctx context.Context, p agents.Provider) error {
					return p.RemoveMarketplace(ctx, mp.Name)
				}, mp.Provider)
			}},
		{label: "Open URL", disabled: url == "", reason: "no URL recorded", run: func() tea.Cmd {
			return openURLCmd(url)
		}},
		{label: "Copy URL", disabled: url == "", reason: "no URL recorded", run: func() tea.Cmd {
			return copyTextCmd(url, "marketplace URL")
		}},
	}
}

func (m *Model) actionsForPlugin(frame agentDetailFrame, row agentRow) []agentAction {
	pl := row.plugin
	if pl == nil {
		return nil
	}
	id := pl.Name
	ref := agents.PluginRef{Name: pl.Name, Marketplace: pl.Marketplace}
	prov := pl.Provider
	skillCount := m.pluginSkillCount(pl)

	var actions []agentAction

	if !pl.Installed {
		// Not installed: offer Install as primary + browse actions.
		actions = append(actions,
			agentAction{label: "Install", highlight: true,
				run: func() tea.Cmd {
					return providerActionCmd("installed "+id, func(ctx context.Context, p agents.Provider) error {
						return p.InstallPlugin(ctx, ref)
					}, prov)
				}},
		)
	} else {
		// Installed: offer Update as primary + management actions.
		actions = append(actions,
			agentAction{label: "Update", highlight: true,
				run: func() tea.Cmd {
					return providerActionCmd("updated "+id, func(ctx context.Context, p agents.Provider) error {
						return p.UpdatePlugin(ctx, id)
					}, prov)
				}},
		)
		if skillCount > 0 {
			actions = append(actions,
				agentAction{label: "View skills →", run: viewPluginSkillsCmd},
			)
		}
		if pl.Enabled {
			actions = append(actions,
				agentAction{label: "Disable",
					run: func() tea.Cmd {
						return providerActionCmd("disabled "+id, func(ctx context.Context, p agents.Provider) error {
							return p.EnablePlugin(ctx, id, false)
						}, prov)
					}},
			)
		} else {
			actions = append(actions,
				agentAction{label: "Enable",
					run: func() tea.Cmd {
						return providerActionCmd("enabled "+id, func(ctx context.Context, p agents.Provider) error {
							return p.EnablePlugin(ctx, id, true)
						}, prov)
					}},
			)
		}
		actions = append(actions,
			agentAction{label: "Launch", run: func() tea.Cmd {
				return launchFromDetailCmd(prov, agents.LaunchSpec{Provider: prov, PluginName: pl.Name})
			}},
			agentAction{label: "Uninstall",
				run: func() tea.Cmd {
					return providerActionCmd("uninstalled "+id, func(ctx context.Context, p agents.Provider) error {
						return p.UninstallPlugin(ctx, id)
					}, prov)
				}},
		)
	}

	// Browse actions — only show when URLs exist.
	if pl.Homepage != "" {
		actions = append(actions, agentAction{label: "Open Homepage", run: func() tea.Cmd {
			return openURLCmd(pl.Homepage)
		}})
	}
	if pl.Repository != "" {
		actions = append(actions, agentAction{label: "Open Repo", run: func() tea.Cmd {
			return openURLCmd(pl.Repository)
		}})
	}
	actions = append(actions,
		agentAction{label: "Copy Install", run: func() tea.Cmd {
			text, _ := rowCopyText(row)
			return copyTextCmd(text, "install command")
		}},
		promoteAction(m.agents, promote.KindPlugin, string(prov), pl.Name, pluginPromoteReason(pl)),
	)
	return actions
}

func pluginPromoteReason(pl *agents.Plugin) string {
	if pl == nil {
		return "no plugin"
	}
	if pl.Marketplace == "" {
		return "plugin has no marketplace; can't be promoted"
	}
	return ""
}

func (m *Model) actionsForSkill(frame agentDetailFrame, row agentRow) []agentAction {
	sk := row.skill
	if sk == nil {
		return nil
	}
	prov := sk.Provider
	return []agentAction{
		{label: "Launch", highlight: true, run: func() tea.Cmd {
			return launchFromDetailCmd(prov, agents.LaunchSpec{Provider: prov, SkillName: sk.Name})
		}},
		{label: "Copy Invocation", run: func() tea.Cmd { return copyTextCmd("/"+sk.Name, "skill invocation") }},
		{label: "Open Path", disabled: sk.Path == "", reason: "no path", run: func() tea.Cmd {
			return openURLCmd(sk.Path)
		}},
		promoteAction(m.agents, promote.KindSkill, string(prov), sk.Name, ""),
	}
}

func (m *Model) actionsForMCP(frame agentDetailFrame, row agentRow) []agentAction {
	mc := row.mcp
	if mc == nil {
		return nil
	}
	prov := mc.Provider
	remote := mc.Scope == agents.ScopeRemote
	return []agentAction{
		{label: "Enable", highlight: !mc.Enabled, disabled: remote || mc.Enabled, reason: enableMCPReason(remote, true, mc.Enabled),
			run: func() tea.Cmd {
				return providerActionCmd("enabled MCP "+mc.Name, func(ctx context.Context, p agents.Provider) error {
					return p.EnableMCP(ctx, mc.Name, true)
				}, prov)
			}},
		{label: "Disable", disabled: remote || !mc.Enabled, reason: enableMCPReason(remote, false, mc.Enabled),
			run: func() tea.Cmd {
				return providerActionCmd("disabled MCP "+mc.Name, func(ctx context.Context, p agents.Provider) error {
					return p.EnableMCP(ctx, mc.Name, false)
				}, prov)
			}},
		{label: "Remove", disabled: remote, reason: "remote MCPs cannot be removed from this view",
			run: func() tea.Cmd {
				return providerActionCmd("removed MCP "+mc.Name, func(ctx context.Context, p agents.Provider) error {
					return p.RemoveMCP(ctx, mc.Name)
				}, prov)
			}},
		{label: "Edit (follow-up)", disabled: true, reason: "MCP edit form coming in a follow-up release"},
		{label: "Open URL", disabled: mc.URL == "", reason: "no URL", run: func() tea.Cmd { return openURLCmd(mc.URL) }},
		{label: "Copy Command", run: func() tea.Cmd {
			text, _ := rowCopyText(row)
			return copyTextCmd(text, "MCP command")
		}},
		promoteAction(m.agents, promote.KindMCP, string(prov), mc.Name, mcpPromoteReason(remote)),
	}
}

func mcpPromoteReason(remote bool) string {
	if remote {
		return "remote MCPs cannot be promoted directly"
	}
	return ""
}

func enableMCPReason(remote, want, current bool) string {
	switch {
	case remote:
		return "remote catalog MCP — install it via an agent provider first"
	case want && current:
		return "already enabled"
	case !want && !current:
		return "already disabled"
	}
	return ""
}

func (m *Model) actionsForSession(frame agentDetailFrame, row agentRow) []agentAction {
	s := row.session
	if s == nil {
		return nil
	}
	prov := s.Provider
	return []agentAction{
		{label: "Resume", highlight: true, run: func() tea.Cmd {
			return launchFromDetailCmd(prov, agents.LaunchSpec{Provider: prov, SessionID: s.ID})
		}},
		{label: "View Transcript", disabled: s.TranscriptPath == "", reason: "no transcript path",
			run: func() tea.Cmd { return viewTranscriptCmd(s.TranscriptPath) }},
		{label: "Open Dir", disabled: s.TranscriptPath == "", reason: "no transcript dir",
			run: func() tea.Cmd { return openURLCmd(s.TranscriptPath) }},
		{label: "Copy Resume", run: func() tea.Cmd {
			text, _ := rowCopyText(row)
			return copyTextCmd(text, "resume command")
		}},
		{label: "Delete", run: func() tea.Cmd {
			return providerActionCmd("deleted session "+s.ID, func(ctx context.Context, p agents.Provider) error {
				return p.DeleteSession(ctx, s.ID)
			}, prov)
		}},
	}
}

// marketplacePluginCount counts plugins in the snapshot whose
// Marketplace field matches this marketplace's name and provider.
func (m *Model) marketplacePluginCount(mp *agents.Marketplace) int {
	st := m.agents
	if st == nil || st.snapshot == nil || mp == nil {
		return 0
	}
	n := 0
	for i := range st.snapshot.Plugins {
		p := &st.snapshot.Plugins[i]
		if p.Provider == mp.Provider && p.Marketplace == mp.Name {
			n++
		}
	}
	return n
}

// pluginSkillCount counts skills in the snapshot whose provider +
// SourcePlugin match the given plugin.
func (m *Model) pluginSkillCount(pl *agents.Plugin) int {
	st := m.agents
	if st == nil || st.snapshot == nil || pl == nil {
		return 0
	}
	n := 0
	for i := range st.snapshot.Skills {
		s := &st.snapshot.Skills[i]
		if s.Provider == pl.Provider && s.SourcePlugin == pl.Name {
			n++
		}
	}
	return n
}

// ---------- command factories ----------

// providerActionCmd runs `op` against the named provider and returns
// an agentActionResultMsg. The label is the success message.
func providerActionCmd(label string, op func(ctx context.Context, p agents.Provider) error, provider agents.ProviderID) tea.Cmd {
	return func() tea.Msg {
		svc := agentsService()
		p := svc.ProviderFor(provider)
		if p == nil {
			return agentActionResultMsg{label: label, err: fmt.Errorf("provider %q not registered", provider)}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		return agentActionResultMsg{label: label, err: op(ctx, p)}
	}
}

// refreshAgentsCmd triggers a full re-scan.
func refreshAgentsCmd() tea.Cmd {
	return loadAgentsCmd(true)
}

// openURLCmd opens a URL (or a filesystem path) in the OS handler.
func openURLCmd(url string) tea.Cmd {
	return func() tea.Msg {
		if url == "" {
			return agentActionResultMsg{label: "open URL", err: errors.New("nothing to open")}
		}
		if err := openBrowser(url); err != nil {
			return agentActionResultMsg{label: "open URL", err: err}
		}
		return agentActionResultMsg{label: "opened " + truncAgentRow(url, 60)}
	}
}

// copyTextCmd copies text to the system clipboard.
func copyTextCmd(text, label string) tea.Cmd {
	return func() tea.Msg {
		if text == "" {
			return agentActionResultMsg{label: "copy", err: errors.New("nothing to copy")}
		}
		cb := systemClipboard{}
		if err := cb.WriteAll(text); err != nil {
			return agentActionResultMsg{label: "copy", err: err}
		}
		return agentActionResultMsg{label: "copied " + label}
	}
}

// transcriptReadLimit caps how many conversation events we load when
// the viewer opens. The modal supports scrolling, so a reasonably
// high cap lets users walk back through the whole conversation
// without re-reading. 500 messages fits a typical long session and
// stays cheap to render (~10ms on commodity hardware).
const transcriptReadLimit = 500

// viewTranscriptCmd reads up to transcriptReadLimit messages of a
// session transcript and opens the viewer modal. Result lands in
// agentTranscriptMsg.
func viewTranscriptCmd(path string) tea.Cmd {
	return func() tea.Msg {
		msgs, err := readSessionTranscript(path, transcriptReadLimit)
		return agentTranscriptMsg{path: path, messages: msgs, err: err}
	}
}

// agentTranscriptMsg lands in handleAgentsMsg with the loaded messages.
type agentTranscriptMsg struct {
	path     string
	messages []transcriptMessage
	err      error
}

// launchFromDetailCmd builds a launch plan from inside the detail page
// and queues the confirmation modal (re-using the existing flow).
func launchFromDetailCmd(provider agents.ProviderID, spec agents.LaunchSpec) tea.Cmd {
	return func() tea.Msg {
		svc := agentsService()
		p := svc.ProviderFor(provider)
		if p == nil {
			return agentActionResultMsg{label: "launch", err: fmt.Errorf("provider %q not registered", provider)}
		}
		plan, err := p.BuildLaunch(spec)
		if err != nil {
			return agentActionResultMsg{label: "launch", err: err}
		}
		return agentLaunchPlanMsg{plan: plan}
	}
}

// agentLaunchPlanMsg arrives when an action wants the user to confirm
// a launch in the standard launch modal.
type agentLaunchPlanMsg struct {
	plan agents.ExecPlan
}

// viewMarketplacePluginsCmd is a marker message that tells the detail
// handler to close the detail page, switch to the Plugins sub-tab,
// and apply a marketplace filter so the user lands on the full
// plugin list scoped to the marketplace they were viewing.
func viewMarketplacePluginsCmd() tea.Cmd {
	return func() tea.Msg { return agentViewMarketplacePluginsMsg{} }
}

type agentViewMarketplacePluginsMsg struct{}

// viewPluginSkillsCmd is a marker message that tells the detail
// handler to close the detail page, switch to the Skills sub-tab,
// and apply a plugin filter so the user lands on the Skills list
// scoped to skills shipped by the plugin they were viewing.
func viewPluginSkillsCmd() tea.Cmd {
	return func() tea.Msg { return agentViewPluginSkillsMsg{} }
}

type agentViewPluginSkillsMsg struct{}
