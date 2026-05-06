// Package textwrap provides shared word-wrapping that respects terminal
// display width. Both `klim info` (CLI) and the TUI detail view consume
// it so the two surfaces wrap GitHub descriptions, summaries, and
// other free-form prose identically — and so neither has to
// re-implement rune-vs-byte handling.
package textwrap

import (
	"strings"

	"github.com/mattn/go-runewidth"
)

// Wrap breaks text into lines that try to fit in maxWidth display
// columns. Width is measured with go-runewidth so wide characters
// (CJK = 2 cols, emoji = 2 cols) and zero-width combining marks are
// accounted for correctly. Splits on whitespace boundaries.
//
// A single word longer than maxWidth is emitted on its own line
// without being split, so the returned line CAN exceed maxWidth in
// that one case — callers that need a hard width cap must hard-cut
// such words separately. This is the conventional behavior for
// word-wrap helpers and keeps URLs / hashes / long identifiers
// readable.
//
// Returns nil for empty input (regardless of maxWidth). Returns the
// input unchanged in a single-element slice when maxWidth <= 0 and
// the input is non-empty — the empty-input check runs first.
func Wrap(text string, maxWidth int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}
	if maxWidth <= 0 {
		return []string{text}
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
