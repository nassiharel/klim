package tui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/nassiharel/clim/internal/registry"
)

const (
	tabInstalled = iota
	tabUpdates
	tabDiscover
	tabDisabled
)

// Model is the Bubbletea model for the interactive TUI.
type Model struct {
	tools   []registry.Tool
	cursor  int
	spinner spinner.Model

	// Tabs.
	activeTab int

	// Filter.
	filterInput   textinput.Model
	filtering     bool
	filterText    string
	filteredIndex []int

	// Detail view.
	detailIdx int  // index into m.tools, -1 = no detail
	showDetail bool

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
		activeTab:   tabInstalled,
		detailIdx:   -1,
		width:       80,
		height:      24,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		func() tea.Msg { return findToolsCmd()() },
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKey(msg)

	case scanResultMsg:
		m.tools = msg.tools
		sort.Slice(m.tools, func(i, j int) bool {
			return strings.ToLower(m.tools[i].Name) < strings.ToLower(m.tools[j].Name)
		})
		m.phase = 1
		m.applyFilter()
		return m, func() tea.Msg { return resolveVersionsCmd(m.tools)() }

	case versionResultMsg:
		m.phase = 2
		m.loading = false
		m.applyFilter()
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

// --- Key handling ---

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Detail view — Esc goes back.
	if m.showDetail {
		switch msg.String() {
		case "esc", "q", "backspace":
			m.showDetail = false
			return m, nil
		}
		return m, nil
	}

	// Filter mode.
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
	case "tab":
		m.activeTab = (m.activeTab + 1) % 4
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "shift+tab":
		m.activeTab = (m.activeTab + 3) % 4
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "1":
		m.activeTab = tabInstalled
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "2":
		m.activeTab = tabUpdates
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "3":
		m.activeTab = tabDiscover
		m.cursor = 0
		m.applyFilter()
		return m, nil
	case "4":
		m.activeTab = tabDisabled
		m.cursor = 0
		m.applyFilter()
		return m, nil
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
	case "enter":
		if m.cursor < len(m.filteredIndex) {
			m.detailIdx = m.filteredIndex[m.cursor]
			m.showDetail = true
		}
		return m, nil
	case "x":
		if m.cursor < len(m.filteredIndex) {
			idx := m.filteredIndex[m.cursor]
			tool := &m.tools[idx]
			if m.activeTab == tabDisabled {
				// Re-enable.
				_ = registry.SetToolEnabled(tool.Name, true)
				tool.Disabled = false
			} else {
				// Disable.
				_ = registry.SetToolEnabled(tool.Name, false)
				tool.Disabled = true
			}
			m.applyFilter()
		}
		return m, nil
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

// --- Filtering ---

func (m *Model) applyFilter() {
	m.filteredIndex = nil
	filter := strings.ToLower(m.filterText)

	for i, tool := range m.tools {
		if !m.matchesTab(tool) {
			continue
		}
		if filter != "" &&
			!strings.Contains(strings.ToLower(tool.DisplayName), filter) &&
			!strings.Contains(strings.ToLower(tool.Name), filter) &&
			!strings.Contains(strings.ToLower(tool.Category), filter) {
			continue
		}
		m.filteredIndex = append(m.filteredIndex, i)
	}
	if m.cursor >= len(m.filteredIndex) {
		m.cursor = max(0, len(m.filteredIndex)-1)
	}
}

func (m *Model) matchesTab(tool registry.Tool) bool {
	switch m.activeTab {
	case tabInstalled:
		return !tool.Disabled && tool.IsInstalled()
	case tabUpdates:
		return !tool.Disabled && tool.HasUpdate()
	case tabDiscover:
		return !tool.Disabled && !tool.IsInstalled()
	case tabDisabled:
		return tool.Disabled
	}
	return false
}

// --- Stats ---

func (m Model) stats() (installed, updates, notInstalled, disabled int) {
	for _, tool := range m.tools {
		if tool.Disabled {
			disabled++
			continue
		}
		if tool.IsInstalled() {
			installed++
			if tool.HasUpdate() {
				updates++
			}
		} else {
			notInstalled++
		}
	}
	return
}
