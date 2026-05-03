// Package textwrap provides shared word-wrapping that respects terminal
// display width. Both `clim info` (CLI) and the TUI detail view consume
// it so the two surfaces wrap GitHub descriptions, summaries, and
// other free-form prose identically — and so neither has to
// re-implement rune-vs-byte handling.
package textwrap

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// Wrap breaks text into lines no wider than maxWidth display columns.
// Width is measured with go-runewidth so wide characters (CJK = 2
// cols, emoji = 2 cols) and zero-width combining marks are accounted
// for correctly. Splits on whitespace; a single word longer than
// maxWidth is emitted on its own line (not hard-cut).
//
// Returns nil for empty input. Returns the input unchanged in a
// single-element slice when maxWidth <= 0.
func Wrap(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	current := words[0]
	for _, word := range words[1:] {
		if runewidth.StringWidth(current)+1+runewidth.StringWidth(word) > maxWidth {
			lines = append(lines, current)
			current = word
		} else {
			current += " " + word
		}
	}
	lines = append(lines, current)
	return lines
}
