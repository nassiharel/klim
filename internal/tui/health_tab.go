package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"charm.land/lipgloss/v2"
	tea "charm.land/bubbletea/v2"

	"github.com/nassiharel/klim/internal/doctor"
	"github.com/nassiharel/klim/internal/pathconflict"
	"github.com/nassiharel/klim/internal/registry"
)

// Health view color palette — mirrors the Security palette so the eye
// instantly recognises severity icons regardless of which tab a row
// shows up in.
var (
	healthHeader    = lipgloss.NewStyle().Bold(true).Foreground(highlightColor)
	healthDim       = lipgloss.NewStyle().Foreground(subtleColor)
	healthAccent    = lipgloss.NewStyle().Foreground(primaryColor)
	healthWarn      = lipgloss.NewStyle().Foreground(warningColor)
	healthBad       = lipgloss.NewStyle().Foreground(lipgloss.Color("167"))
	healthOK        = lipgloss.NewStyle().Foreground(successColor).Bold(true)
	healthSelected  = lipgloss.NewStyle().Foreground(highlightColor).Background(lipgloss.Color("237"))
	healthActiveTag = lipgloss.NewStyle().Foreground(successColor).Bold(true)
)

// renderHealthView routes to the active Health sub-tab. Both views
// require the initial scan to have completed (so tools[] is populated).
func (m Model) renderHealthView() string {
	var b strings.Builder

	// Sub-tab bar
	labels := []struct {
		text string
		idx  int
	}{
		{"Issues", healthSubIssues},
		{"PATH", healthSubPath},
	}
	var tabs []string
	for _, l := range labels {
		if m.healthSubTab == l.idx {
			tabs = append(tabs, activeTabStyle.Render(l.text))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render(l.text))
		}
	}
	b.WriteString("  " + strings.Join(tabs, " ") + "\n\n")

	if m.healthPathStatus != "" {
		b.WriteString("  " + healthAccent.Render(m.healthPathStatus) + "\n\n")
	}

	if !m.doctorChecked {
		b.WriteString("  " + loadingStyle.Render("Waiting for scan to complete..."))
		return b.String()
	}

	switch m.healthSubTab {
	case healthSubPath:
		b.WriteString(m.renderHealthPathView())
	default:
		b.WriteString(m.renderHealthIssuesView())
	}
	return b.String()
}

// renderHealthIssuesView is the long-form diagnostics list (formerly
// the Security → Health sub-tab). The text and severity classification
// come from internal/doctor. Each issue is selectable: ↑/↓ moves the
// cursor, `f`/Enter invokes the issue's structured Action — copy a
// PATH-cleanup command, jump to the PATH view focused on the offender,
// trigger a rescan, or jump to Updates.
func (m Model) renderHealthIssuesView() string {
	var b strings.Builder

	if len(m.doctorIssues) == 0 {
		b.WriteString("  " + healthOK.Render("✓ No issues found — your environment looks healthy!") + "\n\n")
		b.WriteString("  " + dashDim.Render("All PATH entries are valid, no version conflicts detected,") + "\n")
		b.WriteString("  " + dashDim.Render("and your package managers are working correctly.") + "\n")
		return b.String()
	}

	errs, warns, infos := doctor.CountBySeverity(m.doctorIssues)
	var summary []string
	if errs > 0 {
		summary = append(summary, healthBad.Render(fmt.Sprintf("%d error(s)", errs)))
	}
	if warns > 0 {
		summary = append(summary, healthWarn.Render(fmt.Sprintf("%d warning(s)", warns)))
	}
	if infos > 0 {
		summary = append(summary, healthAccent.Render(fmt.Sprintf("%d info(s)", infos)))
	}
	b.WriteString("  " + strings.Join(summary, "  ") + "\n\n")

	flat := flatIssueOrder(m.doctorIssues)
	cursor := clampCursor(m.healthIssueCursor, len(flat))

	// Group for display while remembering the flat-index of each row
	// so the cursor lights up the right entry across category headers.
	currentCat := ""
	for i, issue := range flat {
		if issue.Category != currentCat {
			if currentCat != "" {
				b.WriteString("\n")
			}
			b.WriteString("  " + healthHeader.Render(issue.Category) + "\n")
			currentCat = issue.Category
		}
		selected := i == cursor
		row := severityStyle(issue.Severity) + " " + issue.Title
		if selected {
			b.WriteString("  " + healthSelected.Render("▶ "+row) + "\n")
		} else {
			b.WriteString("    " + row + "\n")
		}
		if issue.Detail != "" {
			for _, line := range strings.Split(issue.Detail, "\n") {
				if line != "" {
					b.WriteString("      " + dashDim.Render(line) + "\n")
				}
			}
		}
		if issue.Fix != "" {
			b.WriteString("      " + healthDim.Render("→ "+issue.Fix) + "\n")
		}
		// Action hint for the selected row only — we don't want the
		// list to balloon vertically when nothing's focused.
		if selected && issue.Action.Kind != doctor.ActionNone {
			actionHint := issue.Action.Label
			if actionHint == "" {
				actionHint = "Apply suggested fix"
			}
			b.WriteString("      " + healthAccent.Render("f: "+actionHint) + "\n")
			if issue.Action.Kind == doctor.ActionCopyCommand && issue.Action.Command != "" {
				// Show the literal command (truncated when long)
				// so the user knows what `f` will copy.
				cmd := issue.Action.Command
				if firstLine, _, ok := strings.Cut(cmd, "\n"); ok {
					cmd = firstLine + " …"
				}
				if len(cmd) > 120 {
					cmd = cmd[:117] + "…"
				}
				b.WriteString("      " + healthDim.Render("  $ "+cmd) + "\n")
			}
		}
	}
	b.WriteString("\n")
	return b.String()
}

// flatIssueOrder returns the doctor issues in the order they should be
// rendered (category-grouped, preserving first-seen category order).
// This is the same order the cursor walks with ↑/↓, so applyIssueAction
// can dispatch on the cursor index unambiguously.
func flatIssueOrder(issues []doctor.Issue) []doctor.Issue {
	grouped := make(map[string][]doctor.Issue)
	var order []string
	for _, issue := range issues {
		if _, ok := grouped[issue.Category]; !ok {
			order = append(order, issue.Category)
		}
		grouped[issue.Category] = append(grouped[issue.Category], issue)
	}
	flat := make([]doctor.Issue, 0, len(issues))
	for _, cat := range order {
		flat = append(flat, grouped[cat]...)
	}
	return flat
}

func clampCursor(c, n int) int {
	if n == 0 {
		return 0
	}
	if c < 0 {
		return 0
	}
	if c >= n {
		return n - 1
	}
	return c
}

// renderHealthPathView shows either the by-tool or by-PATH-dir
// visualization of the same pathconflict.Report.
func (m Model) renderHealthPathView() string {
	report := pathconflict.Analyze(m.tools)

	var b strings.Builder
	// Header showing which sub-view is active.
	views := []struct {
		label string
		idx   int
	}{
		{"By tool", healthPathByTool},
		{"By PATH dir", healthPathByDir},
	}
	var bits []string
	for _, v := range views {
		if v.idx == m.healthPathView {
			bits = append(bits, healthActiveTag.Render("● "+v.label))
		} else {
			bits = append(bits, healthDim.Render("○ "+v.label))
		}
	}
	b.WriteString("  " + strings.Join(bits, "    ") + "    " + healthDim.Render("Tab: switch view") + "\n\n")

	switch m.healthPathView {
	case healthPathByDir:
		b.WriteString(m.renderHealthPathByDir(report))
	default:
		b.WriteString(m.renderHealthPathByTool(report))
	}
	return b.String()
}

func (m Model) renderHealthPathByTool(report pathconflict.Report) string {
	var b strings.Builder
	if len(report.ByTool) == 0 {
		b.WriteString("  " + healthOK.Render("✓ Every tool resolves to a single copy.") + "\n")
		b.WriteString("  " + healthDim.Render("No PATH shadowing detected.") + "\n")
		return b.String()
	}
	b.WriteString("  " + healthHeader.Render(fmt.Sprintf(
		"%d tool(s) with multiple PATH copies — %d shadowed total",
		len(report.ByTool), report.CountShadowed())) + "\n\n")

	for ti, tv := range report.ByTool {
		flags := ""
		switch {
		case tv.VersionConflict:
			flags = "  " + healthBad.Render("⚠ version conflict")
		case tv.PrivilegeRisk:
			flags = "  " + healthWarn.Render("⚠ user-writable shadows system")
		}
		selectedTool := ti == m.healthPathToolIdx
		title := tv.DisplayName + flags
		if selectedTool {
			b.WriteString("  " + healthSelected.Render("▶ "+title) + "\n")
		} else {
			b.WriteString("    " + title + "\n")
		}

		// Active row
		b.WriteString("      " + healthOK.Render("✓ active   ") +
			formatInstanceLine(tv.Active) + "\n")

		// Shadowed rows
		for si, sh := range tv.Shadowed {
			marker := "      ⊘ shadowed "
			line := marker + formatInstanceLine(sh)
			if selectedTool && si == m.healthPathShadowIdx {
				b.WriteString(healthSelected.Render(line) + "\n")
				if sh.UninstallCmd != "" {
					b.WriteString("          " + healthDim.Render("u to run: "+sh.UninstallCmd) + "\n")
				} else {
					b.WriteString("          " + healthDim.Render("manual install — press o to open containing folder") + "\n")
				}
			} else {
				b.WriteString(line + "\n")
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) renderHealthPathByDir(report pathconflict.Report) string {
	var b strings.Builder
	if len(report.ByDir) == 0 {
		b.WriteString("  " + healthDim.Render("PATH is empty.") + "\n")
		return b.String()
	}
	b.WriteString("  " + healthHeader.Render(fmt.Sprintf("%d PATH entries (in order)", len(report.ByDir))) + "\n\n")

	for di, d := range report.ByDir {
		var annotations []string
		switch {
		case !d.Exists:
			annotations = append(annotations, healthBad.Render("missing"))
		case !d.IsDir:
			annotations = append(annotations, healthBad.Render("not a directory"))
		}
		if d.Duplicate {
			annotations = append(annotations, healthWarn.Render("duplicate"))
		}
		if d.UserWrite && d.SystemDir {
			// Both flagged is unusual; show both verbatim.
			annotations = append(annotations, healthWarn.Render("user-writable"))
			annotations = append(annotations, healthDim.Render("system"))
		} else if d.UserWrite {
			annotations = append(annotations, healthWarn.Render("user-writable"))
		} else if d.SystemDir {
			annotations = append(annotations, healthDim.Render("system"))
		}
		ann := ""
		if len(annotations) > 0 {
			ann = "  " + strings.Join(annotations, " ")
		}
		header := fmt.Sprintf("%2d. %s", d.Order, d.Dir) + ann
		if di == m.healthPathDirIdx {
			b.WriteString("  " + healthSelected.Render("▶ "+header) + "\n")
		} else {
			b.WriteString("    " + header + "\n")
		}
		for _, te := range d.Tools {
			marker := "      ⊘"
			label := te.DisplayName
			if te.Active {
				marker = "      " + healthOK.Render("✓")
				label = healthActiveTag.Render(label)
			}
			ver := te.Version
			if ver == "" {
				ver = "?"
			}
			src := string(te.Source)
			if src == "" {
				src = "manual"
			}
			b.WriteString(fmt.Sprintf("%s %s  %s  %s\n",
				marker, label, healthDim.Render(ver), healthDim.Render(src)))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func formatInstanceLine(iv pathconflict.InstanceView) string {
	ver := iv.Version
	if ver == "" {
		ver = "?"
	}
	src := string(iv.Source)
	if src == "" {
		src = "manual"
	}
	return fmt.Sprintf("%s  %s  %s",
		healthDim.Render("("+ver+", "+src+")"), iv.Path, "")
}

// handleKeyHealth handles key events while on the Health tab. It owns
// scroll, sub-tab cycling, parent-tab navigation, and the PATH view's
// row selection + uninstall action.
func (m Model) handleKeyHealth(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	report := pathconflict.Analyze(m.tools)

	switch msg.String() {
	case "q", "ctrl+c":
		m.quitting = true
		return m, tea.Quit

	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if handled, cmd := m.switchToTabByNumber(msg.String()); handled {
			return m, cmd
		}
		return m, nil

	case "right", "tab":
		// Cycle Health sub-tabs first; spill into next parent at the edge.
		if m.healthSubTab < healthSubPath {
			m.healthSubTab++
			m.healthScroll = 0
			return m, nil
		}
		next := parentTabOrder[(parentIndex(m.activeTab)+1)%len(parentTabOrder)]
		m.healthSubTab = healthSubIssues
		m.healthScroll = 0
		return m.gotoParentTab(next)

	case "left", "shift+tab":
		if m.healthSubTab > healthSubIssues {
			m.healthSubTab--
			m.healthScroll = 0
			return m, nil
		}
		prev := parentTabOrder[(parentIndex(m.activeTab)+len(parentTabOrder)-1)%len(parentTabOrder)]
		m.healthSubTab = healthSubIssues
		m.healthScroll = 0
		return m.gotoParentTab(prev)

	case "home", "g":
		m.healthScroll = 0
		return m, nil

	case "r":
		if m.activeBatch != nil && m.activeBatch.isRunning() {
			return m, nil
		}
		m.healthPathStatus = ""
		return m, m.startScan()
	}

	// Sub-tab-specific keys.
	switch m.healthSubTab {
	case healthSubIssues:
		flat := flatIssueOrder(m.doctorIssues)
		switch msg.String() {
		case "up", "k":
			if len(flat) == 0 {
				return m, nil
			}
			if m.healthIssueCursor > 0 {
				m.healthIssueCursor--
			} else if m.healthScroll > 0 {
				m.healthScroll--
			}
		case "down", "j":
			if len(flat) == 0 {
				return m, nil
			}
			if m.healthIssueCursor < len(flat)-1 {
				m.healthIssueCursor++
			} else {
				m.healthScroll++
			}
		case "f", "enter":
			if len(flat) == 0 {
				return m, nil
			}
			cursor := clampCursor(m.healthIssueCursor, len(flat))
			return m.applyIssueAction(flat[cursor])
		}
		return m, nil

	case healthSubPath:
		return m.handleKeyHealthPath(msg, report)
	}
	return m, nil
}

// applyIssueAction dispatches on the selected issue's Action.Kind:
//   - CopyCommand: copies Action.Command to the clipboard.
//   - JumpPathView: switches to Health → PATH, By tool, focused on
//     Action.Target.
//   - Rescan: kicks off m.startScan().
//   - JumpUpdates: switches to My Tools → Updates.
//
// Unknown / None: surface "no automated fix" in the status banner so
// the user knows nothing happened (vs. assuming `f` is broken).
func (m Model) applyIssueAction(issue doctor.Issue) (tea.Model, tea.Cmd) {
	switch issue.Action.Kind {
	case doctor.ActionCopyCommand:
		if issue.Action.Command == "" {
			m.healthPathStatus = "⚠ no command available for this issue"
			return m, nil
		}
		if err := m.clip.WriteAll(issue.Action.Command); err != nil {
			m.healthPathStatus = "⚠ clipboard: " + err.Error()
			return m, nil
		}
		label := issue.Action.Label
		if label == "" {
			label = "fix command"
		}
		m.healthPathStatus = "✓ Copied " + label + " — paste into your shell"
		return m, nil

	case doctor.ActionJumpPathView:
		m.healthSubTab = healthSubPath
		m.healthPathView = healthPathByTool
		m.healthPathShadowIdx = 0
		m.healthScroll = 0
		// Find the offending tool in the analyzer report and put
		// the cursor on it. Falls back to row 0 if the tool isn't
		// in the report (e.g. it disappeared between diagnose and
		// jump — rare but possible after a rescan).
		report := pathconflict.Analyze(m.tools)
		for i, tv := range report.ByTool {
			if tv.Name == issue.Action.Target {
				m.healthPathToolIdx = i
				break
			}
		}
		m.healthPathStatus = "→ Opened PATH view focused on " + issue.Action.Target
		return m, nil

	case doctor.ActionRescan:
		m.healthPathStatus = "Rescanning..."
		return m, m.startScan()

	case doctor.ActionJumpUpdates:
		m.activeTab = tabUpdates
		m.cursor = 0
		m.applyFilter()
		return m, nil
	}
	m.healthPathStatus = "⚠ no automated fix for this issue"
	return m, nil
}

func (m Model) handleKeyHealthPath(msg tea.KeyMsg, report pathconflict.Report) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "t":
		// Toggle between By-tool and By-PATH-dir.
		if m.healthPathView == healthPathByTool {
			m.healthPathView = healthPathByDir
		} else {
			m.healthPathView = healthPathByTool
		}
		m.healthScroll = 0
		return m, nil

	case "up", "k":
		if m.healthPathView == healthPathByTool {
			if m.healthPathShadowIdx > 0 {
				m.healthPathShadowIdx--
				return m, nil
			}
			if m.healthPathToolIdx > 0 {
				m.healthPathToolIdx--
				m.healthPathShadowIdx = m.lastShadowIndex(report)
				return m, nil
			}
			// Scroll up at the top.
			if m.healthScroll > 0 {
				m.healthScroll--
			}
			return m, nil
		}
		if m.healthPathDirIdx > 0 {
			m.healthPathDirIdx--
			return m, nil
		}
		if m.healthScroll > 0 {
			m.healthScroll--
		}
		return m, nil

	case "down", "j":
		if m.healthPathView == healthPathByTool {
			if m.healthPathToolIdx < len(report.ByTool) {
				curShadows := 0
				if m.healthPathToolIdx < len(report.ByTool) {
					curShadows = len(report.ByTool[m.healthPathToolIdx].Shadowed)
				}
				if m.healthPathShadowIdx < curShadows-1 {
					m.healthPathShadowIdx++
					return m, nil
				}
				if m.healthPathToolIdx < len(report.ByTool)-1 {
					m.healthPathToolIdx++
					m.healthPathShadowIdx = 0
					return m, nil
				}
			}
			m.healthScroll++
			return m, nil
		}
		if m.healthPathDirIdx < len(report.ByDir)-1 {
			m.healthPathDirIdx++
			return m, nil
		}
		m.healthScroll++
		return m, nil

	case "u":
		// Uninstall the selected shadowed copy via its PM.
		if m.healthPathView != healthPathByTool {
			return m, nil
		}
		if m.healthPathToolIdx >= len(report.ByTool) {
			return m, nil
		}
		tv := report.ByTool[m.healthPathToolIdx]
		if m.healthPathShadowIdx >= len(tv.Shadowed) {
			return m, nil
		}
		shadow := tv.Shadowed[m.healthPathShadowIdx]
		if shadow.UninstallCmd == "" {
			m.healthPathStatus = "⚠ " + tv.DisplayName + " at " + shadow.Path + " has no automated uninstaller (manual install)"
			return m, nil
		}
		toolIdx, toolPtr := findToolByName(m.tools, tv.Name)
		var args []string
		if toolPtr != nil {
			args = toolPtr.Packages.RemoveArgs(shadow.Source)
		}
		if len(args) == 0 {
			m.healthPathStatus = "⚠ no remove command available for source " + string(shadow.Source)
			return m, nil
		}
		m.pendingAction = &pendingAction{
			toolIdx: toolIdx,
			action:  actionRemove,
			cmdArgs: args,
		}
		return m, nil

	case "o":
		// Open containing folder of the selected entry (works for
		// both views and is the safe fallback when uninstall isn't
		// available).
		var target string
		if m.healthPathView == healthPathByTool {
			if m.healthPathToolIdx < len(report.ByTool) {
				tv := report.ByTool[m.healthPathToolIdx]
				if m.healthPathShadowIdx < len(tv.Shadowed) {
					target = tv.Shadowed[m.healthPathShadowIdx].Path
				} else {
					target = tv.Active.Path
				}
			}
		} else if m.healthPathDirIdx < len(report.ByDir) {
			target = report.ByDir[m.healthPathDirIdx].Dir
		}
		if target == "" {
			return m, nil
		}
		if err := openInFileManager(target); err != nil {
			m.healthPathStatus = "⚠ " + err.Error()
		} else {
			m.healthPathStatus = "✓ Opened " + target
		}
		return m, nil

	case "c":
		// Copy selected path to clipboard.
		var target string
		if m.healthPathView == healthPathByTool {
			if m.healthPathToolIdx < len(report.ByTool) {
				tv := report.ByTool[m.healthPathToolIdx]
				if m.healthPathShadowIdx < len(tv.Shadowed) {
					target = tv.Shadowed[m.healthPathShadowIdx].Path
				} else {
					target = tv.Active.Path
				}
			}
		} else if m.healthPathDirIdx < len(report.ByDir) {
			target = report.ByDir[m.healthPathDirIdx].Dir
		}
		if target == "" {
			return m, nil
		}
		if err := m.clip.WriteAll(target); err != nil {
			m.healthPathStatus = "⚠ clipboard: " + err.Error()
		} else {
			m.healthPathStatus = "✓ Copied " + target
		}
		return m, nil
	}
	return m, nil
}

func (m Model) lastShadowIndex(report pathconflict.Report) int {
	if m.healthPathToolIdx >= len(report.ByTool) {
		return 0
	}
	n := len(report.ByTool[m.healthPathToolIdx].Shadowed)
	if n == 0 {
		return 0
	}
	return n - 1
}

func findToolByName(tools []registry.Tool, name string) (int, *registry.Tool) {
	for i := range tools {
		if tools[i].Name == name {
			return i, &tools[i] //nolint:gosec // G602: index bounded by range.
		}
	}
	return -1, nil
}

// openInFileManager opens the containing directory of path in the
// platform's default file manager. For a directory argument it opens
// the dir itself. Best-effort: errors are surfaced to the caller.
func openInFileManager(path string) error {
	dir := path
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		dir = filepath.Dir(path)
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", dir)
	case "darwin":
		cmd = exec.Command("open", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	return cmd.Start()
}
