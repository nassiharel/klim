package tui

import (
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/registry"
)

// TestRenderScanStrip_HiddenWhenIdle verifies the strip collapses to
// "" when nothing's in flight — so the layout still has a clean
// blank-line gap between the tab bar and the body content.
func TestRenderScanStrip_HiddenWhenIdle(t *testing.T) {
	m := Model{phase: phaseDone, animFrame: 5}
	if got := m.renderScanStrip(); got != "" {
		t.Errorf("strip should be empty when idle, got: %q", got)
	}
}

// TestRenderScanStrip_VisibleDuringResolve verifies the strip shows
// label + progress bar + count when version resolution is in flight.
func TestRenderScanStrip_VisibleDuringResolve(t *testing.T) {
	m := Model{
		phase:     phaseResolving,
		pending:   47,
		animFrame: 5,
		tools: []registry.Tool{
			// 149 installed tools; pending=47 means 102 resolved.
			func() registry.Tool {
				return registry.Tool{
					Name:      "x",
					Instances: []registry.Instance{{Path: "/usr/bin/x"}},
				}
			}(),
		},
	}
	// Pad tools to 149 installed entries so the math works.
	for i := 0; i < 148; i++ {
		m.tools = append(m.tools, registry.Tool{
			Name:      "x",
			Instances: []registry.Instance{{Path: "/usr/bin/x"}},
		})
	}

	plain := stripAnsi(m.renderScanStrip())
	for _, want := range []string{"RESOLVING VERSIONS", "102/149", "tools"} {
		if !strings.Contains(plain, want) {
			t.Errorf("strip should contain %q, got: %s", want, plain)
		}
	}
	if !strings.Contains(plain, "▓") {
		t.Errorf("strip should render a filled bar cell, got: %s", plain)
	}
}

// TestScanProgressBar_FillsProportionally checks the fill cell count
// scales with done/total.
func TestScanProgressBar_FillsProportionally(t *testing.T) {
	cases := []struct {
		done, total, cells int
		minFilled          int
	}{
		{0, 10, 10, 0},
		{5, 10, 10, 5},
		{10, 10, 10, 10},
	}
	for _, c := range cases {
		plain := stripAnsi(scanProgressBar(c.done, c.total, c.cells, 0))
		filled := strings.Count(plain, "▓")
		if filled != c.minFilled {
			t.Errorf("done=%d total=%d: expected %d ▓, got %d (%q)", c.done, c.total, c.minFilled, filled, plain)
		}
	}
}
