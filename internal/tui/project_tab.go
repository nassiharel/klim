package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/teamfile"
)

// Project tab view states.
const (
	projectViewList    = 0 // project list
	projectViewDetail  = 1 // single project detail
	projectViewAddTool = 2 // tool picker for adding to project
)

// Project detail action indices.
const (
	projectActionRecheck        = 0
	projectActionInstallMissing = 1
	projectActionAddRequired    = 2
	projectActionAddOptional    = 3
	projectActionEdit           = 4
	projectActionReinit         = 5
	projectActionCount          = 6
)

// --- Messages ---

type projectCheckMsg struct {
	results []teamfile.CheckResult
	tf      *teamfile.TeamFile
	path    string
	err     error
}

type projectInitMsg struct {
	result *teamfile.DetectResult
	err    error
}

type projectInitDoneMsg struct {
	path  string
	tools int
	err   error
}

type projectAddToolMsg struct {
	toolName string
	optional bool
	err      error
}

type projectListLoadedMsg struct {
	entries []teamfile.ProjectEntry
}

// --- Commands ---

func projectLoadListCmd(tools []registry.Tool) tea.Cmd {
	return func() tea.Msg {
		entries, _ := teamfile.LoadProjects()
		return projectListLoadedMsg{entries: entries}
	}
}

func projectCheckCmd(path string, tools []registry.Tool) tea.Cmd {
	return func() tea.Msg {
		tf, err := teamfile.Parse(path)
		if err != nil {
			return projectCheckMsg{err: err}
		}
		results := teamfile.Check(tf, tools)

		// Auto-register in projects.
		name := tf.Name
		if name == "" {
			name = filepath.Base(filepath.Dir(path))
		}
		_ = teamfile.AddProject(filepath.Dir(path), name, len(tf.Tools)+len(tf.Optional))

		return projectCheckMsg{results: results, tf: tf, path: path}
	}
}

func projectInitDetectCmd(dir string) tea.Cmd {
	return func() tea.Msg {
		result := teamfile.DetectFromProject(dir)
		return projectInitMsg{result: &result}
	}
}

func projectInitWriteCmd(dir string, tools []registry.Tool, detected []teamfile.DetectedTool, all bool) tea.Cmd {
	return func() tea.Msg {
		outPath := filepath.Join(dir, teamfile.FileName)

		var tf *teamfile.TeamFile
		if all {
			tf = teamfile.Generate(tools, false)
		} else {
			installedMap := make(map[string]*registry.Tool, len(tools))
			for i := range tools {
				if tools[i].IsInstalled() {
					installedMap[tools[i].Name] = &tools[i]
				}
			}
			tf = &teamfile.TeamFile{}
			for _, d := range detected {
				if _, ok := installedMap[d.Name]; ok {
					tf.Tools = append(tf.Tools, teamfile.RequiredTool{Name: d.Name})
				}
			}
		}

		if len(tf.Tools) == 0 {
			return projectInitDoneMsg{err: fmt.Errorf("no tools to include")}
		}

		if err := teamfile.Write(tf, outPath); err != nil {
			return projectInitDoneMsg{err: err}
		}

		// Auto-register.
		name := tf.Name
		if name == "" {
			name = filepath.Base(dir)
		}
		_ = teamfile.AddProject(dir, name, len(tf.Tools))

		return projectInitDoneMsg{path: outPath, tools: len(tf.Tools)}
	}
}

func projectAddToolCmd(climeFilePath, toolName string, optional bool) tea.Cmd {
	return func() tea.Msg {
		err := teamfile.AddToolToFile(climeFilePath, toolName, optional)
		return projectAddToolMsg{toolName: toolName, optional: optional, err: err}
	}
}

func projectEditCmd(path string) tea.Cmd {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		for _, e := range []string{"code", "vim", "nano", "notepad"} {
			if _, err := exec.LookPath(e); err == nil {
				editor = e
				break
			}
		}
	}
	if editor == "" {
		return func() tea.Msg {
			return projectCheckMsg{err: fmt.Errorf("no $EDITOR set")}
		}
	}
	cmd := exec.Command(editor, path)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return projectCheckMsg{err: err}
	})
}

// --- Key Handling ---

func (m Model) handleKeyProject(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Lazy-load project list on first interaction.
	if !m.projectsLoaded {
		m.projectsLoaded = true
		return m, projectLoadListCmd(m.tools)
	}

	// Global keys.
	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "right", "tab":
		m.activeTab = (m.activeTab + 1) % tabCount
		m.cursor = 0
		m.dashboardScroll = 0
		m.discoverSubTab = discoverTools
		m.applyFilter()
		return m, nil
	case "left", "shift+tab":
		m.activeTab = (m.activeTab + tabCount - 1) % tabCount
		m.cursor = 0
		m.dashboardScroll = 0
		m.discoverSubTab = discoverTools
		m.applyFilter()
		return m, nil
	}

	// Confirmation for reinit.
	if m.projectConfirmReinit {
		return m.handleKeyProjectConfirmReinit(msg)
	}

	// Delegate to current view.
	switch m.projectView {
	case projectViewList:
		return m.handleKeyProjectList(msg)
	case projectViewDetail:
		return m.handleKeyProjectDetail(msg)
	case projectViewAddTool:
		return m.handleKeyProjectAddTool(msg)
	}
	return m, nil
}

func (m Model) handleKeyProjectList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	totalRows := len(m.projectEntries) + 1 // +1 for "Init new project" row
	switch msg.String() {
	case "up", "k":
		if m.projectCursor > 0 {
			m.projectCursor--
		}
	case "down", "j":
		if m.projectCursor < totalRows-1 {
			m.projectCursor++
		}
	case "enter":
		if m.projectCursor < len(m.projectEntries) {
			// Open existing project.
			entry := m.projectEntries[m.projectCursor]
			climPath := filepath.Join(entry.Path, teamfile.FileName)
			m.projectView = projectViewDetail
			m.projectCursor = 0
			m.projectScroll = 0
			m.statusMsg = "Checking..."
			return m, projectCheckCmd(climPath, m.tools)
		}
		// "Init new project" row — init in CWD.
		m.statusMsg = "Detecting project tools..."
		cwd, _ := os.Getwd()
		return m, projectInitDetectCmd(cwd)
	case "d":
		// Delete project from registry.
		if m.projectCursor < len(m.projectEntries) {
			entry := m.projectEntries[m.projectCursor]
			_ = teamfile.RemoveProject(entry.Path)
			// Reload.
			return m, projectLoadListCmd(m.tools)
		}
	case "n":
		// Init new project in CWD.
		m.statusMsg = "Detecting project tools..."
		cwd, _ := os.Getwd()
		return m, projectInitDetectCmd(cwd)
	}
	return m, nil
}

func (m Model) handleKeyProjectDetail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "backspace":
		m.projectView = projectViewList
		m.projectCursor = 0
		// Reload project list.
		return m, projectLoadListCmd(m.tools)
	case "up", "k":
		if m.projectCursor > 0 {
			m.projectCursor--
		}
	case "down", "j":
		if m.projectCursor < projectActionCount-1 {
			m.projectCursor++
		}
	case "enter":
		switch m.projectCursor {
		case projectActionRecheck:
			if m.teamFilePath != "" {
				m.statusMsg = "Re-checking..."
				return m, projectCheckCmd(m.teamFilePath, m.tools)
			}
		case projectActionInstallMissing:
			return m.projectInstallMissing()
		case projectActionAddRequired:
			m.projectView = projectViewAddTool
			m.projectAddOptional = false
			m.projectAddCursor = 0
			m.projectAddFilter = ""
			m.rebuildProjectAddFiltered()
			return m, nil
		case projectActionAddOptional:
			m.projectView = projectViewAddTool
			m.projectAddOptional = true
			m.projectAddCursor = 0
			m.projectAddFilter = ""
			m.rebuildProjectAddFiltered()
			return m, nil
		case projectActionEdit:
			if m.teamFilePath != "" {
				return m, projectEditCmd(m.teamFilePath)
			}
		case projectActionReinit:
			// Detect first, show results, then confirm before overwriting.
			dir := "."
			if m.teamFilePath != "" {
				dir = filepath.Dir(m.teamFilePath)
			}
			m.projectReinitDir = dir
			m.projectInitResult = nil
			m.statusMsg = "Detecting project tools..."
			return m, projectInitDetectCmd(dir)
		}
	}
	return m, nil
}

func (m Model) handleKeyProjectConfirmReinit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter", "y", "Y":
		m.projectConfirmReinit = false
		// Delete existing and write new.
		if m.teamFilePath != "" {
			_ = os.Remove(m.teamFilePath)
		}
		dir := m.projectReinitDir
		if dir == "" {
			dir, _ = os.Getwd()
		}
		m.statusMsg = "Generating .clim.yaml..."
		return m, projectInitWriteCmd(dir, m.tools, m.projectInitResult.Tools, false)
	case "esc", "n", "N":
		m.projectConfirmReinit = false
		m.projectInitResult = nil
		m.statusMsg = ""
		return m, nil
	}
	return m, nil
}

func (m Model) handleKeyProjectAddTool(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.projectView = projectViewDetail
		m.projectCursor = 0
		return m, nil
	case "up", "k":
		if m.projectAddCursor > 0 {
			m.projectAddCursor--
		}
	case "down", "j":
		if m.projectAddCursor < len(m.projectAddFiltered)-1 {
			m.projectAddCursor++
		}
	case "enter":
		if m.projectAddCursor < len(m.projectAddFiltered) && m.teamFilePath != "" {
			idx := m.projectAddFiltered[m.projectAddCursor]
			toolName := m.tools[idx].Name
			m.statusMsg = fmt.Sprintf("Adding %s...", toolName)
			m.projectView = projectViewDetail
			m.projectCursor = 0
			return m, projectAddToolCmd(m.teamFilePath, toolName, m.projectAddOptional)
		}
	case "backspace":
		if len(m.projectAddFilter) > 0 {
			m.projectAddFilter = m.projectAddFilter[:len(m.projectAddFilter)-1]
			m.projectAddCursor = 0
			m.rebuildProjectAddFiltered()
		}
	default:
		key := msg.String()
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			m.projectAddFilter += key
			m.projectAddCursor = 0
			m.rebuildProjectAddFiltered()
		}
	}
	return m, nil
}

func (m *Model) rebuildProjectAddFiltered() {
	m.projectAddFiltered = nil
	filter := strings.ToLower(m.projectAddFilter)

	// Collect tool names already in project.
	existing := make(map[string]bool)
	if m.teamFile != nil {
		for _, t := range m.teamFile.Tools {
			existing[t.Name] = true
		}
		for _, t := range m.teamFile.Optional {
			existing[t.Name] = true
		}
	}

	for i, tool := range m.tools {
		if existing[tool.Name] {
			continue
		}
		if filter != "" &&
			!strings.Contains(strings.ToLower(tool.Name), filter) &&
			!strings.Contains(strings.ToLower(tool.DisplayName), filter) {
			continue
		}
		m.projectAddFiltered = append(m.projectAddFiltered, i)
	}
}

func (m Model) projectInstallMissing() (tea.Model, tea.Cmd) {
	if m.teamCheckResult == nil {
		return m, nil
	}

	var missing []string
	for _, r := range m.teamCheckResult {
		if r.Status == teamfile.StatusMissing || r.Status == teamfile.StatusOutdated {
			missing = append(missing, r.Tool.Name)
		}
	}
	if len(missing) == 0 {
		m.statusMsg = "Nothing to install — all requirements met!"
		return m, nil
	}

	toolMap := make(map[string]*registry.Tool, len(m.tools))
	for i := range m.tools {
		toolMap[m.tools[i].Name] = &m.tools[i]
	}

	rp := registry.Pack{Name: "project-requirements", ToolNames: missing}
	m.packItems = buildPackInstallItems(m.tools, rp)
	m.packDone = countPackSkipped(m.packItems)
	m.packInstalling = true
	m.showPackDetail = false

	if cmd := m.nextPackItem(); cmd != nil {
		m.statusMsg = "Installing missing tools..."
		return m, cmd
	}
	m.packInstalling = false
	m.statusMsg = "Nothing installable — check package availability."
	return m, nil
}

// --- Rendering ---

func (m Model) renderProjectView() string {
	// Show reinit/init confirmation regardless of current view.
	if m.projectConfirmReinit && m.projectInitResult != nil {
		return m.renderProjectDetail() // renderProjectDetail handles this state
	}
	switch m.projectView {
	case projectViewDetail:
		return m.renderProjectDetail()
	case projectViewAddTool:
		return m.renderProjectAddTool()
	default:
		return m.renderProjectList()
	}
}

func (m Model) renderProjectList() string {
	var b strings.Builder

	b.WriteString("\n  " + detailTitleStyle.Render("Projects") + "\n\n")

	if len(m.projectEntries) == 0 {
		b.WriteString("  " + dimVersion.Render("No projects registered yet.") + "\n\n")
	}

	cwd, _ := os.Getwd()
	cwdAbs, _ := filepath.Abs(cwd)

	for i, entry := range m.projectEntries {
		cursor := "  "
		if i == m.projectCursor {
			cursor = "▸ "
		}

		// Current project indicator.
		indicator := "○"
		if entry.Path == cwdAbs {
			indicator = "●"
		}

		nameCell := nameStyle.Render(fixedWidth(entry.Name, 22))
		toolsCell := dimVersion.Render(fixedWidth(fmt.Sprintf("%d tools", entry.ToolCount), 10))

		// Truncate path.
		pathDisplay := entry.Path
		maxPath := m.width - 50
		if maxPath > 10 && len(pathDisplay) > maxPath {
			pathDisplay = "..." + pathDisplay[len(pathDisplay)-maxPath+3:]
		}
		pathCell := dashDim.Render(pathDisplay)

		line := cursor + indicator + " " + nameCell + "  " + toolsCell + "  " + pathCell
		if i == m.projectCursor {
			w := lipgloss.Width(line)
			if w < m.width {
				line += strings.Repeat(" ", m.width-w)
			}
			line = selectedRowStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	// "Init new project" row.
	i := len(m.projectEntries)
	cursor := "  "
	if i == m.projectCursor {
		cursor = "▸ "
	}
	line := cursor + "+" + " " + nameStyle.Render("Init new project")
	if i == m.projectCursor {
		w := lipgloss.Width(line)
		if w < m.width {
			line += strings.Repeat(" ", m.width-w)
		}
		line = selectedRowStyle.Render(line)
	}
	b.WriteString(line + "\n")

	return b.String()
}

func (m Model) renderProjectDetail() string {
	var b strings.Builder

	// Show reinit detection results with confirmation prompt.
	if m.projectConfirmReinit && m.projectInitResult != nil {
		r := m.projectInitResult
		b.WriteString("\n  " + detailTitleStyle.Render("Re-init: Scan Results") + "\n\n")
		b.WriteString(fmt.Sprintf("  Scanned %d files in %d directories\n\n", r.FilesScanned, r.DirsScanned))

		// Build installed set for icons.
		installedMap := make(map[string]bool, len(m.tools))
		for _, t := range m.tools {
			if t.IsInstalled() {
				installedMap[t.Name] = true
			}
		}

		if len(r.Tools) > 0 {
			b.WriteString(fmt.Sprintf("  Detected %d tools:\n\n", len(r.Tools)))
			for _, d := range r.Tools {
				icon := dashGaugeWarn.Render("✗")
				if installedMap[d.Name] {
					icon = upToDateStyle.Render("✓")
				}
				b.WriteString(fmt.Sprintf("  %s  %s  %s\n",
					icon,
					nameStyle.Render(fixedWidth(d.Name, 20)),
					dashDim.Render("(from "+d.Source+")"),
				))
			}
		}

		if len(r.Suggestions) > 0 {
			b.WriteString(fmt.Sprintf("\n  💡 Suggested tools for this project:\n\n"))
			for _, s := range r.Suggestions {
				icon := dashDim.Render("○")
				if installedMap[s.Name] {
					icon = upToDateStyle.Render("●")
				}
				b.WriteString(fmt.Sprintf("  %s  %s  %s\n",
					icon,
					nameStyle.Render(fixedWidth(s.Name, 20)),
					dashDim.Render("("+s.Source+")"),
				))
			}
		}

		if m.teamFilePath != "" {
			b.WriteString("\n  " + confirmStyle.Render("This will overwrite "+m.teamFilePath) + "\n")
		}
		b.WriteString("\n  " + dimVersion.Render("Enter") + " confirm   " + dimVersion.Render("Esc") + " cancel\n")
		return b.String()
	}

	if m.teamFile == nil {
		// Init result shown.
		if m.projectInitResult != nil {
			b.WriteString("\n  " + detailTitleStyle.Render("Project Detection") + "\n\n")
			r := m.projectInitResult
			b.WriteString(fmt.Sprintf("  Scanned %d files in %d directories\n\n", r.FilesScanned, r.DirsScanned))
			if len(r.Tools) > 0 {
				b.WriteString(fmt.Sprintf("  Detected %d tools:\n", len(r.Tools)))
				for _, d := range r.Tools {
					b.WriteString(fmt.Sprintf("    %s  %s\n", dimVersion.Render(fixedWidth(d.Name, 20)), dashDim.Render(d.Source)))
				}
			}
			if len(r.Suggestions) > 0 {
				b.WriteString(fmt.Sprintf("\n  💡 Suggestions:\n"))
				for _, s := range r.Suggestions {
					b.WriteString(fmt.Sprintf("    %s  %s\n", dimVersion.Render(fixedWidth(s.Name, 20)), dashDim.Render(s.Source)))
				}
			}
		} else {
			b.WriteString("\n  " + dimVersion.Render("Loading...") + "\n")
		}
		return b.String()
	}

	// Header.
	projectLabel := m.teamFile.Name
	if projectLabel == "" {
		projectLabel = filepath.Base(filepath.Dir(m.teamFilePath))
	}
	b.WriteString("\n  " + detailTitleStyle.Render("Project: "+projectLabel) + "  " +
		dashDim.Render(m.teamFilePath) + "\n\n")

	// Summary gauge (required only).
	var reqResults, optResults []teamfile.CheckResult
	for _, r := range m.teamCheckResult {
		if r.Optional {
			optResults = append(optResults, r)
		} else {
			reqResults = append(reqResults, r)
		}
	}

	reqOK := 0
	for _, r := range reqResults {
		if r.Status == teamfile.StatusOK {
			reqOK++
		}
	}
	reqTotal := len(reqResults)

	b.WriteString(fmt.Sprintf("  %s  %s\n",
		gauge(reqOK, reqTotal, 25, dashGaugeFill, dashGaugeEmpty),
		fmt.Sprintf("%s / %s requirements met",
			dashNumber.Render(fmt.Sprintf("%d", reqOK)),
			dashDim.Render(fmt.Sprintf("%d", reqTotal)),
		),
	))
	b.WriteString("\n")

	// Required tools.
	if len(reqResults) > 0 {
		b.WriteString("  " + dashSection.Render("Required") + "\n\n")
		for _, r := range reqResults {
			b.WriteString(m.renderCheckResultLine(r))
		}
		b.WriteString("\n")
	}

	// Optional tools.
	if len(optResults) > 0 {
		b.WriteString("  " + dashSection.Render("Optional") + "\n\n")
		for _, r := range optResults {
			b.WriteString(m.renderCheckResultLine(r))
		}
		b.WriteString("\n")
	}

	// Actions.
	b.WriteString("  " + dashSection.Render("Actions") + "\n\n")

	actions := []struct {
		label string
		desc  string
	}{
		{"Re-check", "Refresh check results"},
		{"Install missing", "Install tools that are missing or outdated"},
		{"Add required tool", "Add a tool to required list"},
		{"Add optional tool", "Add a tool to optional list"},
		{"Edit .clim.yaml", "Open in $EDITOR"},
		{"Re-init", "Scan project files and regenerate"},
	}

	for i, action := range actions {
		cursor := "  "
		if i == m.projectCursor {
			cursor = "▸ "
		}
		line := cursor + nameStyle.Render(fixedWidth(action.label, 22)) + "  " + dimVersion.Render(action.desc)
		if i == m.projectCursor {
			w := lipgloss.Width(line)
			if w < m.width {
				line += strings.Repeat(" ", m.width-w)
			}
			line = selectedRowStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	return b.String()
}

func (m Model) renderCheckResultLine(r teamfile.CheckResult) string {
	var icon, ver, status string
	switch r.Status {
	case teamfile.StatusOK:
		icon = upToDateStyle.Render("✓")
		ver = r.Version
		if r.Tool.Version != "" {
			status = upToDateStyle.Render("(" + r.Tool.Version + ")")
		}
	case teamfile.StatusMissing:
		icon = dashGaugeWarn.Render("✗")
		ver = "—"
		status = dashGaugeWarn.Render("NOT INSTALLED")
	case teamfile.StatusOutdated:
		icon = dashGaugeWarn.Render("⚠")
		ver = r.Version
		status = dashGaugeWarn.Render(r.Message)
	case teamfile.StatusUnknown:
		icon = dashDim.Render("?")
		ver = "—"
		status = dashDim.Render("not in catalog")
	}
	return fmt.Sprintf("  %s  %s  %s  %s\n",
		icon,
		nameStyle.Render(fixedWidth(r.Tool.Name, 20)),
		dimVersion.Render(fixedWidth(ver, 14)),
		status,
	)
}

func (m Model) renderProjectAddTool() string {
	var b strings.Builder

	label := "required"
	if m.projectAddOptional {
		label = "optional"
	}
	b.WriteString("\n  " + detailTitleStyle.Render("Add "+label+" tool") + "\n\n")

	if m.projectAddFilter != "" {
		b.WriteString("  " + filterPromptStyle.Render("filter: ") + m.projectAddFilter + "\n\n")
	}

	visibleRows := m.height - 10
	if visibleRows < 5 {
		visibleRows = 5
	}

	start := 0
	if m.projectAddCursor >= visibleRows {
		start = m.projectAddCursor - visibleRows + 1
	}
	end := start + visibleRows
	if end > len(m.projectAddFiltered) {
		end = len(m.projectAddFiltered)
	}

	for i := start; i < end; i++ {
		idx := m.projectAddFiltered[i]
		tool := m.tools[idx]

		cursor := "  "
		if i == m.projectAddCursor {
			cursor = "▸ "
		}

		installed := ""
		if tool.IsInstalled() {
			installed = " " + upToDateStyle.Render("●")
		}

		line := cursor + nameStyle.Render(fixedWidth(tool.DisplayName, 28)) +
			"  " + dimVersion.Render(fixedWidth(tool.Category, 14)) + installed

		if i == m.projectAddCursor {
			w := lipgloss.Width(line)
			if w < m.width {
				line += strings.Repeat(" ", m.width-w)
			}
			line = selectedRowStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	rendered := end - start
	for range max(visibleRows-rendered, 0) {
		b.WriteString("\n")
	}

	b.WriteString("\n  " + dimVersion.Render("Enter") + " add   " +
		dimVersion.Render("Esc") + " cancel   " +
		dimVersion.Render("type to filter") + "\n")

	return b.String()
}
