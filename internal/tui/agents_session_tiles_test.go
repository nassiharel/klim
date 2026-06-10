package tui

// Regression tests for the Sessions sub-tab tile view.
//
// The renderer has multiple layout pressure points: column count
// must adapt to terminal width, every tile must end up exactly the
// same number of visual rows so the grid stays aligned, and the
// footer alignment fix from the table renderer must keep holding
// when the tiles are on screen.

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/agents"
)

// makeTileSnap builds a snapshot with n sessions for tile layout
// pressure tests. Each session has populated enrichment fields so
// every tile content row has data (no empty-cell special cases).
func makeTileSnap(n int) *agents.Snapshot {
	now := time.Date(2026, 6, 10, 14, 0, 0, 0, time.UTC)
	var sessions []agents.Session
	for i := 0; i < n; i++ {
		sessions = append(sessions, agents.Session{
			ID:             "claude:proj" + itoaT(i),
			Provider:       agents.ProviderClaudeCode,
			ProjectPath:    "/dev/proj" + itoaT(i),
			Title:          "Session title " + itoaT(i),
			Branch:         "main",
			LiveState:      agents.StateWorking,
			Status:         agents.SessionStatusActive,
			LastModified:   now.Add(-time.Duration(i) * time.Minute),
			TurnCount:      100 + i,
			RecentActivity: "doing work right now",
			MCPServers:     []string{"ado", "kusto"},
			Source:         agents.SourceLocalClaude,
		})
	}
	return &agents.Snapshot{
		Sessions: sessions,
		ProviderStatus: map[agents.ProviderID]agents.Status{
			agents.ProviderClaudeCode: {Installed: true, BinPath: "claude"},
		},
	}
}

// TestChooseTileLayout pins the column count at each width band so
// resizes don't flip the layout unpredictably.
func TestChooseTileLayout(t *testing.T) {
	t.Parallel()
	tests := []struct {
		totalWidth int
		wantCols   int
	}{
		{50, 1},  // narrow → single column
		{80, 2},  // medium → 2 cols
		{120, 3}, // wide → 3 cols
		{200, 3}, // very wide → still 3 (we don't want 4+)
		{20, 1},  // ridiculously narrow → 1 col
	}
	for _, tt := range tests {
		_, cols := chooseTileLayout(tt.totalWidth)
		if cols != tt.wantCols {
			t.Errorf("chooseTileLayout(%d): got %d cols, want %d", tt.totalWidth, cols, tt.wantCols)
		}
	}
}

// TestRenderSessionTiles_EachTileIsRightHeight verifies every tile
// renders to exactly tileHeight rows. If this drifts the grid loses
// alignment and the footer math falls apart.
func TestRenderSessionTiles_EachTileIsRightHeight(t *testing.T) {
	t.Parallel()
	snap := makeTileSnap(1)
	rows := []agentRow{{id: snap.Sessions[0].ID, session: &snap.Sessions[0]}}
	out := renderSessionTiles(rows, 0, 100)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != tileHeight {
		t.Errorf("expected %d lines for one tile, got %d:\n%s", tileHeight, len(lines), out)
	}
}

// TestRenderSessionTiles_GridShape lays out 6 sessions at a width
// that fits 3 cols and verifies the output is exactly 2 tile-rows
// (2 × tileHeight visual rows).
func TestRenderSessionTiles_GridShape(t *testing.T) {
	t.Parallel()
	snap := makeTileSnap(6)
	rows := make([]agentRow, len(snap.Sessions))
	for i := range snap.Sessions {
		rows[i] = agentRow{id: snap.Sessions[i].ID, session: &snap.Sessions[i]}
	}
	out := renderSessionTiles(rows, 0, 200)
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// 6 sessions / 3 cols = 2 rows × tileHeight visual lines per row.
	want := 2 * tileHeight
	if len(lines) != want {
		t.Errorf("expected %d lines for 6 tiles at 3 cols, got %d", want, len(lines))
	}
}

// TestRenderSessionTiles_SkipsNonSessions ensures a mixed agentRow
// slice (e.g. a stale plugin / MCP entry by accident) doesn't crash
// the renderer.
func TestRenderSessionTiles_SkipsNonSessions(t *testing.T) {
	t.Parallel()
	snap := makeTileSnap(1)
	mixed := []agentRow{
		{id: "spurious", session: nil},
		{id: snap.Sessions[0].ID, session: &snap.Sessions[0]},
	}
	out := renderSessionTiles(mixed, -1, 100)
	if strings.Contains(out, "spurious") {
		t.Errorf("non-session row leaked into tile output: %s", out)
	}
}

// TestSessionsViewMode_NextCycle pins the toggle path so flipping
// the binding doesn't accidentally land on a third state.
func TestSessionsViewMode_NextCycle(t *testing.T) {
	t.Parallel()
	if sessionsViewList.next() != sessionsViewTiles {
		t.Errorf("list → next should be tiles")
	}
	if sessionsViewTiles.next() != sessionsViewList {
		t.Errorf("tiles → next should be list (cycle back)")
	}
	if sessionsViewList.label() != "list" {
		t.Errorf("list label = %q", sessionsViewList.label())
	}
	if sessionsViewTiles.label() != "tiles" {
		t.Errorf("tiles label = %q", sessionsViewTiles.label())
	}
}

// TestFooterAlignsToBottom_TilesMode reuses the same pin-to-bottom
// assertion the list-mode test uses, but with tile rendering on.
// Catches the case where switching modes inadvertently changes the
// body row count.
func TestFooterAlignsToBottom_TilesMode(t *testing.T) {
	mk := func(sessionCount int) Model {
		m := NewModel()
		m.width = 140
		m.height = 40
		m.phase = phaseDone
		m.bootStart = time.Now().Add(-time.Hour)
		m.activeTab = tabAgents
		m.agents = newAgentsState()
		m.agents.subTab = agentsSubSessions
		m.agents.subTabViewMode = map[int]sessionsViewMode{agentsSubSessions: sessionsViewTiles}
		m.agents.snapshot = makeTileSnap(sessionCount)
		m.agents.loadedAt = time.Now()
		return m
	}
	for _, n := range []int{0, 1, 3, 12, 50} {
		t.Run("sessions_"+itoaT(n), func(t *testing.T) {
			t.Parallel()
			m := mk(n)
			out := m.renderView()
			rows := strings.Count(out, "\n") + 1
			if rows != m.height {
				t.Errorf("tile mode rendered %d rows, want %d (footer not pinned)", rows, m.height)
			}
		})
	}
}

// TestRenderOneTile_HighlightedHasDifferentBar pins the visual
// distinction between the cursor-selected tile and the rest. The
// border swaps to cyberPrimary on selection so the rendered ANSI of
// the first row must differ between selected and idle.
func TestRenderOneTile_HighlightedHasDifferentBar(t *testing.T) {
	t.Parallel()
	s := makeTileSnap(1).Sessions[0]
	idle := renderOneTile(s, 40, false)
	active := renderOneTile(s, 40, true)
	if idle == active {
		t.Error("active tile should differ from idle tile (border color)")
	}
	idleW := lipgloss.Width(strings.Split(idle, "\n")[0])
	activeW := lipgloss.Width(strings.Split(active, "\n")[0])
	if idleW != activeW {
		t.Errorf("active/idle widths differ: %d vs %d", activeW, idleW)
	}
}

// TestStateBarColor_PerLiveState pins the state → bar color mapping
// so a theme tweak doesn't accidentally swap the semantic meanings.
func TestStateBarColor_PerLiveState(t *testing.T) {
	t.Parallel()
	tests := []struct {
		state agents.LiveState
		want  any
	}{
		{agents.StateWorking, cyberOK},
		{agents.StateThinking, cyberPrimary},
		{agents.StateWaiting, cyberAccent},
		{agents.StateIdle, cyberFGDim},
		{agents.LiveState(""), cyberFGDim}, // unknown defaults to dim
	}
	for _, tt := range tests {
		got := stateBarColor(tt.state)
		if got != tt.want {
			t.Errorf("stateBarColor(%q) = %v, want %v", tt.state, got, tt.want)
		}
	}
}

// TestRenderOneTile_StarDoesNotShiftTitle is the regression test for
// the original bug: starring a tile changed the title's column
// position because the star glyph was only inserted when starred.
// The fix reserves a 2-cell slot in both states; this test asserts
// the title lands at the same *visual* column whether starred or not.
// We use a rune index (not byte index) because ⭐ is multi-byte in
// UTF-8 but a single grapheme — byte offsets would mislead.
func TestRenderOneTile_StarDoesNotShiftTitle(t *testing.T) {
	t.Parallel()
	s := makeTileSnap(1).Sessions[0]
	s.Title = "MarkerXYZ"
	s.Starred = false
	unstarred := renderOneTile(s, 50, false)
	s.Starred = true
	starred := renderOneTile(s, 50, false)

	// The title row is index 2 (rounded-border top at 0, subtitle at 1).
	unRow := stripANSIForTest(strings.Split(unstarred, "\n")[2])
	stRow := stripANSIForTest(strings.Split(starred, "\n")[2])

	unCol := visualColumn(unRow, "MarkerXYZ")
	stCol := visualColumn(stRow, "MarkerXYZ")
	if unCol < 0 || stCol < 0 {
		t.Fatalf("title text not found in rows:\n unstarred: %q\n starred:   %q", unRow, stRow)
	}
	if unCol != stCol {
		t.Errorf("title visual column shifted by star: unstarred=%d starred=%d\n unstarred row: %q\n starred row:   %q",
			unCol, stCol, unRow, stRow)
	}
}

// visualColumn returns the visual cell column at which `needle`
// starts in `s`, or -1 if not found. It counts wide characters (CJK,
// emoji) as 2 and skips zero-width sequences so the result matches
// what the terminal actually displays. Test-only helper — pairs
// with stripANSIForTest.
func visualColumn(s, needle string) int {
	idx := strings.Index(s, needle)
	if idx < 0 {
		return -1
	}
	return lipgloss.Width(s[:idx])
}

// TestStripActivityPrefix covers the most common JSONL-emitted role
// prefixes. The viewer is fine with brackets but tiles are tight on
// horizontal space and the prefix is redundant with the dot.
func TestStripActivityPrefix(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"  ", ""},
		{"[user] hello there", "hello there"},
		{"[assistant] working on it", "working on it"},
		{"[tool] Bash(ls)", "Bash(ls)"},
		{"tool: Edit(file=foo)", "Edit(file=foo)"},
		{"asking: pick branch", "pick branch"},
		{"no prefix passthrough", "no prefix passthrough"},
	}
	for _, tt := range tests {
		if got := stripActivityPrefix(tt.in); got != tt.want {
			t.Errorf("stripActivityPrefix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestProviderChip_ClaudeAndCopilot confirms each provider gets a
// visible chip with the expected label text.
func TestProviderChip_ClaudeAndCopilot(t *testing.T) {
	t.Parallel()
	cl := providerChip(agents.ProviderClaudeCode)
	if !strings.Contains(stripANSIForTest(cl), "Claude") {
		t.Errorf("Claude chip missing label: %q", cl)
	}
	co := providerChip(agents.ProviderCopilotCLI)
	if !strings.Contains(stripANSIForTest(co), "Copilot") {
		t.Errorf("Copilot chip missing label: %q", co)
	}
	if providerChip(agents.ProviderID("unknown")) != "" {
		t.Errorf("unknown provider should return empty chip")
	}
}

// TestBranchPill_EmptyBranchReturnsPlaceholder confirms the row keeps
// a constant width when the branch is unknown so the rounded border
// doesn't shrink/grow between tiles.
func TestBranchPill_EmptyBranchReturnsPlaceholder(t *testing.T) {
	t.Parallel()
	empty := branchPill("")
	if empty == "" {
		t.Error("empty branch should render placeholder, not empty string")
	}
	if !strings.Contains(stripANSIForTest(empty), "—") {
		t.Errorf("empty branch placeholder should contain em-dash, got %q", empty)
	}
	withBranch := branchPill("main")
	if !strings.Contains(stripANSIForTest(withBranch), "main") {
		t.Errorf("branch pill should contain branch name, got %q", withBranch)
	}
}

// stripANSIForTest removes ANSI escape sequences from a rendered
// string for substring-level assertions. Test-only helper.
func stripANSIForTest(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		if r == '\x1b' {
			inEsc = true
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
