package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/agents/health"
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

func TestApplyStatusFilter_Marketplaces(t *testing.T) {
	rows := []agentRow{
		{name: "registered", marketplace: &agents.Marketplace{Installed: true}},
		{name: "discoverable", marketplace: &agents.Marketplace{Installed: false}},
	}
	inst := applyStatusFilter(rows, agentsSubMarketplaces, agentsFilterInstalled)
	if len(inst) != 1 || inst[0].name != "registered" {
		t.Errorf("installed marketplaces: %+v", inst)
	}
	avail := applyStatusFilter(rows, agentsSubMarketplaces, agentsFilterCatalog)
	if len(avail) != 1 || avail[0].name != "discoverable" {
		t.Errorf("available marketplaces: %+v", avail)
	}
}

func TestMarketplaceActions_AvailableSurfacesAddToLibrary(t *testing.T) {
	m := detailTestModel()
	mp := &agents.Marketplace{
		ID: "openai-codex-plugin-cc", Name: "openai-codex-plugin-cc",
		Provider:    agents.ProviderClaudeCode,
		URL:         "https://github.com/openai/codex-plugin-cc",
		InstallSpec: "openai/codex-plugin-cc",
		Installed:   false,
	}
	row := agentRow{marketplace: mp, provider: mp.Provider}
	actions := m.actionsForMarketplace(agentDetailFrame{}, row)
	if len(actions) == 0 {
		t.Fatal("expected actions for available marketplace")
	}
	if actions[0].label != "Add to library" {
		t.Errorf("primary action = %q, want 'Add to library'", actions[0].label)
	}
	if !actions[0].highlight {
		t.Error("'Add to library' should be highlighted")
	}
	if actions[0].disabled {
		t.Errorf("'Add to library' should be enabled (spec recorded); reason: %q", actions[0].reason)
	}
	for _, a := range actions {
		if a.label == "Remove" {
			t.Error("Remove action should be hidden for available marketplaces")
		}
		if a.label == "View all plugins →" {
			t.Error("View-all-plugins should be hidden for available marketplaces")
		}
	}
}

func TestMarketplaceActions_AvailableDisablesAddWhenNoSpec(t *testing.T) {
	m := detailTestModel()
	mp := &agents.Marketplace{Name: "x", Provider: agents.ProviderClaudeCode, Installed: false}
	actions := m.actionsForMarketplace(agentDetailFrame{}, agentRow{marketplace: mp, provider: mp.Provider})
	if len(actions) == 0 {
		t.Fatal("expected actions")
	}
	if actions[0].label != "Add to library" || !actions[0].disabled {
		t.Errorf("Add to library should be disabled without spec; got %+v", actions[0])
	}
}

func TestAgentsSupportsFilter(t *testing.T) {
	for _, sub := range []int{agentsSubMarketplaces, agentsSubPlugins, agentsSubMCPs} {
		if !agentsSupportsFilter(sub) {
			t.Errorf("sub %d should support filter", sub)
		}
	}
	for _, sub := range []int{agentsSubSkills, agentsSubSessions} {
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
			{ID: "mp1", Name: "mp1", Provider: agents.ProviderCopilotCLI, URL: "https://example.test", Source: agents.SourceLocalCopilot, Installed: true},
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
	// Create a non-installed plugin with no marketplace so Promote is disabled.
	m.agents.snapshot.Plugins = append(m.agents.snapshot.Plugins,
		agents.Plugin{ID: "pl3", Name: "pl3", Provider: agents.ProviderCopilotCLI, Installed: false},
	)
	m.agents.cursor = 2 // pl3
	_, _ = m.handleAgentsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "enter"}))

	// Find the Promote action index — it should be disabled (no marketplace).
	row, _ := m.resolveDetailRow(m.agents.detailStack[0])
	actions := m.agentsBuildActions(m.agents.detailStack[0], row)
	promoteIdx := -1
	for i, a := range actions {
		if a.label == "Promote ▸" {
			promoteIdx = i
			break
		}
	}
	if promoteIdx < 0 {
		t.Fatal("Promote action not found")
	}
	if !actions[promoteIdx].disabled {
		t.Fatal("Promote should be disabled for plugin with no marketplace")
	}
	// Navigate to the Promote action.
	for i := 0; i < promoteIdx; i++ {
		_, _ = m.handleAgentsDetailKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight, Text: "right"}))
	}

	// Pressing enter on the disabled Promote should set a flash and not
	// schedule the action.
	handled, cmd := m.handleAgentsDetailKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "enter"}))
	if !handled {
		t.Fatal("enter should be handled")
	}
	if cmd != nil {
		t.Error("disabled action should NOT return a tea.Cmd")
	}
	if m.agents.flash == "" {
		t.Error("expected a flash message for disabled action")
	}
}

func TestMarketplaceDetailListsPlugins(t *testing.T) {
	m := detailTestModel()
	m.agents.subTab = agentsSubMarketplaces
	m.agents.cursor = 0
	_, _ = m.handleAgentsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "enter"}))
	out := m.renderAgentsDetailPage()
	// Body should show the Plugins section header + count, plus a
	// hint to use the "View all plugins →" action — but it must NOT
	// embed the per-plugin list (that lives in the Plugins tab).
	for _, want := range []string{"Plugins", "(2)", "View all plugins"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected detail page to mention %q; output:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"pl1", "pl2"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("plugin name %q should NOT appear in marketplace body; output:\n%s", unwanted, out)
		}
	}
}

func TestMarketplaceViewAllPluginsAction(t *testing.T) {
	m := detailTestModel()
	m.agents.subTab = agentsSubMarketplaces
	m.agents.cursor = 0
	// Open the marketplace detail page.
	_, _ = m.handleAgentsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "enter"}))
	if !m.agents.detailPage {
		t.Fatal("expected detailPage=true after enter on marketplace row")
	}

	// "View all plugins →" is the first action; pressing enter should
	// dispatch agentViewMarketplacePluginsMsg.
	_, cmd := m.handleAgentsDetailKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "enter"}))
	if cmd == nil {
		t.Fatal("expected cmd from View all plugins action")
	}
	msg := cmd()
	if _, ok := msg.(agentViewMarketplacePluginsMsg); !ok {
		t.Fatalf("expected agentViewMarketplacePluginsMsg, got %T", msg)
	}

	// Feed the message back into the agents message router and verify
	// it navigates to the Plugins sub-tab with the marketplace filter.
	handled, _ := m.handleAgentsMsg(msg)
	if !handled {
		t.Fatal("agentViewMarketplacePluginsMsg should be handled")
	}
	if m.agents.detailPage {
		t.Error("expected detailPage=false after View all plugins")
	}
	if m.agents.subTab != agentsSubPlugins {
		t.Errorf("subTab = %d, want agentsSubPlugins (%d)", m.agents.subTab, agentsSubPlugins)
	}
	if m.agents.marketplaceFilter != "mp1" {
		t.Errorf("marketplaceFilter = %q, want mp1", m.agents.marketplaceFilter)
	}
}

func TestPluginViewSkillsAction(t *testing.T) {
	m := detailTestModel()
	// Add a skill that belongs to pl1 so View skills is enabled.
	m.agents.snapshot.Skills = append(m.agents.snapshot.Skills, agents.Skill{
		ID: "sk2", Name: "sk2", Provider: agents.ProviderCopilotCLI,
		Scope: agents.ScopePlugin, SourcePlugin: "pl1",
	})
	m.agents.subTab = agentsSubPlugins
	m.agents.cursor = 0 // pl1 (installed)
	_, _ = m.handleAgentsKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "enter"}))
	if !m.agents.detailPage {
		t.Fatal("expected detailPage=true after enter")
	}

	// Plugin actions for an installed plugin with skills: [Update,
	// View skills →, Disable, Launch, Uninstall, ...]. Initial focus
	// is on the first action (Update). Move Right once to land on
	// View skills →.
	_, _ = m.handleAgentsDetailKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight, Text: "right"}))
	if m.agents.detailStack[0].actionIdx != 1 {
		t.Fatalf("actionIdx = %d, want 1 (View skills)", m.agents.detailStack[0].actionIdx)
	}

	_, cmd := m.handleAgentsDetailKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter, Text: "enter"}))
	if cmd == nil {
		t.Fatal("expected cmd from View skills action")
	}
	msg := cmd()
	if _, ok := msg.(agentViewPluginSkillsMsg); !ok {
		t.Fatalf("expected agentViewPluginSkillsMsg, got %T", msg)
	}

	handled, _ := m.handleAgentsMsg(msg)
	if !handled {
		t.Fatal("agentViewPluginSkillsMsg should be handled")
	}
	if m.agents.detailPage {
		t.Error("expected detailPage=false")
	}
	if m.agents.subTab != agentsSubSkills {
		t.Errorf("subTab = %d, want agentsSubSkills (%d)", m.agents.subTab, agentsSubSkills)
	}
	if m.agents.pluginFilter != "pl1" {
		t.Errorf("pluginFilter = %q, want pl1", m.agents.pluginFilter)
	}
}

func TestInstalledHotkeyTogglesPluginsFilter(t *testing.T) {
	m := detailTestModel()
	m.agents.subTab = agentsSubPlugins
	if m.agents.statusFilter[agentsSubPlugins] != agentsFilterAll {
		t.Fatalf("precondition: statusFilter = %v, want All", m.agents.statusFilter[agentsSubPlugins])
	}

	_, _ = m.handleAgentsKey(tea.KeyPressMsg(tea.Key{Code: 'i', Text: "i"}))
	if m.agents.statusFilter[agentsSubPlugins] != agentsFilterInstalled {
		t.Errorf("after first toggle: filter = %v, want Installed", m.agents.statusFilter[agentsSubPlugins])
	}

	_, _ = m.handleAgentsKey(tea.KeyPressMsg(tea.Key{Code: 'i', Text: "i"}))
	if m.agents.statusFilter[agentsSubPlugins] != agentsFilterAll {
		t.Errorf("after second toggle: filter = %v, want All", m.agents.statusFilter[agentsSubPlugins])
	}
}

func TestApplyPluginFilter_OnlySkillsSubTab(t *testing.T) {
	rows := []agentRow{
		{skill: &agents.Skill{Name: "a", SourcePlugin: "pl1"}},
		{skill: &agents.Skill{Name: "b", SourcePlugin: "pl2"}},
		{skill: &agents.Skill{Name: "c", SourcePlugin: ""}},
	}
	out := applyPluginFilter(rows, agentsSubSkills, "pl1")
	if len(out) != 1 || out[0].skill.Name != "a" {
		t.Errorf("Skills filter: got %+v, want [a]", out)
	}
	// No-op on other subtabs.
	if got := applyPluginFilter(rows, agentsSubPlugins, "pl1"); len(got) != 3 {
		t.Errorf("non-Skills subtab should be a no-op: got %d", len(got))
	}
	// Empty filter is a no-op.
	if got := applyPluginFilter(rows, agentsSubSkills, ""); len(got) != 3 {
		t.Errorf("empty filter should be a no-op: got %d", len(got))
	}
}

// The plugin-update tests run against the real claude-code provider
// because rewiring agentsService() requires its own infrastructure;
// what we verify is the provider-level behaviour. See
// internal/agents/providers/claudecode/claudecode_test.go.
func TestPluginUpdateUnsupportedSurfaces(t *testing.T) {
	// Non-installed plugin should show Install (not Update).
	m := detailTestModel()
	row := agentRow{plugin: &m.agents.snapshot.Plugins[1]}
	actions := m.actionsForPlugin(agentDetailFrame{subTab: agentsSubPlugins}, row)

	// Install should be present and enabled.
	var install *agentAction
	for i := range actions {
		if actions[i].label == "Install" {
			install = &actions[i]
			break
		}
	}
	if install == nil {
		t.Fatal("Install action not found for non-installed plugin")
	}
	if install.disabled {
		t.Error("Install should be enabled for non-installed plugin")
	}

	// Update should NOT be present for non-installed plugins.
	for _, a := range actions {
		if a.label == "Update" {
			t.Error("Update should not be shown for non-installed plugin")
		}
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

func TestAgentsHealthCursorMovesAndWindowFollows(t *testing.T) {
	m := NewModel()
	m.activeTab = tabAgents
	m.width = 120
	m.height = 30
	m.agents = newAgentsState()
	m.agents.subTab = agentsSubHealth
	m.agents.snapshot = &agents.Snapshot{}

	// Populate 20 health issues.
	for i := 0; i < 20; i++ {
		m.agents.healthSub.issues = append(m.agents.healthSub.issues,
			health.Issue{Title: fmt.Sprintf("issue-%d", i), Severity: health.SeverityWarn, Provider: "test", Kind: health.KindProvider, Subject: "s"})
	}
	m.agents.healthSub.loaded = true
	m.agents.healthSub.loadedAt = time.Now()

	// Initial cursor should be 0.
	if m.agents.healthSub.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", m.agents.healthSub.cursor)
	}

	// Press down 5 times.
	for i := 0; i < 5; i++ {
		handled, _ := m.handleAgentsHealthKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown, Text: "down"}))
		if !handled {
			t.Fatalf("down key %d not handled", i)
		}
	}
	if m.agents.healthSub.cursor != 5 {
		t.Errorf("cursor after 5 downs = %d, want 5", m.agents.healthSub.cursor)
	}

	// Render should include the cursor row and scroll indicators.
	out := m.renderAgentsHealthView()
	if !strings.Contains(out, "issue-5") {
		t.Error("rendered output should contain cursor row issue-5")
	}

	// Press up — cursor should decrease.
	handled, _ := m.handleAgentsHealthKey(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp, Text: "up"}))
	if !handled {
		t.Fatal("up key not handled")
	}
	if m.agents.healthSub.cursor != 4 {
		t.Errorf("cursor after up = %d, want 4", m.agents.healthSub.cursor)
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

func TestUpdate_AgentsCostsLoadedMsg_ClearsLoadingFlag(t *testing.T) {
	// Regression for the "scanning transcripts… forever" bug.
	// Previously the outer Update switch only dispatched
	// agentsLoaded / Launched / Deleted to handleAgentsMsg, so
	// agentsCostsLoadedMsg arrived but cs.loading was never cleared.
	m := Model{agents: &agentsState{}}
	m.agents.costs.loading = true

	updated, _ := m.Update(agentsCostsLoadedMsg{samples: nil, err: nil})
	mp, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update did not return *Model; got %T", updated)
	}
	if mp.agents.costs.loading {
		t.Error("costs.loading was not cleared after agentsCostsLoadedMsg")
	}
	if !mp.agents.costs.loaded {
		t.Error("costs.loaded was not set after agentsCostsLoadedMsg")
	}
}

func TestUpdate_AgentsHealthLoadedMsg_ClearsLoadingFlag(t *testing.T) {
	// Same regression guard for Health.
	m := Model{agents: &agentsState{}}
	m.agents.healthSub.loading = true

	updated, _ := m.Update(agentsHealthLoadedMsg{issues: nil})
	mp, ok := updated.(Model)
	if !ok {
		t.Fatalf("Update did not return *Model; got %T", updated)
	}
	if mp.agents.healthSub.loading {
		t.Error("healthSub.loading was not cleared after agentsHealthLoadedMsg")
	}
	if !mp.agents.healthSub.loaded {
		t.Error("healthSub.loaded was not set after agentsHealthLoadedMsg")
	}
}
