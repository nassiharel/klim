package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/agents/promote"
)

// agentsPromoteState holds the inline target-picker state used by
// the Promote ▸ action button on detail pages.
type agentsPromoteState struct {
	Open     bool
	Kind     promote.EntityKind
	Source   string          // source provider id
	Subject  string          // subject id (skill name / mcp name / plugin name)
	Targets  []promoteTarget // available (provider, scope) pairs
	Cursor   int             // index into Targets
	Force    bool            // overwrite existing target?
	LastPlan *promote.Plan   // latest computed plan (for preview text)
	LastErr  error
}

// promoteTarget enumerates a (provider, scope) pick. Plugins ignore
// scope but it's harmless to carry it for display.
type promoteTarget struct {
	Provider string
	Scope    string
	Label    string
}

// agentsPromoteResultMsg arrives from the apply step.
type agentsPromoteResultMsg struct {
	err     error
	summary string
}

// openPromotePicker initialises the picker for the entity on the
// current detail frame. Targets are every (provider, scope) other
// than the source one — plus the source provider with a different
// scope (for skills).
func openPromotePicker(st *agentsState, kind promote.EntityKind, source, subject string) {
	st.promotePicker = agentsPromoteState{
		Open:    true,
		Kind:    kind,
		Source:  source,
		Subject: subject,
		Targets: enumeratePromoteTargets(kind, source),
	}
}

// enumeratePromoteTargets returns the providers / scopes a kind can
// be promoted into. We intentionally keep the list small in v1 —
// claude-code, copilot-cli at user/project scopes.
func enumeratePromoteTargets(kind promote.EntityKind, source string) []promoteTarget {
	providers := []string{string(agents.ProviderClaudeCode), string(agents.ProviderCopilotCLI)}
	scopes := []string{string(agents.ScopeUser), string(agents.ScopeProject)}
	var out []promoteTarget
	for _, p := range providers {
		for _, sc := range scopes {
			if p == source && (kind != promote.KindSkill) {
				// Skills are the only kind where same-provider/scope
				// change makes sense.
				continue
			}
			if kind == promote.KindPlugin && sc == string(agents.ScopeProject) {
				// Plugins don't have project scope in v1.
				continue
			}
			out = append(out, promoteTarget{
				Provider: p,
				Scope:    sc,
				Label:    fmt.Sprintf("%s · %s", providerShort(agents.ProviderID(p)), sc),
			})
		}
	}
	return out
}

// handleAgentsPromoteKey routes keys while the picker is open. Returns
// (handled, cmd); when handled is false the caller falls through.
func (m *Model) handleAgentsPromoteKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	st := m.agents
	if st == nil || !st.promotePicker.Open {
		return false, nil
	}
	pk := &st.promotePicker
	switch msg.String() {
	case "esc", "q":
		pk.Open = false
		return true, nil
	case "down", "j":
		if pk.Cursor < len(pk.Targets)-1 {
			pk.Cursor++
		}
		return true, nil
	case "up", "k":
		if pk.Cursor > 0 {
			pk.Cursor--
		}
		return true, nil
	case "F":
		pk.Force = !pk.Force
		return true, nil
	case "enter":
		if pk.Cursor < 0 || pk.Cursor >= len(pk.Targets) {
			return true, nil
		}
		target := pk.Targets[pk.Cursor]
		spec := promote.Spec{
			Kind:           pk.Kind,
			SubjectID:      pk.Subject,
			SourceProvider: pk.Source,
			TargetProvider: target.Provider,
			TargetScope:    target.Scope,
			Force:          pk.Force,
		}
		return true, runPromoteCmd(st, spec)
	}
	return true, nil
}

// runPromoteCmd builds and applies a Plan for the given Spec.
func runPromoteCmd(st *agentsState, spec promote.Spec) tea.Cmd {
	snap := buildPromoteSnapshot(st)
	return func() tea.Msg {
		plan := promote.Build(snap, spec, promote.BuildOpts{
			SkillDir: skillDirForProvider,
		})
		if plan.Conflict != promote.ConflictNone {
			return agentsPromoteResultMsg{err: fmt.Errorf("%s: %s", plan.Conflict, plan.ConflictMsg)}
		}
		svc := agentsService()
		ex := &promoteExecutor{svc: svc}
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := plan.Apply(ctx, ex); err != nil {
			return agentsPromoteResultMsg{err: err}
		}
		return agentsPromoteResultMsg{summary: plan.TargetSummary}
	}
}

// promoteExecutor adapts agents.Service for promote.Executor.
type promoteExecutor struct{ svc *agents.Service }

func (e *promoteExecutor) AddMCP(ctx context.Context, providerID string, op promote.ProviderOp) error {
	p := e.svc.ProviderFor(agents.ProviderID(providerID))
	if p == nil {
		return errors.New("target provider not registered: " + providerID)
	}
	scope := agents.Scope(op.MCPScope)
	if scope == "" {
		scope = agents.ScopeUser
	}
	return p.AddMCP(ctx, agents.MCPSpec{
		Name:      op.MCPName,
		Transport: op.MCPTransport,
		Command:   op.MCPCommand,
		Args:      op.MCPArgs,
		URL:       op.MCPURL,
		Scope:     scope,
	})
}

func (e *promoteExecutor) InstallPlugin(ctx context.Context, providerID string, op promote.ProviderOp) error {
	p := e.svc.ProviderFor(agents.ProviderID(providerID))
	if p == nil {
		return errors.New("target provider not registered: " + providerID)
	}
	return p.InstallPlugin(ctx, agents.PluginRef{
		Name:        op.PluginRefName,
		Marketplace: op.PluginRefMP,
	})
}

// buildPromoteSnapshot adapts agents.Snapshot to promote.Snapshot
// without an import cycle.
func buildPromoteSnapshot(st *agentsState) promote.Snapshot {
	out := promote.Snapshot{}
	if st == nil || st.snapshot == nil {
		return out
	}
	for _, sk := range st.snapshot.Skills {
		out.Skills = append(out.Skills, promote.SkillRef{
			Name:         sk.Name,
			Provider:     string(sk.Provider),
			Scope:        string(sk.Scope),
			SourcePlugin: sk.SourcePlugin,
			Path:         sk.Path,
			Description:  sk.Description,
			WhenToUse:    sk.WhenToUse,
			AllowedTools: sk.AllowedTools,
			Model:        sk.Model,
		})
	}
	for _, m := range st.snapshot.MCPs {
		out.MCPs = append(out.MCPs, promote.MCPRef{
			Name:      m.Name,
			Provider:  string(m.Provider),
			Scope:     string(m.Scope),
			Transport: m.Transport,
			Command:   m.Command,
			Args:      append([]string(nil), m.Args...),
			URL:       m.URL,
			EnvKeys:   append([]string(nil), m.EnvKeys...),
		})
	}
	for _, p := range st.snapshot.Plugins {
		out.Plugins = append(out.Plugins, promote.PluginRef{
			Name:        p.Name,
			Provider:    string(p.Provider),
			Marketplace: p.Marketplace,
			Installed:   p.Installed,
		})
	}
	return out
}

// skillDirForProvider returns the canonical skills directory for a
// provider+scope. Used by the planner when computing target paths.
//
// For user scope we go to ~/.claude/skills or ~/.copilot/skills;
// project scope writes to the current working directory's
// .claude/skills or .github/skills.
func skillDirForProvider(provider, scope string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	cwd, _ := os.Getwd()
	switch provider {
	case string(agents.ProviderClaudeCode):
		if scope == string(agents.ScopeProject) && cwd != "" {
			return filepath.Join(cwd, ".claude", "skills"), nil
		}
		return filepath.Join(home, ".claude", "skills"), nil
	case string(agents.ProviderCopilotCLI):
		if scope == string(agents.ScopeProject) && cwd != "" {
			return filepath.Join(cwd, ".github", "skills"), nil
		}
		return filepath.Join(home, ".copilot", "skills"), nil
	}
	return "", fmt.Errorf("no skill dir for provider %s", provider)
}

// renderAgentsPromotePicker draws the inline target picker on top of
// the detail page.
func renderAgentsPromotePicker(st *agentsState, width int) string {
	if !st.promotePicker.Open {
		return ""
	}
	pk := &st.promotePicker
	if width <= 0 {
		width = 80
	}
	box := lipgloss.NewStyle().
		Foreground(cyberFG).
		Background(cyberSelectedBg).
		BorderForeground(cyberPrimary).
		BorderStyle(lipgloss.RoundedBorder()).
		Padding(0, 1).
		Width(width - 4)

	header := lipgloss.NewStyle().Bold(true).Foreground(cyberPrimary).Render("⇄ Promote ") +
		lipgloss.NewStyle().Bold(true).Render(pk.Subject) +
		dimVersion.Render("  from "+pk.Source)
	lines := []string{header, ""}
	for i, t := range pk.Targets {
		lead := "    "
		if i == pk.Cursor {
			lead = "  ▸ "
		}
		line := lead + t.Label
		if i == pk.Cursor {
			line = cyberSelectedRowStyle.Render(line)
		}
		lines = append(lines, line)
	}
	forceState := "off"
	if pk.Force {
		forceState = lipgloss.NewStyle().Foreground(cyberAccent).Render("on")
	}
	lines = append(lines, "")
	lines = append(lines, dimVersion.Render(fmt.Sprintf("  Force overwrite: %s   (press F to toggle)", forceState)))
	lines = append(lines, dimVersion.Render("  ↑/↓ pick · Enter run · Esc cancel"))
	return "\n" + box.Render(strings.Join(lines, "\n")) + "\n"
}

// promoteAction returns the agentAction button to splice into a
// detail page's action bar. opens the picker when fired.
func promoteAction(st *agentsState, kind promote.EntityKind, source, subject string, disabledReason string) agentAction {
	if disabledReason != "" {
		return agentAction{
			label:    "Promote ▸",
			disabled: true,
			reason:   disabledReason,
		}
	}
	return agentAction{
		label: "Promote ▸",
		run: func() tea.Cmd {
			openPromotePicker(st, kind, source, subject)
			return nil
		},
	}
}
