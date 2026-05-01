package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nassiharel/clim/internal/custompacks"
	"github.com/nassiharel/clim/internal/registry"
)

// Pack creation phases.
const (
	packCreatePhaseName     = 0 // enter pack name
	packCreatePhaseDispName = 1 // enter display name
	packCreatePhaseDesc     = 2 // enter description
	packCreatePhaseTools    = 3 // select tools
)

// packSavedMsg is sent after a pack is saved to custom-packs.yaml.
type packSavedMsg struct {
	name string
	err  error
}

// startPackCreate initialises the pack creation wizard state.
func (m *Model) startPackCreate() {
	m.creatingPack = true
	m.packCreatePhase = packCreatePhaseName
	m.packCreateName.SetValue("")
	m.packCreateDispName.SetValue("")
	m.packCreateDesc.SetValue("")
	m.packCreateSelected = make(map[int]bool)
	m.packCreateCursor = 0
	m.packCreateFilter = ""
	m.rebuildPackCreateFiltered()
	m.statusMsg = ""
}

// resetPackCreate clears pack creation state and returns to backup idle.
func (m *Model) resetPackCreate() {
	m.creatingPack = false
	m.packCreatePhase = 0
	m.packCreateSelected = make(map[int]bool)
	m.packCreateFiltered = nil
	m.packCreateFilter = ""
	m.statusMsg = ""
}

// rebuildPackCreateFiltered rebuilds the filtered tool index list for tool selection.
func (m *Model) rebuildPackCreateFiltered() {
	m.packCreateFiltered = nil
	filter := strings.ToLower(m.packCreateFilter)
	for i, tool := range m.tools {
		if filter != "" &&
			!strings.Contains(strings.ToLower(tool.Name), filter) &&
			!strings.Contains(strings.ToLower(tool.DisplayName), filter) &&
			!strings.Contains(strings.ToLower(tool.Category), filter) {
			continue
		}
		m.packCreateFiltered = append(m.packCreateFiltered, i)
	}
	if m.packCreateCursor >= len(m.packCreateFiltered) {
		m.packCreateCursor = max(0, len(m.packCreateFiltered)-1)
	}
}

// selectedPackToolNames returns sorted tool names from the selection.
func (m *Model) selectedPackToolNames() []string {
	var names []string
	for idx := range m.packCreateSelected {
		if m.packCreateSelected[idx] && idx < len(m.tools) {
			names = append(names, m.tools[idx].Name)
		}
	}
	sort.Strings(names)
	return names
}

// handleKeyPackCreate handles keys during the pack creation wizard.
func (m Model) handleKeyPackCreate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.packCreatePhase {
	case packCreatePhaseName:
		return m.handleKeyPackCreateName(msg)
	case packCreatePhaseDispName:
		return m.handleKeyPackCreateDispName(msg)
	case packCreatePhaseDesc:
		return m.handleKeyPackCreateDescInput(msg)
	case packCreatePhaseTools:
		return m.handleKeyPackCreateTools(msg)
	}
	return m, nil
}

func (m Model) handleKeyPackCreateName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.resetPackCreate()
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.packCreateName.Value())
		if name == "" {
			m.statusMsg = "Pack name cannot be empty."
			return m, nil
		}
		m.packCreatePhase = packCreatePhaseDispName
		m.statusMsg = ""
		return m, m.packCreateDispName.Focus()
	default:
		var cmd tea.Cmd
		m.packCreateName, cmd = m.packCreateName.Update(msg)
		return m, cmd
	}
}

func (m Model) handleKeyPackCreateDispName(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.resetPackCreate()
		return m, nil
	case "enter":
		m.packCreatePhase = packCreatePhaseDesc
		m.statusMsg = ""
		return m, m.packCreateDesc.Focus()
	default:
		var cmd tea.Cmd
		m.packCreateDispName, cmd = m.packCreateDispName.Update(msg)
		return m, cmd
	}
}

func (m Model) handleKeyPackCreateDescInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.resetPackCreate()
		return m, nil
	case "enter":
		m.packCreatePhase = packCreatePhaseTools
		m.packCreateCursor = 0
		m.packCreateFilter = ""
		m.rebuildPackCreateFiltered()
		m.statusMsg = ""
		return m, nil
	default:
		var cmd tea.Cmd
		m.packCreateDesc, cmd = m.packCreateDesc.Update(msg)
		return m, cmd
	}
}

func (m Model) handleKeyPackCreateTools(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// If filtering, clear filter. Otherwise exit wizard.
		if m.packCreateFilter != "" {
			m.packCreateFilter = ""
			m.packCreateCursor = 0
			m.rebuildPackCreateFiltered()
			return m, nil
		}
		m.resetPackCreate()
		return m, nil
	case "up", "k":
		if m.packCreateCursor > 0 {
			m.packCreateCursor--
		}
	case "down", "j":
		if m.packCreateCursor < len(m.packCreateFiltered)-1 {
			m.packCreateCursor++
		}
	case "home", "g":
		m.packCreateCursor = 0
	case "end", "G":
		m.packCreateCursor = max(0, len(m.packCreateFiltered)-1)
	case "space":
		if m.packCreateCursor < len(m.packCreateFiltered) {
			idx := m.packCreateFiltered[m.packCreateCursor]
			m.packCreateSelected[idx] = !m.packCreateSelected[idx]
			if !m.packCreateSelected[idx] {
				delete(m.packCreateSelected, idx)
			}
			if m.packCreateCursor < len(m.packCreateFiltered)-1 {
				m.packCreateCursor++
			}
		}
	case "a":
		// Toggle select all.
		anySelected := false
		for _, idx := range m.packCreateFiltered {
			if m.packCreateSelected[idx] {
				anySelected = true
				break
			}
		}
		for _, idx := range m.packCreateFiltered {
			if anySelected {
				delete(m.packCreateSelected, idx)
			} else {
				m.packCreateSelected[idx] = true
			}
		}
	case "/":
		// Start typing filter inline — next chars update filter.
		// For simplicity, we accumulate in packCreateFilter.
		// Just clear and let default case handle chars.
		return m, nil
	case "backspace":
		if len(m.packCreateFilter) > 0 {
			m.packCreateFilter = m.packCreateFilter[:len(m.packCreateFilter)-1]
			m.packCreateCursor = 0
			m.rebuildPackCreateFiltered()
		}
	case "enter":
		names := m.selectedPackToolNames()
		if len(names) == 0 {
			m.statusMsg = "Select at least one tool."
			return m, nil
		}
		// Save pack and navigate to My Packs.
		packName := strings.TrimSpace(m.packCreateName.Value())
		dispName := strings.TrimSpace(m.packCreateDispName.Value())
		if dispName == "" {
			dispName = packName
		}
		desc := strings.TrimSpace(m.packCreateDesc.Value())
		return m, saveCustomPackCmd(registry.Pack{
			Name:        packName,
			DisplayName: dispName,
			Description: desc,
			ToolNames:   names,
		})
	default:
		// Single printable char → append to filter.
		key := msg.String()
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			m.packCreateFilter += key
			m.packCreateCursor = 0
			m.rebuildPackCreateFiltered()
		}
	}
	return m, nil
}

// --- Pack creation commands ---

// packYAML is the serialization struct for custom pack YAML output.
type packYAML struct {
	Name        string   `yaml:"name"`
	DisplayName string   `yaml:"display_name"`
	Description string   `yaml:"description"`
	Tools       []string `yaml:"tools"`
}

func saveCustomPackCmd(pack registry.Pack) tea.Cmd {
	return func() tea.Msg {
		err := custompacks.Add(pack)
		return packSavedMsg{name: pack.Name, err: err}
	}
}

// --- Pack creation rendering ---

func (m Model) renderPackCreateView() string {
	var b strings.Builder

	b.WriteString("\n")
	b.WriteString("  " + detailTitleStyle.Render("Create Your Own Pack") + "\n")

	switch m.packCreatePhase {
	case packCreatePhaseName:
		b.WriteString("\n  " + detailLabelStyle.Render("Pack name (slug):") + "\n")
		b.WriteString("  " + m.packCreateName.View() + "\n")
		b.WriteString("\n  " + dimVersion.Render("Enter") + " next   " + dimVersion.Render("Esc") + " cancel\n")

	case packCreatePhaseDispName:
		b.WriteString("\n  " + detailLabelStyle.Render("Display name:") + " " + dimVersion.Render("(leave empty to use '"+m.packCreateName.Value()+"')") + "\n")
		b.WriteString("  " + m.packCreateDispName.View() + "\n")
		b.WriteString("\n  " + dimVersion.Render("Enter") + " next   " + dimVersion.Render("Esc") + " cancel\n")

	case packCreatePhaseDesc:
		b.WriteString("\n  " + detailLabelStyle.Render("Description:") + "\n")
		b.WriteString("  " + m.packCreateDesc.View() + "\n")
		b.WriteString("\n  " + dimVersion.Render("Enter") + " next   " + dimVersion.Render("Esc") + " cancel\n")

	case packCreatePhaseTools:
		selected := 0
		for _, v := range m.packCreateSelected {
			if v {
				selected++
			}
		}
		fmt.Fprintf(&b, "\n  Select tools (%d selected, %d total)\n", selected, len(m.tools))

		if m.packCreateFilter != "" {
			b.WriteString("  " + filterPromptStyle.Render("filter: ") + m.packCreateFilter + "\n")
		}

		b.WriteString("\n")

		// Scrollable tool list.
		visibleRows := m.height - 10 - m.footerHeight()
		if visibleRows < 5 {
			visibleRows = 5
		}

		start := 0
		if m.packCreateCursor >= visibleRows {
			start = m.packCreateCursor - visibleRows + 1
		}
		end := start + visibleRows
		if end > len(m.packCreateFiltered) {
			end = len(m.packCreateFiltered)
		}

		for i := start; i < end; i++ {
			idx := m.packCreateFiltered[i]
			tool := m.tools[idx]

			cursor := "  "
			if i == m.packCreateCursor {
				cursor = "▸ "
			}

			checkbox := "[ ] "
			if m.packCreateSelected[idx] {
				checkbox = "[✓] "
			}

			installed := ""
			if tool.IsInstalled() {
				installed = " " + upToDateStyle.Render("●")
			}

			line := cursor + checkbox + nameStyle.Render(fixedWidth(tool.DisplayName, colName)) +
				"  " + dimVersion.Render(fixedWidth(tool.Category, colCategory)) + installed

			if i == m.packCreateCursor {
				w := lipgloss.Width(line)
				if w < m.width {
					line += strings.Repeat(" ", m.width-w)
				}
				line = selectedRowStyle.Render(line)
			}
			b.WriteString(line + "\n")
		}

		// Pad remaining.
		rendered := end - start
		for range max(visibleRows-rendered, 0) {
			b.WriteString("\n")
		}

		b.WriteString("\n  " + dimVersion.Render("Space") + " toggle  " +
			dimVersion.Render("a") + " all  " +
			dimVersion.Render("Enter") + " save & continue  " +
			dimVersion.Render("Esc") + " cancel  " +
			dimVersion.Render("type to filter") + "\n")
	}

	return b.String()
}
