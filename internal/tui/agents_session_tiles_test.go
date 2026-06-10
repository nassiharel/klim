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
// renders to exactly tileHeight rows (border-top + 4 content rows
// + border-bottom). If this drifts the grid loses alignment and
// the footer math falls apart again.
func TestRenderSessionTiles_EachTileIsRightHeight(t *testing.T) {
	t.Parallel()
	snap := makeTileSnap(1)
	rows := []agentRow{{id: snap.Sessions[0].ID, session: &snap.Sessions[0]}}
	out := renderSessionTiles(rows, 0, 100)
	// 1 tile → 1 row of tile output, which itself is tileHeight rows.
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
	// 6 sessions / 3 cols = 2 rows × tileHeight = 12 visual lines.
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
		m.agents.sessionsViewMode = sessionsViewTiles
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

// TestRenderOneTile_HighlightedHasDifferentBorder pins the visual
// distinction between the cursor-selected tile and the rest.
func TestRenderOneTile_HighlightedHasDifferentBorder(t *testing.T) {
	t.Parallel()
	s := makeTileSnap(1).Sessions[0]
	idle := renderOneTile(s, 40, false)
	active := renderOneTile(s, 40, true)
	if idle == active {
		t.Error("active tile should differ from idle tile (border color)")
	}
	// Both should be the same visible width.
	idleW := lipgloss.Width(strings.Split(idle, "\n")[0])
	activeW := lipgloss.Width(strings.Split(active, "\n")[0])
	if idleW != activeW {
		t.Errorf("active/idle widths differ: %d vs %d", activeW, idleW)
	}
}
