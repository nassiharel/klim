package haiku

import (
	"math/rand"
	"strings"
)

// template is a haiku line skeleton. Each template has fixed prefix
// + variable slot + fixed suffix; total syllables = fixedSyllables +
// CountSyllables(slot fill). The builder picks templates whose
// fixedSyllables leave room for a real word from the palette.
type template struct {
	prefix          string // text before the {} placeholder
	suffix          string // text after the {} placeholder
	fixedSyllables  int    // total syllables of prefix+suffix
	slotKind        slot   // which palette bucket the slot draws from
	minSlotSyllable int    // floor on syllable count of the chosen word
	maxSlotSyllable int    // ceiling — 0 means no max
}

// slot names where the variable word is sourced from.
type slot int

const (
	slotName slot = iota
	slotCategory
	slotTag
	slotDescWord
	slotAnyWord
)

// line5Templates produce 5-syllable lines.
//
// Each line totals `fixedSyllables + word_syllables == 5`. Because
// names vary widely we pick templates whose fixed-prefix syllables
// leave the right gap. Multiple templates per target count gives the
// rng meaningful variety so lines 1 and 3 don't collapse to the
// same string.
var line5Templates = []template{
	// fixed=2, slot=3 syllables
	{prefix: "", suffix: " breathes slow", fixedSyllables: 2, slotKind: slotName, minSlotSyllable: 3, maxSlotSyllable: 3},
	{prefix: "", suffix: " hums on", fixedSyllables: 2, slotKind: slotName, minSlotSyllable: 3, maxSlotSyllable: 3},
	{prefix: "", suffix: " stands tall", fixedSyllables: 2, slotKind: slotName, minSlotSyllable: 3, maxSlotSyllable: 3},
	// fixed=3, slot=2 syllables
	{prefix: "Quiet ", suffix: " waits", fixedSyllables: 3, slotKind: slotName, minSlotSyllable: 2, maxSlotSyllable: 2},
	{prefix: "", suffix: " dreams in code", fixedSyllables: 3, slotKind: slotName, minSlotSyllable: 2, maxSlotSyllable: 2},
	{prefix: "Through ", suffix: " I walk", fixedSyllables: 3, slotKind: slotCategory, minSlotSyllable: 2, maxSlotSyllable: 2},
	{prefix: "Tools of ", suffix: " — silent", fixedSyllables: 3, slotKind: slotCategory, minSlotSyllable: 2, maxSlotSyllable: 2},
	{prefix: "", suffix: " — a soft chime", fixedSyllables: 3, slotKind: slotName, minSlotSyllable: 2, maxSlotSyllable: 2},
	{prefix: "Calm ", suffix: " mornings", fixedSyllables: 3, slotKind: slotCategory, minSlotSyllable: 2, maxSlotSyllable: 2},
	{prefix: "The ", suffix: " of dawn", fixedSyllables: 3, slotKind: slotCategory, minSlotSyllable: 2, maxSlotSyllable: 2},
	{prefix: "Each ", suffix: " runs true", fixedSyllables: 3, slotKind: slotName, minSlotSyllable: 2, maxSlotSyllable: 2},
	{prefix: "", suffix: " — silent flame", fixedSyllables: 3, slotKind: slotName, minSlotSyllable: 2, maxSlotSyllable: 2},
	{prefix: "Yes, ", suffix: " glows on", fixedSyllables: 3, slotKind: slotName, minSlotSyllable: 2, maxSlotSyllable: 2},
	// fixed=4, slot=1 syllable
	{prefix: "", suffix: " stirs the soil", fixedSyllables: 4, slotKind: slotName, minSlotSyllable: 1, maxSlotSyllable: 1},
	{prefix: "Old ", suffix: " whispers home", fixedSyllables: 4, slotKind: slotName, minSlotSyllable: 1, maxSlotSyllable: 1},
	{prefix: "Just one ", suffix: " on the wind", fixedSyllables: 4, slotKind: slotName, minSlotSyllable: 1, maxSlotSyllable: 1},
	{prefix: "Cold steel ", suffix: " falls slow", fixedSyllables: 4, slotKind: slotName, minSlotSyllable: 1, maxSlotSyllable: 1},
}

// line7Templates produce 7-syllable lines (the middle line).
var line7Templates = []template{
	{prefix: "Sharp ", suffix: " threads weave the day", fixedSyllables: 5, slotKind: slotName, minSlotSyllable: 2, maxSlotSyllable: 2},
	{prefix: "I trust ", suffix: " with my prompts", fixedSyllables: 5, slotKind: slotName, minSlotSyllable: 2, maxSlotSyllable: 2},
	{prefix: "", suffix: " sings of paths gone", fixedSyllables: 4, slotKind: slotName, minSlotSyllable: 3, maxSlotSyllable: 3},
	{prefix: "A clean ", suffix: " holds the world", fixedSyllables: 5, slotKind: slotName, minSlotSyllable: 2, maxSlotSyllable: 2},
	{prefix: "Whispers of ", suffix: " in the shell", fixedSyllables: 6, slotKind: slotCategory, minSlotSyllable: 1, maxSlotSyllable: 1},
	{prefix: "Whispers of ", suffix: " in shells", fixedSyllables: 5, slotKind: slotCategory, minSlotSyllable: 2, maxSlotSyllable: 2},
	{prefix: "Each ", suffix: " is its own kind world", fixedSyllables: 6, slotKind: slotName, minSlotSyllable: 1, maxSlotSyllable: 1},
	{prefix: "Many ", suffix: " bloom in the night", fixedSyllables: 6, slotKind: slotCategory, minSlotSyllable: 1, maxSlotSyllable: 1},
	{prefix: "Quiet hands ", suffix: " the running streams", fixedSyllables: 5, slotKind: slotAnyWord, minSlotSyllable: 2, maxSlotSyllable: 2},
	{prefix: "An old ", suffix: " breathes again at dawn", fixedSyllables: 6, slotKind: slotName, minSlotSyllable: 1, maxSlotSyllable: 1},
	{prefix: "Day breaks; ", suffix: " starts the slow climb", fixedSyllables: 5, slotKind: slotName, minSlotSyllable: 2, maxSlotSyllable: 2},
}

// palette holds the words available to fill template slots, bucketed
// by slot kind. Each bucket may contain duplicates from different
// metadata fields — that's fine, the rng samples freely.
type palette struct {
	names      []string
	categories []string
	tags       []string
	descWords  []string
	anyWords   []string
}

func buildPalette(t Tool) palette {
	p := palette{}
	if n := strings.TrimSpace(t.DisplayName); n != "" {
		p.names = append(p.names, n)
		p.anyWords = append(p.anyWords, n)
	}
	if n := strings.TrimSpace(t.Name); n != "" {
		p.names = append(p.names, n)
		p.anyWords = append(p.anyWords, n)
	}
	if c := strings.TrimSpace(t.Category); c != "" {
		p.categories = append(p.categories, c)
		p.anyWords = append(p.anyWords, c)
	}
	for _, tag := range t.Tags {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			p.tags = append(p.tags, tag)
			p.anyWords = append(p.anyWords, tag)
		}
	}
	for _, w := range strings.Fields(t.Description) {
		// Strip trailing punctuation.
		w = strings.TrimFunc(w, isPunct)
		if w == "" || len(w) > 14 {
			continue
		}
		p.descWords = append(p.descWords, w)
		p.anyWords = append(p.anyWords, w)
	}
	return p
}

func isPunct(r rune) bool {
	switch r {
	case ',', '.', ';', ':', '!', '?', '(', ')', '[', ']', '"', '\'':
		return true
	}
	return false
}

// pickSlotWord returns a palette word whose syllable count fits
// minSyl..maxSyl, or "" when nothing fits. The rng deterministically
// shuffles the candidate list per call so the same tool always picks
// the same word.
func pickSlotWord(rng *rand.Rand, p palette, kind slot, minSyl, maxSyl int) string {
	candidates := bucket(p, kind)
	if len(candidates) == 0 {
		return ""
	}
	// rng.Shuffle is destructive on the slice; copy first.
	order := append([]string(nil), candidates...)
	rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	for _, w := range order {
		n := CountSyllables(w)
		if n >= minSyl && (maxSyl == 0 || n <= maxSyl) {
			return w
		}
	}
	return ""
}

func bucket(p palette, kind slot) []string {
	switch kind {
	case slotName:
		return p.names
	case slotCategory:
		return p.categories
	case slotTag:
		return p.tags
	case slotDescWord:
		return p.descWords
	case slotAnyWord:
		return p.anyWords
	}
	return nil
}

// buildLine picks templates from `pool` until it finds one whose
// slot can be filled from `pal` at the required syllable count. The
// fallback line uses the tool's display name (or "klim" when even
// that's empty) so output is never blank.
func buildLine(rng *rand.Rand, pool []template, target int, pal palette, t Tool) string {
	order := append([]int(nil), seq(len(pool))...)
	rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	for _, idx := range order {
		tpl := pool[idx]
		needed := target - tpl.fixedSyllables
		if needed < 1 {
			continue
		}
		// Apply template-specific syllable bounds.
		lo := tpl.minSlotSyllable
		hi := tpl.maxSlotSyllable
		if lo == 0 {
			lo = needed
		}
		if hi == 0 {
			hi = needed
		}
		if needed < lo || needed > hi {
			continue
		}
		word := pickSlotWord(rng, pal, tpl.slotKind, needed, needed)
		if word == "" {
			continue
		}
		candidate := tpl.prefix + capitalise(word) + tpl.suffix
		// PR-78 review: validate the built line against the same
		// syllable counter we use to score templates. Template
		// metadata can drift from CountLine — when it does (e.g.
		// a contraction we didn't account for, or a word in the
		// prefix whose count surprises us), reject the candidate
		// and try the next template rather than emit a line that
		// breaks the 5-7-5 contract.
		if CountLine(candidate) != target {
			continue
		}
		return candidate
	}
	// Fallback — a syllable-safe line built from the tool name only.
	name := strings.TrimSpace(t.DisplayName)
	if name == "" {
		name = strings.TrimSpace(t.Name)
	}
	if name == "" {
		name = "klim"
	}
	return fallbackLine(name, target)
}

// fallbackLine returns a line guaranteed to count to `target`
// syllables via CountLine. We try a pool of name-aware templates
// first; if none match (rare — e.g. a tool name whose syllable count
// the heuristic gets unusually wrong) we fall through to a hard-coded
// constant per target, verified by TestFallback_HardLines.
//
// PR-78 review: previously fallbackLine returned hand-written
// strings that broke the 5-7-5 contract for many tool names.
func fallbackLine(name string, target int) string {
	name = capitalise(strings.TrimSpace(name))

	// 5-syllable templates first; each has a fixed syllable count
	// excluding the name. We pick the first one whose remaining
	// syllables match the name's count.
	type opt struct {
		prefix, suffix string
		fixed          int
	}
	var pool []opt
	switch target {
	case 5:
		pool = []opt{
			{"", " runs on", 2},              // name + 2 = 5 → 3-syl name
			{"", " is good", 2},              // 3-syl name
			{"Quiet ", "", 2},                // 3-syl name
			{"", " sings", 1},                // 4-syl name
			{"", " whispers softly here", 5}, // 0
			{"", " glows", 1},                // 4-syl name
			{"Soft ", " hums", 2},            // 3-syl name
			{"", " — a soft tone", 3},        // 2-syl name
			{"", " is here today", 4},        // 1-syl name
			{"", "; here we go", 3},          // 2-syl name
		}
	case 7:
		pool = []opt{
			{"", " is calling me again", 6},               // 1-syl name
			{"", " — a quiet kind of art", 5},             // 2-syl name
			{"Soft ", " sings while the world sleeps", 5}, // 2-syl name
			{"", " runs as the day grows long", 5},        // 2-syl name
			{"", " hums in the quiet shell", 5},           // 2-syl name
			{"", " breathes through the slow morning", 5}, // 2-syl name
			{"", " is good and is kind tonight", 6},       // 1-syl name
			{"I trust ", "; the rest may wait", 4},        // 3-syl name
		}
	default:
		return name
	}
	for _, o := range pool {
		candidate := o.prefix + name + o.suffix
		if CountLine(candidate) == target {
			return candidate
		}
	}
	// Hard fallback: lines built from 1-2 syllable words whose
	// CountLine value we verify in haiku_test (TestFallback_HardLines)
	// so they cannot silently drift out of 5-7-5.
	switch target {
	case 5:
		return "Code hums in the night"
	case 7:
		return "Code hums softly through the night"
	}
	return name
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if r[0] >= 'a' && r[0] <= 'z' {
		r[0] -= 'a' - 'A'
	}
	return string(r)
}

func seq(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}
