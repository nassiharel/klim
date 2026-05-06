// Package search provides fuzzy search over the tool marketplace catalog.
// Matches against tool names, display names, descriptions, categories,
// tags, and GitHub topics, ranked by relevance and GitHub stars.
package search

import (
	"sort"
	"strings"

	"github.com/nassiharel/klim/internal/registry"
)

// Result represents a single search match.
type Result struct {
	Tool  *registry.Tool
	Score int // higher = more relevant
}

// Search finds tools matching the query, returning results ranked by relevance.
// Matches against name, display_name, description, category, tags, and topics.
func Search(tools []registry.Tool, query string) []Result {
	if query == "" {
		return nil
	}

	terms := tokenize(query)
	if len(terms) == 0 {
		return nil
	}

	var results []Result
	for i := range tools {
		score := scoreTool(&tools[i], terms)
		if score > 0 {
			results = append(results, Result{Tool: &tools[i], Score: score})
		}
	}

	// Sort by score descending, then by stars descending.
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		si, sj := 0, 0
		if results[i].Tool.GitHubInfo != nil {
			si = results[i].Tool.GitHubInfo.Stars
		}
		if results[j].Tool.GitHubInfo != nil {
			sj = results[j].Tool.GitHubInfo.Stars
		}
		return si > sj
	})

	return results
}

// scoreTool computes a relevance score for a tool against search terms.
func scoreTool(t *registry.Tool, terms []string) int {
	score := 0

	nameLower := strings.ToLower(t.Name)
	displayLower := strings.ToLower(t.DisplayName)
	categoryLower := strings.ToLower(t.Category)

	descLower := ""
	var topicsLower []string
	if t.GitHubInfo != nil {
		descLower = strings.ToLower(t.GitHubInfo.Description)
		for _, topic := range t.GitHubInfo.Topics {
			topicsLower = append(topicsLower, strings.ToLower(topic))
		}
	}

	tagsLower := make([]string, len(t.Tags))
	for i, tag := range t.Tags {
		tagsLower[i] = strings.ToLower(tag)
	}

	for _, term := range terms {
		matched := false

		// Exact name match (highest weight).
		if nameLower == term || displayLower == term {
			score += 100
			matched = true
		} else if strings.Contains(nameLower, term) || strings.Contains(displayLower, term) {
			// Partial name match.
			score += 50
			matched = true
		}

		// Category match.
		if strings.Contains(categoryLower, term) {
			score += 30
			matched = true
		}

		// Tag match.
		for _, tag := range tagsLower {
			if tag == term {
				score += 25
				matched = true
				break
			} else if strings.Contains(tag, term) {
				score += 15
				matched = true
				break
			}
		}

		// Topic match.
		for _, topic := range topicsLower {
			if topic == term {
				score += 20
				matched = true
				break
			} else if strings.Contains(topic, term) {
				score += 10
				matched = true
				break
			}
		}

		// Description match (lowest weight).
		if strings.Contains(descLower, term) {
			score += 10
			matched = true
		}

		// If this term didn't match anything, reduce overall score.
		if !matched {
			score -= 50
		}
	}

	// Star boost.
	if t.GitHubInfo != nil && t.GitHubInfo.Stars > 0 {
		if t.GitHubInfo.Stars > 10000 {
			score += 5
		} else if t.GitHubInfo.Stars > 1000 {
			score += 2
		}
	}

	return score
}

// tokenize splits a query into lowercase search terms.
func tokenize(query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	parts := strings.Fields(query)
	var terms []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			terms = append(terms, p)
		}
	}
	return terms
}
