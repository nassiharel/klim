package haiku

import "strings"

// CountSyllables returns the syllable count for an English word. The
// algorithm is the classic "count vowel groups, subtract silent-e,
// minimum one" heuristic with a few extra rules for diphthongs and
// CLI-tool oddities (kubectl, kustomize, etc.). It is approximate
// but consistent — and for our purposes (filling templates) any
// over- or under-count by one is hidden because templates have
// fixed-syllable scaffolding around the variable word.
func CountSyllables(word string) int {
	word = strings.ToLower(strings.TrimSpace(word))
	if word == "" {
		return 0
	}
	// Strip surrounding punctuation. The "not letter / digit / apostrophe"
	// expression is intentional (clearer than De Morgan's form).
	isWordRune := func(r rune) bool {
		return (r >= 'a' && r <= 'z') || r == '\'' || (r >= '0' && r <= '9')
	}
	word = strings.TrimFunc(word, func(r rune) bool { return !isWordRune(r) })
	if word == "" {
		return 0
	}
	// Special-cases — common CLI tool names whose syllable count
	// the heuristic gets wrong.
	if n, ok := overrides[word]; ok {
		return n
	}

	count := 0
	prevVowel := false
	for _, r := range word {
		v := isVowel(r)
		if v && !prevVowel {
			count++
		}
		prevVowel = v
	}
	// Silent trailing 'e' — but keep at least one syllable.
	if strings.HasSuffix(word, "e") && count > 1 && !endsInVowelE(word) {
		count--
	}
	// "le" trailing after a consonant adds a syllable back: "table"
	// → ta-ble.
	if strings.HasSuffix(word, "le") && len(word) > 2 && !isVowel(rune(word[len(word)-3])) {
		count++
	}
	if count < 1 {
		count = 1
	}
	return count
}

func isVowel(r rune) bool {
	switch r {
	case 'a', 'e', 'i', 'o', 'u', 'y':
		return true
	}
	return false
}

// endsInVowelE returns true when the trailing 'e' is part of a
// diphthong (e.g. "agree", "guarantee") and should not be silent.
func endsInVowelE(word string) bool {
	if len(word) < 2 {
		return false
	}
	prev := rune(word[len(word)-2])
	return isVowel(prev)
}

// CountLine returns the total syllable count for a whole line by
// summing CountSyllables across every whitespace-separated word.
func CountLine(line string) int {
	total := 0
	for _, w := range strings.Fields(line) {
		total += CountSyllables(w)
	}
	return total
}

// overrides handles short tool names where the vowel-group heuristic
// fails. Conservative list — only words whose haiku output looks
// noticeably off without an override.
var overrides = map[string]int{
	"klim":      1,
	"go":        1,
	"node":      1,
	"npm":       3, // n-p-m
	"yarn":      1,
	"git":       1,
	"docker":    2,
	"kubectl":   3, // ku-be-ctl (pronounced)
	"kustomize": 3,
	"terraform": 3,
	"helm":      1,
	"vault":     1,
	"awscli":    3, // a-w-s + cli
	"gh":        2, // g-h
	"jq":        2, // j-q
	"yq":        2, // y-q
	"rg":        2, // r-g
	"fd":        2, // f-d
	"bat":       1,
	"eza":       2, // e-za
	"fzf":       3, // f-z-f
	"tmux":      1,
	"vim":       1,
	"emacs":     2, // e-macs
	"nano":      2,
	"curl":      1,
	"wget":      2, // w-get
	"httpie":    2, // http-ie
	"make":      1,
	"cmake":     2,
	"ninja":     2,
	"bazel":     2,
	"gradle":    2,
	"maven":     2,
	"sbt":       3, // s-b-t
	"rust":      1,
	"rustc":     2, // rust-c
	"cargo":     2,
	"deno":      2,
	"bun":       1,
	"pnpm":      4, // p-n-p-m
	"poetry":    3,
	"pip":       1,
	"pyenv":     2,
	"rbenv":     2,
	"nvm":       3, // n-v-m
	"sdkman":    2,
}
