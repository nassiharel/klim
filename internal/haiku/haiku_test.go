package haiku

import (
	"strings"
	"testing"
)

func TestGenerate_DeterministicByDefault(t *testing.T) {
	tool := Tool{Name: "terraform", DisplayName: "Terraform", Category: "infrastructure", Tags: []string{"iac", "hashicorp"}, Description: "infrastructure as code"}
	h1 := Generate(tool, Options{})
	h2 := Generate(tool, Options{})
	if h1.String() != h2.String() {
		t.Errorf("two Generate calls produced different haiku:\n%s\n---\n%s", h1.String(), h2.String())
	}
}

func TestGenerate_DifferentSeedGivesDifferentOutput(t *testing.T) {
	// Less brittle than asserting a specific seed pair: scan a
	// bounded set of seeds and require at least one to diverge
	// from seed=1. Template additions or reorderings can't break
	// this without genuinely collapsing seeded variety.
	tool := Tool{Name: "terraform", Category: "infrastructure", Tags: []string{"iac"}, Description: "infrastructure as code"}
	baseline := Generate(tool, Options{Seed: 1}).String()
	seeds := []int64{2, 7, 42, 99, 1000, 99999}
	diverged := false
	for _, s := range seeds {
		if Generate(tool, Options{Seed: s}).String() != baseline {
			diverged = true
			break
		}
	}
	if !diverged {
		t.Errorf("expected at least one of %v to produce different output from seed=1; all collapsed to:\n%s", seeds, baseline)
	}
}

func TestGenerate_AlwaysReturnsThreeLines(t *testing.T) {
	tools := []Tool{
		{Name: "go", DisplayName: "Go"},
		{Name: "kubectl", Category: "kubernetes"},
		{}, // empty
		{Name: "x"},
	}
	for _, tool := range tools {
		h := Generate(tool, Options{})
		for i, line := range h.Lines {
			if strings.TrimSpace(line) == "" {
				t.Errorf("tool=%q line %d empty (haiku: %s)", tool.Name, i, h.String())
			}
		}
	}
}

func TestCountSyllables(t *testing.T) {
	cases := []struct {
		word string
		want int
	}{
		{"git", 1},
		{"docker", 2},
		{"kubernetes", 4},
		{"terraform", 3},
		{"klim", 1},
		{"make", 1},
		{"agree", 2},
		{"silent-e-ish", 4},
	}
	for _, c := range cases {
		got := CountSyllables(c.word)
		if got != c.want {
			t.Errorf("CountSyllables(%q) = %d, want %d", c.word, got, c.want)
		}
	}
}

func TestCountSyllables_EmptyAndPunctOnly(t *testing.T) {
	if got := CountSyllables(""); got != 0 {
		t.Errorf("empty: got %d, want 0", got)
	}
	if got := CountSyllables("!!!"); got != 0 {
		t.Errorf("punct-only: got %d, want 0", got)
	}
}

func TestDefaultSeed_DeterministicAcrossCase(t *testing.T) {
	if defaultSeed("Terraform") != defaultSeed("terraform") {
		t.Error("default seed should be case-insensitive")
	}
}

func TestBuildPalette_SkipsEmpty(t *testing.T) {
	p := buildPalette(Tool{Name: "go", Description: ""})
	// Names should include "go" once (and DisplayName empty so not twice).
	if len(p.names) != 1 || p.names[0] != "go" {
		t.Errorf("names = %v, want [go]", p.names)
	}
	if len(p.descWords) != 0 {
		t.Errorf("descWords should be empty: %v", p.descWords)
	}
}

func TestFallback_HardLines(t *testing.T) {
	// PR-78 review: hard-fallback strings must match CountLine; if a
	// future syllable rule shifts these out of 5/7 the test fires.
	if got := CountLine("Code hums in the night"); got != 5 {
		t.Errorf("hard 5-syllable fallback: got %d want 5", got)
	}
	if got := CountLine("Code hums softly through the night"); got != 7 {
		t.Errorf("hard 7-syllable fallback: got %d want 7", got)
	}
}

func TestFallbackLine_VariousNames(t *testing.T) {
	// Stress fallbackLine: every name × target must produce a line
	// CountLine accepts as the target.
	names := []string{"klim", "go", "git", "docker", "kubectl", "terraform", "x", "weirdname", "rustc", "fzf"}
	for _, n := range names {
		for _, target := range []int{5, 7} {
			got := fallbackLine(n, target)
			if c := CountLine(got); c != target {
				t.Errorf("fallbackLine(%q, %d) = %q (%d syllables); want %d", n, target, got, c, target)
			}
		}
	}
}
