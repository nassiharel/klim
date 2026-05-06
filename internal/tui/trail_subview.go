package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/nassiharel/klim/internal/registry"
	"github.com/nassiharel/klim/internal/service"
	"github.com/nassiharel/klim/internal/trail"
)

// Trail sub-view states.
//
// trailViewIdle shows the entry list. trailViewLabelInput captures a
// label before firing the capture command.
const (
	trailViewIdle       = ""
	trailViewLabelInput = "label-input"
)

// trailLogResultMsg arrives after an async trail.Log call.
type trailLogResultMsg struct {
	entries []trail.Entry
	err     error
}

// trailCaptureResultMsg arrives after an async trail.Capture.
type trailCaptureResultMsg struct {
	entry *trail.Entry
	err   error
}

// startTrailSubview enters the trail list view and queues a fresh log
// load. The previous state is left untouched so the screen never
// flashes empty between views — the next render uses the existing
// trailEntries (initially nil → "Loading...") until the load returns.
func (m *Model) startTrailSubview() tea.Cmd {
	m.viewingTrail = true
	m.trailState = trailViewIdle
	m.trailError = ""
	m.trailCursor = 0
	return loadTrailLogCmd(0)
}

// handleKeyTrail dispatches keystrokes for the trail sub-view.
func (m Model) handleKeyTrail(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.trailState == trailViewLabelInput {
		return m.handleKeyTrailLabel(msg)
	}
	switch msg.String() {
	case "esc", "q", "backspace":
		m.viewingTrail = false
		m.trailState = trailViewIdle
		m.statusMsg = ""
		return m, nil
	case "up", "k":
		if m.trailCursor > 0 {
			m.trailCursor--
		}
	case "down", "j":
		if m.trailCursor < len(m.trailEntries)-1 {
			m.trailCursor++
		}
	case "r":
		// Refresh log from disk.
		m.statusMsg = "Loading trail..."
		return m, loadTrailLogCmd(0)
	case "c":
		// Begin capture: prompt for an optional label first so the
		// user can tag the entry without dropping to the CLI. We
		// don't auto-capture without input because the label is
		// the most-loved feature of trail (per docs/trail.md).
		ti := textinput.New()
		ti.Placeholder = "optional label (e.g. before-kubectl-upgrade)"
		ti.CharLimit = 80
		ti.SetWidth(50)
		m.trailLabelInput = ti
		m.trailState = trailViewLabelInput
		return m, m.trailLabelInput.Focus()
	}
	return m, nil
}

// handleKeyTrailLabel collects the label text and fires the capture.
func (m Model) handleKeyTrailLabel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.trailState = trailViewIdle
		return m, nil
	case "enter":
		label := strings.TrimSpace(m.trailLabelInput.Value())
		m.trailState = trailViewIdle
		m.statusMsg = "Capturing trail entry..."
		return m, captureTrailCmd(m.svc, label)
	}
	var cmd tea.Cmd
	m.trailLabelInput, cmd = m.trailLabelInput.Update(msg)
	return m, cmd
}

// renderTrailSubview is the entry point used by view_backup.go.
func (m Model) renderTrailSubview() string {
	if m.trailState == trailViewLabelInput {
		return m.renderTrailLabelInput()
	}
	return m.renderTrailList()
}

func (m Model) renderTrailList() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  " + detailTitleStyle.Render("Trail History") + "  " +
		dimVersion.Render("git-style snapshots of your toolchain") + "\n\n")

	if m.trailError != "" {
		b.WriteString("  " + complianceErrorStyle.Render("✗ "+m.trailError) + "\n\n")
	}

	if m.trailEntries == nil && m.trailError == "" {
		b.WriteString("  " + dimVersion.Render("Loading...") + "\n")
		return b.String()
	}
	if len(m.trailEntries) == 0 {
		b.WriteString("  " + dimVersion.Render("No trail entries yet — press c to capture the current toolchain.") + "\n")
	} else {
		// Header row.
		b.WriteString("  " + headerStyle.Render(fixedWidth("WHEN", 19)) + "  " +
			headerStyle.Render(fixedWidth("OP", 9)) + "  " +
			headerStyle.Render(fixedWidth("HASH", 9)) + "  " +
			headerStyle.Render(fixedWidth("LABEL", 22)) + "  " +
			headerStyle.Render("SUMMARY") + "\n")

		visibleRows := m.height - 14 - m.footerHeight()
		if visibleRows < 4 {
			visibleRows = 4
		}
		start := 0
		if m.trailCursor >= visibleRows {
			start = m.trailCursor - visibleRows + 1
		}
		end := start + visibleRows
		if end > len(m.trailEntries) {
			end = len(m.trailEntries)
		}

		for i := start; i < end; i++ {
			e := m.trailEntries[i]
			when := e.Time.Local().Format("2006-01-02 15:04")
			op := e.Operation
			if op == "" {
				op = "capture"
			}
			hash := string(e.Object)
			if len(hash) > 7 {
				hash = hash[:7]
			}
			label := e.Label
			if label == "" {
				label = "—"
			}
			summary := e.Summary
			if summary == "" {
				summary = "—"
			}
			cursor := "  "
			if i == m.trailCursor {
				cursor = "▸ "
			}
			line := cursor +
				dimVersion.Render(fixedWidth(when, 19)) + "  " +
				nameStyle.Render(fixedWidth(op, 9)) + "  " +
				dimVersion.Render(fixedWidth(hash, 9)) + "  " +
				dimVersion.Render(fixedWidth(label, 22)) + "  " +
				dimVersion.Render(summary)
			if i == m.trailCursor {
				line = selectedRowStyle.Render(line)
			}
			b.WriteString(line + "\n")
		}
	}
	b.WriteString("\n  " + detailLabelStyle.Render("Actions") + "\n")
	b.WriteString("    " + dimVersion.Render("c") + "  capture current toolchain\n")
	b.WriteString("    " + dimVersion.Render("r") + "  reload\n")
	b.WriteString("    " + dimVersion.Render("Esc") + "  back to Backup menu\n")
	return b.String()
}

func (m Model) renderTrailLabelInput() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  " + detailTitleStyle.Render("Capture trail entry") + "\n\n")
	b.WriteString("  " + confirmStyle.Render("Label (optional):") + "  " + m.trailLabelInput.View() + "\n\n")
	b.WriteString("  " + dimVersion.Render("Enter") + "  capture   " +
		dimVersion.Render("Esc") + "  cancel\n")
	return b.String()
}

// --- Commands ---

// loadTrailLogCmd reads the trail log async. limit=0 means no limit
// (TUI shows all entries with internal scrolling — no need for the
// CLI's page size).
func loadTrailLogCmd(limit int) tea.Cmd {
	return func() tea.Msg {
		entries, err := trail.Log(trail.LogOptions{Limit: limit})
		return trailLogResultMsg{entries: entries, err: err}
	}
}

// captureTrailCmd records the current toolchain. Uses a fresh PATH
// scan (matching the CLI's default), so the captured snapshot reflects
// the user's actual toolchain at the moment of capture rather than
// stale cache content.
func captureTrailCmd(svc *service.ToolService, label string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		tools, _, err := svc.ScanOnly(ctx)
		if err != nil {
			return trailCaptureResultMsg{err: fmt.Errorf("scanning tools: %w", err)}
		}
		entry, err := trail.Capture(trail.OpCapture, label, tools)
		if err != nil {
			return trailCaptureResultMsg{err: err}
		}
		return trailCaptureResultMsg{entry: entry}
	}
}

// touch keeps the package-local registry import live in case the
// trail package adds tool-shape helpers we want to surface later.
var _ = registry.Tool{}
