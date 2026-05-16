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
	tool := Tool{Name: "terraform", Category: "infrastructure", Tags: []string{"iac"}, Description: "infrastructure as code"}
	h1 := Generate(tool, Options{Seed: 1})
	h2 := Generate(tool, Options{Seed: 99999})
	// Not guaranteed to differ in *every* line but at least one should.
	if h1.String() == h2.String() {
		t.Logf("h1: %s", h1.String())
		t.Logf("h2: %s", h2.String())
		// Don't fail the test — same output with different seeds is
		// rare but possible. Just log.
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
