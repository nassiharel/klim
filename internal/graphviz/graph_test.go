package graphviz

import (
	"strings"
	"testing"
)

func TestStep_NonNegativeDisplacement(t *testing.T) {
	g := New()
	g.AddNode("a", "A", 0)
	g.AddNode("b", "B", 1)
	g.AddEdge("a", "b")
	d := g.Step()
	if d < 0 {
		t.Errorf("displacement < 0: %v", d)
	}
}

func TestLayout_Converges(t *testing.T) {
	g := New()
	for i, name := range []string{"a", "b", "c", "d", "e"} {
		g.AddNode(name, name, i%4)
	}
	g.AddEdge("a", "b")
	g.AddEdge("b", "c")
	g.AddEdge("c", "d")
	g.AddEdge("d", "e")
	g.AddEdge("a", "e")
	// PR-78 review: previously this test logged but never failed.
	// Assert that Layout converges well under the iteration ceiling
	// for this small graph; if a future tunable change blows past
	// the threshold we want the test to fail loudly.
	iters := g.Layout(500, 1e-4)
	if iters >= 500 {
		t.Errorf("layout did not converge under 500 iterations (last threshold reached at %d)", iters)
	}
}

func TestRender_ProducesNonEmptyOutput(t *testing.T) {
	g := New()
	g.AddNode("a", "Alpha", 0)
	g.AddNode("b", "Beta", 1)
	g.AddEdge("a", "b")
	g.Layout(50, 0)
	out := g.Render(40, 10)
	if strings.TrimSpace(out) == "" {
		t.Error("expected non-empty render output")
	}
	if strings.Count(out, "\n") < 10 {
		t.Errorf("expected at least 10 newlines, got %d", strings.Count(out, "\n"))
	}
}

func TestRender_HandlesZeroDimensions(t *testing.T) {
	g := New()
	g.AddNode("a", "A", 0)
	if g.Render(0, 0) != "" {
		t.Error("zero dimensions should return empty string")
	}
	if g.Render(-1, 10) != "" {
		t.Error("negative width should return empty string")
	}
}

func TestStep_DeterministicWithSameSeed(t *testing.T) {
	mk := func() *Graph {
		g := New()
		g.Seed = 42
		g.AddNode("a", "", 0)
		g.AddNode("b", "", 1)
		g.AddNode("c", "", 2)
		g.AddEdge("a", "b")
		return g
	}
	a := mk()
	b := mk()
	for i := 0; i < 20; i++ {
		a.Step()
		b.Step()
	}
	for i := range a.Nodes {
		if a.Nodes[i].x != b.Nodes[i].x || a.Nodes[i].y != b.Nodes[i].y {
			t.Errorf("node %d positions diverge: %v vs %v", i, a.Nodes[i], b.Nodes[i])
		}
	}
}

func TestEdge_UnknownNodesAreNoOps(t *testing.T) {
	g := New()
	g.AddNode("a", "", 0)
	g.AddNode("b", "", 1)
	g.AddEdge("a", "ghost") // ghost doesn't exist
	if d := g.Step(); d < 0 {
		t.Errorf("displacement < 0 with unknown edge node: %v", d)
	}
}

func TestRender_UnstyledHasNoANSIEscapes(t *testing.T) {
	// PR-78 review: regression guard for README/pipe rendering.
	// When RenderOpts.Unstyled is true the output must contain no
	// ANSI escape (\x1b) characters so it can be pasted into a
	// markdown code fence or piped into a file unchanged.
	g := New()
	g.AddNode("a", "Alpha", 1)
	g.AddNode("b", "Beta", 2)
	g.AddEdge("a", "b")
	g.Layout(50, 1e-4)
	got := g.Render(40, 10, RenderOpts{Unstyled: true})
	if strings.ContainsRune(got, 0x1b) {
		t.Errorf("Unstyled render still contains ANSI escape (\\x1b):\n%q", got)
	}
}

func TestRender_StyledMayContainANSIEscapes(t *testing.T) {
	// Counterpart sanity check: the default styled path *does*
	// use lipgloss styling, so the absence of escapes there would
	// mean lipgloss became a no-op (worth knowing).
	g := New()
	g.AddNode("a", "Alpha", 1)
	g.Layout(10, 1e-4)
	got := g.Render(40, 10)
	if !strings.ContainsRune(got, 0x1b) {
		t.Logf("note: styled render produced no ANSI escapes; lipgloss may have disabled colour (e.g. NO_COLOR)")
	}
}

func TestLayout_NegativeItersReturnsZero(t *testing.T) {
	// PR-78 review: contract is "iterations actually executed".
	g := New()
	g.AddNode("a", "A", 0)
	if got := g.Layout(-5, 1e-4); got != 0 {
		t.Errorf("Layout(-5) = %d; want 0", got)
	}
}

func TestRender_LabelSanitization_StripsControlChars(t *testing.T) {
	// A maliciously-named tool must not be able to inject ANSI or
	// row-terminator characters into the canvas. unicode.IsPrint
	// keeps the visible parts of the escape (the [31m brackets etc.)
	// because those are printable characters; we only need the ESC
	// and the label's own '\n' gone for the output to be safe.
	const label = "ok\x1b[31mRED\x1b[0m\nNEWROW"
	g := New()
	g.AddNode("a", label, 1)
	g.Layout(20, 1e-4)
	got := g.Render(60, 10, RenderOpts{Unstyled: true})

	if strings.ContainsRune(got, 0x1b) {
		t.Errorf("Unstyled render leaked ESC despite control char in label:\n%q", got)
	}
	// Render itself separates rows with '\n', so we can't just
	// count newlines. The proof the label's own '\n' was stripped:
	// the prefix "ok" and the suffix "NEWROW" must appear on the
	// same rendered row.
	sameRow := false
	for _, row := range strings.Split(got, "\n") {
		if strings.Contains(row, "ok") && strings.Contains(row, "NEWROW") {
			sameRow = true
			break
		}
	}
	if !sameRow {
		t.Fatalf("label's embedded \\n was not stripped: 'ok' and 'NEWROW' landed on different rows.\nfull output:\n%s", got)
	}
}

func TestRender_DimensionCapPreventsOOM(t *testing.T) {
	// Pathological dimensions must be clamped, not honoured. Without
	// the cap this allocates ~width*height bytes which can OOM.
	g := New()
	g.AddNode("a", "x", 0)
	got := g.Render(10_000_000, 10_000_000, RenderOpts{Unstyled: true})
	if got == "" {
		t.Error("Render returned empty string for capped dimensions")
	}
	// Sanity-check the output isn't actually 10M^2 bytes: it should
	// be far smaller than 100MB (cap is 2000*2000 ≈ 4M cells).
	if len(got) > 20_000_000 {
		t.Errorf("Render output suspiciously large (%d bytes); cap not applied?", len(got))
	}
}

func TestRender_AreaCap(t *testing.T) {
	// Dimensions inside the per-axis cap can still exceed the area
	// cap (2000 * 2000 = 4M cells). Render must scale them down
	// before allocating the canvas.
	g := New()
	g.AddNode("a", "x", 0)
	got := g.Render(2000, 2000, RenderOpts{Unstyled: true})
	if got == "" {
		t.Fatal("Render returned empty for 2000x2000")
	}
	// Each rendered row ends with '\n'. Count rows: should be far
	// below 2000 because the area cap shrinks both axes.
	rows := strings.Count(got, "\n")
	if rows >= 2000 {
		t.Errorf("Render at 2000x2000 produced %d rows; area cap not applied", rows)
	}
	// Total output bytes should be well under the un-capped size.
	if len(got) > 5_000_000 {
		t.Errorf("Render output %d bytes; area cap not applied", len(got))
	}
}
