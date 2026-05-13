package tui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// renderScanStrip returns a one-line cyber loading strip when the
// app is mid-version-resolution (phaseResolving), running a batch
// op, or executing a fix-modal command. Returns "" when nothing's
// in flight so the slot collapses back to a blank gap.
//
// Note: phaseLoading is intentionally NOT covered here. During the
// initial PATH scan / catalog load the full-screen boot splash
// takes over the view entirely; the strip would never be visible.
//
// Layout:
//
//	⟦ ◜ ⟧  RESOLVING VERSIONS  [▓▓▓▓▓░░░░░░░░░]  47/149 tools
//
// The strip is one row tall by construction so every layout
// computation that budgets "blank(1)" stays correct.
func (m Model) renderScanStrip() string {
	mode, ok := m.scanStripMode()
	if !ok {
		return ""
	}
	return mode.render(m)
}

// scanStripMode picks the most relevant in-flight activity to
// display in the strip. Returns (mode, true) when there's something
// to show, ({}, false) otherwise. Priority: scan/resolve first
// (because the user just pressed `r`), then batch upgrade, then
// fix-modal exec.
func (m Model) scanStripMode() (scanStripState, bool) {
	if m.phase == phaseResolving && m.pending > 0 {
		// pending = installed tools still resolving. Compute the
		// resolved count from m.stats() at the call site so the
		// strip's progress matches what the user expects.
		return scanStripState{
			label:   "RESOLVING VERSIONS",
			unit:    "tools",
			pending: m.pending,
		}, true
	}
	if m.activeBatch != nil && m.activeBatch.isRunning() {
		total := len(m.activeBatch.items)
		done := m.activeBatch.done
		return scanStripState{
			label:   strings.ToUpper(m.activeBatch.label),
			unit:    "items",
			pending: total - done,
			total:   total,
			done:    done,
		}, true
	}
	if m.fixModal.Open && m.fixModal.State == fixModalRunning {
		return scanStripState{
			label:   "APPLYING FIX",
			unit:    "command",
			pending: 1,
			total:   1,
		}, true
	}
	return scanStripState{}, false
}

// scanStripState is a small mode struct so each kind of in-flight
// work can describe its progress in the same shape.
type scanStripState struct {
	label string
	unit  string

	pending int // items not yet done
	total   int // total items (0 = derive from m.stats inside render)
	done    int // items completed (0 = derive)
}

func (s scanStripState) render(m Model) string {
	// Derive total / done if not explicitly set (scan/resolve path).
	total := s.total
	done := s.done
	if total == 0 {
		inst, _, _ := m.stats()
		total = inst
		done = inst - s.pending
	}
	if total <= 0 {
		// Defensive: avoid divide-by-zero in the bar width math.
		return ""
	}

	// Budget: strip must fit in m.width so it stays exactly one
	// row tall (every per-tab layout calc budgets blank(1) for
	// this slot). Subtract 2 cells for the leading indent.
	budget := m.width - 2
	if budget < 20 {
		// Terminal too narrow to render any sensible strip. Let
		// the caller fall back to a blank gap.
		return ""
	}

	spinner := cyberSpinnerStyle.Render(spinnerArc(m.animFrame))
	label := scanStripLabelStyle.Render(s.label)
	bar := scanProgressBar(done, total, scanBarCells, m.animFrame)
	count := hudValueStyle.Render(strconv.Itoa(done)+hudLabelDimSlash) +
		hudLabelStyle.Render(strconv.Itoa(total)) + " " +
		hudLabelStyle.Render(s.unit)

	bracketOpen := cyberTabBracketStyle.Render("⟦")
	bracketClose := cyberTabBracketStyle.Render("⟧")
	spinnerCell := bracketOpen + " " + spinner + " " + bracketClose

	// Try ordered fallbacks from richest to leanest until one fits
	// in the available cell budget. Returning the lean form is
	// strictly better than letting the strip wrap to 2+ rows and
	// silently breaking the header-row budgeting that downstream
	// per-tab layout code relies on.
	candidates := []string{
		spinnerCell + "  " + label + "  " + bar + "  " + count,
		spinnerCell + "  " + label + "  " + count,
		spinnerCell + "  " + count,
		spinnerCell,
	}
	for _, c := range candidates {
		if visualLen(c) <= budget {
			return "  " + c
		}
	}
	return ""
}

// scanBarCells is the visual width of the progress bar in cells.
// Chosen to fit comfortably alongside the rest of the strip on 80-
// column terminals.
const scanBarCells = 18

// hudLabelDimSlash is a single styled "/" used between value and
// total in the strip's "done/total" counter — extracted so the
// render path doesn't allocate a fresh style per call.
//
// (Style itself is created via hudLabelStyle which is already a
// shared token.)
var hudLabelDimSlash = hudLabelStyle.Render("/")

// scanStripLabelStyle is the strip's headline style: bright cyan,
// bold, lightly spaced — meant to read as a status banner without
// drawing more attention than the actual progress data.
var scanStripLabelStyle = lipgloss.NewStyle().
	Foreground(cyberPrimary).
	Bold(true)

// spinnerArc returns the rotating-arc glyph for the given animation
// frame. Mirrors the cyberSpinner used by the boot splash so the
// visual vocabulary stays consistent across loading surfaces.
func spinnerArc(frame int) string {
	const frames = "◜◝◞◟"
	r := []rune(frames)
	return string(r[(frame/2)%len(r)])
}

// scanProgressBar renders a filled / unfilled bar of `cells` cells.
// `done` cells are bright; `total - done` cells are dim. A single
// "scanline" cell pulses through the filled region every frame to
// hint that progress is live (not stuck).
//
// All inputs are pre-validated by the caller (total > 0).
func scanProgressBar(done, total, cells, frame int) string {
	if cells < 4 {
		cells = 4
	}
	filled := done * cells / total
	if filled > cells {
		filled = cells
	}
	if filled < 0 {
		filled = 0
	}
	// Scanline: when there's at least one filled cell, slide a
	// bright "head" through the dim region just past the fill so
	// the eye registers continued motion even when `done` hasn't
	// changed.
	headPos := -1
	if filled < cells {
		headPos = filled + ((frame / 2) % (cells - filled))
	}

	var b strings.Builder
	b.WriteString(scanBarBracketStyle.Render("["))
	for i := 0; i < cells; i++ {
		switch {
		case i < filled:
			b.WriteString(scanBarFilledStyle.Render("▓"))
		case i == headPos:
			b.WriteString(scanBarHeadStyle.Render("▒"))
		default:
			b.WriteString(scanBarEmptyStyle.Render("░"))
		}
	}
	b.WriteString(scanBarBracketStyle.Render("]"))
	return b.String()
}

// Style tokens for the scan progress bar. Prebuilt so the hot path
// (one per cell, per frame) doesn't allocate.
var (
	scanBarBracketStyle = lipgloss.NewStyle().Foreground(cyberPrimaryDim).Bold(true)
	scanBarFilledStyle  = lipgloss.NewStyle().Foreground(cyberPrimary).Bold(true)
	scanBarHeadStyle    = lipgloss.NewStyle().Foreground(cyberInfo).Bold(true)
	scanBarEmptyStyle   = lipgloss.NewStyle().Foreground(cyberRuleBright)
)
