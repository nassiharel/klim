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
		// Wide enough that every candidate (richest first) fits;
		// the new width-clamp falls back to leaner candidates
		// otherwise so this confirms the full-feature path.
		width:     120,
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

// TestRenderScanStrip_NarrowTerminalDoesNotWrap guards against the
// strip emitting more than one visual row when the terminal is too
// narrow for the full layout. The width-clamp must either return ""
// (slot collapses to a blank gap) or a single ≤ m.width string.
func TestRenderScanStrip_NarrowTerminalDoesNotWrap(t *testing.T) {
	widths := []int{0, 20, 40, 50, 60, 80, 120, 200}
	for _, w := range widths {
		m := Model{
			width:     w,
			phase:     phaseResolving,
			pending:   47,
			animFrame: 5,
		}
		for i := 0; i < 149; i++ {
			m.tools = append(m.tools, registry.Tool{
				Name:      "x",
				Instances: []registry.Instance{{Path: "/usr/bin/x"}},
			})
		}
		out := m.renderScanStrip()
		if out == "" {
			continue // strip collapsed; acceptable for tiny widths
		}
		if strings.Contains(out, "\n") {
			t.Errorf("width=%d: strip wrapped to multiple lines: %q", w, out)
		}
		if vl := visualLen(out); vl > w {
			t.Errorf("width=%d: strip visualLen=%d exceeds budget", w, vl)
		}
	}
}
