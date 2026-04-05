package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/nassiharel/clim/internal/registry"
)

// Model is the Bubbletea model for the interactive TUI.
type Model struct {
	tools   []registry.Tool
	cursor  int
	spinner spinner.Model

	// Filter.
	filterInput   textinput.Model
	filtering     bool
	filterText    string
	filteredIndex []int

	// Loading state.
	phase   int // 0=scanning, 1=resolving, 2=done
	loading bool

	// Layout.
	width  int
	height int

	// Quitting.
	quitting bool
}

// NewModel creates a new TUI model.
func NewModel() Model {
	s := spinner.New()
	s.Spinner = spinner.Dot

	ti := textinput.New()
	ti.Placeholder = "filter..."
	ti.CharLimit = 30

	return Model{
		spinner:     s,
		filterInput: ti,
		loading:     true,
		phase:       0,
		width:       80,
		height:      24,
	}
}

// Init fires off the tool finding command (Phase 1).
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg { return findToolsCmd()() },
	)
}

// Update handles all messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		return m.handleKey(msg)

	case scanResultMsg:
		// Phase 1 complete: tools found, now resolve versions.
		m.tools = msg.tools
		m.phase = 1
		m.applyFilter()
		// Kick off Phase 2: version resolution.
		return m, func() tea.Msg { return resolveVersionsCmd(m.tools)() }

	case versionResultMsg:
		// Phase 2 complete: all versions resolved.
		m.phase = 2
		m.loading = false
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	default:
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
	}

	return m, nil
}

// View renders the full TUI.
func (m Model) View() tea.View {
	v := tea.NewView(m.renderView())
	v.AltScreen = true
	return v
}

// Run starts the interactive TUI.
func Run() error {
	model := NewModel()
	p := tea.NewProgram(model)

	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	return nil
}

// --- Private helpers ---

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		m.loading = true
		m.phase = 0
		m.tools = nil
		m.filteredIndex = nil
		m.cursor = 0
		return m, tea.Batch(
			m.spinner.Tick,
			func() tea.Msg { return findToolsCmd()() },
		)
	}

	return m, nil
}

func (m *Model) applyFilter() {
	m.filteredIndex = nil
	filter := strings.ToLower(m.filterText)

	for i, tool := range m.tools {
		if !tool.IsInstalled() {
			continue
		}
		if filter == "" ||
			strings.Contains(strings.ToLower(tool.DisplayName), filter) ||
			strings.Contains(strings.ToLower(tool.Name), filter) {
			m.filteredIndex = append(m.filteredIndex, i)
		}
	}
	if m.cursor >= len(m.filteredIndex) {
		m.cursor = max(0, len(m.filteredIndex)-1)
	}
}
