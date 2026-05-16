package graphviz

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// Render produces a terminal-printable string representation of the
// graph using box-drawing characters. The arena is mapped from
// [0,1]x[0,1] to width×height grid cells; nodes render as a single
// coloured dot followed (when there's room) by an inline label.
// Edges are drawn as straight lines using Bresenham's algorithm.
//
// The renderer is deterministic given the same Positions(): no rng
// inside this function. Animated layouts can call Render after each
// Step() to produce frames.
func (g *Graph) Render(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	canvas := newCanvas(width, height)

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
		if n.Label != "" && x+len(n.Label)+1 < width {
			for i, r := range n.Label {
				canvas.setStyled(x+1+i, y, r, labelStyle())
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
	w, h  int
	cells [][]styledCell
}

type styledCell struct {
	r     rune
	style lipgloss.Style
	set   bool
}

func newCanvas(w, h int) *canvas {
	c := &canvas{w: w, h: h, cells: make([][]styledCell, h)}
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
			if cell.set {
				b.WriteString(cell.style.Render(string(cell.r)))
			} else {
				b.WriteRune(cell.r)
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
