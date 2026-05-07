package search

import (
	"testing"

	"github.com/nassiharel/klim/internal/registry"
)

func tool(name, displayName, category string) registry.Tool {
	return registry.Tool{Name: name, DisplayName: displayName, Category: category}
}

func TestSearch_EmptyQueryReturnsNil(t *testing.T) {
	tools := []registry.Tool{tool("git", "Git", "VCS")}
	if got := Search(tools, ""); got != nil {
		t.Fatalf("empty query: want nil, got %+v", got)
	}
	// Whitespace-only query also tokenises to nothing.
	if got := Search(tools, "   "); got != nil {
		t.Fatalf("whitespace query: want nil, got %+v", got)
	}
}

func TestSearch_ExactNameMatchBeatsPartial(t *testing.T) {
	tools := []registry.Tool{
		tool("kubernetes-cli", "Kubernetes CLI", "Containers"), // partial: contains "kub"
		tool("kub", "kub", "Misc"),                             // exact
	}
	results := Search(tools, "kub")
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	if results[0].Tool.Name != "kub" {
		t.Errorf("exact match should rank first; got %q first", results[0].Tool.Name)
	}
	if results[0].Score <= results[1].Score {
		t.Errorf("exact-match score (%d) should beat partial (%d)", results[0].Score, results[1].Score)
	}
}

func TestSearch_StarsBreakTiesAtSameScore(t *testing.T) {
	starry := tool("toola", "Tool A", "X")
	starry.GitHubInfo = &registry.GitHubInfo{Stars: 50_000}
	plain := tool("toolb", "Tool B", "X")
	results := Search([]registry.Tool{plain, starry}, "tool")
	if len(results) < 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	// Both match "tool" partially so they get the same name-match score.
	// Star-boost adds 5 to the >10k tool, breaking the tie.
	if results[0].Tool.Name != "toola" {
		t.Errorf("starred tool should rank first; got %s", results[0].Tool.Name)
	}
}

func TestSearch_TagAndTopicMatching(t *testing.T) {
	tagged := tool("tooltag", "Tool Tag", "Misc")
	tagged.Tags = []string{"k8s", "containers"}

	topiced := tool("tooltopic", "Tool Topic", "Misc")
	topiced.GitHubInfo = &registry.GitHubInfo{Topics: []string{"k8s", "containers"}}

	tools := []registry.Tool{tagged, topiced}

	// Exact tag match (25) > exact topic match (20).
	r := Search(tools, "k8s")
	if len(r) < 2 {
		t.Fatalf("want 2 results, got %d", len(r))
	}
	if r[0].Tool.Name != "tooltag" {
		t.Errorf("tag-matched tool should rank first; got %s", r[0].Tool.Name)
	}
}

func TestSearch_DescriptionMatch(t *testing.T) {
	tools := []registry.Tool{
		func() registry.Tool {
			t := tool("docs", "Docs", "Misc")
			t.GitHubInfo = &registry.GitHubInfo{Description: "kubernetes manifest renderer"}
			return t
		}(),
		tool("unrelated", "Unrelated", "Other"),
	}
	r := Search(tools, "kubernetes")
	if len(r) != 1 || r[0].Tool.Name != "docs" {
		t.Fatalf("description should produce a match; got %+v", r)
	}
}

func TestSearch_UnmatchedTermPenalisesScore(t *testing.T) {
	// Single-term match yields ~100 (exact name match). A second
	// completely-unrelated term knocks 50 off.
	matches := tool("git", "Git", "VCS")
	r1 := Search([]registry.Tool{matches}, "git")
	r2 := Search([]registry.Tool{matches}, "git zzznotreal")
	if len(r1) == 0 || len(r2) == 0 {
		t.Fatalf("expected results in both cases (got %d / %d)", len(r1), len(r2))
	}
	if r2[0].Score >= r1[0].Score {
		t.Errorf("unmatched second term should reduce score; r1=%d r2=%d", r1[0].Score, r2[0].Score)
	}
}

func TestSearch_NoMatchesAtAllReturnsEmpty(t *testing.T) {
	tools := []registry.Tool{tool("git", "Git", "VCS")}
	if got := Search(tools, "zzznevergonnamatch"); len(got) != 0 {
		t.Fatalf("want no results, got %+v", got)
	}
}

func TestTokenize(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"git", []string{"git"}},
		{"  Git CLI  ", []string{"git", "cli"}},
		{"K8S\tManifest", []string{"k8s", "manifest"}},
	}
	for _, c := range cases {
		got := tokenize(c.in)
		if !equalSlices(got, c.want) {
			t.Errorf("tokenize(%q): want %v, got %v", c.in, c.want, got)
		}
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestScoreTool_StarBoostBands(t *testing.T) {
	bands := []struct {
		stars int
		bonus int // boost only — score is 0 because no match terms here
	}{
		{0, 0},
		{500, 0},
		{1500, 2},
		{50_000, 5},
	}
	terms := []string{"git"} // matches none of the tools below
	for _, b := range bands {
		thing := tool("nope", "Nope", "X")
		if b.stars > 0 {
			thing.GitHubInfo = &registry.GitHubInfo{Stars: b.stars}
		}
		got := scoreTool(&thing, terms)
		// Each unmatched term subtracts 50; star boost is added afterwards.
		// So expected = -50 + bonus.
		want := -50 + b.bonus
		if got != want {
			t.Errorf("stars=%d: want score %d (= -50 unmatched + %d boost), got %d", b.stars, want, b.bonus, got)
		}
	}
}
