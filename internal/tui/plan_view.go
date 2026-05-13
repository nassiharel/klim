package tui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/nassiharel/klim/internal/checkpoint"
	"github.com/nassiharel/klim/internal/plan"
	"github.com/nassiharel/klim/internal/registry"
)

// Plan / apply / checkpoint / rollback TUI integration.
//
// User journey:
//
//   1. From any tab the user presses `P` to open the Plan modal.
//      It renders a plan over the current tool slice through the
//      same plan.Build + plan.RenderText that powers `klim plan`.
//   2. Inside the modal:
//        a   open Apply confirmation; on confirm, shell out to
//            `klim apply` via tea.ExecProcess to inherit the full
//            safety wrapper (auto-checkpoint + postcheck).
//        c   capture a named checkpoint (prompts for the name).
//        b   open the checkpoint browser.
//        r   rebuild plan (after install/upgrade outside this view).
//        Esc / q   close.
//   3. Checkpoint browser:
//        ↑↓   navigate
//        Enter   load rollback plan for the selected checkpoint
//        d       delete
//        Esc     back to plan view
//
// Delegating the actual apply to the CLI command (rather than
// re-implementing the safety wrapper inside the TUI) keeps the
// checkpoint + postcheck behaviour consistent across both surfaces.

// planLoadedMsg carries the result of a plan rebuild back to the
// Update loop.
type planLoadedMsg struct {
	plan   *plan.Plan
	mode   string
	target string
}

// checkpointsLoadedMsg carries the result of a checkpoint list load.
type checkpointsLoadedMsg struct {
	list []checkpoint.Checkpoint
	err  error
}

// applyFinishedMsg fires after `klim apply` returns control.
type applyFinishedMsg struct {
	err error
}

// --- styles ---

var (
	planTitle    = lipgloss.NewStyle().Bold(true).Foreground(cyberFG)
	planDim      = lipgloss.NewStyle().Foreground(cyberFGDim)
	planAccent   = lipgloss.NewStyle().Foreground(cyberPrimary)
	planSelected = lipgloss.NewStyle().Foreground(cyberFG).Background(cyberSelectedBg).Bold(true)
)

// --- commands ---

// buildPlanCmd computes a plan over the current tool slice. plan.Build
// is pure and microseconds-fast, but we still wrap it in a Cmd so
// the renderer can react to its result with the same machinery as
// any other tea.Msg.
func buildPlanCmd(tools []registry.Tool) tea.Cmd {
	return func() tea.Msg {
		p := plan.Build(tools, plan.Options{IncludeUpgrades: true})
		return planLoadedMsg{plan: &p, mode: "forward"}
	}
}

// buildRollbackPlanCmd computes the restore plan against a saved
// checkpoint. Mirrors the CLI's `klim rollback <name>` logic.
func buildRollbackPlanCmd(name string, tools []registry.Tool) tea.Cmd {
	return func() tea.Msg {
		cp, err := checkpoint.Load(name)
		if err != nil {
			return planLoadedMsg{plan: &plan.Plan{}, mode: "rollback", target: name}
		}
		desired := make(map[string]plan.DesiredState, len(cp.Tools))
		for _, t := range cp.Tools {
			desired[t.Name] = plan.DesiredState{Version: t.Version}
		}
		p := plan.Build(tools, plan.Options{
			IncludeInstalls: true,
			IncludeUpgrades: true,
			Desired:         desired,
		})
		return planLoadedMsg{plan: &p, mode: "rollback", target: name}
	}
}

// loadCheckpointsCmd reads every saved checkpoint from disk.
func loadCheckpointsCmd() tea.Cmd {
	return func() tea.Msg {
		list, err := checkpoint.List()
		return checkpointsLoadedMsg{list: list, err: err}
	}
}

// captureCheckpointCmd captures + saves a named snapshot using the
// current tool slice. Runs sync in a Cmd so the Update loop stays
// the single mutation site for model state.
func captureCheckpointCmd(name string, tools []registry.Tool) tea.Cmd {
	return func() tea.Msg {
		cp := checkpoint.Capture(name, "Captured from TUI", tools)
		if _, err := checkpoint.Save(cp); err != nil {
			return checkpointsLoadedMsg{err: err}
		}
		// Reload list so the browser refreshes.
		list, _ := checkpoint.List()
		return checkpointsLoadedMsg{list: list}
	}
}

// deleteCheckpointCmd removes a checkpoint by name and reloads the
// list. Reuses checkpointsLoadedMsg as the result channel so the
// browser can re-render without a dedicated message type.
func deleteCheckpointCmd(name string) tea.Cmd {
	return func() tea.Msg {
		if err := checkpoint.Delete(name); err != nil {
			return checkpointsLoadedMsg{err: err}
		}
		list, _ := checkpoint.List()
		return checkpointsLoadedMsg{list: list}
	}
}

// runKlimApplyCmd shells out to `klim apply` so the user gets the
// full safety wrapper (auto-checkpoint + postcheck + regression
// detection). We avoid re-implementing that pipeline inside the TUI
// because the cross-platform branches and the postcheck regression
// classification are already proven in the CLI.
func runKlimApplyCmd() tea.Cmd {
	exe, err := exec.LookPath("klim")
	if err != nil {
		return func() tea.Msg { return applyFinishedMsg{err: err} }
	}
	cmd := exec.Command(exe, "apply", "--yes")
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return applyFinishedMsg{err: err}
	})
}

// --- view rendering ---

// renderPlanView is a full-screen modal showing the plan text. It
// handles forward plans, rollback plans, and the various transient
// states (loading, post-apply, etc.).
func (m Model) renderPlanView() string {
	width := m.width - 6
	if width < 60 {
		width = 60
	}
	if width > 110 {
		width = 110
	}

	var b strings.Builder

	title := "Plan"
	if m.planMode == "rollback" {
		title = "Rollback plan → " + m.planRollbackTarget
	}
	b.WriteString(planTitle.Render(title) + "  " +
		planDim.Render("preview of what would change — read-only") + "\n\n")

	if m.planStatus != "" {
		b.WriteString(planAccent.Render(m.planStatus) + "\n\n")
	}

	if m.enteringCheckpointName {
		b.WriteString(planDim.Render("Checkpoint name") + "  " + m.checkpointNameInput.View() + "\n")
		b.WriteString(planDim.Render("Enter to save · Esc to cancel") + "\n")
		return wrapModal(b.String(), width, m)
	}

	if m.applyConfirm {
		b.WriteString(planAccent.Render("Apply this plan?") + "\n")
		b.WriteString(planDim.Render("klim apply will: (1) capture pre-apply-<UTC> checkpoint, (2) upgrade, (3) postcheck.") + "\n\n")
		b.WriteString(planDim.Render("y to confirm · n / Esc to cancel") + "\n")
		return wrapModal(b.String(), width, m)
	}

	if m.planResult == nil {
		b.WriteString(planDim.Render("Building plan…"))
		return wrapModal(b.String(), width, m)
	}

	body := plan.RenderText(*m.planResult)
	// Fit body to the available rows so the action strip stays
	// pinned at the bottom of the modal regardless of plan length.
	headerRows := 4
	if m.planStatus != "" {
		headerRows += 2
	}
	footerRows := 6 // action strip + key hints
	bodyHeight := m.height - headerRows - footerRows - 4
	if bodyHeight < 8 {
		bodyHeight = 8
	}
	fitted, _, _ := fitToVisibleRows(body, m.planScroll, bodyHeight)
	b.WriteString(fitted + "\n")

	// Action strip.
	b.WriteString("\n" + planDim.Render("Actions") + "\n")
	if m.planMode == "rollback" {
		b.WriteString(planDim.Render("  Rollback execution is CLI-only — copy the suggested commands or run") + "\n")
		b.WriteString(planDim.Render("  `klim rollback "+m.planRollbackTarget+"` in your shell to see them.") + "\n")
	} else {
		b.WriteString("  " + planAccent.Render("a") + planDim.Render("  Apply (runs klim apply with checkpoint + postcheck)") + "\n")
	}
	b.WriteString("  " + planAccent.Render("c") + planDim.Render("  Capture a named checkpoint") + "\n")
	b.WriteString("  " + planAccent.Render("b") + planDim.Render("  Browse / restore from a saved checkpoint") + "\n")
	b.WriteString("  " + planAccent.Render("r") + planDim.Render("  Rebuild this plan") + "\n")
	b.WriteString("  " + planAccent.Render("Esc / q") + planDim.Render("  Close") + "\n")

	return wrapModal(b.String(), width, m)
}

// renderCheckpointBrowser shows the saved checkpoints with a cursor
// that navigates them. Enter loads the rollback plan, d deletes, Esc
// returns to the plan view.
func (m Model) renderCheckpointBrowser() string {
	width := m.width - 6
	if width < 60 {
		width = 60
	}
	if width > 110 {
		width = 110
	}
	var b strings.Builder
	b.WriteString(planTitle.Render("Checkpoints") + "  " +
		planDim.Render("snapshots you can restore") + "\n\n")
	if len(m.checkpointsList) == 0 {
		b.WriteString(planDim.Render("No checkpoints saved yet.") + "\n")
		b.WriteString(planDim.Render("Capture one from the Plan view (press c).") + "\n\n")
		b.WriteString(planDim.Render("Esc to back to plan · q to close") + "\n")
		return wrapModal(b.String(), width, m)
	}
	for i, cp := range m.checkpointsList {
		desc := cp.Description
		if desc == "" {
			desc = "—"
		}
		line := fmt.Sprintf("%-26s  %s  %d tools  %s",
			cp.Name,
			cp.CreatedAt.Local().Format("2006-01-02 15:04"),
			len(cp.Tools),
			planDim.Render(desc),
		)
		if i == m.checkpointCursor {
			b.WriteString(planSelected.Render("▶ "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	b.WriteString("\n" + planDim.Render("↑↓ navigate · Enter to preview rollback · d to delete · Esc back · q close") + "\n")
	return wrapModal(b.String(), width, m)
}

// wrapModal wraps body in a rounded border and centres it vertically
// when there's room. Reuses the same approach the Health fix modal
// uses so both modals feel consistent.
func wrapModal(body string, width int, m Model) string {
	box := lipgloss.NewStyle().
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(primaryColor).
		Width(width).
		Render(body)
	totalRows := visualRows(box, m.width)
	padTop := (m.height - totalRows) / 2
	if padTop < 1 {
		padTop = 1
	}
	if padTop+totalRows >= m.height {
		padTop = 0
	}
	return strings.Repeat("\n", padTop) + box
}

// --- key handling ---

// handleKeyPlanView is the dispatcher used when m.viewingPlan == true.
// Forwards to the checkpoint browser when that nested modal is also
// open, then to the per-state key handlers (input mode, confirm
// mode, idle mode).
func (m Model) handleKeyPlanView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.viewingCheckpoints {
		return m.handleKeyCheckpointBrowser(msg)
	}
	if m.enteringCheckpointName {
		return m.handleKeyCheckpointNameInput(msg)
	}
	if m.applyConfirm {
		return m.handleKeyApplyConfirm(msg)
	}

	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc", "q":
		m.viewingPlan = false
		m.planResult = nil
		m.planScroll = 0
		m.planMode = ""
		m.planRollbackTarget = ""
		m.planStatus = ""
		return m, nil
	case "up", "k":
		if m.planScroll > 0 {
			m.planScroll--
		}
		return m, nil
	case "down", "j":
		m.planScroll++
		return m, nil
	case "home", "g":
		m.planScroll = 0
		return m, nil
	case "pgup":
		m.planScroll -= 10
		if m.planScroll < 0 {
			m.planScroll = 0
		}
		return m, nil
	case "pgdown", " ":
		m.planScroll += 10
		return m, nil
	case "r":
		m.planResult = nil
		m.planStatus = "Rebuilding plan…"
		return m, buildPlanCmd(m.tools)
	case "a":
		if m.planMode == "rollback" {
			m.planStatus = "Rollback execution is CLI-only. Run `klim rollback " + m.planRollbackTarget + "`."
			return m, nil
		}
		if m.planResult == nil || len(m.planResult.Changes) == 0 {
			m.planStatus = "Nothing to apply — plan is empty."
			return m, nil
		}
		m.applyConfirm = true
		return m, nil
	case "c":
		ti := textinput.New()
		ti.Placeholder = "before-upgrade-" + time.Now().UTC().Format("0102-1504")
		ti.SetWidth(40)
		ti.Focus()
		m.checkpointNameInput = ti
		m.enteringCheckpointName = true
		return m, textinput.Blink
	case "b":
		m.viewingCheckpoints = true
		return m, loadCheckpointsCmd()
	}
	return m, nil
}

func (m Model) handleKeyApplyConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.applyConfirm = false
		m.planStatus = "Running klim apply…"
		return m, runKlimApplyCmd()
	case "n", "N", "esc":
		m.applyConfirm = false
		return m, nil
	}
	return m, nil
}

func (m Model) handleKeyCheckpointNameInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.enteringCheckpointName = false
		m.checkpointNameInput.SetValue("")
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.checkpointNameInput.Value())
		if name == "" {
			name = m.checkpointNameInput.Placeholder
		}
		m.enteringCheckpointName = false
		m.checkpointNameInput.SetValue("")
		m.planStatus = "Captured checkpoint: " + name
		return m, captureCheckpointCmd(name, m.tools)
	}
	var cmd tea.Cmd
	m.checkpointNameInput, cmd = m.checkpointNameInput.Update(msg)
	return m, cmd
}

func (m Model) handleKeyCheckpointBrowser(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "esc":
		m.viewingCheckpoints = false
		return m, nil
	case "q":
		m.viewingCheckpoints = false
		m.viewingPlan = false
		return m, nil
	case "up", "k":
		if m.checkpointCursor > 0 {
			m.checkpointCursor--
		}
		return m, nil
	case "down", "j":
		if m.checkpointCursor < len(m.checkpointsList)-1 {
			m.checkpointCursor++
		}
		return m, nil
	case "enter":
		if m.checkpointCursor >= len(m.checkpointsList) {
			return m, nil
		}
		cp := m.checkpointsList[m.checkpointCursor]
		m.viewingCheckpoints = false
		m.planResult = nil
		m.planScroll = 0
		m.planStatus = "Building rollback plan…"
		return m, buildRollbackPlanCmd(cp.Name, m.tools)
	case "d":
		if m.checkpointCursor >= len(m.checkpointsList) {
			return m, nil
		}
		name := m.checkpointsList[m.checkpointCursor].Name
		return m, deleteCheckpointCmd(name)
	}
	return m, nil
}

// openPlanView is called from the global key router when `P` is
// pressed outside any other modal. It seeds an empty plan + kicks
// off the build cmd; the renderer shows "Building plan…" until the
// message arrives.
func (m *Model) openPlanView() tea.Cmd {
	m.viewingPlan = true
	m.planResult = nil
	m.planScroll = 0
	m.planMode = "forward"
	m.planRollbackTarget = ""
	m.planStatus = ""
	return buildPlanCmd(m.tools)
}

// helpers — pull these in to silence unused-import warnings without
// further plumbing if the user removes a feature later.
var (
	_ = filepath.Base
	_ = lipgloss.NewStyle
)
