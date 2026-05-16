package haiku

import (
	"math/rand"
	"strings"
)

// template is a haiku line skeleton. Each template has fixed prefix
// + variable slot + fixed suffix; total syllables = fixedSyllables +
// CountSyllables(slot fill). fixedSyllables is computed automatically
// in init() from CountLine(prefix+suffix) so template authors can't
// drift the metadata away from the syllable heuristic (a previous
// hand-maintained list had several entries whose declared count
// disagreed with CountLine, silently disqualifying every candidate).
// The builder picks templates whose fixedSyllables leave room for a
// real word from the palette.
type template struct {
	prefix         string // text before the {} placeholder
	suffix         string // text after the {} placeholder
	fixedSyllables int    // auto-computed; do not set by hand
	slotKind       slot   // which palette bucket the slot draws from
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
	{prefix: "", suffix: " breathes slow", slotKind: slotName},
	{prefix: "", suffix: " hums on", slotKind: slotName},
	{prefix: "", suffix: " stands tall", slotKind: slotName},
	{prefix: "Quiet ", suffix: " waits", slotKind: slotName},
	{prefix: "", suffix: " dreams in code", slotKind: slotName},
	{prefix: "Through ", suffix: " I walk", slotKind: slotCategory},
	{prefix: "Tools of ", suffix: " — silent", slotKind: slotCategory},
	{prefix: "", suffix: " — a soft chime", slotKind: slotName},
	{prefix: "Calm ", suffix: " mornings", slotKind: slotCategory},
	{prefix: "The ", suffix: " of dawn", slotKind: slotCategory},
	{prefix: "Each ", suffix: " runs true", slotKind: slotName},
	{prefix: "", suffix: " — silent flame", slotKind: slotName},
	{prefix: "Yes, ", suffix: " glows on", slotKind: slotName},
	{prefix: "", suffix: " stirs the soil", slotKind: slotName},
	{prefix: "Old ", suffix: " whispers home", slotKind: slotName},
	{prefix: "Just one ", suffix: " on the wind", slotKind: slotName},
	{prefix: "Cold steel ", suffix: " falls slow", slotKind: slotName},
}

// line7Templates produce 7-syllable lines (the middle line).
var line7Templates = []template{
	{prefix: "Sharp ", suffix: " threads weave the day", slotKind: slotName},
	{prefix: "I trust ", suffix: " with my prompts", slotKind: slotName},
	{prefix: "", suffix: " sings of paths gone", slotKind: slotName},
	{prefix: "A clean ", suffix: " holds the world", slotKind: slotName},
	{prefix: "Whispers of ", suffix: " in the shell", slotKind: slotCategory},
	{prefix: "Whispers of ", suffix: " in shells", slotKind: slotCategory},
	{prefix: "Each ", suffix: " is its own kind world", slotKind: slotName},
	{prefix: "Many ", suffix: " bloom in the night", slotKind: slotCategory},
	{prefix: "Quiet hands ", suffix: " the running streams", slotKind: slotAnyWord},
	{prefix: "An old ", suffix: " breathes again at dawn", slotKind: slotName},
	{prefix: "Day breaks; ", suffix: " starts the slow climb", slotKind: slotName},
}

// init auto-computes each template's fixedSyllables from
// CountLine(prefix+suffix) so the metadata can never drift from the
// syllable heuristic. Callers should leave the field at its zero
// value (or any value — it's overwritten here). Without this, a
// template whose declared count disagreed with CountLine would be
// silently rejected by buildLine for every candidate.
func init() {
	for i := range line5Templates {
		line5Templates[i].fixedSyllables = CountLine(line5Templates[i].prefix + line5Templates[i].suffix)
	}
	for i := range line7Templates {
		line7Templates[i].fixedSyllables = CountLine(line7Templates[i].prefix + line7Templates[i].suffix)
	}
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
		// fixedSyllables is auto-computed in init() so the gap is
		// always accurate; pickSlotWord uses (needed, needed) as
		// both the floor and the ceiling, so any palette word that
		// counts to `needed` syllables fills the slot.
		word := pickSlotWord(rng, pal, tpl.slotKind, needed, needed)
		if word == "" {
			continue
		}
		candidate := tpl.prefix + capitalise(word) + tpl.suffix
		// Validate the built line against the same syllable
		// counter that ranked templates. Template authors can't
		// always anticipate CountLine's edge cases (contractions,
		// silent-e shapes, acronyms), so reject any candidate
		// whose total count doesn't match the target rather than
		// emit a line that breaks the 5-7-5 contract.
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
// syllables via CountLine. It tries a pool of name-aware phrases
// first; if none of those match (rare — e.g. a tool name whose
// syllable count the heuristic gets unusually wrong) it falls
// through to a constant per target, verified by TestFallback_HardLines.
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
