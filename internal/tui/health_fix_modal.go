package tui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"charm.land/lipgloss/v2"
	tea "charm.land/bubbletea/v2"

	"github.com/nassiharel/klim/internal/doctor"
	"github.com/nassiharel/klim/internal/pathbackup"
	"github.com/nassiharel/klim/internal/pathconflict"
)

// fixModalState tracks where the user is inside the Health → Issues
// "fix" wizard: still picking an option, watching the command run, or
// looking at the final outcome. Modal state lives on the Model so the
// usual Update/View loop can drive transitions.
type fixModalState int

const (
	fixModalIdle    fixModalState = iota // showing the options menu
	fixModalRunning                      // command in flight
	fixModalDone                         // result rendered, waiting for dismiss
)

// fixModal is the per-session state of the modal. Zero value means
// "closed" (Open == false).
type fixModal struct {
	Open     bool
	Issue    doctor.Issue
	Options  []fixModalOption
	Cursor   int
	State    fixModalState
	Output   string // captured stdout+stderr of the run
	Err      error  // run error, if any
	Resolved bool   // post-run: did the issue disappear from the diagnostic set?
	// BackupPath is the path of the PATH-backup file written just
	// before a PATH-modifying command was executed. Empty when no
	// backup was taken (non-PATH action, or backup write failed).
	BackupPath string
	// IsRestore distinguishes a "rolling back a previous fix" run
	// from a normal first-time fix run. When true the Done view
	// uses different copy and skips offering yet another Restore
	// button (no infinite undo of the undo).
	IsRestore bool
}

// fixModalOption is one button in the modal's choice strip. `action` is
// the closure executed on Enter; returns an optional tea.Cmd (for
// async work like running a shell command).
type fixModalOption struct {
	Key   string // short identifier — used in tests and for accessibility hints
	Label string
	Desc  string
	// Run returns (newModel, cmd). For the Run option this kicks off
	// an exec; for Copy it writes to the clipboard; for Cancel it
	// closes the modal.
	Run func(m Model) (Model, tea.Cmd)
}

// healthFixResultMsg is emitted when the shell command finishes.
type healthFixResultMsg struct {
	Output string
	Err    error
}

// openHealthFixModal builds the modal for an issue and stages it. The
// option list is tailored to the issue's Action kind — CopyCommand
// gets [Run, Copy, Cancel]; the jump/rescan kinds offer a single
// confirm button + Cancel.
func (m *Model) openHealthFixModal(issue doctor.Issue) {
	opts := buildFixOptions(issue)
	if len(opts) == 0 {
		// Issue has no action — surface a non-modal status so the
		// user understands `f` wasn't ignored.
		m.healthPathStatus = "⚠ No automated fix for this issue"
		return
	}
	m.fixModal = fixModal{
		Open:    true,
		Issue:   issue,
		Options: opts,
		Cursor:  0,
		State:   fixModalIdle,
	}
}

// buildFixOptions translates a doctor.Issue into the actionable buttons
// the modal exposes.
func buildFixOptions(issue doctor.Issue) []fixModalOption {
	switch issue.Action.Kind {
	case doctor.ActionCopyCommand:
		cmd := issue.Action.Command
		opts := []fixModalOption{}
		if cmd != "" {
			opts = append(opts, fixModalOption{
				Key:   "run",
				Label: "Run command",
				Desc:  "Execute the suggested command for you and stream output here.",
				Run: func(m Model) (Model, tea.Cmd) {
					// Capture a PATH backup BEFORE the command
					// runs so the user can roll back from the
					// Done state. Backup failures are
					// non-fatal — we log them through the
					// modal output and continue, because
					// blocking a fix on a backup write would
					// be more annoying than helpful.
					if issue.Action.TouchesPATH {
						snap := pathbackup.Capture("doctor.fix", issue.Title, cmd)
						if path, err := pathbackup.Save(snap); err == nil {
							m.fixModal.BackupPath = path
						} else {
							m.fixModal.Output = "[backup warning: " + err.Error() + "]\n"
						}
					}
					m.fixModal.State = fixModalRunning
					m.fixModal.Err = nil
					return m, runHealthFixCmd(cmd)
				},
			})
			opts = append(opts, fixModalOption{
				Key:   "copy",
				Label: "Copy to clipboard",
				Desc:  "Copy the command — paste it into your shell when you're ready.",
				Run: func(m Model) (Model, tea.Cmd) {
					if err := m.clip.WriteAll(cmd); err != nil {
						m.fixModal.State = fixModalDone
						m.fixModal.Err = err
						m.fixModal.Output = "Clipboard error: " + err.Error()
						return m, nil
					}
					m.fixModal.State = fixModalDone
					m.fixModal.Output = "Command copied to clipboard. Paste it into your shell to apply."
					return m, nil
				},
			})
		}
		opts = append(opts, fixModalOptionCancel())
		return opts

	case doctor.ActionJumpPathView:
		return []fixModalOption{
			{
				Key:   "open",
				Label: "Open PATH view",
				Desc:  "Switch to Health → PATH, focused on the offending tool.",
				Run: func(m Model) (Model, tea.Cmd) {
					m.fixModal = fixModal{}
					return m.applyJumpPathFromIssue(issue), nil
				},
			},
			fixModalOptionCancel(),
		}

	case doctor.ActionRescan:
		return []fixModalOption{
			{
				Key:   "rescan",
				Label: "Rescan now",
				Desc:  "Re-walk PATH and re-resolve every installed tool. The cache file is updated on success.",
				Run: func(m Model) (Model, tea.Cmd) {
					m.fixModal = fixModal{}
					m.healthPathStatus = "Rescanning..."
					return m, m.startScan()
				},
			},
			fixModalOptionCancel(),
		}

	case doctor.ActionJumpUpdates:
		return []fixModalOption{
			{
				Key:   "open",
				Label: "Open Updates tab",
				Desc:  "Jump to My Tools → Updates and review the available upgrades.",
				Run: func(m Model) (Model, tea.Cmd) {
					m.fixModal = fixModal{}
					m.activeTab = tabUpdates
					m.cursor = 0
					m.applyFilter()
					return m, nil
				},
			},
			fixModalOptionCancel(),
		}
	}
	return nil
}

// fixModalDoneOptions returns the labelled buttons shown in the Done
// state. Order: Close (always) → Restore PATH (only when a backup
// exists and this run wasn't itself a restore — no infinite undo).
func (m Model) fixModalDoneOptions() []fixModalOption {
	opts := []fixModalOption{{
		Key:   "close",
		Label: "Close",
		Desc:  "Dismiss this dialog. Successful fixes trigger a rescan automatically.",
		Run: func(m Model) (Model, tea.Cmd) {
			return m.closeFixModalAfterDoneCmd()
		},
	}}
	if !m.fixModal.IsRestore && m.fixModal.BackupPath != "" && m.fixModal.Err == nil {
		opts = append(opts, fixModalOption{
			Key:   "restore",
			Label: "Restore previous PATH",
			Desc:  "Roll back to the PATH that was captured just before this fix ran.",
			Run: func(m Model) (Model, tea.Cmd) {
				return m.startFixModalRestore()
			},
		})
	}
	return opts
}

// closeFixModalAfterDone is the no-cmd variant used by raw key paths
// (q / esc) where we only need the new model back.
func (m Model) closeFixModalAfterDone() Model {
	newM, _ := m.closeFixModalAfterDoneCmd()
	return newM
}

// closeFixModalAfterDoneCmd dismisses the Done modal and triggers a
// rescan when the fix succeeded so the resolved issue disappears
// from the list.
func (m Model) closeFixModalAfterDoneCmd() (Model, tea.Cmd) {
	didSucceed := m.fixModal.Err == nil
	backup := m.fixModal.BackupPath
	wasRestore := m.fixModal.IsRestore
	m.fixModal = fixModal{}
	switch {
	case wasRestore && didSucceed:
		m.healthPathStatus = "↶ PATH restored — rescanning..."
		return m, m.startScan()
	case didSucceed:
		if backup != "" {
			m.healthPathStatus = "✓ Fix applied (backup at " + backup + ") — rescanning..."
		} else {
			m.healthPathStatus = "✓ Fix applied — rescanning to confirm..."
		}
		return m, m.startScan()
	}
	return m, nil
}

// startFixModalRestore runs the saved restore command for the current
// backup file. The modal stays open and transitions into a fresh
// Running → Done cycle; IsRestore tells the Done renderer to use
// rollback copy ("PATH restored" instead of "Fix applied") and to
// hide a second Restore button.
func (m Model) startFixModalRestore() (Model, tea.Cmd) {
	backups, err := pathbackup.List()
	if err != nil {
		m.fixModal.State = fixModalDone
		m.fixModal.Err = err
		m.fixModal.Output = "Could not read backup directory: " + err.Error()
		return m, nil
	}
	var target *pathbackup.Backup
	for i := range backups {
		if backups[i].File == m.fixModal.BackupPath {
			target = &backups[i]
			break
		}
	}
	if target == nil {
		m.fixModal.State = fixModalDone
		m.fixModal.Err = fmt.Errorf("backup file not found: %s", m.fixModal.BackupPath)
		return m, nil
	}
	cmd := pathbackup.RestoreCommand(*target)
	m.fixModal.State = fixModalRunning
	m.fixModal.IsRestore = true
	m.fixModal.Err = nil
	m.fixModal.Output = ""
	// Replace the displayed command so the running view shows the
	// restore command (not the original fix that was just applied).
	m.fixModal.Issue.Action = doctor.Action{
		Kind:    doctor.ActionCopyCommand,
		Label:   "Restore PATH from backup " + filepath.Base(m.fixModal.BackupPath),
		Command: cmd,
	}
	return m, runHealthFixCmd(cmd)
}

func fixModalOptionCancel() fixModalOption {
	return fixModalOption{
		Key:   "cancel",
		Label: "Cancel",
		Desc:  "Close this dialog without doing anything.",
		Run: func(m Model) (Model, tea.Cmd) {
			m.fixModal = fixModal{}
			return m, nil
		},
	}
}

// applyJumpPathFromIssue moves the user to Health → PATH with the
// cursor on the offending tool. Extracted so the modal "Open PATH
// view" button can re-use the same routing logic the direct-action
// path used.
func (m Model) applyJumpPathFromIssue(issue doctor.Issue) Model {
	m.healthSubTab = healthSubPath
	m.healthPathView = healthPathByTool
	m.healthPathShadowIdx = 0
	m.healthScroll = 0
	if issue.Action.Target != "" {
		report := pathconflict.Analyze(m.tools)
		for i, tv := range report.ByTool {
			if tv.Name == issue.Action.Target {
				m.healthPathToolIdx = i
				break
			}
		}
		m.healthPathStatus = "→ Opened PATH view focused on " + issue.Action.Target
	}
	return m
}

// runHealthFixCmd executes a shell snippet in the user's default shell
// and reports captured stdout+stderr via healthFixResultMsg. On
// Windows the snippet runs through PowerShell because the action
// generators emit PowerShell syntax there (User-PATH update via
// SetEnvironmentVariable).
func runHealthFixCmd(snippet string) tea.Cmd {
	return func() tea.Msg {
		var c *exec.Cmd
		if runtime.GOOS == "windows" {
			c = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", snippet)
		} else {
			c = exec.Command("sh", "-c", snippet)
		}
		out, err := c.CombinedOutput()
		return healthFixResultMsg{Output: string(out), Err: err}
	}
}

// handleKeyHealthFixModal routes keys while the modal is open. Up/down
// changes the highlighted button (when idle), Enter activates it,
// Esc/q closes the modal. In the running state the only safe action
// is Esc — to detach from the live exec and let it finish in the
// background (the result message still arrives, but the model ignores
// it because Open == false).
func (m Model) handleKeyHealthFixModal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.fixModal.State {
	case fixModalIdle:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "esc", "q":
			m.fixModal = fixModal{}
			return m, nil
		case "up", "k":
			if m.fixModal.Cursor > 0 {
				m.fixModal.Cursor--
			}
			return m, nil
		case "down", "j":
			if m.fixModal.Cursor < len(m.fixModal.Options)-1 {
				m.fixModal.Cursor++
			}
			return m, nil
		case "enter", " ":
			if m.fixModal.Cursor < 0 || m.fixModal.Cursor >= len(m.fixModal.Options) {
				return m, nil
			}
			opt := m.fixModal.Options[m.fixModal.Cursor]
			newM, cmd := opt.Run(m)
			return newM, cmd
		case "1", "2", "3":
			idx := int(msg.String()[0]-'1') //nolint:gosec
			if idx >= 0 && idx < len(m.fixModal.Options) {
				m.fixModal.Cursor = idx
				opt := m.fixModal.Options[idx]
				newM, cmd := opt.Run(m)
				return newM, cmd
			}
			return m, nil
		}
	case fixModalRunning:
		// Allow detaching from the live run; the result still flows
		// through (we just ignore it later via Open == false).
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "esc":
			m.fixModal = fixModal{}
			m.healthPathStatus = "Fix cancelled (it may still be running in the background)"
			return m, nil
		}
	case fixModalDone:
		// ctrl+c always quits. Up/down navigates the post-run
		// option strip (Close / Restore PATH); Enter activates the
		// selection. q/Esc map to Close.
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "q", "esc":
			return m.closeFixModalAfterDone(), nil
		case "up", "k":
			doneOpts := m.fixModalDoneOptions()
			if m.fixModal.Cursor > 0 {
				m.fixModal.Cursor--
				_ = doneOpts
			}
			return m, nil
		case "down", "j":
			doneOpts := m.fixModalDoneOptions()
			if m.fixModal.Cursor < len(doneOpts)-1 {
				m.fixModal.Cursor++
			}
			return m, nil
		case "enter", " ":
			doneOpts := m.fixModalDoneOptions()
			if m.fixModal.Cursor < 0 || m.fixModal.Cursor >= len(doneOpts) {
				return m.closeFixModalAfterDone(), nil
			}
			return doneOpts[m.fixModal.Cursor].Run(m)
		case "1", "2", "3":
			idx := int(msg.String()[0]-'1') //nolint:gosec
			doneOpts := m.fixModalDoneOptions()
			if idx >= 0 && idx < len(doneOpts) {
				m.fixModal.Cursor = idx
				return doneOpts[idx].Run(m)
			}
		}
		return m, nil
	}
	return m, nil
}

// renderFixModal returns the full-screen modal body. Layout:
//
//	┌─────────────────────────────────────────────┐
//	│ Fix: <issue title>                           │
//	│ <severity> · <category>                      │
//	│                                              │
//	│ <issue detail>                               │
//	│                                              │
//	│ Command:                                     │
//	│   $ <command line 1>                         │
//	│   $ <command line 2>                         │
//	│                                              │
//	│ ▶ Run command       Execute it here          │
//	│   Copy to clipboard Paste manually           │
//	│   Cancel            Dismiss                  │
//	└─────────────────────────────────────────────┘
//	  ↑↓ select   Enter run   Esc cancel
//
// When State == fixModalRunning it replaces the option list with a
// "Running…" spinner+banner and shows the rolling output. When
// fixModalDone it shows ✓/✗ plus the full output and a single
// "Press any key" hint.
func (m Model) renderFixModal() string {
	box := lipgloss.NewStyle().
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primaryColor)

	width := m.width - 6
	if width < 50 {
		width = 50
	}
	if width > 100 {
		width = 100
	}
	box = box.Width(width)

	var b strings.Builder

	// Header.
	title := m.fixModal.Issue.Title
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(highlightColor).Render("Fix: "+title) + "\n")
	sev := severityStyle(m.fixModal.Issue.Severity)
	b.WriteString(sev + "  " + healthDim.Render(m.fixModal.Issue.Category) + "\n")
	if m.fixModal.Issue.Detail != "" {
		b.WriteString("\n")
		for _, line := range strings.Split(m.fixModal.Issue.Detail, "\n") {
			if line == "" {
				continue
			}
			b.WriteString(healthDim.Render(line) + "\n")
		}
	}

	switch m.fixModal.State {
	case fixModalIdle:
		b.WriteString(renderFixCommandBlock(m.fixModal.Issue))
		if m.fixModal.Issue.Action.TouchesPATH {
			b.WriteString("\n" + healthAccent.Render("💾 A PATH backup will be created before this runs.") + "\n")
			b.WriteString("  " + healthDim.Render("(in ~/.klim/backups/path/, restorable from the next dialog)") + "\n")
		}
		b.WriteString("\n")
		for i, opt := range m.fixModal.Options {
			b.WriteString(renderFixOption(opt, i, i == m.fixModal.Cursor))
		}
		b.WriteString("\n" + healthDim.Render("↑↓ select   Enter run   1-9 quick pick   Esc cancel"))
	case fixModalRunning:
		b.WriteString(renderFixCommandBlock(m.fixModal.Issue))
		b.WriteString("\n" + healthAccent.Render("● Running...") + " " + healthDim.Render("(Esc to detach)") + "\n")
	case fixModalDone:
		b.WriteString("\n")
		switch {
		case m.fixModal.IsRestore && m.fixModal.Err == nil:
			b.WriteString(healthOK.Render("↶ PATH restored from backup") + "\n")
		case m.fixModal.Err != nil:
			b.WriteString(healthBad.Render("✗ Fix failed") + "\n\n")
			b.WriteString(healthDim.Render("Error: "+m.fixModal.Err.Error()) + "\n")
		default:
			b.WriteString(healthOK.Render("✓ Fix applied") + "\n")
		}
		if m.fixModal.BackupPath != "" && !m.fixModal.IsRestore {
			b.WriteString("\n" + healthAccent.Render("💾 PATH backup saved") + "\n")
			b.WriteString("  " + healthDim.Render(m.fixModal.BackupPath) + "\n")
		}
		if m.fixModal.Output != "" {
			b.WriteString("\n" + healthDim.Render("Output:") + "\n")
			b.WriteString(renderCodeBlock(m.fixModal.Output, width-6))
		}
		b.WriteString("\n")
		for i, opt := range m.fixModalDoneOptions() {
			b.WriteString(renderFixOption(opt, i, i == m.fixModal.Cursor))
		}
		b.WriteString("\n" + healthDim.Render("↑↓ select   Enter / 1-9 confirm   q/Esc close"))
	}

	rendered := box.Render(b.String())

	// Center the modal vertically and pad with blank lines so the
	// surrounding TUI chrome doesn't bleed through visibly.
	totalRows := visualRows(rendered, m.width)
	padTop := (m.height - totalRows) / 2
	if padTop < 1 {
		padTop = 1
	}
	return strings.Repeat("\n", padTop) + rendered
}

func renderFixCommandBlock(issue doctor.Issue) string {
	if issue.Action.Kind != doctor.ActionCopyCommand {
		var label string
		switch issue.Action.Kind {
		case doctor.ActionJumpPathView:
			label = "Open the interactive PATH view for " + issue.Action.Target
		case doctor.ActionRescan:
			label = "Re-walk PATH and re-resolve every tool's version"
		case doctor.ActionJumpUpdates:
			label = "Switch to My Tools → Updates"
		default:
			label = issue.Action.Label
		}
		if label == "" {
			return ""
		}
		return "\n" + healthDim.Render("Action:") + " " + label + "\n"
	}
	var b strings.Builder
	b.WriteString("\n" + healthDim.Render("Command:") + "\n")
	b.WriteString(renderCodeBlock(issue.Action.Command, 0))
	return b.String()
}

// renderFixOption renders one row of the modal's option strip. The
// active row is highlighted; row index 1-N is shown as a quick-pick
// hint for keyboard accelerators.
func renderFixOption(opt fixModalOption, idx int, selected bool) string {
	marker := "  "
	label := fmt.Sprintf("%d. %-20s", idx+1, opt.Label)
	desc := healthDim.Render(opt.Desc)
	if selected {
		marker = "▶ "
		return healthSelected.Render(marker+label+"  "+opt.Desc) + "\n"
	}
	return "  " + marker + label + "  " + desc + "\n"
}

// renderCodeBlock wraps text in a soft-bordered, dim-bg block so the
// command lines look like an embedded code snippet. width=0 means "no
// explicit width — let lipgloss size to the content".
func renderCodeBlock(content string, width int) string {
	style := lipgloss.NewStyle().
		Padding(0, 1).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(subtleColor).
		Foreground(highlightColor)
	if width > 0 {
		style = style.Width(width)
	}
	// Prefix non-empty lines with `$ ` so multi-line PowerShell
	// snippets still read like shell commands.
	var b strings.Builder
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString("$ " + line + "\n")
	}
	return style.Render(strings.TrimRight(b.String(), "\n"))
}
