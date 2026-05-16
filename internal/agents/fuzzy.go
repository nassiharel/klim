package agents

import (
	"sort"
	"strings"
	"unicode"
)

// SearchResult is one hit from a Service.Search call.
type SearchResult struct {
	Score    int
	Type     EntityType
	ID       string
	Name     string
	Subtitle string
	Provider ProviderID
	Matches  []int // indices into Name of matched characters (for UI highlighting)
}

// FuzzyMatch scores a candidate string against a query using a
// subsequence matcher with bonuses for prefix matches, word-boundary
// matches, consecutive runs, and case-insensitive whole-word equality.
//
// Returns (score, matchedIndices). A zero score means "no match"
// unless the query is empty (then every candidate matches with score 1
// and no highlights).
//
// Algorithm: walk candidate, greedily match the next query char.
//   - +20 base per matched char
//   - +30 bonus if match at start of candidate
//   - +20 bonus if match at a word boundary ('-', '_', '.', '/', space,
//     or transition from lower to upper)
//   - +10 bonus per consecutive matched run beyond the first
//   - −1 per non-matched char skipped between matches (penalty cap −20)
//   - +50 bonus if the lowered query equals a whole word in candidate
//   - +80 bonus if the lowered query equals the entire candidate
//
// This is intentionally simple; it tracks just enough state to produce
// stable, intuitive rankings without bringing in a new dependency.
func FuzzyMatch(query, candidate string) (int, []int) {
	if query == "" {
		return 1, nil
	}
	if candidate == "" {
		return 0, nil
	}

	q := strings.ToLower(query)
	c := strings.ToLower(candidate)

	// Quick whole-string / whole-word bonuses computed up front.
	wholeBonus := 0
	if q == c {
		wholeBonus = 80
	} else if containsWholeWord(c, q) {
		wholeBonus = 50
	}

	qr := []rune(q)
	cr := []rune(c)
	origRunes := []rune(candidate)

	score := 0
	matches := make([]int, 0, len(qr))
	qi := 0
	skipPenalty := 0
	runLen := 0

	for i := 0; i < len(cr) && qi < len(qr); i++ {
		if cr[i] == qr[qi] {
			gain := 20
			if i == 0 {
				gain += 30
			}
			if i > 0 && isWordBoundary(cr[i-1], origRunes[i]) {
				gain += 20
			}
			if runLen > 0 {
				gain += 10
			}
			score += gain
			matches = append(matches, i)
			qi++
			runLen++
			skipPenalty = 0
		} else {
			runLen = 0
			if skipPenalty < 20 {
				skipPenalty++
				score--
			}
		}
	}

	if qi < len(qr) {
		return 0, nil // not all chars matched
	}
	return score + wholeBonus, matches
}

func isWordBoundary(prev, cur rune) bool {
	switch prev {
	case '-', '_', '.', '/', ' ', '\t', ':', '@':
		return true
	}
	if unicode.IsLower(prev) && unicode.IsUpper(cur) {
		return true
	}
	return false
}

func containsWholeWord(haystack, word string) bool {
	if word == "" {
		return false
	}
	idx := strings.Index(haystack, word)
	for idx >= 0 {
		left := idx == 0 || isSeparator(rune(haystack[idx-1]))
		end := idx + len(word)
		right := end == len(haystack) || isSeparator(rune(haystack[end]))
		if left && right {
			return true
		}
		// keep scanning for a word-boundary occurrence later in the string
		next := strings.Index(haystack[idx+1:], word)
		if next < 0 {
			return false
		}
		idx = idx + 1 + next
	}
	return false
}

func isSeparator(r rune) bool {
	switch r {
	case '-', '_', '.', '/', ' ', '\t', ':', '@':
		return true
	}
	return false
}

// ParseScopedQuery parses an optional `<type>:<query>` prefix.
// Returns (typeFilter, residualQuery). If no recognized prefix is
// present, typeFilter is "" and the full query is returned unchanged.
//
// Recognized prefixes (case-insensitive): marketplace, plugin, skill,
// mcp, session, plus short forms: mk, pl, sk, mc, se.
func ParseScopedQuery(query string) (EntityType, string) {
	colon := strings.IndexByte(query, ':')
	if colon <= 0 || colon == len(query)-1 {
		return "", query
	}
	prefix := strings.ToLower(strings.TrimSpace(query[:colon]))
	rest := strings.TrimSpace(query[colon+1:])
	switch prefix {
	case "marketplace", "marketplaces", "mk":
		return EntityMarketplace, rest
	case "plugin", "plugins", "pl":
		return EntityPlugin, rest
	case "skill", "skills", "sk":
		return EntitySkill, rest
	case "mcp", "mcps", "mc":
		return EntityMCP, rest
	case "session", "sessions", "se":
		return EntitySession, rest
	}
	return "", query
}

// rankResults sorts in-place by score desc, then name asc for stable
// display. Exported only for testing.
func rankResults(results []SearchResult) {
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return strings.ToLower(results[i].Name) < strings.ToLower(results[j].Name)
	})
}
