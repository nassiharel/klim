// Package haiku generates a 5-7-5 syllable poem about a klim tool
// from its catalog metadata. The output is deterministic per tool by
// default (seeded with a hash of the tool name) so the same tool
// always produces the same haiku — running `klim haiku terraform`
// twice gives identical output. A custom Seed value overrides the
// default and lets users get variety on demand.
//
// No network calls. No agent. The generator is template-based: each
// of the three lines picks a template from a small library and fills
// it with words drawn from the tool's name, display name, category,
// description, and tags. Templates are tagged with their fixed
// syllable count so we know what the variable word's count needs to
// be, but every built candidate is then re-validated against
// CountLine before it is returned. The fallback path also uses
// CountLine so the 5-7-5 contract is preserved even when the
// template's nominal count and the line's real count disagree (e.g.
// for tool names with silent letters CountSyllables can't predict).
package haiku

import (
	"hash/fnv"
	"math/rand"
	"strings"
)

// Tool is the minimal view of a klim registry.Tool the haiku
// generator needs. Defined here (rather than importing the registry
// package) so this package stays cycle-free and trivially testable.
type Tool struct {
	Name        string
	DisplayName string
	Category    string
	Tags        []string
	Description string
}

// Options tunes the generator.
type Options struct {
	// Seed overrides the default (deterministic) seed. Zero means
	// "use the default hash of Tool.Name".
	Seed int64
}

// Haiku is the rendered three-line poem.
type Haiku struct {
	Lines [3]string
}

// String renders the haiku as a single multiline string.
func (h Haiku) String() string {
	return strings.Join(h.Lines[:], "\n")
}

// Generate returns a Haiku for the supplied tool. Always returns a
// valid 3-line poem — tools with no metadata get a fallback haiku
// constructed from the bare name.
func Generate(t Tool, opts Options) Haiku {
	seed := opts.Seed
	if seed == 0 {
		seed = defaultSeed(t.Name)
	}
	pal := buildPalette(t)

	// Derive a distinct sub-seed per line so lines 1 and 3 (both
	// 5-syllable lines) don't end up identical when the same seed
	// shuffles the same template pool the same way.
	line1 := buildLine(rand.New(rand.NewSource(seed^0xA5A5A5A5)), line5Templates, 5, pal, t) //nolint:gosec
	line2 := buildLine(rand.New(rand.NewSource(seed^0x5A5A5A5A)), line7Templates, 7, pal, t) //nolint:gosec
	line3 := buildLine(rand.New(rand.NewSource(seed^0x3C3C3C3C)), line5Templates, 5, pal, t) //nolint:gosec
	return Haiku{Lines: [3]string{line1, line2, line3}}
}

// defaultSeed derives a stable int64 seed from a tool name so the
// same tool always produces the same haiku across runs and machines.
func defaultSeed(name string) int64 {
	if name == "" {
		return 1
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.ToLower(name)))
	// Ensure non-zero — Generate's `seed == 0` means "use default".
	v := int64(h.Sum64() & 0x7fffffffffffffff)
	if v == 0 {
		v = 1
	}
	return v
}
