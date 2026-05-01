// Package onboard provides role-based tool recommendation logic
// shared between the CLI wizard and TUI onboard sub-tab.
package onboard

import (
	"sort"
	"strings"

	"github.com/nassiharel/clim/internal/registry"
)

// Role defines a development role with categories and tags used
// to score marketplace tools for relevance.
type Role struct {
	Name        string
	Description string
	Categories  []string
	Tags        []string
}

// Roles is the list of available development roles.
var Roles = []Role{
	{
		Name:        "web",
		Description: "Web Development (Frontend/Backend)",
		Categories:  []string{"JavaScript", "Web", "API"},
		Tags:        []string{"javascript", "typescript", "node", "web", "frontend", "backend", "api", "http"},
	},
	{
		Name:        "devops",
		Description: "DevOps / Cloud / Infrastructure",
		Categories:  []string{"Cloud", "IaC", "Containers", "K8s", "CI/CD"},
		Tags:        []string{"cloud", "aws", "azure", "gcp", "kubernetes", "docker", "terraform", "ci", "cd", "devops", "infrastructure"},
	},
	{
		Name:        "data",
		Description: "Data / ML / AI",
		Categories:  []string{"Data", "ML", "Python"},
		Tags:        []string{"data", "ml", "ai", "python", "jupyter", "analytics"},
	},
	{
		Name:        "mobile",
		Description: "Mobile Development (iOS/Android)",
		Categories:  []string{"Mobile", "JavaScript"},
		Tags:        []string{"mobile", "ios", "android", "flutter", "react-native"},
	},
	{
		Name:        "systems",
		Description: "Systems / Embedded / Low-level",
		Categories:  []string{"Systems", "Compilers", "Debug"},
		Tags:        []string{"systems", "c", "c++", "rust", "embedded", "compiler", "debug", "performance"},
	},
	{
		Name:        "security",
		Description: "Security / Pen-testing",
		Categories:  []string{"Security", "Network"},
		Tags:        []string{"security", "pentest", "crypto", "network", "vulnerability"},
	},
}

// ScoredTool holds a tool and its relevance score for a given role.
type ScoredTool struct {
	Tool  registry.Tool
	Index int // index into the original tools slice
	Score int
}

// Recommend returns tools scored and ranked for the given role.
// Only uninstalled tools with packages for the current OS are included.
// Results are capped at maxResults (0 = no cap).
func Recommend(role *Role, tools []registry.Tool, maxResults int) []ScoredTool {
	catSet := make(map[string]bool, len(role.Categories))
	for _, c := range role.Categories {
		catSet[strings.ToLower(c)] = true
	}
	tagSet := make(map[string]bool, len(role.Tags))
	for _, t := range role.Tags {
		tagSet[strings.ToLower(t)] = true
	}

	var scored []ScoredTool
	for i, t := range tools {
		if t.IsInstalled() {
			continue
		}
		if !t.Packages.HasAnyPackageForOS() {
			continue
		}

		s := 0
		if catSet[strings.ToLower(t.Category)] {
			s += 10
		}
		for _, tag := range t.Tags {
			if tagSet[strings.ToLower(tag)] {
				s += 5
			}
		}
		if t.GitHubInfo != nil && t.GitHubInfo.Stars > 1000 {
			s += 2
		}
		if t.GitHubInfo != nil && t.GitHubInfo.Stars > 10000 {
			s += 3
		}

		if s > 0 {
			scored = append(scored, ScoredTool{Tool: t, Index: i, Score: s})
		}
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].Tool.Name < scored[j].Tool.Name
	})

	if maxResults > 0 && len(scored) > maxResults {
		scored = scored[:maxResults]
	}

	return scored
}

// FindRole returns a pointer to the role with the given name (case-insensitive),
// or nil if not found.
func FindRole(name string) *Role {
	for i := range Roles {
		if strings.EqualFold(Roles[i].Name, name) {
			return &Roles[i]
		}
	}
	return nil
}
