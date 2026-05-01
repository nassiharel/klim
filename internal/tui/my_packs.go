package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/custompacks"
	"github.com/nassiharel/clim/internal/fileutil"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/share"
)

// My Packs detail action indices.
const (
	myPackActionExport  = 0
	myPackActionShare   = 1
	myPackActionInstall = 2
	myPackActionDelete  = 3
	myPackActionCount   = 4
)

// myPackActionMsg is sent when a My Packs action completes.
type myPackActionMsg struct {
	action string
	result string
	token  string // non-empty for share action
	err    error
}

// myPackDeletedMsg is sent after a pack is deleted from storage.
type myPackDeletedMsg struct {
	name string
	err  error
}

// sanitizeFilename strips path separators and unsafe chars from a name
// so it's safe to use as a filename in the current directory.
func sanitizeFilename(name string) string {
	name = filepath.Base(name) // strip directory components
	// Remove characters unsafe for filenames.
	replacer := strings.NewReplacer(
		"/", "-", "\\", "-", ":", "-", "*", "-",
		"?", "-", "\"", "-", "<", "-", ">", "-", "|", "-",
	)
	name = replacer.Replace(name)
	name = strings.TrimSpace(name)
	if name == "." || name == ".." {
		return ""
	}
	return name
}

// --- Key handling ---

func (m Model) handleKeyMyPacks(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "backspace":
		m.viewingMyPacks = false
		m.statusMsg = ""
		return m, nil
	case "up", "k":
		if m.myPacksCursor > 0 {
			m.myPacksCursor--
		} else if len(m.customPacks) > 0 {
			m.myPacksCursor = len(m.customPacks) - 1
		}
	case "down", "j":
		if m.myPacksCursor < len(m.customPacks)-1 {
			m.myPacksCursor++
		} else {
			m.myPacksCursor = 0
		}
	case "enter":
		if m.myPacksCursor < len(m.customPacks) {
			m.viewingMyPackDetail = true
			m.myPackDetailIdx = m.myPacksCursor
			m.myPackMenuCursor = 0
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleKeyMyPackDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "backspace":
		m.viewingMyPackDetail = false
		m.myPackToken = ""
		m.statusMsg = ""
		return m, nil
	case "c":
		if m.myPackToken != "" {
			if err := m.clip.WriteAll(m.myPackToken); err != nil {
				m.statusMsg = "Clipboard unavailable"
			} else {
				m.statusMsg = "✓ Copied to clipboard!"
			}
		}
		return m, nil
	case "up", "k":
		if m.myPackMenuCursor > 0 {
			m.myPackMenuCursor--
		} else {
			m.myPackMenuCursor = myPackActionCount - 1
		}
	case "down", "j":
		if m.myPackMenuCursor < myPackActionCount-1 {
			m.myPackMenuCursor++
		} else {
			m.myPackMenuCursor = 0
		}
	case "enter":
		if m.myPackDetailIdx >= len(m.customPacks) {
			return m, nil
		}
		pack := m.customPacks[m.myPackDetailIdx]
		switch m.myPackMenuCursor {
		case myPackActionExport:
			return m, exportMyPackFileCmd(pack)
		case myPackActionShare:
			return m, shareMyPackCmd(pack)
		case myPackActionInstall:
			// Reuse existing pack install flow.
			if len(m.tools) == 0 {
				m.statusMsg = "No tools loaded yet."
				return m, nil
			}
			rp := registry.Pack{
				Name:        pack.Name,
				DisplayName: pack.DisplayName,
				Description: pack.Description,
				ToolNames:   pack.ToolNames,
			}
			m.packItems = buildPackInstallItems(m.tools, rp)
			m.packDone = countPackSkipped(m.packItems)
			m.packInstalling = true
			m.showPackDetail = false
			m.viewingMyPackDetail = false
			m.viewingMyPacks = false
			if cmd := m.nextPackItem(); cmd != nil {
				return m, cmd
			}
			m.packInstalling = false
			m.statusMsg = "Nothing to install — all tools already present."
			return m, nil
		case myPackActionDelete:
			return m, deleteMyPackCmd(pack.Name)
		}
		return m, nil
	}
	return m, nil
}

// --- Commands ---

func exportMyPackFileCmd(pack registry.Pack) tea.Cmd {
	return func() tea.Msg {
		p := packYAML{
			Name:        pack.Name,
			DisplayName: pack.DisplayName,
			Description: pack.Description,
			Tools:       pack.ToolNames,
		}
		data, err := yaml.Marshal(&p)
		if err != nil {
			return myPackActionMsg{action: "export", err: err}
		}
		// Sanitize pack name for safe filename.
		safeName := sanitizeFilename(pack.Name)
		if safeName == "" {
			safeName = "custom-pack"
		}
		filename := safeName + ".yaml"
		for i := 1; ; i++ {
			_, err := os.Stat(filename)
			if os.IsNotExist(err) {
				break
			}
			if err != nil {
				// Permission or other I/O error — don't loop forever.
				return myPackActionMsg{action: "export", err: fmt.Errorf("checking filename: %w", err)}
			}
			filename = fmt.Sprintf("%s-%d.yaml", safeName, i)
		}
		header := fmt.Sprintf("# clim — Custom Pack: %s\n# %s\n\n", pack.DisplayName, pack.Description)
		if err := fileutil.AtomicWrite(filename, []byte(header+string(data)), 0o644); err != nil {
			return myPackActionMsg{action: "export", err: err}
		}
		abs, _ := filepath.Abs(filename)
		return myPackActionMsg{action: "export", result: "Exported to " + abs}
	}
}

func shareMyPackCmd(pack registry.Pack) tea.Cmd {
	return func() tea.Msg {
		token, err := share.Encode(pack.ToolNames)
		if err != nil {
			return myPackActionMsg{action: "share", err: err}
		}
		return myPackActionMsg{
			action: "share",
			result: fmt.Sprintf("Token generated (%d tools)", len(pack.ToolNames)),
			token:  token,
		}
	}
}

func deleteMyPackCmd(name string) tea.Cmd {
	return func() tea.Msg {
		err := custompacks.Delete(name)
		return myPackDeletedMsg{name: name, err: err}
	}
}

// --- Rendering ---

func (m Model) renderMyPacksView() string {
	var b strings.Builder

	if m.viewingMyPackDetail && m.myPackDetailIdx < len(m.customPacks) {
		return m.renderMyPackDetailView()
	}

	b.WriteString("\n  " + detailTitleStyle.Render("My Packs") + "\n\n")

	if len(m.customPacks) == 0 {
		b.WriteString("  " + dimVersion.Render("No custom packs yet. Use Create Pack to make one.") + "\n")
		b.WriteString("\n  " + dimVersion.Render("Esc") + " back\n")
		return b.String()
	}

	// Header.
	b.WriteString("  " +
		headerStyle.Render(fixedWidth("PACK", colName)) + "  " +
		headerStyle.Render(fixedWidth("TOOLS", colPackTools)) + "  " +
		headerStyle.Render("DESCRIPTION") + "\n")

	visibleRows := m.height - 9 - m.footerHeight()
	if visibleRows < 3 {
		visibleRows = 3
	}
	start := 0
	if m.myPacksCursor >= visibleRows {
		start = m.myPacksCursor - visibleRows + 1
	}

	for i := start; i < len(m.customPacks) && i < start+visibleRows; i++ {
		pack := m.customPacks[i]
		cursor := "  "
		if i == m.myPacksCursor {
			cursor = "▸ "
		}

		toolCount := strconv.Itoa(len(pack.ToolNames))
		desc := pack.Description
		maxDesc := m.width - colName - colPackTools - 12
		if maxDesc > 0 && len(desc) > maxDesc {
			desc = desc[:maxDesc-1] + "…"
		}

		line := cursor +
			nameStyle.Render(fixedWidth(pack.DisplayName, colName)) + "  " +
			dimVersion.Render(fixedWidth(toolCount, colPackTools)) + "  " +
			dimVersion.Render(desc)

		if i == m.myPacksCursor {
			w := lipgloss.Width(line)
			if w < m.width {
				line += strings.Repeat(" ", m.width-w)
			}
			line = selectedRowStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	// Pad.
	rendered := min(len(m.customPacks)-start, visibleRows)
	for range max(visibleRows-rendered, 0) {
		b.WriteString("\n")
	}

	b.WriteString("\n  " + dimVersion.Render("Enter") + " view   " + dimVersion.Render("Esc") + " back\n")
	return b.String()
}

func (m Model) renderMyPackDetailView() string {
	pack := m.customPacks[m.myPackDetailIdx]

	var b strings.Builder
	b.WriteString("\n  " + detailTitleStyle.Render(pack.DisplayName) + "\n")
	if pack.Description != "" {
		b.WriteString("  " + dimVersion.Render(pack.Description) + "\n")
	}
	b.WriteString("  " + dimVersion.Render(fmt.Sprintf("%d tools: %s", len(pack.ToolNames), strings.Join(pack.ToolNames, ", "))) + "\n")

	b.WriteString("\n")

	actions := []struct {
		label string
		desc  string
	}{
		{"Export File", "Save as YAML file"},
		{"Share Token", "Generate share token"},
		{"Install All", "Install all tools in this pack"},
		{"Delete", "Remove this pack"},
	}

	for i, action := range actions {
		cursor := "  "
		if i == m.myPackMenuCursor {
			cursor = "▸ "
		}
		line := cursor + nameStyle.Render(fixedWidth(action.label, 16)) + "  " + dimVersion.Render(action.desc)
		if i == m.myPackMenuCursor {
			w := lipgloss.Width(line)
			if w < m.width {
				line += strings.Repeat(" ", m.width-w)
			}
			line = selectedRowStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	b.WriteString("\n  " + dimVersion.Render("Enter") + " select   " + dimVersion.Render("Esc") + " back")
	if m.myPackToken != "" {
		b.WriteString("   " + dimVersion.Render("c") + " copy token")
	}
	b.WriteString("\n")

	// Show token if generated.
	if m.myPackToken != "" {
		b.WriteString("\n  " + detailLabelStyle.Render("Share Token:") + "\n\n")
		maxW := m.width - 6
		if maxW < 40 {
			maxW = 40
		}
		token := m.myPackToken
		for len(token) > maxW {
			b.WriteString("    " + token[:maxW] + "\n")
			token = token[maxW:]
		}
		if token != "" {
			b.WriteString("    " + token + "\n")
		}
		b.WriteString("\n  " + dimVersion.Render("Recipients install with:") + " clim open <token>\n")
	}

	return b.String()
}
