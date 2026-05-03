// Package recommend ranks not-installed tools by overlap with the
// user's currently installed set, so both the TUI's "For You" sub-tab
// and the web UI's /foryou page can suggest related tools using the
// same algorithm.
//
// The scoring rules are intentionally simple and deterministic:
//
//   - +1 for each shared tag or GitHub topic with an installed tool.
//   - +2 if the candidate's category matches one already represented
//     among installed tools.
//   - +1 if the project has been pushed within the last 6 months.
//   - +1 (or +2 above 10k) for popularity by GitHub star count.
//
// Candidates with score 0 are dropped, archived projects are skipped,
// and tools without a package available for the host OS are skipped
// because installing them would always fail.
package recommend

import (
	"sort"
	"strings"
	"time"

	"github.com/nassiharel/clim/internal/registry"
)

// Recommendation is one ranked suggestion. ToolIdx is the candidate's
// index into the tools slice the caller passed to Compute, so the TUI
// can avoid a follow-up name lookup; web callers usually ignore it.
type Recommendation struct {
	ToolIdx     int
	Score       int
	MatchPct    int
	Reason      string // sorted installed tool names that motivated the score
	Category    string
	Description string
	Stars       int
}

// Max is the cap applied to Compute's output. The TUI's For You sub-tab
// shares the same default and clim's UX is built around an at-a-glance
// shortlist rather than a full search interface; users wanting more
// browse via Discover.
const Max = 25

// Compute ranks not-installed tools by tag/topic overlap with installed
// tools, applies the scoring rules described in the package doc, and
// returns the top results sorted by score descending (and name as a
// tiebreaker). Returns nil when nothing is installed, since there's
// nothing to compare against.
func Compute(tools []registry.Tool) []Recommendation {
	tagFreq := make(map[string]int)
	tagSources := make(map[string][]string)
	installedCats := make(map[string]bool)

	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		if t.Category != "" {
			installedCats[t.Category] = true
		}
		for _, tag := range t.Tags {
			tagFreq[tag]++
			tagSources[tag] = append(tagSources[tag], t.Name)
		}
		if t.GitHubInfo != nil {
			for _, topic := range t.GitHubInfo.Topics {
				tagFreq[topic]++
				tagSources[topic] = append(tagSources[topic], t.Name)
			}
		}
	}
	if len(tagFreq) == 0 {
		return nil
	}

	var recs []Recommendation
	maxScore := 0
	for i, t := range tools {
		if t.IsInstalled() {
			continue
		}
		if t.GitHubInfo != nil && t.GitHubInfo.Archived {
			continue
		}
		// Tools with no package configured for the host OS are
		// unreachable from the UI. Recommending them would surface a
		// "no install source available" error on click — better to
		// just hide them up-front.
		if !t.Packages.HasAnyPackageForOS() {
			continue
		}

		score := 0
		matched := make(map[string]struct{})
		for _, tag := range t.Tags {
			if freq, ok := tagFreq[tag]; ok {
				score += freq
				for _, src := range tagSources[tag] {
					matched[src] = struct{}{}
				}
			}
		}
		if t.GitHubInfo != nil {
			for _, topic := range t.GitHubInfo.Topics {
				if freq, ok := tagFreq[topic]; ok {
					score += freq
					for _, src := range tagSources[topic] {
						matched[src] = struct{}{}
					}
				}
			}
		}
		if t.Category != "" && installedCats[t.Category] {
			score += 2
		}
		if t.GitHubInfo != nil {
			if t.GitHubInfo.Stars > 10000 {
				score += 2
			} else if t.GitHubInfo.Stars > 1000 {
				score++
			}
		}
		if t.GitHubInfo != nil && t.GitHubInfo.PushedAt != "" {
			if pushed, err := time.Parse(time.RFC3339, t.GitHubInfo.PushedAt); err == nil {
				if time.Since(pushed) < 6*30*24*time.Hour {
					score++
				}
			}
		}
		if score == 0 {
			continue
		}

		var reasons []string
		for n := range matched {
			reasons = append(reasons, n)
		}
		sort.Strings(reasons)
		if len(reasons) > 3 {
			reasons = reasons[:3]
		}

		desc, stars := "", 0
		if t.GitHubInfo != nil {
			desc = t.GitHubInfo.Description
			stars = t.GitHubInfo.Stars
		}

		recs = append(recs, Recommendation{
			ToolIdx:     i,
			Score:       score,
			Reason:      strings.Join(reasons, ", "),
			Category:    t.Category,
			Description: desc,
			Stars:       stars,
		})
		if score > maxScore {
			maxScore = score
		}
	}

	sort.Slice(recs, func(i, j int) bool {
		if recs[i].Score != recs[j].Score {
			return recs[i].Score > recs[j].Score
		}
		return tools[recs[i].ToolIdx].Name < tools[recs[j].ToolIdx].Name
	})
	if len(recs) > Max {
		recs = recs[:Max]
	}
	if maxScore > 0 {
		for i := range recs {
			recs[i].MatchPct = recs[i].Score * 100 / maxScore
			if recs[i].MatchPct < 1 {
				recs[i].MatchPct = 1
			}
		}
	}
	return recs
}
