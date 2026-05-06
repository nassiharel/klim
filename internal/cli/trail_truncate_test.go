package cli

import "testing"

// TestTruncate_PreservesUTF8AndDisplayWidth guards against two
// distinct bugs:
//   - splitting multi-byte runes mid-codepoint (would emit invalid UTF-8)
//   - counting runes instead of display columns (would misalign the
//     tabwriter table for CJK/emoji where one rune occupies two cells)
//
// All expected widths use go-runewidth's accounting: ASCII = 1 col,
// CJK = 2 cols.
func TestTruncate_PreservesUTF8AndDisplayWidth(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		// ASCII fits.
		{"hello", 10, "hello"},
		// ASCII exactly at limit.
		{"hello", 5, "hello"},
		// ASCII truncation: keep budget-1 cells, then ellipsis.
		{"hello world", 5, "hell…"},
		// CJK fits exactly: "中文" is 4 columns, n=4 ⇒ no truncation.
		{"中文", 4, "中文"},
		// CJK shorter limit: budget=3 ⇒ "中" (2 cols) + "…" (1 col) = 3.
		{"中文中文", 3, "中…"},
		// Mixed: "中a" is 3 cols, n=3 ⇒ no truncation.
		{"中a", 3, "中a"},
		// Mixed truncation: "中文label" is 9 cols; n=4 ⇒ "中" (2) + "…" (1).
		{"中文label", 4, "中…"},
		// Accented char (1 col) preserved.
		{"naïve renaming", 6, "naïve…"},
		// n=1 returns input untouched (no room for ellipsis).
		{"é", 1, "é"},
	}
	for _, c := range cases {
		got := truncate(c.in, c.n)
		if got != c.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}
