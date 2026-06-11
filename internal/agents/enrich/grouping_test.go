package enrich

import (
	"path/filepath"
	"testing"
)

// p builds an OS-appropriate absolute path for the test fixtures
// without tripping gocritic's filepathJoin check (which warns when
// `filepath.Join("/", …)` is used to construct an absolute path).
func p(segments ...string) string {
	rel := filepath.Join(segments...)
	return string(filepath.Separator) + rel
}

func TestResolveGrouping(t *testing.T) {
	t.Parallel()

	mappings := map[string]string{
		"klim":      "Klim",
		"dashboard": "Dashboards",
	}

	tests := []struct {
		name  string
		cwd   string
		repo  string
		title string
		home  string
		want  string
	}{
		{
			name: "user mapping wins over repo name",
			cwd:  p("Users", "n", "dev", "klim"),
			repo: "klim-fork",
			want: "Klim",
		},
		{
			name: "repo name when no mapping matches",
			cwd:  p("Users", "n", "dev", "myproject"),
			repo: "myrepo",
			want: "myrepo",
		},
		{
			name: "last cwd segment when no mapping and no repo",
			cwd:  p("Users", "n", "dev", "my-project"),
			want: "my-project",
		},
		{
			name:  "noisy parent skipped, falls through to title bucket",
			cwd:   p("Users"),
			title: "pr review feedback",
			want:  "PR Reviews",
		},
		{
			name: "home directory gets the home label",
			cwd:  p("Users", "n"),
			home: p("Users", "n"),
			want: "🏠 Home",
		},
		{
			name: "title bucket: bug",
			cwd:  p("Users"),
			title: "fix a small bug in widget",
			want:  "Bug Fixes",
		},
		{
			name: "all empty falls through to Other",
			cwd:  "",
			want: "Other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Resolve(tt.cwd, tt.repo, tt.title, tt.home, mappings)
			if got != tt.want {
				t.Errorf("Resolve(cwd=%q, repo=%q, title=%q, home=%q) = %q, want %q",
					tt.cwd, tt.repo, tt.title, tt.home, got, tt.want)
			}
		})
	}
}

// TestResolveMappingOverridesHome pins the documented precedence in
// Resolve: when both a user mapping matches the cwd AND the cwd is
// the home directory, the mapping wins. The previous version of this
// test sat inside the table above and asserted "🏠 Home" without
// supplying any mapping that matched — leaving the precedence
// untested even though the test name claimed it.
func TestResolveMappingOverridesHome(t *testing.T) {
	t.Parallel()
	home := p("Users", "n")
	mappings := map[string]string{
		// "Users" matches the home cwd; without the mapping-first
		// precedence this would resolve to "🏠 Home".
		"Users": "PersonalLab",
	}
	got := Resolve(home, "", "", home, mappings)
	if got != "PersonalLab" {
		t.Errorf("mapping should override home label: got %q, want %q", got, "PersonalLab")
	}
}

// TestResolveLongestMappingWins verifies that overlapping mapping
// patterns are resolved deterministically by longest-key-first, so
// `klim-fork` beats `klim` no matter what order the YAML store
// happened to enumerate them in.
func TestResolveLongestMappingWins(t *testing.T) {
	t.Parallel()
	mappings := map[string]string{
		"klim":      "Klim",
		"klim-fork": "Klim Fork",
	}
	got := Resolve(p("dev", "klim-fork"), "", "", "", mappings)
	if got != "Klim Fork" {
		t.Errorf("longest-key wins: got %q, want %q", got, "Klim Fork")
	}
}

// TestResolveMappingDeterministic asserts the resolver returns the
// same group for the same input across many repeated calls, even
// when the input has multiple overlapping mappings. Without sorted
// iteration the map's randomised range order would flicker between
// groups.
func TestResolveMappingDeterministic(t *testing.T) {
	t.Parallel()
	mappings := map[string]string{
		"alpha": "Alpha",
		"beta":  "Beta",
		"gamma": "Gamma",
		"delta": "Delta",
		"epsil": "Epsilon",
	}
	// A cwd that contains MULTIPLE mapping keys — the resolver must
	// pick the same one every call.
	cwd := p("alpha", "beta", "gamma", "delta", "epsil")
	first := Resolve(cwd, "", "", "", mappings)
	for i := 0; i < 100; i++ {
		got := Resolve(cwd, "", "", "", mappings)
		if got != first {
			t.Fatalf("non-deterministic: call %d got %q, want %q", i, got, first)
		}
	}
}
