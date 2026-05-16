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
