package tui

import "testing"

// TestAnimationActive_StopsWhenIdle verifies the animation loop
// pauses cleanly once nothing on screen needs to animate, so idle
// terminals don't pay a perpetual 10 fps render cost.
func TestAnimationActive_StopsWhenIdle(t *testing.T) {
	cases := []struct {
		name string
		m    Model
		want bool
	}{
		{"loading boots animation", Model{phase: phaseLoading}, true},
		{"resolving keeps animation", Model{phase: phaseResolving}, true},
		{"pending keeps animation", Model{phase: phaseDone, pending: 3}, true},
		{"fully idle stops animation", Model{phase: phaseDone}, false},
	}
	for _, c := range cases {
		if got := c.m.animationActive(); got != c.want {
			t.Errorf("%s: animationActive = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestEnsureAnimating_NoOpWhenTickerLive verifies the helper doesn't
// produce duplicate ticks when the loop is already running.
func TestEnsureAnimating_NoOpWhenTickerLive(t *testing.T) {
	m := Model{animTicking: true}
	if cmd := m.ensureAnimating(); cmd != nil {
		t.Errorf("ensureAnimating on a live ticker should return nil, got %v", cmd)
	}
	m = Model{animTicking: false}
	if cmd := m.ensureAnimating(); cmd == nil {
		t.Errorf("ensureAnimating on a paused ticker should return a tickFrame cmd")
	}
	if !m.animTicking {
		t.Errorf("ensureAnimating should flip animTicking to true")
	}
}
