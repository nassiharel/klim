package registry

import "testing"

func TestVersionsMatch(t *testing.T) {
	tests := []struct {
		name      string
		installed string
		latest    string
		want      bool
	}{
		// Exact matches.
		{"exact equal", "1.2.3", "1.2.3", true},
		{"single segment equal", "7", "7", true},
		{"two segments equal", "1.0", "1.0", true},
		{"four segments equal", "1.2.3.4", "1.2.3.4", true},

		// Different versions.
		{"major differs", "1.2.3", "2.2.3", false},
		{"minor differs", "1.2.3", "1.3.3", false},
		{"patch differs", "1.2.3", "1.2.4", false},
		{"single segment differs", "7", "8", false},

		// Trailing segments — extra zeros should match, non-zeros should not.
		{"trailing zero", "1.2.3", "1.2.3.0", true},
		{"trailing zeros", "1.2.3", "1.2.3.0.0", true},
		{"trailing non-zero", "1.0.0", "1.0.0.999", false},
		{"trailing build number", "7.6.0", "7.6.0.500", false},

		// PE version padding.
		{"PE padding x100", "1.23.1400", "1.23.14", true},
		{"PE padding x10", "1.23.140", "1.23.14", true},
		{"PE padding x1000", "1.23.14000", "1.23.14", true},
		{"PE padding reverse", "1.23.14", "1.23.1400", true},
		{"PE padding not factor", "1.23.1401", "1.23.14", false},

		// Non-numeric segments (e.g. git "2.53.0.windows.1").
		{"stops at non-numeric", "2.53.0", "2.53.0", true},
		{"non-numeric ignored", "2.53.0.windows.1", "2.53.0", true},

		// Empty strings.
		{"both empty", "", "", true},
		{"installed empty", "", "1.2.3", false},
		{"latest empty", "1.2.3", "", false},

		// Build metadata: non-zero trailing segment does not match.
		{"build metadata extra", "2.53.0.2", "2.53.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VersionsMatch(tt.installed, tt.latest)
			if got != tt.want {
				t.Errorf("VersionsMatch(%q, %q) = %v, want %v",
					tt.installed, tt.latest, got, tt.want)
			}
		})
	}
}

func TestIsPaddedMatch(t *testing.T) {
	tests := []struct {
		name string
		a, b int
		want bool
	}{
		{"equal", 14, 14, true},
		{"x10", 140, 14, true},
		{"x100", 1400, 14, true},
		{"x1000", 14000, 14, true},
		{"reverse x10", 14, 140, true},
		{"reverse x100", 14, 1400, true},
		{"not factor", 1401, 14, false},
		{"x2 not allowed", 28, 14, false},
		{"zero a", 0, 14, false},
		{"zero b", 14, 0, false},
		{"both zero", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPaddedMatch(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("isPaddedMatch(%d, %d) = %v, want %v",
					tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestParseSegments(t *testing.T) {
	tests := []struct {
		name string
		v    string
		want []int
	}{
		{"simple", "1.2.3", []int{1, 2, 3}},
		{"single", "7", []int{7}},
		{"four segments", "1.2.3.4", []int{1, 2, 3, 4}},
		{"stops at non-numeric", "2.53.0.windows.1", []int{2, 53, 0}},
		{"empty string", "", nil},
		{"all non-numeric", "abc", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseSegments(tt.v)
			if len(got) == 0 && len(tt.want) == 0 {
				return // both nil/empty
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseSegments(%q) = %v (len %d), want %v (len %d)",
					tt.v, got, len(got), tt.want, len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseSegments(%q)[%d] = %d, want %d",
						tt.v, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"equal", "1.2.3", "1.2.3", 0},
		{"a less than b", "1.2.3", "1.2.4", -1},
		{"a greater than b", "1.2.4", "1.2.3", 1},
		{"major differs", "1.0.0", "2.0.0", -1},
		{"9 vs 10", "9.0.0", "10.0.0", -1},
		{"10 vs 9", "10.0.0", "9.0.0", 1},
		{"different lengths a shorter", "1.2", "1.2.1", -1},
		{"different lengths b shorter", "1.2.1", "1.2", 1},
		{"trailing zeros equal", "1.2.0", "1.2", 0},
		{"both empty", "", "", 0},
		{"empty vs non-empty", "", "1.0", -1},
		{"non-empty vs empty", "1.0", "", 1},
		{"single segment", "7", "8", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompareVersions(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("CompareVersions(%q, %q) = %d, want %d",
					tt.a, tt.b, got, tt.want)
			}
		})
	}
}
