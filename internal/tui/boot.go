package tui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)

// bootSplashMinDuration is the minimum wall-clock time the boot
// splash stays on screen, even if the catalog load + PATH scan
// finishes faster (e.g. on a warm cache hit). Gives the brand
// reveal animation time to actually be seen.
const bootSplashMinDuration = 3 * time.Second

// klimASCII is the bootscreen logo. Each row must have the same rune
// count so the reveal animation progresses left-to-right at a stable
// rate; renderBootSplash reads the first row's rune count as the
// canonical width.
var klimASCII = []string{
	"  ██  ██  ██       ██ ██▄  ▄██  ",
	"  ██▄██   ██       ██ ██ ▀▀ ██  ",
	"  ████    ██       ██ ██    ██  ",
	"  ██▀██   ██       ██ ██    ██  ",
	"  ██  ██  ██▄▄▄▄▄  ██ ██    ██  ",
	"  ▀▀  ▀▀  ▀▀▀▀▀▀▀  ▀▀ ▀▀    ▀▀  ",
}

// renderBootSplash draws a full-screen, cyber-styled boot splash:
// the KLIM logo with a left-to-right character reveal animation, a
// brand subtitle ("// Reignite Dev Experience"), a
// rotating cyber spinner, and a horizontal boot-progress bar that
// fills left-to-right.
//
// The splash is shown while m.phase == phaseLoading OR while less
// than bootSplashMinDuration has elapsed since model start, whichever
// is longer — so cache-hit launches still display the brand reveal
// for a noticeable beat instead of flashing past.
func (m Model) renderBootSplash() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	// Reveal progress: each frame uncovers 4 more cells of the logo,
	// so the full ~36-cell line is exposed in 9 frames (~900 ms).
	// After full reveal we keep the scanline animating until the
	// real boot finishes.
	reveal := m.animFrame * 4
	// Convert the first row to runes to compute the canonical width
	// (every row is the same rune-length by construction).
	rowRunes := []rune(klimASCII[0])
	maxRow := len(rowRunes)
	if reveal > maxRow {
		reveal = maxRow
	}

	// Build the logo: each row gets the first `reveal` cells styled
	// bright; cells past `reveal` are not emitted, so the row looks
	// like it's being typed in from the left. Slicing operates on
	// rune indices so multi-byte glyphs (█ = 3 UTF-8 bytes) stay
	// intact.
	var logoBuf strings.Builder
	for i, row := range klimASCII {
		runes := []rune(row)
		end := reveal
		if end > len(runes) {
			end = len(runes)
		}
		visible := string(runes[:end])
		// Halo: render the last 3 visible cells in the dim halo
		// color so the leading edge "glows" as it moves.
		var styled string
		if end > 3 {
			styled = bootGlowStyle.Render(string(runes[:end-3])) + bootHaloStyle.Render(string(runes[end-3:end]))
		} else {
			styled = bootHaloStyle.Render(visible)
		}
		if i > 0 {
			logoBuf.WriteString("\n")
		}
		logoBuf.WriteString(styled)
	}
	logo := logoBuf.String()

	// Subtitle pill — appears once the logo is fully revealed, so
	// the eye lands on it after the reveal completes.
	subtitle := ""
	if reveal >= maxRow {
		spinner := cyberSpinner(m.animFrame)
		brand := bootSubtitleStyle.Render("Reignite Dev Experience")
		boot := bootScanStyle.Render(scanProgress(m.animFrame))
		subtitle = "  " + spinner + "  " + brand + "  " + boot
	}

	// Centre the logo block vertically inside the terminal.
	verticalPad := (m.height - len(klimASCII) - 4) / 2
	if verticalPad < 1 {
		verticalPad = 1
	}
	pad := strings.Repeat("\n", verticalPad)

	// Horizontal centring: lipgloss alignment handles ANSI-stripped
	// width correctly.
	block := lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(logo)
	if subtitle != "" {
		block += "\n\n" + lipgloss.NewStyle().Width(m.width).Align(lipgloss.Center).Render(subtitle)
	}

	return pad + block
}

// cyberSpinner returns a single rotating arc character that ticks
// once per frame. Use it inline next to live status text to signal
// "system is doing work right now".
func cyberSpinner(frame int) string {
	const frames = "◜◝◞◟"
	r := []rune(frames)
	return cyberSpinnerStyle.Render(string(r[frame%len(r)]))
}

// scanProgress returns a short "BOOTING [▓▓▓░░░░░]" style strip
// whose filled-cells count tracks the animation frame. Caps at a
// fixed cell count so the strip width never changes (avoids layout
// jitter when the rest of the line is centred).
func scanProgress(frame int) string {
	const cells = 12
	filled := (frame / 2) % (cells + 1)
	var b strings.Builder
	b.WriteString("[")
	for i := 0; i < cells; i++ {
		if i < filled {
			b.WriteRune('▓')
		} else {
			b.WriteRune('░')
		}
	}
	b.WriteString("]")
	return b.String()
}
