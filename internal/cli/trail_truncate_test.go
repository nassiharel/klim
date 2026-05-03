package cli

import "testing"

// TestTruncate_PreservesUTF8 guards against splitting multi-byte runes
// mid-codepoint when shortening labels/summaries for the trail-log
// table. A byte-wise truncation would emit broken UTF-8 for valid
// non-ASCII input ("中" is 3 bytes; cutting at byte index 2 produces
// invalid output).
func TestTruncate_PreservesUTF8(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"hello", 10, "hello"},          // shorter than limit, returned as-is
		{"hello", 5, "hello"},           // exactly at limit
		{"hello world", 5, "hell…"},     // ASCII truncation
		{"中文label", 4, "中文l…"},          // mixed, rune-counted
		{"中文中文中文", 3, "中文…"},            // pure CJK
		{"中文", 2, "中文"},                 // exactly at limit, returned as-is
		{"中文", 5, "中文"},                 // CJK fits
		{"é", 1, "é"},                   // n=1 returns input untouched
		{"naïve renaming", 6, "naïve…"}, // accented char preserved
	}
	for _, c := range cases {
		got := truncate(c.in, c.n)
		if got != c.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}
