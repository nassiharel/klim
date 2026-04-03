package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/version"
)

// ToolRow represents a single row in the TUI table.
type ToolRow struct {
	Tool         registry.Tool
	InstalledVer string
	LatestVer    string
	Path         string
	Status       version.Status
	DetectDone   bool
	LatestDone   bool
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
	filteredIndex []int // maps visible row → index in tools

	// Loading state.
	loading int // count of pending operations

	// Layout.
	width  int
	height int

	// Dependencies.
	ctx     context.Context
	checker version.Checker
	cache   *version.Cache

	// Quitting.
	quitting bool
}

// NewModel creates a new TUI model.
func NewModel(ctx context.Context, checker version.Checker, cache *version.Cache) Model {
	tools := registry.DefaultTools()
	rows := make([]ToolRow, len(tools))
	for i, t := range tools {
		rows[i] = ToolRow{
			Tool:   t,
			Status: version.StatusLoading,
		}
	}

	s := spinner.New()
	s.Spinner = spinner.Dot

	ti := textinput.New()
	ti.Placeholder = "filter..."
	ti.CharLimit = 30

	indices := make([]int, len(tools))
	for i := range indices {
		indices[i] = i
	}

	return Model{
		tools:         rows,
		spinner:       s,
		filterInput:   ti,
		filteredIndex: indices,
		loading:       len(tools) * 2, // detect + latest for each
		ctx:           ctx,
		checker:       checker,
		cache:         cache,
		width:         80,
		height:        24,
	}
}

// Init fires off all detection and version check commands simultaneously.
func (m Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, m.spinner.Tick)

	for i, row := range m.tools {
		cmds = append(cmds, detectToolCmd(m.ctx, i, row.Tool))
		cmds = append(cmds, checkLatestCmd(m.ctx, i, row.Tool, m.checker, m.cache))
	}

	return tea.Batch(cmds...)
}

// Update handles all messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.KeyMsg:
		return m.handleKey(msg)

	case DetectionCompleteMsg:
		row := &m.tools[msg.Index]
		row.DetectDone = true
		if msg.Result.Found {
			row.Path = msg.Result.Path
			row.InstalledVer = msg.Result.Version
		}
		m.loading--
		m.recalculateStatus(msg.Index)
		return m, nil

	case LatestVersionMsg:
		row := &m.tools[msg.Index]
		row.LatestDone = true
		if msg.Result.Error == nil {
			row.LatestVer = msg.Result.Version
		}
		m.loading--
		m.recalculateStatus(msg.Index)
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	default:
		// Delegate to spinner.
		if m.loading > 0 {
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
func Run() error {
	ctx := context.Background()
	cache := version.LoadCache()
	checker := version.NewHTTPChecker(version.TokenFromEnv())

	model := NewModel(ctx, checker, cache)
	p := tea.NewProgram(model)

	_, err := p.Run()
	if err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	// Save cache on exit.
	cache.Save()
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
		// Refresh all tools.
		m.loading = len(m.tools) * 2
		for i := range m.tools {
			m.tools[i].DetectDone = false
			m.tools[i].LatestDone = false
			m.tools[i].Status = version.StatusLoading
		}
		var cmds []tea.Cmd
		cmds = append(cmds, m.spinner.Tick)
		for i, row := range m.tools {
			cmds = append(cmds, detectToolCmd(m.ctx, i, row.Tool))
			cmds = append(cmds, checkLatestCmd(m.ctx, i, row.Tool, m.checker, m.cache))
		}
		return m, tea.Batch(cmds...)
	}

	return m, nil
}

// recalculateStatus updates a row's status when both detect and latest are done.
func (m *Model) recalculateStatus(idx int) {
	row := &m.tools[idx]

	if !row.DetectDone || !row.LatestDone {
		row.Status = version.StatusLoading
		return
	}

	if row.Path == "" {
		row.Status = version.StatusNotInstalled
		return
	}

	if row.InstalledVer == "" {
		row.Status = version.StatusError
		return
	}

	if row.LatestVer == "" {
		row.Status = version.StatusError
		return
	}

	s, _ := version.CompareVersions(row.InstalledVer, row.LatestVer)
	row.Status = s
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
		name := strings.ToLower(row.Tool.DisplayName)
		short := strings.ToLower(row.Tool.Name)
		if strings.Contains(name, filter) || strings.Contains(short, filter) {
			m.filteredIndex = append(m.filteredIndex, i)
		}
	}

	// Clamp cursor.
	if m.cursor >= len(m.filteredIndex) {
		m.cursor = max(0, len(m.filteredIndex)-1)
	}
}
