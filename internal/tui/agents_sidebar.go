package tui

import (
	"fmt"
	"sort"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/agents"
)

// agentsSidebarColWidth is the fixed column width of the Agents
// sidebar (matches colSidebar for the Tools tab so the visual feel
// is consistent).
const agentsSidebarColWidth = 22

// buildAgentsSidebarItems constructs the sidebar item list for the
// current sub-tab. Counts come from the snapshot before any filter is
// applied so each section's "All (N)" matches "all rows in this
// sub-tab".
func buildAgentsSidebarItems(st *agentsState) []agentSidebarItem {
	if st == nil || st.snapshot == nil {
		return nil
	}
	rows := sidebarRowsForSubTab(st)
	switch st.subTab {
	case agentsSubMarketplaces:
		return appendSections(nil,
			statusSectionMarketplaces(rows),
			providerSection(rows),
		)
	case agentsSubPlugins:
		return appendSections(nil,
			statusSectionPlugins(rows),
			providerSection(rows),
			marketplaceSection(rows),
		)
	case agentsSubSkills:
		return appendSections(nil,
			scopeSection(rows),
			providerSection(rows),
		)
	case agentsSubMCPs:
		return appendSections(nil,
			statusSectionMCPs(rows),
			transportSection(rows),
			scopeSection(rows),
			providerSection(rows),
		)
	case agentsSubSessions:
		return appendSections(nil,
			statusSectionSessions(rows),
			providerSection(rows),
		)
	}
	return nil
}

// sidebarRowsForSubTab returns the unfiltered row set for the active
// sub-tab. Counts are computed against this set so they reflect "all
// rows in this sub-tab" regardless of currently-active filters.
func sidebarRowsForSubTab(st *agentsState) []agentRow {
	if st == nil || st.snapshot == nil {
		return nil
	}
	var rows []agentRow
	switch st.subTab {
	case agentsSubMarketplaces:
		for i := range st.snapshot.Marketplaces {
			x := st.snapshot.Marketplaces[i]
			rows = append(rows, agentRow{provider: x.Provider, source: x.Source, marketplace: &x})
		}
	case agentsSubPlugins:
		for i := range st.snapshot.Plugins {
			x := st.snapshot.Plugins[i]
			rows = append(rows, agentRow{provider: x.Provider, source: x.Source, enabled: x.Enabled, plugin: &x})
		}
	case agentsSubSkills:
		for i := range st.snapshot.Skills {
			x := st.snapshot.Skills[i]
			rows = append(rows, agentRow{provider: x.Provider, source: x.Source, scope: x.Scope, enabled: x.Enabled, skill: &x})
		}
	case agentsSubMCPs:
		for i := range st.snapshot.MCPs {
			x := st.snapshot.MCPs[i]
			rows = append(rows, agentRow{provider: x.Provider, source: x.Source, scope: x.Scope, enabled: x.Enabled, mcp: &x})
		}
	case agentsSubSessions:
		bm := agentsBookmarks(st)
		for i := range st.snapshot.Sessions {
			x := st.snapshot.Sessions[i]
			row := agentRow{provider: x.Provider, source: x.Source, session: &x, id: x.ID}
			if bm.Contains(x.ID) {
				row.bookmarked = true
			}
			rows = append(rows, row)
		}
	}
	return rows
}

// appendSections chains section builders while skipping any section
// that only contained a header (i.e. zero non-header items).
func appendSections(items []agentSidebarItem, sections ...[]agentSidebarItem) []agentSidebarItem {
	for _, s := range sections {
		// At least one header + one selectable to count as a real section.
		nonHeader := 0
		for _, it := range s {
			if !it.isHeader {
				nonHeader++
			}
		}
		if nonHeader < 2 {
			// Just "All (0)" — not interesting.
			continue
		}
		items = append(items, s...)
	}
	return items
}

func statusSectionMarketplaces(rows []agentRow) []agentSidebarItem {
	var installed, available, local, builtin int
	for _, r := range rows {
		if r.marketplace == nil {
			continue
		}
		if r.marketplace.Installed {
			installed++
		} else {
			available++
		}
		switch r.marketplace.Source {
		case agents.SourceCatalogClaude, agents.SourceCatalogMCP, agents.SourceCatalogCopilot:
			builtin++
		default:
			local++
		}
	}
	return []agentSidebarItem{
		{label: "STATUS", isHeader: true},
		{label: fmt.Sprintf("All (%d)", len(rows)), section: "status", value: "all", count: len(rows)},
		{label: fmt.Sprintf("Installed (%d)", installed), section: "status", value: "installed", count: installed},
		{label: fmt.Sprintf("Available (%d)", available), section: "status", value: "available", count: available},
		{label: fmt.Sprintf("Built-in (%d)", builtin), section: "status", value: "builtin", count: builtin},
		{label: fmt.Sprintf("Local (%d)", local), section: "status", value: "local", count: local},
	}
}

func statusSectionPlugins(rows []agentRow) []agentSidebarItem {
	var installed, available, enabled, disabled int
	for _, r := range rows {
		if r.plugin == nil {
			continue
		}
		if r.plugin.Installed {
			installed++
			if r.plugin.Enabled {
				enabled++
			} else {
				disabled++
			}
		} else {
			available++
		}
	}
	return []agentSidebarItem{
		{label: "STATUS", isHeader: true},
		{label: fmt.Sprintf("All (%d)", len(rows)), section: "status", value: "all", count: len(rows)},
		{label: fmt.Sprintf("Installed (%d)", installed), section: "status", value: "installed", count: installed},
		{label: fmt.Sprintf("Available (%d)", available), section: "status", value: "available", count: available},
		{label: fmt.Sprintf("Enabled (%d)", enabled), section: "status", value: "enabled", count: enabled},
		{label: fmt.Sprintf("Disabled (%d)", disabled), section: "status", value: "disabled", count: disabled},
	}
}

func statusSectionMCPs(rows []agentRow) []agentSidebarItem {
	var installed, available, enabled, disabled int
	for _, r := range rows {
		if r.mcp == nil {
			continue
		}
		inst := r.mcp.Scope != agents.ScopeRemote
		if inst {
			installed++
			if r.mcp.Enabled {
				enabled++
			} else {
				disabled++
			}
		} else {
			available++
		}
	}
	return []agentSidebarItem{
		{label: "STATUS", isHeader: true},
		{label: fmt.Sprintf("All (%d)", len(rows)), section: "status", value: "all", count: len(rows)},
		{label: fmt.Sprintf("Installed (%d)", installed), section: "status", value: "installed", count: installed},
		{label: fmt.Sprintf("Available (%d)", available), section: "status", value: "available", count: available},
		{label: fmt.Sprintf("Enabled (%d)", enabled), section: "status", value: "enabled", count: enabled},
		{label: fmt.Sprintf("Disabled (%d)", disabled), section: "status", value: "disabled", count: disabled},
	}
}

func statusSectionSessions(rows []agentRow) []agentSidebarItem {
	var active, completed, stopped, bookmarked int
	for _, r := range rows {
		if r.session == nil {
			continue
		}
		if r.bookmarked {
			bookmarked++
		}
		switch r.session.Status {
		case agents.SessionStatusActive:
			active++
		case agents.SessionStatusCompleted:
			completed++
		case agents.SessionStatusStopped:
			stopped++
		}
	}
	items := []agentSidebarItem{
		{label: "STATUS", isHeader: true},
		{label: fmt.Sprintf("All (%d)", len(rows)), section: "status", value: "all", count: len(rows)},
	}
	if bookmarked > 0 {
		items = append(items, agentSidebarItem{
			label:   fmt.Sprintf("★ Bookmarked (%d)", bookmarked),
			section: "status",
			value:   "bookmarked",
			count:   bookmarked,
		})
	}
	items = append(items,
		agentSidebarItem{label: fmt.Sprintf("Active (%d)", active), section: "status", value: "active", count: active},
		agentSidebarItem{label: fmt.Sprintf("Completed (%d)", completed), section: "status", value: "completed", count: completed},
		agentSidebarItem{label: fmt.Sprintf("Stopped (%d)", stopped), section: "status", value: "stopped", count: stopped},
	)
	return items
}

func providerSection(rows []agentRow) []agentSidebarItem {
	counts := map[agents.ProviderID]int{}
	for _, r := range rows {
		counts[r.provider]++
	}
	order := []agents.ProviderID{agents.ProviderClaudeCode, agents.ProviderCopilotCLI, agents.ProviderMCPRegistry}
	items := []agentSidebarItem{
		{label: "PROVIDER", isHeader: true},
		{label: fmt.Sprintf("All (%d)", len(rows)), section: "provider", value: "", count: len(rows)},
	}
	for _, id := range order {
		c := counts[id]
		if c == 0 {
			continue
		}
		items = append(items, agentSidebarItem{
			label:   fmt.Sprintf("%s (%d)", providerShort(id), c),
			section: "provider",
			value:   string(id),
			count:   c,
		})
	}
	return items
}

func marketplaceSection(rows []agentRow) []agentSidebarItem {
	counts := map[string]int{}
	for _, r := range rows {
		if r.plugin != nil && r.plugin.Marketplace != "" {
			counts[r.plugin.Marketplace]++
		}
	}
	if len(counts) == 0 {
		return nil
	}
	names := make([]string, 0, len(counts))
	for k := range counts {
		names = append(names, k)
	}
	sort.Strings(names)
	items := []agentSidebarItem{
		{label: "MARKETPLACE", isHeader: true},
		{label: fmt.Sprintf("All (%d)", len(rows)), section: "marketplace", value: "", count: len(rows)},
	}
	for _, n := range names {
		items = append(items, agentSidebarItem{
			label:   fmt.Sprintf("%s (%d)", truncSidebar(n, agentsSidebarColWidth-6), counts[n]),
			section: "marketplace",
			value:   n,
			count:   counts[n],
		})
	}
	return items
}

func scopeSection(rows []agentRow) []agentSidebarItem {
	counts := map[agents.Scope]int{}
	for _, r := range rows {
		counts[r.scope]++
	}
	order := []agents.Scope{agents.ScopeUser, agents.ScopeProject, agents.ScopePlugin, agents.ScopeRemote}
	items := []agentSidebarItem{
		{label: "SCOPE", isHeader: true},
		{label: fmt.Sprintf("All (%d)", len(rows)), section: "scope", value: "", count: len(rows)},
	}
	for _, s := range order {
		c := counts[s]
		if c == 0 {
			continue
		}
		items = append(items, agentSidebarItem{
			label:   fmt.Sprintf("%s (%d)", s, c),
			section: "scope",
			value:   string(s),
			count:   c,
		})
	}
	return items
}

func transportSection(rows []agentRow) []agentSidebarItem {
	counts := map[string]int{}
	for _, r := range rows {
		if r.mcp == nil || r.mcp.Transport == "" {
			continue
		}
		counts[r.mcp.Transport]++
	}
	if len(counts) == 0 {
		return nil
	}
	names := make([]string, 0, len(counts))
	for k := range counts {
		names = append(names, k)
	}
	sort.Strings(names)
	items := []agentSidebarItem{
		{label: "TRANSPORT", isHeader: true},
		{label: fmt.Sprintf("All (%d)", len(rows)), section: "transport", value: "", count: len(rows)},
	}
	for _, n := range names {
		items = append(items, agentSidebarItem{
			label:   fmt.Sprintf("%s (%d)", n, counts[n]),
			section: "transport",
			value:   n,
			count:   counts[n],
		})
	}
	return items
}

func truncSidebar(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	if n < 2 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

// agentsApplySidebarSelection persists the sidebar pick into the
// per-dimension filter fields on agentsState. Selecting "All (N)" in
// a section clears that dimension.
func agentsApplySidebarSelection(st *agentsState, item agentSidebarItem) {
	if st == nil || item.isHeader {
		return
	}
	if st.statusFilter == nil {
		st.statusFilter = make(map[int]agentsFilter)
	}
	switch item.section {
	case "status":
		switch item.value {
		case "", "all":
			st.statusFilter[st.subTab] = agentsFilterAll
			st.statusFilterValue = ""
		case "installed":
			st.statusFilter[st.subTab] = agentsFilterInstalled
			st.statusFilterValue = ""
		case "available":
			st.statusFilter[st.subTab] = agentsFilterCatalog
			st.statusFilterValue = ""
		case "enabled":
			st.statusFilter[st.subTab] = agentsFilterEnabled
			st.statusFilterValue = ""
		case "disabled":
			st.statusFilter[st.subTab] = agentsFilterDisabled
			st.statusFilterValue = ""
		default:
			// Marketplace builtin/local, session active/completed/stopped.
			st.statusFilter[st.subTab] = agentsFilterAll
			st.statusFilterValue = item.value
		}
	case "provider":
		st.providerFilter = agents.ProviderID(item.value)
	case "marketplace":
		st.marketplaceFilter = item.value
	case "scope":
		st.scopeFilter = agents.Scope(item.value)
	case "transport":
		st.transportFilter = item.value
	}
}

// buildAgentsSidebarLines renders the sidebar as a slice of styled
// lines, with a focus highlight on the cursor row when sidebarOpen is
// true and the active-filter chip styling on selected items.
func buildAgentsSidebarLines(st *agentsState, maxRows int) []string {
	items := st.sidebarItems
	if len(items) == 0 {
		return nil
	}
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(cyberInfo)
	activeStyle := lipgloss.NewStyle().Foreground(cyberAccent).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(cyberFGDim)
	cursorStyle := cyberSelectedRowStyle

	lines := make([]string, 0, len(items))
	for i, item := range items {
		var line string
		if item.isHeader {
			line = headerStyle.Render(fixedWidth(item.label, agentsSidebarColWidth-2))
			lines = append(lines, line)
			continue
		}
		active := agentsSidebarItemActive(st, item)
		label := fixedWidth(item.label, agentsSidebarColWidth-4)
		switch {
		case active:
			line = "  " + activeStyle.Render(label)
		default:
			line = "  " + dimStyle.Render(label)
		}
		if st.sidebarOpen && i == st.sidebarIdx {
			line = cursorStyle.Render(fixedWidth(line, agentsSidebarColWidth))
		}
		lines = append(lines, line)
	}
	if maxRows > 0 && len(lines) > maxRows {
		start := 0
		if st.sidebarOpen && st.sidebarIdx >= maxRows {
			start = st.sidebarIdx - maxRows + 1
		}
		end := start + maxRows
		if end > len(lines) {
			end = len(lines)
		}
		lines = lines[start:end]
	}
	return lines
}

// agentsSidebarItemActive reports whether `item` is currently selected
// in its dimension.
func agentsSidebarItemActive(st *agentsState, item agentSidebarItem) bool {
	switch item.section {
	case "status":
		switch st.statusFilter[st.subTab] {
		case agentsFilterAll:
			if st.statusFilterValue == "" {
				return item.value == "all" || item.value == ""
			}
			return item.value == st.statusFilterValue
		case agentsFilterInstalled:
			return item.value == "installed"
		case agentsFilterCatalog:
			return item.value == "available"
		case agentsFilterEnabled:
			return item.value == "enabled"
		case agentsFilterDisabled:
			return item.value == "disabled"
		}
	case "provider":
		return string(st.providerFilter) == item.value
	case "marketplace":
		return st.marketplaceFilter == item.value
	case "scope":
		return string(st.scopeFilter) == item.value
	case "transport":
		return st.transportFilter == item.value
	}
	return false
}

// agentsSidebarMove advances the cursor by `delta`, skipping headers.
func agentsSidebarMove(st *agentsState, delta int) {
	if len(st.sidebarItems) == 0 {
		return
	}
	i := st.sidebarIdx
	if delta > 0 {
		for k := 0; k < delta; k++ {
			j := i + 1
			for j < len(st.sidebarItems) && st.sidebarItems[j].isHeader {
				j++
			}
			if j >= len(st.sidebarItems) {
				break
			}
			i = j
		}
	} else if delta < 0 {
		for k := 0; k < -delta; k++ {
			j := i - 1
			for j >= 0 && st.sidebarItems[j].isHeader {
				j--
			}
			if j < 0 {
				break
			}
			i = j
		}
	}
	st.sidebarIdx = i
}

// agentsSidebarSelect applies the item at the cursor and rebuilds the
// item list so counts reflect the new filter combination.
func agentsSidebarSelect(m *Model) tea.Cmd {
	st := m.agents
	if len(st.sidebarItems) == 0 || st.sidebarIdx < 0 || st.sidebarIdx >= len(st.sidebarItems) {
		return nil
	}
	item := st.sidebarItems[st.sidebarIdx]
	if item.isHeader {
		return nil
	}
	agentsApplySidebarSelection(st, item)
	st.cursor = 0
	st.sidebarItems = buildAgentsSidebarItems(st)
	if st.sidebarIdx >= len(st.sidebarItems) {
		st.sidebarIdx = len(st.sidebarItems) - 1
	}
	return nil
}
