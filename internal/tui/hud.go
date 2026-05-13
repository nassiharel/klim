package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
)

// renderCyberHUD renders the cyber-styled header: a bracket-framed
// brand on the left, a live status grid in the middle (TOOLS /
// UPDATES / HEALTH), and a pulsing activity dot on the right. The
// frame wraps in matching brackets so the whole row reads as a
// single HUD strip.
//
// Layout:
//
//	╭─[ KLIM // READY ]──── 149/149 TOOLS ┃ 3 UPDATES ┃ HEALTHY ─── ◉ ─╮
//
// Falls back gracefully on narrow terminals: drops segments from the
// right (least-critical first) until the line fits the available
// width. The bracket "╭─" / "─╮" terminators are always kept so the
// HUD silhouette stays recognisable.
func (m Model) renderCyberHUD() string {
	if m.width < 30 {
		// Too narrow for a HUD — fall back to a minimal brand line.
		return "  " + hudBrandStyle.Render("KLIM")
	}

	brand := buildBrand(m.phase, m.pending)
	segments := m.buildHUDSegments()
	// Pulse only while there's something to signal. When the app
	// is fully idle (scan done, no pending work), show a static
	// dot so the HUD stays visually quiet and the animation loop
	// can pause without losing the indicator.
	dot := staticDot()
	if m.animationActive() {
		dot = pulseDot(m.animFrame)
	}

	// Outer frame characters.
	left := hudBracketStyle.Render("╭─")
	right := hudBracketStyle.Render("─╮")
	sep := hudSepStyle.Render("─")

	// Build right-side: dot + frame end.
	rightSide := sep + " " + dot + " " + right

	// Build left-side: frame start + brand.
	leftSide := left + " " + brand

	// Try to fit all segments; drop from the right until it fits.
	for len(segments) > 0 {
		mid := joinSegments(segments)
		// Approximate visible width with runewidth-aware visualRows
		// helper (single-line content).
		probe := leftSide + " " + mid + " " + rightSide
		// 8 = padding budget for the two-space indent prefix below.
		if visualLen(probe) <= m.width-2 {
			return "  " + probe
		}
		segments = segments[:len(segments)-1]
	}
	// All segments dropped — render brand + dot only.
	return "  " + leftSide + " " + rightSide
}

// buildBrand renders the brand pill, swapping in a scan-status word
// while the initial discovery is running so the user gets immediate
// confirmation that work is happening.
func buildBrand(phase int, pending int) string {
	const brandName = "KLIM"
	var status string
	switch phase {
	case phaseLoading:
		status = "// SCAN"
	case phaseResolving:
		status = fmt.Sprintf("// RESOLVE (%d)", pending)
	default:
		status = "// READY"
	}
	return hudBracketStyle.Render("[") + " " +
		hudBrandStyle.Render(brandName) + " " +
		hudLabelStyle.Render(status) + " " +
		hudBracketStyle.Render("]")
}

// buildHUDSegments returns the HUD's middle status segments in
// priority order (most important FIRST). renderCyberHUD drops
// segments from the end of this slice until the line fits, so
// putting the most-critical info first guarantees it survives the
// longest under narrow-terminal pressure.
func (m Model) buildHUDSegments() []string {
	var segs []string

	// 1. Tools count — the headline number. Most important.
	inst, upd, notInst := m.stats()
	active := inst + notInst
	segs = append(segs,
		hudValueStyle.Render(strconv.Itoa(inst))+hudLabelStyle.Render("/")+
			hudValueStyle.Render(strconv.Itoa(active))+" "+hudLabelStyle.Render("TOOLS"))

	// 2. Updates count — only when there's something actionable.
	if upd > 0 {
		segs = append(segs, hudAlertStyle.Render(fmt.Sprintf("%d UPDATES", upd)))
	}

	// 3. Health badge — secondary signal.
	segs = append(segs, m.healthBadge())

	return segs
}

// healthBadge renders a short "HEALTHY" / "N ISSUES" / "—" badge.
func (m Model) healthBadge() string {
	if !m.doctorChecked {
		return hudLabelStyle.Render("HEALTH ") + hudLabelStyle.Render("—")
	}
	if n := len(m.doctorIssues); n > 0 {
		return hudAlertStyle.Render(fmt.Sprintf("%d ISSUES", n))
	}
	return hudOKStyle.Render("HEALTHY")
}

// joinSegments places the cyber-sep between every middle segment so
// the HUD always reads as a continuous strip rather than a string of
// disconnected pills.
func joinSegments(segs []string) string {
	var b strings.Builder
	for i, s := range segs {
		if i > 0 {
			b.WriteString(" ")
			b.WriteString(hudSepStyle.Render("┃"))
			b.WriteString(" ")
		}
		b.WriteString(s)
	}
	return b.String()
}

// visualLen returns the printable column width of s, stripping ANSI
// SGR sequences so styled text reports its real on-screen width.
// Uses runewidth so wide characters (CJK, certain emoji) count their
// double-column footprint correctly.
//
// We assume only SGR sequences (ESC [ ... m) appear in lipgloss
// output — true for every style token we emit. Other escape types
// would need a more elaborate parser, but lipgloss never produces
// them.
func visualLen(s string) int {
	var b strings.Builder
	skip := false
	for _, r := range s {
		if skip {
			// SGR final byte is always 'm'. Any earlier
			// candidate-final byte (like '[' or ';') is part of
			// the parameter sequence and must NOT end the skip.
			if r == 'm' {
				skip = false
			}
			continue
		}
		if r == 0x1b {
			skip = true
			continue
		}
		b.WriteRune(r)
	}
	return runewidth.StringWidth(b.String())
}
