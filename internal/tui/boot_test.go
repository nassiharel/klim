package tui

import (
	"strings"
	"testing"
)

// TestRenderBootSplash_RevealsLogoOverFrames verifies the boot
// splash starts blank-ish, reveals more of the logo each frame, and
// shows the subtitle once the logo is fully revealed.
func TestRenderBootSplash_RevealsLogoOverFrames(t *testing.T) {
	// Frame 0 — almost nothing revealed.
	m := Model{width: 100, height: 24, phase: phaseLoading, animFrame: 0}
	early := stripAnsi(m.renderBootSplash())
	// Frame 12 — well past full reveal (9 frames × 4 cells = 36 ≥ logo width).
	m.animFrame = 12
	late := stripAnsi(m.renderBootSplash())

	if strings.Contains(early, "Reignite") {
		t.Errorf("subtitle should not appear before full reveal, got: %s", early)
	}
	if !strings.Contains(late, "Reignite") {
		t.Errorf("subtitle should appear after full reveal, got: %s", late)
	}
}

// TestRenderBootSplash_HandlesNarrowTerminal ensures the splash
// degrades gracefully on tiny terminals (no panic, returns
// something).
func TestRenderBootSplash_HandlesNarrowTerminal(t *testing.T) {
	m := Model{width: 0, height: 0, phase: phaseLoading, animFrame: 5}
	if got := m.renderBootSplash(); got != "" {
		t.Errorf("0×0 should produce empty splash, got: %q", got)
	}
	m = Model{width: 30, height: 10, phase: phaseLoading, animFrame: 5}
	got := m.renderBootSplash()
	if got == "" {
		t.Errorf("non-zero terminal should produce non-empty splash")
	}
}

// TestCyberSpinner_RotatesAcrossFrames ensures the spinner emits
// distinct glyphs across consecutive frames so animation is
// visible.
func TestCyberSpinner_RotatesAcrossFrames(t *testing.T) {
	seen := map[string]bool{}
	for f := 0; f < 4; f++ {
		seen[stripAnsi(cyberSpinner(f))] = true
	}
	if len(seen) < 4 {
		t.Errorf("expected 4 distinct spinner glyphs, got %d: %v", len(seen), seen)
	}
}
