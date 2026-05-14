package tui

import (
	"testing"
	"time"

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
	if filterName(agentsFilterCatalog) != "catalog" {
		t.Error("agentsFilterCatalog name")
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

func TestSortColumnFor(t *testing.T) {
	if col := sortColumnFor(agentsSubSessions, agentsSortDefault); col != -1 {
		t.Errorf("default should be -1, got %d", col)
	}
	if col := sortColumnFor(agentsSubSessions, agentsSortTurns); col != 4 {
		t.Errorf("sessions/turns should be column 4, got %d", col)
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
	want := [5]int{1, 2, 3, 4, 5}
	if got != want {
		t.Errorf("counts = %v, want %v", got, want)
	}
	if agentsSnapshotCounts(nil) != [5]int{} {
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
