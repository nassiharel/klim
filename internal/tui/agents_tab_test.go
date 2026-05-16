package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/nassiharel/klim/internal/agents"
)

func TestSortAgentRows_Sessions(t *testing.T) {
	now := time.Now()
	mk := func(name string, modified time.Time, turns int) agentRow {
		s := &agents.Session{LastModified: modified, TurnCount: turns}
		return agentRow{name: name, session: s}
	}
	rows := []agentRow{
		mk("c", now.Add(-1*time.Hour), 5),
		mk("a", now.Add(-2*time.Hour), 50),
		mk("b", now.Add(-30*time.Minute), 1),
	}

	// Sort by name (alphabetical, ascending).
	got := sortAgentRows(append([]agentRow(nil), rows...), agentsSubSessions, agentsSortName)
	if got[0].name != "a" || got[2].name != "c" {
		t.Errorf("name sort wrong: %v", names(got))
	}

	// Sort by modified (most recent first).
	got = sortAgentRows(append([]agentRow(nil), rows...), agentsSubSessions, agentsSortModified)
	if got[0].name != "b" || got[2].name != "a" {
		t.Errorf("modified sort wrong: %v", names(got))
	}

	// Sort by turns (highest first).
	got = sortAgentRows(append([]agentRow(nil), rows...), agentsSubSessions, agentsSortTurns)
	if got[0].name != "a" || got[1].name != "c" || got[2].name != "b" {
		t.Errorf("turns sort wrong: %v", names(got))
	}

	// Default leaves the input order alone.
	got = sortAgentRows(append([]agentRow(nil), rows...), agentsSubSessions, agentsSortDefault)
	if got[0].name != "c" {
		t.Errorf("default should preserve input order; got %v", names(got))
	}
}

func TestNextSortMode_CyclesThroughList(t *testing.T) {
	modes := []agentsSortMode{agentsSortDefault, agentsSortName, agentsSortModified}
	seen := map[agentsSortMode]bool{}
	cur := modes[0]
	for i := 0; i < len(modes)+1; i++ {
		seen[cur] = true
		cur = nextSortMode(modes, cur)
	}
	if len(seen) != len(modes) {
		t.Errorf("expected to cycle through %d modes, saw %d", len(modes), len(seen))
	}
}

func TestApplyStatusFilter_Plugins(t *testing.T) {
	rows := []agentRow{
		{name: "installed", plugin: &agents.Plugin{Installed: true}},
		{name: "available", plugin: &agents.Plugin{Installed: false}},
	}
	all := applyStatusFilter(rows, agentsSubPlugins, agentsFilterAll)
	if len(all) != 2 {
		t.Errorf("all filter: got %d rows, want 2", len(all))
	}
	inst := applyStatusFilter(rows, agentsSubPlugins, agentsFilterInstalled)
	if len(inst) != 1 || inst[0].name != "installed" {
		t.Errorf("installed filter: %+v", inst)
	}
	cat := applyStatusFilter(rows, agentsSubPlugins, agentsFilterCatalog)
	if len(cat) != 1 || cat[0].name != "available" {
		t.Errorf("catalog filter: %+v", cat)
	}
}

func TestApplyStatusFilter_MCPs(t *testing.T) {
	rows := []agentRow{
		{name: "user-mcp", mcp: &agents.MCP{Scope: agents.ScopeUser}},
		{name: "remote-mcp", mcp: &agents.MCP{Scope: agents.ScopeRemote}},
	}
	inst := applyStatusFilter(rows, agentsSubMCPs, agentsFilterInstalled)
	if len(inst) != 1 || inst[0].name != "user-mcp" {
		t.Errorf("installed MCPs: %+v", inst)
	}
	cat := applyStatusFilter(rows, agentsSubMCPs, agentsFilterCatalog)
	if len(cat) != 1 || cat[0].name != "remote-mcp" {
		t.Errorf("catalog MCPs: %+v", cat)
	}
}

func TestAgentsSupportsFilter(t *testing.T) {
	for _, sub := range []int{agentsSubPlugins, agentsSubMCPs} {
		if !agentsSupportsFilter(sub) {
			t.Errorf("sub %d should support filter", sub)
		}
	}
	for _, sub := range []int{agentsSubMarketplaces, agentsSubSkills, agentsSubSessions} {
		if agentsSupportsFilter(sub) {
			t.Errorf("sub %d should NOT support filter", sub)
		}
	}
}

func TestFilterName(t *testing.T) {
	if filterName(agentsFilterAll) != "all" {
		t.Error("agentsFilterAll name")
	}
	if filterName(agentsFilterInstalled) != "installed" {
		t.Error("agentsFilterInstalled name")
	}
	if filterName(agentsFilterCatalog) != "available" {
		t.Error("agentsFilterCatalog name")
	}
	if filterName(agentsFilterEnabled) != "enabled" {
		t.Error("agentsFilterEnabled name")
	}
	if filterName(agentsFilterDisabled) != "disabled" {
		t.Error("agentsFilterDisabled name")
	}
}

func TestWindowAgentRows(t *testing.T) {
	// 100 rows, window of 10.
	rows := make([]agentRow, 100)
	for i := range rows {
		rows[i] = agentRow{name: "r" + string(rune('0'+i%10))}
	}

	// Cursor near top — window starts at 0.
	vis, start, dc, ha, hb := windowAgentRows(rows, 2, 10)
	if len(vis) != 10 || start != 0 || dc != 2 || ha != 0 || hb != 90 {
		t.Errorf("near-top: vis=%d start=%d dc=%d ha=%d hb=%d", len(vis), start, dc, ha, hb)
	}

	// Cursor near middle — window centers around it.
	vis, start, dc, ha, hb = windowAgentRows(rows, 50, 10)
	if len(vis) != 10 || ha != start || dc != 50-start {
		t.Errorf("middle: start=%d dc=%d ha=%d hb=%d", start, dc, ha, hb)
	}
	if ha+hb+len(vis) != len(rows) {
		t.Errorf("counts don't add up: ha=%d hb=%d vis=%d", ha, hb, len(vis))
	}

	// Cursor near bottom — window pinned to end.
	vis, start, dc, ha, hb = windowAgentRows(rows, 99, 10)
	if len(vis) != 10 || start != 90 || dc != 9 || hb != 0 {
		t.Errorf("near-bottom: start=%d dc=%d hb=%d", start, dc, hb)
	}
	if ha != 90 {
		t.Errorf("hidden above should be 90, got %d", ha)
	}

	// Small list — entire list visible.
	short := rows[:5]
	vis, start, dc, ha, hb = windowAgentRows(short, 2, 10)
	if len(vis) != 5 || start != 0 || dc != 2 || ha != 0 || hb != 0 {
		t.Errorf("short: len=%d start=%d dc=%d ha=%d hb=%d", len(vis), start, dc, ha, hb)
	}

	// Empty list.
	vis, _, _, _, _ = windowAgentRows(nil, 0, 10)
	if vis != nil {
		t.Errorf("empty list should return nil; got %v", vis)
	}
}

func TestRowCopyText(t *testing.T) {
	cases := []struct {
		name     string
		row      agentRow
		wantText string
		wantTag  string
	}{
		{
			name: "claude session resume command",
			row: agentRow{session: &agents.Session{
				ID: "claude:abc-123", Provider: agents.ProviderClaudeCode,
			}},
			wantText: "claude -r abc-123",
			wantTag:  "resume command",
		},
		{
			name: "copilot session resume command",
			row: agentRow{session: &agents.Session{
				ID: "copilot:xyz", Provider: agents.ProviderCopilotCLI,
			}},
			wantText: "copilot --resume=xyz",
			wantTag:  "resume command",
		},
		{
			name:     "skill produces slash invocation",
			row:      agentRow{skill: &agents.Skill{Name: "summarize"}},
			wantText: "/summarize",
			wantTag:  "skill invocation",
		},
		{
			name: "plugin produces install command with marketplace",
			row: agentRow{plugin: &agents.Plugin{
				Name: "workiq", Marketplace: "copilot-plugins",
				Provider: agents.ProviderCopilotCLI,
			}},
			wantText: "copilot plugin install workiq@copilot-plugins",
			wantTag:  "install command",
		},
	}
	for _, c := range cases {
		gotText, gotTag := rowCopyText(c.row)
		if gotText != c.wantText || gotTag != c.wantTag {
			t.Errorf("%s: got (%q,%q), want (%q,%q)", c.name, gotText, gotTag, c.wantText, c.wantTag)
		}
	}
}

func TestRowOpenURL(t *testing.T) {
	cases := []struct {
		row  agentRow
		want string
	}{
		{agentRow{marketplace: &agents.Marketplace{URL: "https://a"}}, "https://a"},
		{agentRow{plugin: &agents.Plugin{Homepage: "https://h", Repository: "https://r"}}, "https://h"},
		{agentRow{plugin: &agents.Plugin{Repository: "https://r"}}, "https://r"},
		{agentRow{mcp: &agents.MCP{URL: "https://m"}}, "https://m"},
		{agentRow{}, ""},
	}
	for i, c := range cases {
		if got := rowOpenURL(c.row); got != c.want {
			t.Errorf("case %d: got %q want %q", i, got, c.want)
		}
	}
}

func TestApplyProviderFilter(t *testing.T) {
	rows := []agentRow{
		{name: "a", provider: agents.ProviderClaudeCode},
		{name: "b", provider: agents.ProviderCopilotCLI},
		{name: "c", provider: agents.ProviderClaudeCode},
	}
	if got := applyProviderFilter(rows, ""); len(got) != 3 {
		t.Errorf("empty provider should keep all rows, got %d", len(got))
	}
	got := applyProviderFilter(rows, agents.ProviderClaudeCode)
	if len(got) != 2 || got[0].name != "a" || got[1].name != "c" {
		t.Errorf("claude filter: %+v", got)
	}
}

func TestApplyMarketplaceFilter_OnlyPluginsSubTab(t *testing.T) {
	rows := []agentRow{
		{plugin: &agents.Plugin{Marketplace: "mp1"}},
		{plugin: &agents.Plugin{Marketplace: "mp2"}},
	}
	got := applyMarketplaceFilter(rows, agentsSubPlugins, "mp1")
	if len(got) != 1 || got[0].plugin.Marketplace != "mp1" {
		t.Errorf("filter for mp1: %+v", got)
	}
	// Empty filter is no-op.
	if got := applyMarketplaceFilter(rows, agentsSubPlugins, ""); len(got) != 2 {
		t.Errorf("empty filter should keep all, got %d", len(got))
	}
	// On non-Plugins sub-tabs the filter is a no-op.
	if got := applyMarketplaceFilter(rows, agentsSubSkills, "mp1"); len(got) != 2 {
		t.Errorf("skills sub-tab should ignore marketplace filter, got %d", len(got))
	}
}

func TestApplyStatusFilter_PluginsEnabledDisabled(t *testing.T) {
	rows := []agentRow{
		{name: "enabled", plugin: &agents.Plugin{Installed: true, Enabled: true}},
		{name: "disabled", plugin: &agents.Plugin{Installed: true, Enabled: false}},
		{name: "available", plugin: &agents.Plugin{Installed: false}},
	}
	if got := applyStatusFilter(rows, agentsSubPlugins, agentsFilterEnabled); len(got) != 1 || got[0].name != "enabled" {
		t.Errorf("enabled filter: %+v", got)
	}
	if got := applyStatusFilter(rows, agentsSubPlugins, agentsFilterDisabled); len(got) != 1 || got[0].name != "disabled" {
		t.Errorf("disabled filter: %+v", got)
	}
}

func TestCycleProviderFilter(t *testing.T) {
	avail := []agents.ProviderID{agents.ProviderClaudeCode, agents.ProviderCopilotCLI}
	// Empty → first.
	if got := cycleProviderFilter("", avail); got != agents.ProviderClaudeCode {
		t.Errorf("empty→%s, want claude", got)
	}
	// First → second.
	if got := cycleProviderFilter(agents.ProviderClaudeCode, avail); got != agents.ProviderCopilotCLI {
		t.Errorf("claude→%s, want copilot", got)
	}
	// Last → empty.
	if got := cycleProviderFilter(agents.ProviderCopilotCLI, avail); got != "" {
		t.Errorf("copilot→%s, want empty", got)
	}
	// No providers available → empty.
	if got := cycleProviderFilter(agents.ProviderClaudeCode, nil); got != "" {
		t.Errorf("no providers→%s, want empty", got)
	}
}

func TestCycleMarketplaceFilter(t *testing.T) {
	avail := []string{"mp-a", "mp-b"}
	if got := cycleMarketplaceFilter("", avail); got != "mp-a" {
		t.Errorf("empty→%s, want mp-a", got)
	}
	if got := cycleMarketplaceFilter("mp-a", avail); got != "mp-b" {
		t.Errorf("mp-a→%s, want mp-b", got)
	}
	if got := cycleMarketplaceFilter("mp-b", avail); got != "" {
		t.Errorf("mp-b→%s, want empty", got)
	}
}

func TestAgentsAvailableMarketplaces_IncludesPluginRefs(t *testing.T) {
	snap := &agents.Snapshot{
		Marketplaces: []agents.Marketplace{{Name: "explicit"}},
		Plugins:      []agents.Plugin{{Marketplace: "implicit"}, {Marketplace: ""}},
	}
	got := agentsAvailableMarketplaces(snap)
	want := map[string]bool{"explicit": true, "implicit": true}
	if len(got) != 2 {
		t.Fatalf("got %d marketplaces, want 2: %v", len(got), got)
	}
	for _, name := range got {
		if !want[name] {
			t.Errorf("unexpected marketplace %q", name)
		}
	}
}

func TestSortColumnFor(t *testing.T) {
	if col := sortColumnFor(agentsSubSessions, agentsSortDefault); col != -1 {
		t.Errorf("default should be -1, got %d", col)
	}
	if col := sortColumnFor(agentsSubSessions, agentsSortTurns); col != 5 {
		t.Errorf("sessions/turns should be column 5, got %d", col)
	}
	if col := sortColumnFor(agentsSubMarketplaces, agentsSortName); col != 1 {
		t.Errorf("marketplaces/name should be column 1, got %d", col)
	}
}

func TestAgentsSnapshotCounts(t *testing.T) {
	snap := &agents.Snapshot{
		Marketplaces: []agents.Marketplace{{}},
		Plugins:      []agents.Plugin{{}, {}},
		Skills:       []agents.Skill{{}, {}, {}},
		MCPs:         []agents.MCP{{}, {}, {}, {}},
		Sessions:     []agents.Session{{}, {}, {}, {}, {}},
	}
	got := agentsSnapshotCounts(snap)
	want := [7]int{1, 2, 3, 4, 5, 0, 0}
	if got != want {
		t.Errorf("counts = %v, want %v", got, want)
	}
	if agentsSnapshotCounts(nil) != [7]int{} {
		t.Error("nil snapshot should return zeros")
	}
}

func names(rows []agentRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.name
	}
	return out
}

// ---------------- detail-page tests ----------------

// detailTestModel returns a Model whose agents snapshot has one of
// each entity type, so detail-page tests can drive navigation without
// touching the filesystem or real providers.
func detailTestModel() Model {
	m := NewModel()
	m.activeTab = tabAgents
	m.width = 120
	m.agents = newAgentsState()
	m.agents.snapshot = &agents.Snapshot{
		Marketplaces: []agents.Marketplace{
			{ID: "mp1", Name: "mp1", Provider: agents.ProviderCopilotCLI, URL: "https://example.test", Source: agents.SourceLocalCopilot},
		},
		Plugins: []agents.Plugin{
			{ID: "pl1", Name: "pl1", Provider: agents.ProviderCopilotCLI, Marketplace: "mp1", Installed: true, Enabled: true, Version: "1.0.0"},
			{ID: "pl2", Name: "pl2", Provider: agents.ProviderCopilotCLI, Marketplace: "mp1", Installed: false},
		},
		Skills:   []agents.Skill{{ID: "sk1", Name: "sk1", Provider: agents.ProviderCopilotCLI, Scope: agents.ScopeUser}},
		MCPs:     []agents.MCP{{ID: "mc1", Name: "mc1", Provider: agents.ProviderCopilotCLI, Scope: agents.ScopeUser, Enabled: true}},
		Sessions: []agents.Session{{ID: "se1", Name: "se1", Provider: agents.ProviderCopilotCLI, TranscriptPath: "/tmp/x"}},
	}
	m.agents.loadedAt = time.Now()
	return m
}

func TestAgentsDetailPageOpensOnEnter(t *testing.T) {
	m := detailTestModel()
	m.agents.subTab = agentsSubPlugins
	m.agents.cursor = 0
	handled, _ := m.handleAgentsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "enter"}))
	if !handled {
		t.Fatal("enter should be handled")
	}
	if !m.agents.detailPage {
		t.Fatal("expected detailPage=true after enter")
	}
	if len(m.agents.detailStack) != 1 {
		t.Fatalf("expected 1 frame on stack, got %d", len(m.agents.detailStack))
	}
	if got := m.agents.detailStack[0].entityID; got != "pl1" {
		t.Errorf("frame entityID = %q, want pl1", got)
	}
}

func TestAgentsDetailEscReturnsToList(t *testing.T) {
	m := detailTestModel()
	m.agents.subTab = agentsSubPlugins
	m.agents.cursor = 1
	_, _ = m.handleAgentsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "enter"}))
	if !m.agents.detailPage {
		t.Fatal("expected detailPage=true after enter")
	}
	handled, _ := m.handleAgentsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape, Text: "esc"}))
	if !handled {
		t.Fatal("esc should be handled")
	}
	if m.agents.detailPage {
		t.Error("expected detailPage=false after esc")
	}
	if m.agents.cursor != 1 {
		t.Errorf("cursor changed to %d; expected preserved 1", m.agents.cursor)
	}
}

func TestAgentsDetailDisabledActionFlashes(t *testing.T) {
	m := detailTestModel()
	m.agents.subTab = agentsSubPlugins
	m.agents.cursor = 1 // pl2 is not installed → Update is disabled
	_, _ = m.handleAgentsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "enter"}))

	// First action for plugins is "Install" — focus is at 0. Move to
	// "Update" (index 1).
	_, _ = m.handleAgentsDetailKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight, Text: "right"}))
	if got := m.agents.detailStack[0].actionIdx; got != 1 {
		t.Fatalf("actionIdx after one right = %d, want 1", got)
	}

	// Pressing enter on the disabled Update should set a flash and not
	// schedule the action.
	handled, cmd := m.handleAgentsDetailKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "enter"}))
	if !handled {
		t.Fatal("enter should be handled")
	}
	if cmd != nil {
		t.Error("disabled action should NOT return a tea.Cmd")
	}
	if !strings.Contains(m.agents.flash, "Update") {
		t.Errorf("expected flash about Update, got %q", m.agents.flash)
	}
}

func TestMarketplaceDetailListsPlugins(t *testing.T) {
	m := detailTestModel()
	m.agents.subTab = agentsSubMarketplaces
	m.agents.cursor = 0
	_, _ = m.handleAgentsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "enter"}))
	out := m.renderAgentsDetailPage()
	for _, want := range []string{"pl1", "pl2", "Plugins"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected detail page to mention %q; output:\n%s", want, out)
		}
	}
	// Body should also include count "(2)".
	if !strings.Contains(out, "(2)") {
		t.Errorf("expected '(2)' plugin count in body")
	}
}

// The plugin-update tests run against the real claude-code provider
// because rewiring agentsService() requires its own infrastructure;
// what we verify is the provider-level behaviour. See
// internal/agents/providers/claudecode/claudecode_test.go.
func TestPluginUpdateUnsupportedSurfaces(t *testing.T) {
	// Action build for a non-installed plugin should disable Update.
	m := detailTestModel()
	row := agentRow{plugin: &m.agents.snapshot.Plugins[1]}
	actions := m.actionsForPlugin(agentDetailFrame{subTab: agentsSubPlugins}, row)
	var update *agentAction
	for i := range actions {
		if actions[i].label == "Update" {
			update = &actions[i]
			break
		}
	}
	if update == nil {
		t.Fatal("Update action not found")
	}
	if !update.disabled {
		t.Error("Update should be disabled for non-installed plugin")
	}
	if !strings.Contains(update.reason, "not installed") {
		t.Errorf("reason %q should mention 'not installed'", update.reason)
	}
}

func TestBuildAgentsSidebarItems_Plugins_HasExpectedSections(t *testing.T) {
	m := detailTestModel()
	m.agents.subTab = agentsSubPlugins
	items := buildAgentsSidebarItems(m.agents)
	if len(items) == 0 {
		t.Fatal("expected sidebar items for Plugins")
	}
	sections := map[string]bool{}
	for _, it := range items {
		if it.isHeader {
			sections[it.label] = true
		}
	}
	for _, want := range []string{"STATUS", "PROVIDER", "MARKETPLACE"} {
		if !sections[want] {
			t.Errorf("missing %q header section; got %v", want, sections)
		}
	}
}

func TestAgentsApplySidebarSelection_Provider(t *testing.T) {
	m := detailTestModel()
	m.agents.subTab = agentsSubPlugins
	m.agents.sidebarItems = buildAgentsSidebarItems(m.agents)
	var item agentSidebarItem
	for _, it := range m.agents.sidebarItems {
		if it.section == "provider" && it.value == string(agents.ProviderCopilotCLI) {
			item = it
			break
		}
	}
	if item.label == "" {
		t.Skip("copilot provider not in fixture")
	}
	agentsApplySidebarSelection(m.agents, item)
	if m.agents.providerFilter != agents.ProviderCopilotCLI {
		t.Errorf("providerFilter = %q, want copilot-cli", m.agents.providerFilter)
	}
}

func TestAgentsSidebarMove_SkipsHeaders(t *testing.T) {
	st := &agentsState{
		sidebarItems: []agentSidebarItem{
			{label: "A", isHeader: true},
			{label: "a1", section: "x", value: "1"},
			{label: "a2", section: "x", value: "2"},
			{label: "B", isHeader: true},
			{label: "b1", section: "y", value: "3"},
		},
		sidebarIdx: 1,
	}
	agentsSidebarMove(st, 1)
	if got := st.sidebarItems[st.sidebarIdx].label; got != "a2" {
		t.Errorf("down once: %q, want a2", got)
	}
	agentsSidebarMove(st, 1)
	if got := st.sidebarItems[st.sidebarIdx].label; got != "b1" {
		t.Errorf("down twice (skipping header B): %q, want b1", got)
	}
	agentsSidebarMove(st, -1)
	if got := st.sidebarItems[st.sidebarIdx].label; got != "a2" {
		t.Errorf("back up: %q, want a2", got)
	}
}

func TestApplyScopeFilter(t *testing.T) {
	rows := []agentRow{
		{scope: agents.ScopeUser, name: "u"},
		{scope: agents.ScopeProject, name: "p"},
	}
	if got := applyScopeFilter(rows, ""); len(got) != 2 {
		t.Errorf("empty scope: %d", len(got))
	}
	got := applyScopeFilter(rows, agents.ScopeUser)
	if len(got) != 1 || got[0].name != "u" {
		t.Errorf("user scope: %+v", got)
	}
}

func TestApplyTransportFilter(t *testing.T) {
	rows := []agentRow{
		{mcp: &agents.MCP{Transport: "stdio"}},
		{mcp: &agents.MCP{Transport: "http"}},
		{name: "no-mcp"},
	}
	got := applyTransportFilter(rows, "http")
	if len(got) != 1 {
		t.Errorf("http filter: %+v", got)
	}
}

func TestApplyStatusValueFilter_Marketplaces(t *testing.T) {
	rows := []agentRow{
		{marketplace: &agents.Marketplace{Source: agents.SourceCatalogClaude}},
		{marketplace: &agents.Marketplace{Source: agents.SourceLocalClaude}},
	}
	got := applyStatusValueFilter(rows, agentsSubMarketplaces, "builtin")
	if len(got) != 1 || got[0].marketplace.Source != agents.SourceCatalogClaude {
		t.Errorf("builtin filter: %+v", got)
	}
	got = applyStatusValueFilter(rows, agentsSubMarketplaces, "local")
	if len(got) != 1 || got[0].marketplace.Source != agents.SourceLocalClaude {
		t.Errorf("local filter: %+v", got)
	}
}
