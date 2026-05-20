package tui

import "testing"

func TestWindowDetailList(t *testing.T) {
	cases := []struct {
		name                      string
		termHeight, total, cursor int
		wantStart, wantEnd        int
	}{
		{"empty list", 30, 0, 0, 0, 0},
		{"fits entirely", 30, 8, 3, 0, 8},
		{"cursor near top, windowed", 24, 100, 2, 0, 10},
		{"cursor mid, centered", 24, 100, 20, 15, 25},
		{"cursor at end, clamps", 24, 100, 99, 90, 100},
		{"tiny terminal floors at 5, centered", 5, 100, 50, 48, 53},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end := windowDetailList(tc.termHeight, tc.total, tc.cursor)
			if start != tc.wantStart || end != tc.wantEnd {
				t.Errorf("windowDetailList(%d, %d, %d) = (%d, %d); want (%d, %d)",
					tc.termHeight, tc.total, tc.cursor, start, end, tc.wantStart, tc.wantEnd)
			}
		})
	}
}
