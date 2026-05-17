package tui

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/agents"
)

// handleAgentsDetailKey routes keystrokes when the Agents detail page
// is active. Returns (handled, cmd) the same way handleAgentsKey does.
func (m *Model) handleAgentsDetailKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	st := m.agents
	if st == nil || !st.detailPage || len(st.detailStack) == 0 {
		return false, nil
	}

	// Promote picker takes precedence over every other detail-page UI.
	if st.promotePicker.Open {
		return m.handleAgentsPromoteKey(msg)
	}

	// Viewer/launch/delete modals overlay the detail page; let them own
	// input the same way the list view does.
	if st.viewerOpen {
		switch msg.String() {
		case "esc", "enter", "q":
			st.viewerOpen = false
		}
		return true, nil
	}
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
	if st.helpOpen {
		st.helpOpen = false
		return true, nil
	}

	top := &st.detailStack[len(st.detailStack)-1]
	row, ok := m.resolveDetailRow(*top)
	if !ok {
		// Entity vanished from snapshot (e.g. uninstalled). Pop.
		return true, m.agentsPopDetail()
	}
	actions := m.agentsBuildActions(*top, row)

	switch msg.String() {
	case "esc", "q":
		return true, m.agentsPopDetail()
	case "tab", "right", "l":
		if len(actions) > 0 {
			top.actionIdx = (top.actionIdx + 1) % len(actions)
		}
		return true, nil
	case "shift+tab", "left", "h":
		if len(actions) > 0 {
			top.actionIdx = (top.actionIdx + len(actions) - 1) % len(actions)
		}
		return true, nil
	case "down", "j":
		// For marketplace pages with a plugin list, j/k navigates that
		// list. Other pages: scroll the body.
		if top.subTab == agentsSubMarketplaces && row.marketplace != nil {
			n := m.marketplacePluginCount(row.marketplace)
			if n > 0 && top.bodyCursor < n-1 {
				top.bodyCursor++
			}
		} else {
			top.scroll++
		}
		return true, nil
	case "up", "k":
		if top.subTab == agentsSubMarketplaces && row.marketplace != nil {
			if top.bodyCursor > 0 {
				top.bodyCursor--
			}
		} else if top.scroll > 0 {
			top.scroll--
		}
		return true, nil
	case "enter":
		if top.actionIdx < 0 || top.actionIdx >= len(actions) {
			return true, nil
		}
		act := actions[top.actionIdx]
		if act.disabled {
			reason := act.reason
			if reason == "" {
				reason = "action not available"
			}
			st.flash = "✗ " + act.label + ": " + reason
			st.flashEnd = time.Now().Add(3 * time.Second)
			return true, nil
		}
		if act.run == nil {
			return true, nil
		}
		st.actionRunning = act.label + "…"
		return true, act.run()
	case "o":
		url := rowOpenURL(row)
		if url == "" {
			st.flash = "no URL to open"
			st.flashEnd = time.Now().Add(2 * time.Second)
			return true, nil
		}
		return true, openURLCmd(url)
	case "c":
		text, label := rowCopyText(row)
		if text == "" {
			st.flash = "nothing to copy"
			st.flashEnd = time.Now().Add(2 * time.Second)
			return true, nil
		}
		return true, copyTextCmd(text, label)
	case "?":
		st.helpOpen = true
		return true, nil
	case "r":
		st.flash = "refreshing…"
		st.flashEnd = time.Now().Add(2 * time.Second)
		return true, refreshAgentsCmd()
	}
	return true, nil
}

// agentsPopDetail pops one frame from the navigation stack. When the
// stack empties, the detail page closes and control returns to the
// list view at the originally-captured cursor.
func (m *Model) agentsPopDetail() tea.Cmd {
	st := m.agents
	if st == nil || len(st.detailStack) == 0 {
		return nil
	}
	st.detailStack = st.detailStack[:len(st.detailStack)-1]
	if len(st.detailStack) == 0 {
		st.detailPage = false
	}
	return nil
}

// agentsPushDetail pushes a new frame onto the navigation stack. Used
// when the user drills from a marketplace into one of its plugins.
func (m *Model) agentsPushDetail(subTab int, entityID string) {
	st := m.agents
	if st == nil {
		return
	}
	st.detailStack = append(st.detailStack, agentDetailFrame{
		subTab:   subTab,
		entityID: entityID,
	})
}

// resolveDetailRow looks up the entity referenced by the given frame
// in the current snapshot. Returns (zero, false) when no longer present.
func (m *Model) resolveDetailRow(frame agentDetailFrame) (agentRow, bool) {
	st := m.agents
	if st == nil || st.snapshot == nil {
		return agentRow{}, false
	}
	switch frame.subTab {
	case agentsSubMarketplaces:
		for i := range st.snapshot.Marketplaces {
			x := st.snapshot.Marketplaces[i]
			if x.ID == frame.entityID {
				return agentRow{id: x.ID, name: x.Name, subtitle: x.Description, provider: x.Provider, source: x.Source, marketplace: &x}, true
			}
		}
	case agentsSubPlugins:
		for i := range st.snapshot.Plugins {
			x := st.snapshot.Plugins[i]
			if x.ID == frame.entityID {
				return agentRow{id: x.ID, name: x.Name, subtitle: x.Description, provider: x.Provider, source: x.Source, enabled: x.Enabled, plugin: &x}, true
			}
		}
	case agentsSubSkills:
		for i := range st.snapshot.Skills {
			x := st.snapshot.Skills[i]
			if x.ID == frame.entityID {
				return agentRow{id: x.ID, name: x.Name, subtitle: x.Description, provider: x.Provider, source: x.Source, scope: x.Scope, enabled: x.Enabled, skill: &x}, true
			}
		}
	case agentsSubMCPs:
		for i := range st.snapshot.MCPs {
			x := st.snapshot.MCPs[i]
			if x.ID == frame.entityID {
				return agentRow{id: x.ID, name: x.Name, subtitle: x.URL, provider: x.Provider, source: x.Source, scope: x.Scope, enabled: x.Enabled, mcp: &x}, true
			}
		}
	case agentsSubSessions:
		for i := range st.snapshot.Sessions {
			x := st.snapshot.Sessions[i]
			if x.ID == frame.entityID {
				return agentRow{id: x.ID, name: x.Name, subtitle: x.ProjectPath, provider: x.Provider, source: x.Source, session: &x}, true
			}
		}
	}
	return agentRow{}, false
}

// ---------- render ----------

// renderAgentsDetailPage renders the full-screen Agents detail view.
func (m *Model) renderAgentsDetailPage() string {
	st := m.agents
	if st == nil || len(st.detailStack) == 0 {
		// Shouldn't happen — view.go gates on detailPage — but be safe.
		return m.renderAgentsView()
	}
	top := st.detailStack[len(st.detailStack)-1]
	row, ok := m.resolveDetailRow(top)
	if !ok {
		var b strings.Builder
		b.WriteString("\n  entity no longer present — press Esc to return\n")
		return b.String()
	}
	actions := m.agentsBuildActions(top, row)

	var b strings.Builder
	b.WriteString(m.renderTitleBar() + "\n")

	// Breadcrumb / header
	crumbs := agentsDetailBreadcrumb(st.detailStack)
	b.WriteString("  " + crumbs + "\n")

	// Title line
	title := lipgloss.NewStyle().Bold(true).Foreground(cyberPrimary).Render(row.name)
	b.WriteString("  " + agentsProviderChip(row.provider) + "  " + title)
	if sub := agentsDetailSubtitle(row); sub != "" {
		b.WriteString("  " + dimVersion.Render(sub))
	}
	b.WriteString("\n\n")

	// Metadata section
	b.WriteString(renderAgentDetailFull(row, st.snapshot))
	b.WriteString("\n")

	// Action bar
	b.WriteString(renderAgentActionBar(actions, top.actionIdx, m.width))
	b.WriteString("\n")

	// Contextual body
	body := renderAgentDetailBody(m, top, row)
	if body != "" {
		b.WriteString(body)
	}

	// Footer hints + running/flash. Captured separately so we can
	// bottom-align this block to the terminal height.
	var footer strings.Builder
	footer.WriteString("  " + dimVersion.Render("←/→ action · ↑/↓ body · Enter run · o open · c copy · r refresh · Esc back · ? help") + "\n")
	if st.actionRunning != "" {
		footer.WriteString("  " + lipgloss.NewStyle().Foreground(cyberAccent).Render("⏳ "+st.actionRunning) + "\n")
	}
	if st.flash != "" && time.Now().Before(st.flashEnd) {
		footer.WriteString("  " + st.flash + "\n")
	}

	// Pad with blank lines so footer reaches the bottom of the screen,
	// OR clip the body from the bottom when it would push the footer
	// off-screen. The previous version clamped gap to 1 but did NOT
	// trim the body — so a marketplace with many plugins (or a
	// session with a long detail block) overflowed the terminal,
	// hiding the action bar and footer.
	body0 := b.String()
	bodyRows := strings.Count(body0, "\n")
	footerStr := footer.String()
	footerRows := strings.Count(footerStr, "\n")
	if m.height > 0 {
		maxBody := m.height - footerRows - 1
		if maxBody < 1 {
			maxBody = 1
		}
		if bodyRows > maxBody {
			// Trim from the bottom — losing detail-body tail lines is
			// recoverable (cursor scrolls within the windowed list);
			// losing the footer / action bar leaves the page unusable.
			lines := strings.Split(body0, "\n")
			if len(lines) > maxBody {
				lines = lines[:maxBody]
			}
			body0 = strings.Join(lines, "\n")
			if !strings.HasSuffix(body0, "\n") {
				body0 += "\n"
			}
			bodyRows = strings.Count(body0, "\n")
		}
		gap := m.height - bodyRows - footerRows - 1
		if gap < 1 {
			gap = 1
		}
		body0 += strings.Repeat("\n", gap)
	} else {
		body0 += "\n"
	}
	b.Reset()
	b.WriteString(body0)
	b.WriteString(footerStr)

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
	if st.promotePicker.Open {
		b.WriteString(renderAgentsPromotePicker(st, m.width))
	}
	return b.String()
}

// agentsDetailBreadcrumb returns the "Agents › Marketplaces › <name>" trail.
func agentsDetailBreadcrumb(stack []agentDetailFrame) string {
	parts := []string{dimVersion.Render("Agents")}
	for _, f := range stack {
		parts = append(parts, dimVersion.Render("›"), agentsSubTabName(f.subTab))
		if f.entityID != "" {
			parts = append(parts, dimVersion.Render("›"), f.entityID)
		}
	}
	return strings.Join(parts, " ")
}

func agentsSubTabName(subTab int) string {
	switch subTab {
	case agentsSubMarketplaces:
		return "Marketplaces"
	case agentsSubPlugins:
		return "Plugins"
	case agentsSubSkills:
		return "Skills"
	case agentsSubMCPs:
		return "MCPs"
	case agentsSubSessions:
		return "Sessions"
	}
	return "?"
}

// agentsDetailSubtitle returns a one-line context line (status chips).
func agentsDetailSubtitle(r agentRow) string {
	switch {
	case r.plugin != nil:
		status := "available"
		switch {
		case r.plugin.Installed && r.plugin.Enabled:
			status = "installed · enabled"
		case r.plugin.Installed:
			status = "installed · disabled"
		}
		return status
	case r.mcp != nil:
		if r.mcp.Enabled {
			return "enabled · " + string(r.mcp.Scope)
		}
		return "disabled · " + string(r.mcp.Scope)
	case r.session != nil:
		if r.session.Status != "" {
			return string(r.session.Status)
		}
		return "session"
	case r.skill != nil:
		return "scope: " + string(r.skill.Scope)
	case r.marketplace != nil:
		return string(r.marketplace.Source)
	}
	return ""
}

// renderAgentDetailFull renders the metadata block as a two-column
// key/value table. Reuses the same fields as the legacy inline detail
// but with section headers for readability.
func renderAgentDetailFull(row agentRow, snap *agents.Snapshot) string {
	var b strings.Builder
	tw := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	// We hand off to renderAgentDetail (the existing function) so we
	// don't duplicate the per-entity field list. The legacy version
	// already writes "  key\tvalue\n" — we just prepend a section header.
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(cyberInfo)
	switch {
	case row.marketplace != nil:
		_, _ = fmt.Fprintln(tw, "  "+headerStyle.Render("Marketplace"))
	case row.plugin != nil:
		_, _ = fmt.Fprintln(tw, "  "+headerStyle.Render("Plugin"))
	case row.skill != nil:
		_, _ = fmt.Fprintln(tw, "  "+headerStyle.Render("Skill"))
	case row.mcp != nil:
		_, _ = fmt.Fprintln(tw, "  "+headerStyle.Render("MCP server"))
	case row.session != nil:
		_, _ = fmt.Fprintln(tw, "  "+headerStyle.Render("Session"))
	}
	_ = tw.Flush()
	b.WriteString(renderAgentDetail(row, snap))
	return b.String()
}

// renderAgentActionBar renders the horizontal action buttons.
func renderAgentActionBar(actions []agentAction, focus, totalWidth int) string {
	if len(actions) == 0 {
		return "  " + dimVersion.Render("(no actions for this entity)") + "\n"
	}
	var pieces []string
	for i, a := range actions {
		label := a.label
		var style lipgloss.Style
		switch {
		case a.disabled:
			style = lipgloss.NewStyle().Foreground(cyberFGDim).Background(cyberChipBg).Padding(0, 1)
			label += " ✗"
		case i == focus:
			style = lipgloss.NewStyle().Foreground(cyberSelectedBg).Background(cyberPrimary).Bold(true).Padding(0, 1)
		case a.highlight:
			style = lipgloss.NewStyle().Foreground(cyberAccent).Background(cyberChipBg).Bold(true).Padding(0, 1)
		default:
			style = lipgloss.NewStyle().Foreground(cyberFG).Background(cyberChipBg).Padding(0, 1)
		}
		pieces = append(pieces, style.Render(label))
	}
	header := lipgloss.NewStyle().Bold(true).Foreground(cyberInfo).Render("Actions")
	return "  " + header + "\n  " + strings.Join(pieces, "  ") + "\n"
}

// renderAgentDetailBody renders the contextual body section — plugin
// list for marketplaces, env keys for MCPs, contained skills for
// plugins, etc.
func renderAgentDetailBody(m *Model, frame agentDetailFrame, row agentRow) string {
	switch {
	case row.marketplace != nil:
		return renderMarketplaceBody(m, frame, row.marketplace)
	case row.plugin != nil:
		return renderPluginBody(m, row.plugin)
	case row.mcp != nil:
		return renderMCPBody(row.mcp)
	case row.session != nil:
		return renderSessionBody(row.session)
	case row.skill != nil:
		return renderSkillBody(row.skill)
	}
	return ""
}

func renderMarketplaceBody(m *Model, frame agentDetailFrame, mp *agents.Marketplace) string {
	plugins := m.marketplacePlugins(mp)
	header := lipgloss.NewStyle().Bold(true).Foreground(cyberInfo).Render("Plugins")
	var b strings.Builder
	fmt.Fprintf(&b, "  %s  %s\n", header, dimVersion.Render(fmt.Sprintf("(%d)", len(plugins))))
	if len(plugins) == 0 {
		b.WriteString("  " + dimVersion.Render("none discovered in the current snapshot") + "\n")
		return b.String()
	}
	// Window the plugin list around the cursor so the body can't
	// outgrow the terminal height (which would push the action bar
	// and footer off-screen — visually 'no footer / no scroll').
	// Budget: terminal_height - (title 1 + breadcrumb 1 + title row 1
	// + blank 1 + metadata 3-6 + action bar 1 + blank 1 + plugin
	// header 1 + plugin hint 1 + footer 3) ≈ m.height - 14. Floor
	// at 5 so very small terminals still show something.
	maxRows := m.height - 14
	if maxRows < 5 {
		maxRows = 5
	}
	if maxRows > len(plugins) {
		maxRows = len(plugins)
	}
	start := 0
	if frame.bodyCursor >= maxRows {
		start = frame.bodyCursor - maxRows + 1
	}
	if start < 0 {
		start = 0
	}
	end := start + maxRows
	if end > len(plugins) {
		end = len(plugins)
		if end-maxRows >= 0 {
			start = end - maxRows
		}
	}
	if start > 0 {
		b.WriteString("    " + dimVersion.Render(fmt.Sprintf("↑ %d above", start)) + "\n")
	}
	for i := start; i < end; i++ {
		p := plugins[i]
		marker := "    "
		if i == frame.bodyCursor {
			marker = "  ▸ "
		}
		status := "available"
		switch {
		case p.Installed && p.Enabled:
			status = "installed"
		case p.Installed:
			status = "disabled"
		}
		line := fmt.Sprintf("%s%s  %s  %s",
			marker,
			lipgloss.NewStyle().Width(28).Render(truncAgentRow(p.Name, 28)),
			lipgloss.NewStyle().Width(10).Render(truncAgentRow(p.Version, 10)),
			truncAgentRow(p.Description, 64),
		)
		if i == frame.bodyCursor {
			line = cyberSelectedRowStyle.Render(line)
		}
		b.WriteString(line + "  " + dimVersion.Render(status) + "\n")
	}
	if end < len(plugins) {
		b.WriteString("    " + dimVersion.Render(fmt.Sprintf("↓ %d below", len(plugins)-end)) + "\n")
	}
	b.WriteString("  " + dimVersion.Render("↑/↓ pick · Enter (with action 'Open plugin →') drills in") + "\n")
	return b.String()
}

func renderPluginBody(m *Model, p *agents.Plugin) string {
	st := m.agents
	var b strings.Builder
	header := lipgloss.NewStyle().Bold(true).Foreground(cyberInfo)
	// Contained skills (snapshot lookup).
	if st != nil && st.snapshot != nil {
		var skills []string
		for _, s := range st.snapshot.Skills {
			if s.Provider == p.Provider && s.SourcePlugin == p.Name {
				skills = append(skills, s.Name)
			}
		}
		if len(skills) > 0 {
			fmt.Fprintf(&b, "  %s\n", header.Render("Contained skills"))
			for _, name := range skills {
				b.WriteString("    • " + name + "\n")
			}
		}
	}
	if len(p.Keywords) > 0 {
		fmt.Fprintf(&b, "  %s\n", header.Render("Keywords"))
		b.WriteString("    " + strings.Join(p.Keywords, ", ") + "\n")
	}
	return b.String()
}

func renderMCPBody(mc *agents.MCP) string {
	var b strings.Builder
	header := lipgloss.NewStyle().Bold(true).Foreground(cyberInfo)
	if len(mc.Tools) > 0 {
		fmt.Fprintf(&b, "  %s\n", header.Render("Tools"))
		for _, t := range mc.Tools {
			b.WriteString("    • " + t + "\n")
		}
	}
	if len(mc.EnvKeys) > 0 {
		fmt.Fprintf(&b, "  %s\n", header.Render("Env keys"))
		b.WriteString("    " + strings.Join(mc.EnvKeys, ", ") + "\n")
	}
	return b.String()
}

func renderSessionBody(s *agents.Session) string {
	var b strings.Builder
	header := lipgloss.NewStyle().Bold(true).Foreground(cyberInfo)
	fmt.Fprintf(&b, "  %s\n", header.Render("Session metadata"))
	if s.Title != "" {
		b.WriteString("    title: " + s.Title + "\n")
	}
	if !s.Created.IsZero() {
		b.WriteString("    created: " + s.Created.Format(time.RFC3339) + "\n")
	}
	if !s.LastModified.IsZero() {
		b.WriteString("    modified: " + s.LastModified.Format(time.RFC3339) + "\n")
	}
	if s.TurnCount > 0 {
		b.WriteString(fmt.Sprintf("    turns: %d\n", s.TurnCount))
	}
	return b.String()
}

func renderSkillBody(s *agents.Skill) string {
	var b strings.Builder
	header := lipgloss.NewStyle().Bold(true).Foreground(cyberInfo)
	if s.WhenToUse != "" {
		fmt.Fprintf(&b, "  %s\n", header.Render("When to use"))
		b.WriteString("    " + s.WhenToUse + "\n")
	}
	if s.AllowedTools != "" {
		fmt.Fprintf(&b, "  %s\n", header.Render("Allowed tools"))
		b.WriteString("    " + s.AllowedTools + "\n")
	}
	return b.String()
}
