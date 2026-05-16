package graphviz

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// RenderOpts tunes Render's output.
//
// Unstyled disables all ANSI escapes so the snapshot is safe to paste
// into a Markdown code fence, an issue body, or a log file. PR-78
// review: stdout is now treated as untrustworthy for ANSI when the
// caller passes Unstyled=true.
type RenderOpts struct {
	Unstyled bool
}

// Render produces a terminal-printable string representation of the
// graph using box-drawing characters. Nodes render as a single dot
// followed (when there's room) by an inline label. Edges are drawn
// as straight lines using Bresenham's algorithm.
//
// The renderer is deterministic given the same node positions; no
// rng inside this function. Animated callers re-Render after each
// Step().
func (g *Graph) Render(width, height int, opts ...RenderOpts) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	var o RenderOpts
	if len(opts) > 0 {
		o = opts[0]
	}
	canvas := newCanvas(width, height, o.Unstyled)

	for _, e := range g.Edges {
		a, b, ok := g.edgePositions(e)
		if !ok {
			continue
		}
		x1, y1 := worldToGrid(a.x, a.y, width, height)
		x2, y2 := worldToGrid(b.x, b.y, width, height)
		canvas.line(x1, y1, x2, y2, '·')
	}

	for _, n := range g.Nodes {
		x, y := worldToGrid(n.x, n.y, width, height)
		canvas.setStyled(x, y, '●', nodeStyle(n.Color))
		// Label fits when its last rune lands inside the canvas.
		// First rune sits at x+1, so the last rune is at
		// x+utf8.RuneCountInString(label); that needs to be < width.
		if n.Label != "" {
			runes := []rune(n.Label)
			if x+len(runes) < width {
				for i, r := range runes {
					canvas.setStyled(x+1+i, y, r, labelStyle())
				}
			}
		}
	}
	return canvas.String()
}

func (g *Graph) edgePositions(e Edge) (a, b Node, ok bool) {
	for _, n := range g.Nodes {
		if n.ID == e.From {
			a = n
		}
		if n.ID == e.To {
			b = n
		}
	}
	return a, b, a.ID != "" && b.ID != ""
}

func worldToGrid(x, y float64, width, height int) (int, int) {
	gx := int(x * float64(width-1))
	gy := int(y * float64(height-1))
	if gx < 0 {
		gx = 0
	}
	if gx >= width {
		gx = width - 1
	}
	if gy < 0 {
		gy = 0
	}
	if gy >= height {
		gy = height - 1
	}
	return gx, gy
}

type canvas struct {
	w, h     int
	cells    [][]styledCell
	unstyled bool
}

type styledCell struct {
	r     rune
	style lipgloss.Style
	set   bool
}

func newCanvas(w, h int, unstyled bool) *canvas {
	c := &canvas{w: w, h: h, cells: make([][]styledCell, h), unstyled: unstyled}
	for i := range c.cells {
		c.cells[i] = make([]styledCell, w)
		for j := range c.cells[i] {
			c.cells[i][j] = styledCell{r: ' '}
		}
	}
	return c
}

func (c *canvas) setStyled(x, y int, r rune, style lipgloss.Style) {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return
	}
	c.cells[y][x] = styledCell{r: r, style: style, set: true}
}

func (c *canvas) line(x1, y1, x2, y2 int, ch rune) {
	dx := abs(x2 - x1)
	dy := -abs(y2 - y1)
	sx := step(x1, x2)
	sy := step(y1, y2)
	err := dx + dy
	x, y := x1, y1
	style := edgeStyle()
	for {
		if x >= 0 && y >= 0 && x < c.w && y < c.h {
			if !c.cells[y][x].set {
				c.cells[y][x] = styledCell{r: ch, style: style, set: true}
			}
		}
		if x == x2 && y == y2 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x += sx
		}
		if e2 <= dx {
			err += dx
			y += sy
		}
	}
}

func (c *canvas) String() string {
	var b strings.Builder
	for _, row := range c.cells {
		for _, cell := range row {
			switch {
			case !cell.set:
				b.WriteRune(cell.r)
			case c.unstyled:
				b.WriteRune(cell.r)
			default:
				b.WriteString(cell.style.Render(string(cell.r)))
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
func step(a, b int) int {
	switch {
	case a < b:
		return 1
	case a > b:
		return -1
	}
	return 0
}

var nodePalette = []color.Color{
	lipgloss.Color("4"),  // blue
	lipgloss.Color("5"),  // magenta
	lipgloss.Color("3"),  // yellow
	lipgloss.Color("2"),  // green
	lipgloss.Color("6"),  // cyan
	lipgloss.Color("1"),  // red
	lipgloss.Color("7"),  // white
	lipgloss.Color("13"), // bright magenta
}

func nodeStyle(idx int) lipgloss.Style {
	if idx < 0 || idx >= len(nodePalette) {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(nodePalette[idx]).Bold(true)
}

func edgeStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
}

func labelStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
}
