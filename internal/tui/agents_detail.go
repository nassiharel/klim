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
		// For pages whose body renders a windowed list with a
		// cursor (marketplace plugins, plugin's contained skills,
		// MCP tools), j/k navigates that list via bodyCursor —
		// which is what the windowing renderer reads. Other pages
		// fall back to top.scroll for free-form body scroll.
		if bodyCursorLen, ok := detailBodyCursorLen(m, *top, row); ok {
			if top.bodyCursor < bodyCursorLen-1 {
				top.bodyCursor++
			}
		} else {
			top.scroll++
		}
		return true, nil
	case "up", "k":
		if _, ok := detailBodyCursorLen(m, *top, row); ok {
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
		// Handled globally — but we need to return false to let it
		// reach the global handler. The detail page catches all keys
		// with `return true, nil` at the bottom, so we explicitly
		// return false here.
		return false, nil
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

// dimVersionDetailHint renders a tiny "↑ N above · ↓ M below" hint
// for the detail page's outer scroll. Both counts may be zero; the
// returned string is empty when neither direction has hidden lines.
func dimVersionDetailHint(above, below int) string {
	if above == 0 && below == 0 {
		return ""
	}
	switch {
	case above > 0 && below > 0:
		return dimVersion.Render(fmt.Sprintf("↑ %d above · ↓ %d below", above, below))
	case above > 0:
		return dimVersion.Render(fmt.Sprintf("↑ %d above", above))
	default:
		return dimVersion.Render(fmt.Sprintf("↓ %d below", below))
	}
}

// detailBodyCursorLen returns the length of the windowed body list
// currently rendered for the given detail frame, plus a bool that
// reports whether the body uses cursor-driven navigation at all.
//
// Three detail-body renderers (marketplace plugin list, plugin's
// contained skills, MCP tools) window their list around
// frame.bodyCursor, so the key handler has to update bodyCursor —
// not top.scroll — when the user presses ↑/↓ on those pages.
// Returning a length lets the handler clamp to the list bounds.
func detailBodyCursorLen(m *Model, frame agentDetailFrame, row agentRow) (int, bool) {
	switch {
	case frame.subTab == agentsSubMarketplaces && row.marketplace != nil:
		// Marketplace body no longer renders a plugin list — use
		// "View all plugins →" to open the Plugins tab filtered to
		// this marketplace.
		return 0, false
	case row.plugin != nil:
		// Contained-skills list (snapshot lookup mirrors renderPluginBody).
		st := m.agents
		if st == nil || st.snapshot == nil {
			return 0, false
		}
		n := 0
		for _, s := range st.snapshot.Skills {
			if s.Provider == row.plugin.Provider && s.SourcePlugin == row.plugin.Name {
				n++
			}
		}
		if n == 0 {
			return 0, false
		}
		return n, true
	case row.mcp != nil && len(row.mcp.Tools) > 0:
		return len(row.mcp.Tools), true
	}
	return 0, false
}

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
	footerStr := footer.String()
	footerRows := strings.Count(footerStr, "\n")
	if m.height > 0 {
		maxBody := m.height - footerRows - 1
		if maxBody < 1 {
			maxBody = 1
		}
		lines := strings.Split(strings.TrimRight(body0, "\n"), "\n")
		total := len(lines)
		// Apply the page's free-form scroll offset (top.scroll,
		// driven by ↑/↓ on non-windowed pages). Clamp so we can't
		// scroll past the bottom of the visible window.
		maxScroll := total - maxBody
		if maxScroll < 0 {
			maxScroll = 0
		}
		scroll := top.scroll
		if scroll < 0 {
			scroll = 0
		}
		if scroll > maxScroll {
			scroll = maxScroll
		}
		if scroll > 0 {
			lines = lines[scroll:]
		}
		// Cap the displayed window so the action bar and footer
		// always stay on-screen. Plugin / MCP detail bodies append
		// tail sections (Keywords / EnvKeys) after their windowed
		// list — those lines are now reachable via top.scroll, but
		// if the user hasn't scrolled and the body still overflows
		// we trim from the bottom to keep the footer visible.
		hiddenAbove := scroll
		hiddenBelow := 0
		if len(lines) > maxBody {
			hiddenBelow = len(lines) - maxBody
			lines = lines[:maxBody]
		}
		// Surface scroll hints inside the body so the user knows
		// content extends beyond the viewport.
		if hiddenAbove > 0 || hiddenBelow > 0 {
			hint := dimVersionDetailHint(hiddenAbove, hiddenBelow)
			if hint != "" && len(lines) > 0 {
				// Replace the last visible line with itself + hint
				// so we don't reserve another row.
				lines[len(lines)-1] = lines[len(lines)-1] + "  " + hint
			}
		}
		body0 = strings.Join(lines, "\n") + "\n"
		bodyRows := strings.Count(body0, "\n")
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
		return renderPluginBody(m, frame, row.plugin)
	case row.mcp != nil:
		return renderMCPBody(m, frame, row.mcp)
	case row.session != nil:
		return renderSessionBody(row.session)
	case row.skill != nil:
		return renderSkillBody(row.skill)
	}
	return ""
}

func renderMarketplaceBody(m *Model, frame agentDetailFrame, mp *agents.Marketplace) string {
	count := m.marketplacePluginCount(mp)
	header := lipgloss.NewStyle().Bold(true).Foreground(cyberInfo).Render("Plugins")
	var b strings.Builder
	fmt.Fprintf(&b, "  %s  %s\n", header, dimVersion.Render(fmt.Sprintf("(%d)", count)))
	if count == 0 {
		b.WriteString("  " + dimVersion.Render("none discovered in the current snapshot") + "\n")
		return b.String()
	}
	b.WriteString("  " + dimVersion.Render("Press Enter on 'View all plugins →' to open the Plugins tab filtered to this marketplace.") + "\n")
	return b.String()
}

// windowDetailList returns a [start, end) range over a body list so
// the visible slice fits inside a detail page on a terminal of the
// given total height. The cursor is centered in the window (the same
// scroll style used by the Agents tab table) so the user can always
// see context above and below the highlighted row. If the list is
// short enough to fit entirely, [0, n) is returned and start/end
// indicators are suppressed by the caller.
//
// Budget rationale: the detail page header section (title bar +
// breadcrumb + title row + blank + 3-6 metadata rows + action bar
// + blank) plus the body's own header / hint rows + footer (3) is
// ≈ 14 rows. Floored at 5 so very small terminals still show
// something useful.
func windowDetailList(termHeight, total, cursor int) (start, end int) {
	if total == 0 {
		return 0, 0
	}
	maxRows := termHeight - 14
	if maxRows < 5 {
		maxRows = 5
	}
	if maxRows >= total {
		return 0, total
	}
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= total {
		cursor = total - 1
	}
	// Center the cursor in the window when possible.
	start = cursor - maxRows/2
	if start < 0 {
		start = 0
	}
	end = start + maxRows
	if end > total {
		end = total
		start = end - maxRows
		if start < 0 {
			start = 0
		}
	}
	return start, end
}

func renderPluginBody(m *Model, frame agentDetailFrame, p *agents.Plugin) string {
	st := m.agents
	var b strings.Builder
	header := lipgloss.NewStyle().Bold(true).Foreground(cyberInfo)
	// Contained skills (snapshot lookup). Windowed around bodyCursor
	// so a plugin with many skills can't push the action bar / footer
	// off-screen — same scroll behaviour as the marketplace's plugin
	// list (↑/↓ to walk, indicators tell you the rest is below).
	if st != nil && st.snapshot != nil {
		var skills []string
		for _, s := range st.snapshot.Skills {
			if s.Provider == p.Provider && s.SourcePlugin == p.Name {
				skills = append(skills, s.Name)
			}
		}
		if len(skills) > 0 {
			fmt.Fprintf(&b, "  %s  %s\n", header.Render("Contained skills"), dimVersion.Render(fmt.Sprintf("(%d)", len(skills))))
			start, end := windowDetailList(m.height, len(skills), frame.bodyCursor)
			if start > 0 {
				b.WriteString("    " + dimVersion.Render(fmt.Sprintf("↑ %d above", start)) + "\n")
			}
			for i := start; i < end; i++ {
				marker := "    • "
				if i == frame.bodyCursor {
					marker = "  ▸ • "
				}
				b.WriteString(marker + skills[i] + "\n")
			}
			if end < len(skills) {
				b.WriteString("    " + dimVersion.Render(fmt.Sprintf("↓ %d below", len(skills)-end)) + "\n")
			}
		}
	}
	if len(p.Keywords) > 0 {
		fmt.Fprintf(&b, "  %s\n", header.Render("Keywords"))
		b.WriteString("    " + strings.Join(p.Keywords, ", ") + "\n")
	}
	return b.String()
}

func renderMCPBody(m *Model, frame agentDetailFrame, mc *agents.MCP) string {
	var b strings.Builder
	header := lipgloss.NewStyle().Bold(true).Foreground(cyberInfo)
	// Tools windowed around bodyCursor — some MCPs (filesystem,
	// shell, etc.) expose dozens of tools and the body otherwise
	// overflows the terminal.
	if len(mc.Tools) > 0 {
		fmt.Fprintf(&b, "  %s  %s\n", header.Render("Tools"), dimVersion.Render(fmt.Sprintf("(%d)", len(mc.Tools))))
		start, end := windowDetailList(m.height, len(mc.Tools), frame.bodyCursor)
		if start > 0 {
			b.WriteString("    " + dimVersion.Render(fmt.Sprintf("↑ %d above", start)) + "\n")
		}
		for i := start; i < end; i++ {
			marker := "    • "
			if i == frame.bodyCursor {
				marker = "  ▸ • "
			}
			b.WriteString(marker + mc.Tools[i] + "\n")
		}
		if end < len(mc.Tools) {
			b.WriteString("    " + dimVersion.Render(fmt.Sprintf("↓ %d below", len(mc.Tools)-end)) + "\n")
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
