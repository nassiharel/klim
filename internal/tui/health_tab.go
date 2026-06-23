package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/doctor"
)

// Health view color palette — mirrors the Security palette so the eye
// instantly recognises severity icons regardless of which tab a row
// shows up in.
var (
	healthHeader    = lipgloss.NewStyle().Bold(true).Foreground(cyberFG)
	healthDim       = lipgloss.NewStyle().Foreground(cyberFGDim)
	healthAccent    = lipgloss.NewStyle().Foreground(cyberPrimary)
	healthWarn      = lipgloss.NewStyle().Foreground(cyberAccent)
	healthBad       = lipgloss.NewStyle().Foreground(cyberAlert)
	healthOK        = lipgloss.NewStyle().Foreground(cyberOK).Bold(true)
	healthSelected  = lipgloss.NewStyle().Foreground(cyberFG).Background(cyberSelectedBg)
	healthActiveTag = lipgloss.NewStyle().Foreground(cyberOK).Bold(true)
)

// renderHealthView renders the Health tab body (the diagnostics list).
// It requires the initial scan to have completed (so tools[] is
// populated).
func (m Model) renderHealthView() string {
	var b strings.Builder

	if m.healthStatus != "" {
		b.WriteString("  " + healthAccent.Render(m.healthStatus) + "\n\n")
	}

	if !m.doctorChecked {
		b.WriteString("  " + loadingStyle.Render("Waiting for scan to complete..."))
		return b.String()
	}

	b.WriteString(m.renderHealthIssuesView())
	return b.String()
}

// renderHealthIssuesView is the long-form diagnostics list (formerly
// the Security → Health sub-tab). The text and severity classification
// come from internal/doctor. Each issue is selectable: ↑/↓ moves the
// cursor, `f`/Enter invokes the issue's structured Action — copy a
// command, trigger a rescan, or jump to Updates.
func (m Model) renderHealthIssuesView() string {
	var b strings.Builder

	if len(m.doctorIssues) == 0 {
		b.WriteString("  " + healthOK.Render("✓ No issues found — your environment looks healthy!") + "\n\n")
		b.WriteString("  " + dashDim.Render("No version conflicts detected, and your package") + "\n")
		b.WriteString("  " + dashDim.Render("managers are working correctly.") + "\n")
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
		if selected && issue.Action != nil && issue.Action.Kind != doctor.ActionNone {
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

// handleKeyHealth handles key events while on the Health tab. It owns
// scroll, parent-tab navigation, and the issue list's cursor + fix
// action.
func (m Model) handleKeyHealth(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		next := parentTabOrder[(parentIndex(m.activeTab)+1)%len(parentTabOrder)]
		m.healthScroll = 0
		return m.gotoParentTab(next)

	case "left", "shift+tab":
		prev := parentTabOrder[(parentIndex(m.activeTab)+len(parentTabOrder)-1)%len(parentTabOrder)]
		m.healthScroll = 0
		return m.gotoParentTab(prev)

	case "home", "g":
		m.healthScroll = 0
		return m, nil

	case "r":
		if m.activeBatch != nil && m.activeBatch.isRunning() {
			return m, nil
		}
		m.healthStatus = ""
		scan := m.startScan()
		return m, scan
	}

	flat := flatIssueOrder(m.doctorIssues)
	switch msg.String() {
	case "up", "k":
		if len(flat) == 0 {
			return m, nil
		}
		if m.healthIssueCursor > 0 {
			m.healthIssueCursor--
		}
		m.healthScroll = clampScrollToCursor(m.healthScroll, flat, m.healthIssueCursor, m.visibleHealthRows())
	case "down", "j":
		if len(flat) == 0 {
			return m, nil
		}
		if m.healthIssueCursor < len(flat)-1 {
			m.healthIssueCursor++
		}
		m.healthScroll = clampScrollToCursor(m.healthScroll, flat, m.healthIssueCursor, m.visibleHealthRows())
	case "home", "g":
		m.healthIssueCursor = 0
		m.healthScroll = 0
	case "end", "G":
		if len(flat) > 0 {
			m.healthIssueCursor = len(flat) - 1
			m.healthScroll = clampScrollToCursor(m.healthScroll, flat, m.healthIssueCursor, m.visibleHealthRows())
		}
	case "pgup":
		if len(flat) == 0 {
			return m, nil
		}
		step := m.visibleHealthRows() / 6
		if step < 1 {
			step = 1
		}
		m.healthIssueCursor -= step
		if m.healthIssueCursor < 0 {
			m.healthIssueCursor = 0
		}
		m.healthScroll = clampScrollToCursor(m.healthScroll, flat, m.healthIssueCursor, m.visibleHealthRows())
	case "pgdown", " ":
		if len(flat) == 0 {
			return m, nil
		}
		step := m.visibleHealthRows() / 6
		if step < 1 {
			step = 1
		}
		m.healthIssueCursor += step
		if m.healthIssueCursor >= len(flat) {
			m.healthIssueCursor = len(flat) - 1
		}
		m.healthScroll = clampScrollToCursor(m.healthScroll, flat, m.healthIssueCursor, m.visibleHealthRows())
	case "f", "enter":
		if len(flat) == 0 {
			return m, nil
		}
		cursor := clampCursor(m.healthIssueCursor, len(flat))
		return m.applyIssueAction(flat[cursor])
	}
	return m, nil
}

// visibleHealthRows returns the row count available for the Issues
// body (everything between the parent tab bar and the help footer).
// Mirrors the math used in renderView when slicing the Health body.
func (m Model) visibleHealthRows() int {
	const headerRows = 4 // title + tab bar + rule + blank
	v := m.height - headerRows - m.subtabRows() - m.footerHeight() - 1
	if v < 5 {
		v = 5
	}
	return v
}

// issueLineOffset returns the rendered line index of the issue at the
// given cursor position. Used to keep the cursor inside the visible
// window when the user navigates through a long issue list. The
// counts here must mirror renderHealthIssuesView exactly; that's the
// price of the cursor-follow feature, but the formatting is stable
// enough that it's a worthwhile trade.
func issueLineOffset(flat []doctor.Issue, idx int) int {
	// Header: summary line + blank.
	line := 2
	currentCat := ""
	for i, issue := range flat {
		if issue.Category != currentCat {
			if currentCat != "" {
				line++ // blank line between previous category and next
			}
			line++ // category header row
			currentCat = issue.Category
		}
		if i == idx {
			return line
		}
		line++ // title row (▶ or plain)
		if issue.Detail != "" {
			for _, ln := range strings.Split(issue.Detail, "\n") {
				if ln != "" {
					line++
				}
			}
		}
		if issue.Fix != "" {
			line++
		}
	}
	return line
}

// clampScrollToCursor keeps the cursor's rendered line inside the
// visible window. Scrolls up when the cursor would otherwise be
// above the viewport, scrolls down when it would be below.
func clampScrollToCursor(scroll int, flat []doctor.Issue, cursor, visibleRows int) int {
	if len(flat) == 0 {
		return 0
	}
	cursorLine := issueLineOffset(flat, cursor)
	switch {
	case cursorLine < scroll:
		// Land the cursor one row into the viewport so the user
		// sees a hint of context above it.
		return max(cursorLine-1, 0)
	case cursorLine >= scroll+visibleRows-2:
		// Keep two rows of context below the cursor.
		return cursorLine - visibleRows + 3
	}
	return scroll
}

// applyIssueAction dispatches on the selected issue's Action.Kind by
// opening the fix wizard modal. The modal handles the actual run /
// copy / jump steps and exposes them as labelled buttons so users can
// see the proposed command, choose to either run it for them or copy
// it for manual review, and watch the result.
//
// Unknown / None: surface "no automated fix" in the status banner so
// the user knows nothing happened (vs. assuming `f` is broken).
func (m Model) applyIssueAction(issue doctor.Issue) (tea.Model, tea.Cmd) {
	if issue.Action == nil || issue.Action.Kind == doctor.ActionNone {
		m.healthStatus = "⚠ no automated fix for this issue"
		return m, nil
	}
	m.openHealthFixModal(issue)
	return m, nil
}
