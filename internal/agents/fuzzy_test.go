package agents

import "testing"

func TestFuzzyMatch_BasicSubsequence(t *testing.T) {
	tests := []struct {
		query string
		cand  string
		want  bool // want a non-zero score
	}{
		{"", "anything", true},
		{"abc", "abc", true},
		{"abc", "axbxc", true},
		{"abc", "acb", false}, // order matters
		{"react", "react-helper", true},
		{"helper", "react-helper", true},
		{"xyz", "react-helper", false},
		{"Mcp", "mcp-server", true}, // case-insensitive
	}
	for _, tt := range tests {
		s, _ := FuzzyMatch(tt.query, tt.cand)
		got := s > 0 || (tt.query == "" && s == 1)
		if got != tt.want {
			t.Errorf("FuzzyMatch(%q,%q) score=%d want match=%v", tt.query, tt.cand, s, tt.want)
		}
	}
}

func TestFuzzyMatch_Ranking(t *testing.T) {
	// Higher-quality matches should outrank weaker ones.
	cands := []string{"github-mcp-server", "github", "ungrouped-thingyub", "gh"}
	scores := make([]int, len(cands))
	for i, c := range cands {
		scores[i], _ = FuzzyMatch("github", c)
	}
	// Exact match should be highest.
	if scores[1] < scores[0] || scores[1] < scores[2] {
		t.Errorf("expected exact 'github' to outrank others; scores=%v", scores)
	}
	// 'gh' doesn't subsequence-match 'github' so it should be zero.
	if scores[3] != 0 {
		t.Errorf("expected 'gh' query against 'github' to be a match; got non-match")
	}
	// (Note: 'gh' query matches 'github' since g,h are subsequences. Above is sanity.)
}

func TestFuzzyMatch_PrefixBonus(t *testing.T) {
	prefix, _ := FuzzyMatch("react", "react-helper")
	mid, _ := FuzzyMatch("react", "old-react-helper")
	if prefix <= mid {
		t.Errorf("expected prefix match to outrank embedded match: prefix=%d mid=%d", prefix, mid)
	}
}

func TestFuzzyMatch_NoMatch(t *testing.T) {
	s, m := FuzzyMatch("abcd", "abx")
	if s != 0 || len(m) != 0 {
		t.Errorf("expected no match for insufficient candidate; got score=%d matches=%v", s, m)
	}
}

func TestParseScopedQuery(t *testing.T) {
	tests := []struct {
		in       string
		wantType EntityType
		wantRest string
	}{
		{"react", "", "react"},
		{"plugin:react", EntityPlugin, "react"},
		{"  skill:foo bar  ", EntitySkill, "foo bar"},
		{"sk:bar", EntitySkill, "bar"},
		{"mcp:postgres", EntityMCP, "postgres"},
		{"mc:postgres", EntityMCP, "postgres"},
		{"unknown:foo", "", "unknown:foo"},
		{"plugin:", "", "plugin:"}, // missing residual → not scoped
	}
	for _, tt := range tests {
		gotType, gotRest := ParseScopedQuery(tt.in)
		if gotType != tt.wantType || gotRest != tt.wantRest {
			t.Errorf("ParseScopedQuery(%q) = (%q,%q), want (%q,%q)",
				tt.in, gotType, gotRest, tt.wantType, tt.wantRest)
		}
	}
}

func TestParseScopedQuery_KeepsQueryColon(t *testing.T) {
	// A query that's literally `react:hooks` (unknown prefix) should
	// pass through unchanged so we don't accidentally strip user intent.
	gotType, gotRest := ParseScopedQuery("react:hooks")
	if gotType != "" || gotRest != "react:hooks" {
		t.Errorf("unknown prefix should pass through; got (%q,%q)", gotType, gotRest)
	}
}
