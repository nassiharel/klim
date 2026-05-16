package cli

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/nassiharel/klim/internal/graphviz"
	"github.com/nassiharel/klim/internal/registry"
)

var (
	graphTUI        bool
	graphBy         string
	graphMaxIters   int
	graphTermWidth  int
	graphTermHeight int
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Render a force-directed graph of installed tools",
	Long: `Visualise the relationships between every tool klim has detected
on PATH. Nodes are tools; edges connect tools that share a property
(default: category; alternatives: tag, pm).

By default the command prints a static snapshot to stdout, ready to
paste into a README. Use --tui for an animated, fullscreen version.

Examples:
  klim graph                       # static snapshot, default --by category
  klim graph --tui                 # animated 10fps fullscreen
  klim graph --by tag              # connect tools that share any tag
  klim graph --by pm               # connect tools that share an installed PM`,
	Args: cobra.NoArgs,
	RunE: runGraph,
}

func init() {
	graphCmd.Flags().BoolVar(&graphTUI, "tui", false, "open the animated fullscreen TUI viewer")
	graphCmd.Flags().StringVar(&graphBy, "by", "category", "edge meaning: category|tag|pm")
	graphCmd.Flags().IntVar(&graphMaxIters, "iters", 200, "max layout iterations for the static snapshot")
	graphCmd.Flags().IntVar(&graphTermWidth, "width", 0, "render width (0 = autodetect; static snapshot only — ignored / rejected with --tui)")
	graphCmd.Flags().IntVar(&graphTermHeight, "height", 0, "render height (0 = autodetect; static snapshot only — ignored / rejected with --tui)")
	// Registered in root.go.
}

// validGraphByValues is the closed set the --by flag accepts.
// PR-78 review: invalid values used to fall through the switch and
// render a graph with nodes but no edges. We now reject unknown
// values with a UsageError so the help text doesn't lie.
var validGraphByValues = []string{"category", "tag", "pm"}

func runGraph(cmd *cobra.Command, _ []string) error {
	by := strings.ToLower(strings.TrimSpace(graphBy))
	if by == "" {
		by = "category"
	}
	valid := false
	for _, v := range validGraphByValues {
		if v == by {
			valid = true
			break
		}
	}
	if !valid {
		return usageErrorf("--by %q is not supported (valid: %s)", graphBy, strings.Join(validGraphByValues, ", "))
	}
	// PR-78 review: --width/--height only configure the static
	// snapshot renderer; the TUI sizes itself from WindowSizeMsg.
	// Silently dropping them surprises users, so reject the combo.
	if graphTUI && (graphTermWidth > 0 || graphTermHeight > 0) {
		return usageErrorf("--width/--height are static-snapshot only; the --tui viewer sizes itself from the terminal")
	}
	svc := svcFrom(cmd)
	// PR-78 review: graph only needs installed/not + instance
	// sources; full version resolution is wasted work (cold cache
	// can fan out package-manager subprocesses for every catalog
	// tool). ScanOnly gives us exactly what buildToolGraph reads.
	tools, _, err := svc.ScanOnly(cmd.Context())
	if err != nil {
		return fmt.Errorf("klim graph: %w", err)
	}
	installed := installedOnly(tools)
	if len(installed) == 0 {
		return errors.New("klim graph: no installed tools to draw")
	}

	g := buildToolGraph(installed, by)

	if graphTUI {
		return runGraphTUI(g)
	}

	g.Layout(graphMaxIters, 1e-4)
	w, h := resolveGraphDimensions(graphTermWidth, graphTermHeight)

	unstyled := !isStdoutTerminal()
	// PR-78 review: Render already terminates each row with '\n',
	// so fmt.Println would add a trailing blank line.
	fmt.Print(g.Render(w, h, graphviz.RenderOpts{Unstyled: unstyled}))
	return nil
}

// isStdoutTerminal reports whether stdout is attached to a real
// terminal. Pure-go via golang.org/x/term so it works on every
// platform klim ships.
func isStdoutTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd())) //nolint:gosec // fd values fit comfortably in int
}

// installedOnly filters the tool list down to tools the user has
// installed locally. The graph is most interesting when it reflects
// the user's actual environment.
func installedOnly(tools []registry.Tool) []registry.Tool {
	out := make([]registry.Tool, 0, len(tools))
	for _, t := range tools {
		if t.IsInstalled() {
			out = append(out, t)
		}
	}
	return out
}

// buildToolGraph constructs a graph where each installed tool is a
// node and edges are drawn between tools that share a property.
func buildToolGraph(tools []registry.Tool, by string) *graphviz.Graph {
	g := graphviz.New()

	// Sort tools so node colour assignment is stable across runs.
	sort.SliceStable(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })

	// Map category -> colour index for stable colouring.
	colorByCategory := map[string]int{}
	var cats []string
	for _, t := range tools {
		if t.Category != "" {
			if _, ok := colorByCategory[t.Category]; !ok {
				colorByCategory[t.Category] = len(cats) % 8
				cats = append(cats, t.Category)
			}
		}
	}

	for _, t := range tools {
		color := colorByCategory[t.Category]
		// Keep the label short — terminals are narrow.
		label := t.Name
		if len(label) > 10 {
			label = label[:10]
		}
		g.AddNode(t.Name, label, color)
	}

	// Build edges per the --by mode. Each pair of nodes that shares a
	// property gets exactly one edge.
	key := strings.ToLower(strings.TrimSpace(by))
	switch key {
	case "category", "":
		groupByEdges(g, tools, func(t registry.Tool) []string {
			if t.Category == "" {
				return nil
			}
			return []string{t.Category}
		})
	case "tag":
		groupByEdges(g, tools, func(t registry.Tool) []string { return append([]string(nil), t.Tags...) })
	case "pm":
		groupByEdges(g, tools, func(t registry.Tool) []string {
			seen := make(map[string]bool)
			var out []string
			for _, inst := range t.Instances {
				s := string(inst.Source)
				if s == "" || seen[s] {
					continue
				}
				seen[s] = true
				out = append(out, s)
			}
			return out
		})
	}
	return g
}

// groupByEdges adds one edge between every pair of tools that share
// at least one bucket. Buckets come from getBuckets(tool); pairs
// already linked stay linked (we don't add duplicates).
func groupByEdges(g *graphviz.Graph, tools []registry.Tool, getBuckets func(registry.Tool) []string) {
	buckets := map[string][]string{}
	for _, t := range tools {
		for _, b := range getBuckets(t) {
			buckets[b] = append(buckets[b], t.Name)
		}
	}
	// Iterate buckets in a deterministic order so the seeded force
	// simulation produces the same layout snapshot across runs for
	// the same installed tools (map iteration order is random).
	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	seen := map[string]bool{}
	for _, k := range keys {
		members := buckets[k]
		for i := 0; i < len(members); i++ {
			for j := i + 1; j < len(members); j++ {
				a, b := members[i], members[j]
				if a > b {
					a, b = b, a
				}
				key := a + "|" + b
				if seen[key] {
					continue
				}
				seen[key] = true
				g.AddEdge(a, b)
			}
		}
	}
}

// resolveGraphDimensions returns (width, height) for the renderer.
// Zero values try to autodetect from the controlling terminal; if
// detection fails (CI / pipe / non-TTY) we fall back to 80×24 — the
// same defaults the help text advertised.
func resolveGraphDimensions(w, h int) (int, int) {
	if w <= 0 || h <= 0 {
		//nolint:gosec // fd values fit comfortably in int
		if tw, th, err := term.GetSize(int(os.Stdout.Fd())); err == nil && tw > 0 && th > 0 {
			if w <= 0 {
				w = tw
			}
			if h <= 0 {
				h = th - 1 // leave one row for the shell prompt
			}
		}
	}
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	return w, h
}

// ----- TUI mode -----

type graphTickMsg time.Time

type graphModel struct {
	g     *graphviz.Graph
	w, h  int
	frame int
}

// Init starts the periodic redraw tick.
func (m graphModel) Init() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return graphTickMsg(t) })
}

// Update advances the simulation one step on each tick and handles
// window resize / quit keys.
func (m graphModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case graphTickMsg:
		m.g.Step()
		m.frame++
		return m, tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return graphTickMsg(t) })
	case tea.WindowSizeMsg:
		m.w = msg.Width
		// Reserve one row for the footer; tea's WindowSizeMsg gives
		// us total area.
		m.h = msg.Height - 2
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

// View renders the current frame plus a one-line footer.
func (m graphModel) View() tea.View {
	w := m.w
	h := m.h
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 22
	}
	body := m.g.Render(w, h)
	footer := fmt.Sprintf("klim graph · %d nodes · frame %d · q to quit", len(m.g.Nodes), m.frame)
	v := tea.NewView(body + "\n" + footer)
	v.AltScreen = true
	return v
}

func runGraphTUI(g *graphviz.Graph) error {
	model := graphModel{g: g}
	p := tea.NewProgram(model)
	_, err := p.Run()
	return err
}
