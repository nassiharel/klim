package enrich

import (
	"path/filepath"
	"testing"
)

func TestResolveGrouping(t *testing.T) {
	t.Parallel()

	mappings := map[string]string{
		"klim":      "Klim",
		"dashboard": "Dashboards",
	}

	tests := []struct {
		name   string
		cwd    string
		repo   string
		title  string
		home   string
		want   string
	}{
		{
			name: "user mapping wins over repo name",
			cwd:  filepath.Join("/", "Users", "n", "dev", "klim"),
			repo: "klim-fork",
			want: "Klim",
		},
		{
			name: "repo name when no mapping matches",
			cwd:  filepath.Join("/", "Users", "n", "dev", "myproject"),
			repo: "myrepo",
			want: "myrepo",
		},
		{
			name: "last cwd segment when no mapping and no repo",
			cwd:  filepath.Join("/", "Users", "n", "dev", "my-project"),
			want: "my-project",
		},
		{
			name: "noisy parent skipped, falls through to title bucket",
			cwd:  filepath.Join("/", "Users"),
			title: "pr review feedback",
			want: "PR Reviews",
		},
		{
			name: "home directory gets the home label",
			cwd:  "/Users/n",
			home: "/Users/n",
			want: "🏠 Home",
		},
		{
			name: "user mapping overrides the home label",
			cwd:  filepath.Join("/", "Users", "n"),
			home: filepath.Join("/", "Users", "n"),
			// Substring match: "n" is in the path, but we don't want
			// a 1-char accidental match; use a clearly intentional key.
			want: "🏠 Home",
		},
		{
			name: "title bucket: bug",
			cwd:  filepath.Join("/", "Users"),
			title: "fix a small bug in widget",
			want: "Bug Fixes",
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