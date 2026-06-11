package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/nassiharel/klim/internal/agents"
)

// TestComputeColumnWidths_NoOverflow verifies the off-by-one
// regression that broke footer alignment on the Sessions sub-tab:
// when totalWidth was just below `leadPad + fixed + gaps + minGrow`,
// the function used to bump the grow column up to minGrow even
// though that pushed the row past totalWidth. Every row then wrapped
// to two visual rows and the footer floated above the bottom.
//
// The fix accepts a sub-minGrow grow column rather than overflowing.
// This test pins both invariants for terminals wide enough to hold
// the fixed columns at all (≥ leadPad + fixed-sum + gaps = 100 cols
// for the Sessions schema). Below that the table can't fit no matter
// what we do; we don't pretend otherwise.
func TestComputeColumnWidths_NoOverflow(t *testing.T) {
	t.Parallel()
	// Mirror the Sessions sub-tab schema, which is the worst case
	// because it has the most columns (8 → 7 gaps).
	tmpl := []column{
		{header: "SOURCE", width: 10},
		{header: "ID", width: 10},
		{header: "TITLE", grow: true},
		{header: "TYPE", width: 11},
		{header: "STATUS", width: 9},
		{header: "TURNS", width: 5},
		{header: "MODIFIED", width: 11},
		{header: "PROJECT", width: 26},
	}
	// 4 (leadPad) + 82 (fixed) + 14 (7 gaps) = 100 — the minimum
	// totalWidth that the fixed cols alone can fit. Sweep across
	// the boundary where the old minGrow=16 floor would overflow
	// (totalWidth < 100 + 16 = 116).
	for _, totalWidth := range []int{100, 110, 115, 116, 120, 140, 200} {
		t.Run("totalWidth_"+itoaT(totalWidth), func(t *testing.T) {
			t.Parallel()
			cols := computeColumnWidths(tmpl, totalWidth)
			rowBudget := 4 + 14 // leadPad + 7 gaps
			for _, c := range cols {
				if c.width < 0 {
					t.Errorf("col %q has negative width %d", c.header, c.width)
				}
				rowBudget += c.width
			}
			if rowBudget > totalWidth {
				t.Errorf("totalWidth=%d: computed row width=%d (overflow by %d)",
					totalWidth, rowBudget, rowBudget-totalWidth)
			}
		})
	}
}

func itoaT(n int) string {
	if n < 0 {
		return "-" + itoaT(-n)
	}
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// TestRenderTranscriptLine covers the user/assistant/tool extraction
// the viewer relies on. Raw JSON garbage used to land in the modal;
// now we summarise it.
func TestRenderTranscriptLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string // substring match (the full string includes role/tool prefix)
	}{
		{
			name: "claude user text message",
			in:   `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hello world"}]}}`,
			want: "[user] hello world",
		},
		{
			name: "claude assistant text message",
			in:   `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hi back"}]}}`,
			want: "[assistant] hi back",
		},
		{
			name: "claude tool_use renders tool name + first arg",
			in:   `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{"command":"ls -la"}}]}}`,
			want: `[tool]      Bash(command="ls -la")`,
		},
		{
			name: "copilot user.message",
			in:   `{"type":"user.message","data":{"message":{"text":"copilot user text"}}}`,
			want: "[user] copilot user text",
		},
		{
			name: "telemetry / unknown event is skipped",
			in:   `{"type":"queue-operation","operation":"enqueue"}`,
			want: "",
		},
		{
			name: "non-JSON line passes through unchanged (legacy formats)",
			in:   "plain text line",
			want: "plain text line",
		},
		{
			name: "collapses internal whitespace in text",
			in:   `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"hi\n\nthere\t\tworld"}]}}`,
			want: "[user] hi there world",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := renderTranscriptLine([]byte(tt.in))
			if got != tt.want {
				t.Errorf("renderTranscriptLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestFooterAlignsToBottom verifies layoutWithFooter pins the footer
// to the last visual row of the rendered output, for both the empty
// Sessions sub-tab and one populated with a session. This catches
// regressions like the one where every row wrapped to 2 visual rows
// and the footer floated above the bottom.
func TestFooterAlignsToBottom(t *testing.T) {
	mk := func(sessionCount int) Model {
		m := NewModel()
		m.width = 140
		m.height = 40
		m.phase = phaseDone
		m.bootStart = time.Now().Add(-time.Hour)
		m.activeTab = tabAgents
		m.agents = newAgentsState()
		m.agents.subTab = agentsSubSessions
		m.agents.snapshot = makeTestSnap(sessionCount)
		m.agents.loadedAt = time.Now()
		return m
	}
	for _, n := range []int{0, 1, 5, 20, 100} {
		t.Run("sessions_"+itoaT(n), func(t *testing.T) {
			t.Parallel()
			m := mk(n)
			out := m.renderView()
			rows := strings.Count(out, "\n") + 1
			if rows != m.height {
				t.Errorf("rendered %d rows, want %d (footer not pinned to bottom)", rows, m.height)
			}
		})
	}
}

func makeTestSnap(n int) *agents.Snapshot {
	now := time.Date(2026, 6, 10, 14, 0, 0, 0, time.UTC)
	var sessions []agents.Session
	for i := 0; i < n; i++ {
		sessions = append(sessions, agents.Session{
			ID:           "claude:proj" + itoaT(i),
			Provider:     agents.ProviderClaudeCode,
			ProjectPath:  "/dev/proj" + itoaT(i),
			Title:        "Test session " + itoaT(i),
			Type:         "interactive",
			Status:       agents.SessionStatusActive,
			LiveState:    agents.StateWorking,
			LastModified: now.Add(-time.Duration(i) * time.Minute),
			Source:       agents.SourceLocalClaude,
		})
	}
	return &agents.Snapshot{
		Sessions: sessions,
		ProviderStatus: map[agents.ProviderID]agents.Status{
			agents.ProviderClaudeCode: {Installed: true, BinPath: "claude"},
		},
	}
}
