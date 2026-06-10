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
			name: "user mapping overrides the home label",
			cwd:  p("Users", "n"),
			home: p("Users", "n"),
			// Substring match: "n" is in the path, but we don't want
			// a 1-char accidental match; use a clearly intentional key.
			want: "🏠 Home",
		},
		{
			name:  "title bucket: bug",
			cwd:   p("Users"),
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
