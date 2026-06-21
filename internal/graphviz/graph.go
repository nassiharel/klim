// Package graphviz is a tiny force-directed layout + terminal
// renderer used by `klim tool graph`. The simulator is intentionally
// minimal: Coulomb repulsion between every pair of nodes, Hooke
// attraction along edges, velocity damping per tick. No external
// deps beyond lipgloss for the renderer styling.
//
// Coordinates are normalised to a unit box; Render() maps them to a
// (width, height) terminal grid at draw time. Step() advances the
// simulation by one frame; users typically call Step+Render in a
// loop at ~10fps for animated mode, or call Layout(N) followed by
// Render once for a static snapshot.
package graphviz

import (
	"math"
	"math/rand"
)

// Node is one point on the graph.
type Node struct {
	ID     string
	Label  string
	Color  int     // index into the renderer's palette
	x, y   float64 // current position in [0,1]
	vx, vy float64 // velocity
}

// Edge connects two nodes by ID. The weight is multiplied into the
// Hooke attraction; sensible defaults are 1.0..3.0.
type Edge struct {
	From   string
	To     string
	Weight float64
}

// Graph is a simulation state: nodes + edges + their current
// positions/velocities.
type Graph struct {
	Nodes []Node
	Edges []Edge

	// Tunables. Defaults are reasonable for graphs up to ~50 nodes.
	Repulsion  float64 // node-node repulsion strength
	Attraction float64 // edge spring constant
	Damping    float64 // velocity damping factor per step
	Gravity    float64 // pull toward centre to prevent drift
	Seed       int64   // 0 = use a stable default
}

// New constructs a Graph with default tunables.
func New() *Graph {
	return &Graph{
		Repulsion:  0.008,
		Attraction: 0.08,
		Damping:    0.85,
		Gravity:    0.01,
		Seed:       1,
	}
}

// AddNode appends a node, scattering its initial position randomly
// around the centre. Subsequent Step() calls converge it.
func (g *Graph) AddNode(id, label string, color int) {
	rng := rand.New(rand.NewSource(g.Seed + int64(len(g.Nodes)))) //nolint:gosec // deterministic test seam
	x := 0.5 + (rng.Float64()-0.5)*0.4
	y := 0.5 + (rng.Float64()-0.5)*0.4
	g.Nodes = append(g.Nodes, Node{ID: id, Label: label, Color: color, x: x, y: y})
}

// AddEdge appends an edge with weight 1. Edges to or from unknown
// nodes are kept in the list but contribute nothing to the
// simulation (lookup misses are no-ops).
func (g *Graph) AddEdge(from, to string) {
	g.Edges = append(g.Edges, Edge{From: from, To: to, Weight: 1})
}

// Step advances the simulation by one tick. Returns the maximum
// node displacement on this step — useful for detecting convergence.
func (g *Graph) Step() float64 {
	idx := make(map[string]int, len(g.Nodes))
	for i, n := range g.Nodes {
		idx[n.ID] = i
	}

	// Repulsion: every pair of nodes pushes each other apart with
	// strength ∝ 1/r².
	for i := range g.Nodes {
		var fx, fy float64
		for j := range g.Nodes {
			if i == j {
				continue
			}
			dx := g.Nodes[i].x - g.Nodes[j].x
			dy := g.Nodes[i].y - g.Nodes[j].y
			r2 := dx*dx + dy*dy + 1e-6 // avoid div-by-zero
			fx += g.Repulsion * dx / r2
			fy += g.Repulsion * dy / r2
		}
		// Gentle pull to centre so disconnected subgraphs don't
		// drift to opposite corners.
		fx += -g.Gravity * (g.Nodes[i].x - 0.5)
		fy += -g.Gravity * (g.Nodes[i].y - 0.5)
		g.Nodes[i].vx = (g.Nodes[i].vx + fx) * g.Damping
		g.Nodes[i].vy = (g.Nodes[i].vy + fy) * g.Damping
	}

	// Attraction along edges (Hooke).
	for _, e := range g.Edges {
		i, iOK := idx[e.From]
		j, jOK := idx[e.To]
		if !iOK || !jOK {
			continue
		}
		w := e.Weight
		if w <= 0 {
			w = 1
		}
		dx := g.Nodes[j].x - g.Nodes[i].x
		dy := g.Nodes[j].y - g.Nodes[i].y
		fx := g.Attraction * w * dx
		fy := g.Attraction * w * dy
		g.Nodes[i].vx += fx
		g.Nodes[i].vy += fy
		g.Nodes[j].vx -= fx
		g.Nodes[j].vy -= fy
	}

	// Integrate velocities and clamp to the unit box.
	var maxDisp float64
	for i := range g.Nodes {
		oldX, oldY := g.Nodes[i].x, g.Nodes[i].y
		g.Nodes[i].x += g.Nodes[i].vx
		g.Nodes[i].y += g.Nodes[i].vy
		g.Nodes[i].x = clamp(g.Nodes[i].x, 0, 1)
		g.Nodes[i].y = clamp(g.Nodes[i].y, 0, 1)
		d := math.Hypot(g.Nodes[i].x-oldX, g.Nodes[i].y-oldY)
		if d > maxDisp {
			maxDisp = d
		}
	}
	return maxDisp
}

// Layout runs Step up to maxIters times and returns the number of
// iterations actually executed. Stops early when the maximum
// per-frame displacement falls below threshold.
func (g *Graph) Layout(maxIters int, threshold float64) int {
	if maxIters < 0 {
		// Contract: return the number of iterations actually
		// executed. Negative inputs become 0 rather than echoing
		// the bogus value back to the caller.
		return 0
	}
	for i := 0; i < maxIters; i++ {
		if g.Step() < threshold {
			return i + 1
		}
	}
	return maxIters
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
