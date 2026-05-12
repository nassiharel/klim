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

// TestRenderTabBar_ActiveTabHasBracketsAndUnderline verifies the new
// cyber tab bar wraps the active tab in brackets and emits a heavy
// underline cell beneath it.
func TestRenderTabBar_ActiveTabHasBracketsAndUnderline(t *testing.T) {
	m := Model{width: 140, activeTab: tabDashboard}
	bar := stripAnsi(m.renderTabBar())
	// Active tab is bracket-wrapped (inner padding from the active
	// style adds a space on each side).
	if !strings.Contains(bar, "[ Dashboard ]") {
		t.Errorf("active tab should be bracket-wrapped, got: %s", bar)
	}
	// Heavy box-drawing underline char beneath the active label.
	if !strings.Contains(bar, "━") {
		t.Errorf("tab bar should emit heavy underline (━), got: %s", bar)
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
