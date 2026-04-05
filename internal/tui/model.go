package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/latest"
)

// ToolRow represents a single row in the TUI table.
type ToolRow struct {
	Name          string
	DisplayName   string
	Path          string
	Version       string // "" = still loading, "—" = not detectable
	LatestVersion string // "" = no source or still loading
	VersionDone   bool
	LatestDone    bool
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
	filteredIndex []int

	// Loading state.
	scanning bool // Phase 1: PATH scan in progress
	pending  int  // Phase 2: count of pending version/latest ops

	// Layout.
	width  int
	height int

	// Dependencies.
	cfg   config.Config
	cache *latest.Cache

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
		scanning:    true,
		cfg:         cfg,
		cache:       latest.DefaultCache(),
		width:       80,
		height:      24,
	}
}

// Init fires off the PATH scan (Phase 1).
func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg { return scanPATHCmd(m.cfg)() },
	)
}

// Update handles all messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		return m.handleKey(msg)

	case scanResultMsg:
		// Phase 1 complete: populate tool rows and kick off Phase 2.
		m.scanning = false
		if msg.err != nil {
			return m, nil
		}

		m.tools = make([]ToolRow, len(msg.tools))
		var cmds []tea.Cmd

		for i, t := range msg.tools {
			m.tools[i] = ToolRow{
				Name:        t.Name,
				DisplayName: t.DisplayName,
				Path:        t.Path,
			}
			// Fire version detection for each tool.
			m.pending++
			idx := i
			path := t.Path
			cmds = append(cmds, func() tea.Msg { return detectVersionCmd(idx, path)() })

			// Fire latest-version check for known tools.
			name := t.Name
			m.pending++
			cmds = append(cmds, func() tea.Msg { return checkLatestCmd(idx, name, m.cache)() })
		}

		m.applyFilter()
		return m, tea.Batch(cmds...)

	case versionResultMsg:
		if msg.index < len(m.tools) {
			row := &m.tools[msg.index]
			row.VersionDone = true
			if msg.version != "" {
				row.Version = msg.version
			} else {
				row.Version = "—"
			}
		}
		m.pending--
		if m.pending <= 0 {
			m.cache.Save()
		}
		return m, nil

	case latestResultMsg:
		if msg.index < len(m.tools) {
			row := &m.tools[msg.index]
			row.LatestDone = true
			row.LatestVersion = msg.version
		}
		m.pending--
		if m.pending <= 0 {
			m.cache.Save()
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	default:
		if m.scanning || m.pending > 0 {
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
		m.scanning = true
		m.tools = nil
		m.filteredIndex = nil
		m.cursor = 0
		m.pending = 0
		return m, tea.Batch(
			m.spinner.Tick,
			func() tea.Msg { return scanPATHCmd(m.cfg)() },
		)
	}

	return m, nil
}

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
		if strings.Contains(strings.ToLower(row.DisplayName), filter) ||
			strings.Contains(strings.ToLower(row.Name), filter) {
			m.filteredIndex = append(m.filteredIndex, i)
		}
	}
	if m.cursor >= len(m.filteredIndex) {
		m.cursor = max(0, len(m.filteredIndex)-1)
	}
}
