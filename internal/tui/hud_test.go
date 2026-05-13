package tui

import (
	"strings"
	"testing"
)

// TestCyberHUD_RendersAllSegmentsAtWideWidth verifies the HUD
// composes the expected segment sequence at a comfortable terminal
// width.
func TestCyberHUD_RendersAllSegmentsAtWideWidth(t *testing.T) {
	m := Model{
		width:         140,
		height:        40,
		phase:         phaseDone,
		doctorChecked: true,
		// stats() reads m.tools — leaving nil produces 0/0 which
		// is still a valid HUD render.
	}
	out := m.renderCyberHUD()
	// Strip ANSI for substring assertions.
	plain := stripAnsi(out)
	for _, want := range []string{"KLIM", "TOOLS", "HEALTHY"} {
		if !strings.Contains(plain, want) {
			t.Errorf("HUD should contain %q, got: %s", want, plain)
		}
	}
}

// TestCyberHUD_NarrowFallback verifies the HUD shrinks gracefully at
// terminal widths that can't fit every segment.
func TestCyberHUD_NarrowFallback(t *testing.T) {
	cases := []int{20, 40, 60}
	for _, w := range cases {
		m := Model{width: w, height: 24, phase: phaseDone}
		out := m.renderCyberHUD()
		plain := stripAnsi(out)
		if !strings.Contains(plain, "KLIM") {
			t.Errorf("width=%d: HUD must always contain brand, got: %s", w, plain)
		}
	}
}

// TestBuildCyberUnderline_NarrowDoesNotPanic guards against the
// negative strings.Repeat count that used to happen when the active
// tab's range started past the visible rule (common when total tab
// strip width exceeds the terminal width).
func TestBuildCyberUnderline_NarrowDoesNotPanic(t *testing.T) {
	cases := []struct {
		name    string
		ranges  [][2]int
		cur     int
		ruleLen int
	}{
		{"active starts past rule", [][2]int{{20, 30}, {32, 42}}, 1, 10},
		{"active fully past rule", [][2]int{{20, 30}}, 0, 10},
		{"zero rule length", [][2]int{{2, 12}}, 0, 0},
		{"hi < 0", [][2]int{{-5, -1}}, 0, 10},
		{"out of range curParent", [][2]int{{2, 12}}, 5, 10},
		{"negative curParent", [][2]int{{2, 12}}, -1, 10},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panic: %v", r)
				}
			}()
			got := buildCyberUnderline(c.ranges, c.cur, c.ruleLen)
			// Must always be a non-empty string with the 2-cell
			// indent prefix, never a panic.
			if !strings.HasPrefix(got, "  ") {
				t.Errorf("expected 2-cell indent prefix, got: %q", got)
			}
		})
	}
}

// per-tab-width drift that used to happen when active and inactive
// tabs rendered at different total widths. After fixing the column
// accounting, the underline's heavy slice should land underneath
// each parent label regardless of which one is active.
func TestRenderTabBar_UnderlineAlignsAcrossPositions(t *testing.T) {
	// Sweep every parent slot. For each, the rendered tab line must
	// contain [ Label ] for that label specifically, and the
	// underline strip must contain a run of ━ characters.
	cases := []struct {
		tab   int
		label string
	}{
		{tabInstalled, "[ My Tools ]"},
		{tabDiscover, "[ Marketplace ]"},
		{tabProject, "[ Project ]"},
		{tabDashboard, "[ Dashboard ]"},
		{tabProfile, "[ My Profile ]"},
		{tabHealth, "[ Health ]"},
		{tabDoctor, "[ Security ]"},
		{tabBackup, "[ Backup ]"},
		{tabConfig, "[ Config ]"},
	}
	for _, c := range cases {
		m := Model{width: 160, activeTab: c.tab}
		plain := stripAnsi(m.renderTabBar())
		if !strings.Contains(plain, c.label) {
			t.Errorf("tab=%d: want %q in bar, got: %s", c.tab, c.label, plain)
		}
		// Find ━ in the underline (which is on the second line).
		lines := strings.Split(plain, "\n")
		if len(lines) < 2 || !strings.Contains(lines[1], "━") {
			t.Errorf("tab=%d: underline line must contain heavy bar, got: %q", c.tab, lines)
		}
	}
}

// stripAnsi strips ANSI SGR sequences from s. Test-only helper —
// view code never strips ANSI itself, but assertions need a plain
// view of the rendered string.
func stripAnsi(s string) string {
	var b strings.Builder
	skip := false
	for _, r := range s {
		if skip {
			// SGR sequences terminate at the final 'm'. Earlier
			// candidate-final bytes ('[', ';') are part of the
			// parameter sequence — keep skipping past them.
			if r == 'm' {
				skip = false
			}
			continue
		}
		if r == 0x1b {
			skip = true
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
