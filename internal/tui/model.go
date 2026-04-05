package tui

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/registry"
)

// ToolRow represents a single row in the TUI table.
type ToolRow struct {
	Name    string
	Path    string
	Version string // empty while loading, "(unknown)" if detection failed
}

// Model is the Bubbletea model for the interactive TUI.
type Model struct {
	tools   []ToolRow
	cursor  int
	spinner spinner.Model

	// Filter.
	filterInput   textinput.Model
	filtering     bool
	filterText    string
	filteredIndex []int // maps visible row -> index in tools

	// Loading state.
	loading bool

	// Layout.
	width  int
	height int

	// Config.
	cfg config.Config

	// Quitting.
	quitting bool
}

// NewModel creates a new TUI model.
func NewModel(cfg config.Config) Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	ti := textinput.New()
	ti.Placeholder = "filter..."
	ti.CharLimit = 30

	return Model{
		spinner:     s,
		filterInput: ti,
		loading:     true,
		cfg:         cfg,
		width:       80,
		height:      24,
	}
}

// Init fires off the PATH scan and version detection command.
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		scanAndDetectCmd(m.cfg),
	)
}

// Update handles all messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		return m.handleKey(msg)

	case scanResultMsg:
		m.loading = false
		if msg.err != nil {
			// Show error in the title area — tools stay empty.
			return m, nil
		}
		m.tools = make([]ToolRow, len(msg.tools))
		for i, t := range msg.tools {
			ver := t.Version
			if ver == "" {
				ver = "(unknown)"
			}
			m.tools[i] = ToolRow{
				Name:    t.Name,
				Path:    t.Path,
				Version: ver,
			}
		}
		m.applyFilter()
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	default:
		// Delegate to spinner while loading.
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// View renders the full TUI using the alt screen.
func (m Model) View() tea.View {
	v := tea.NewView(m.renderView())
	v.AltScreen = true
	return v
}

// Run starts the interactive TUI.
func Run(cfg config.Config) error {
	model := NewModel(cfg)
	p := tea.NewProgram(model)

	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	return nil
}

// --- Private helpers ---

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// When filtering, delegate to the text input.
	if m.filtering {
		switch msg.String() {
		case "esc":
			m.filtering = false
			m.filterText = ""
			m.filterInput.SetValue("")
			m.applyFilter()
			return m, nil
		case "enter":
			m.filtering = false
			return m, nil
		default:
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			m.filterText = m.filterInput.Value()
			m.applyFilter()
			return m, cmd
		}
	}

	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		if m.cursor < len(m.filteredIndex)-1 {
			m.cursor++
		}

	case "home", "g":
		m.cursor = 0

	case "end", "G":
		m.cursor = max(0, len(m.filteredIndex)-1)

	case "/":
		m.filtering = true
		return m, m.filterInput.Focus()

	case "r":
		// Refresh — re-scan PATH.
		m.loading = true
		m.tools = nil
		m.filteredIndex = nil
		m.cursor = 0
		return m, tea.Batch(
			m.spinner.Tick,
			scanAndDetectCmd(m.cfg),
		)
	}

	return m, nil
}

// applyFilter updates the filteredIndex based on the current filter text.
func (m *Model) applyFilter() {
	if m.filterText == "" {
		m.filteredIndex = make([]int, len(m.tools))
		for i := range m.filteredIndex {
			m.filteredIndex[i] = i
		}
		return
	}

	filter := strings.ToLower(m.filterText)
	m.filteredIndex = nil
	for i, row := range m.tools {
		if strings.Contains(strings.ToLower(row.Name), filter) {
			m.filteredIndex = append(m.filteredIndex, i)
		}
	}

	// Clamp cursor.
	if m.cursor >= len(m.filteredIndex) {
		m.cursor = max(0, len(m.filteredIndex)-1)
	}
}

// scanAndDetectCmd returns a Bubbletea command that scans PATH and detects versions.
func scanAndDetectCmd(cfg config.Config) tea.Cmd {
	return func() tea.Msg {
		tools, err := scanAndDetect(cfg)
		return scanResultMsg{tools: tools, err: err}
	}
}

// scanAndDetect performs the actual PATH scan and version detection.
func scanAndDetect(cfg config.Config) ([]registry.Tool, error) {
	// Import scanner here to avoid circular imports — use the scanner package.
	// We inline the import at the package level.
	tools, err := scanPATH(cfg)
	if err != nil {
		return nil, err
	}

	timeout := time.Duration(cfg.EffectiveTimeout()) * time.Second
	detectAll(context.Background(), tools, timeout, runtime.NumCPU()*2)

	return tools, nil
}
