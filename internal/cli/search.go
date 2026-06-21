package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/registry"
	"github.com/nassiharel/klim/internal/search"
)

var searchCategoryFlag string
var searchLimitFlag int
var searchOutput func() (OutputFormat, error)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search the tool marketplace",
	Long: `Search the tool marketplace by name, description, category, or tags.

Results are ranked by relevance and GitHub stars, with platform badges
showing which package managers support each tool.

Examples:
  klim tool search json                   # find JSON tools
  klim tool search "kubernetes dashboard" # multi-word search
  klim tool search cloud --category Cloud # filter by category
  klim tool search json --output json     # machine-readable results`,
	Args: requireMinArgs(1, "klim tool search <query>"),
	RunE: runSearch,
}

func init() {
	searchCmd.Flags().StringVarP(&searchCategoryFlag, "category", "c", "", "Filter by category")
	searchCmd.Flags().IntVarP(&searchLimitFlag, "limit", "n", 15, "Max results to show")
	searchOutput = addOutputFlag(searchCmd, OutputText, OutputJSON, OutputYAML)
	// Registered in root.go with command group.
}

func runSearch(cmd *cobra.Command, args []string) error {
	out, err := searchOutput()
	if err != nil {
		return err
	}
	query := strings.Join(args, " ")

	// Load catalog + scan PATH for install status (no version resolution).
	tools, _, catErr := svcFrom(cmd).ScanOnly(cmd.Context())
	if catErr != nil {
		return fmt.Errorf("loading catalog: %w", catErr)
	}

	// Category filter.
	if searchCategoryFlag != "" {
		var filtered []registry.Tool
		for _, t := range tools {
			if strings.EqualFold(t.Category, searchCategoryFlag) {
				filtered = append(filtered, t)
			}
		}
		tools = filtered
	}

	results := search.Search(tools, query)

	// Limit results.
	totalMatches := len(results)
	if searchLimitFlag > 0 && len(results) > searchLimitFlag {
		results = results[:searchLimitFlag]
	}

	if out == OutputJSON || out == OutputYAML {
		return printSearchStructured(out, query, results, totalMatches)
	}

	if len(results) == 0 {
		fmt.Fprintf(os.Stderr, "No tools found matching %q\n", query)
		return nil
	}

	// Display.
	if totalMatches > len(results) {
		fmt.Fprintf(os.Stderr, "\nShowing %d of %d result(s) for %q:\n\n", len(results), totalMatches, query)
	} else {
		fmt.Fprintf(os.Stderr, "\n%d result(s) for %q:\n\n", len(results), query)
	}

	for i, r := range results {
		t := r.Tool
		installed := " "
		if t.IsInstalled() {
			installed = "✓"
		}

		stars := "      "
		if t.GitHubInfo != nil && t.GitHubInfo.Stars > 0 {
			if t.GitHubInfo.Stars >= 1000 {
				stars = fmt.Sprintf("⭐%4.1fk", float64(t.GitHubInfo.Stars)/1000)
			} else {
				stars = fmt.Sprintf("⭐ %4d", t.GitHubInfo.Stars)
			}
		}

		desc := ""
		if t.GitHubInfo != nil && t.GitHubInfo.Description != "" {
			desc = t.GitHubInfo.Description
			runes := []rune(desc)
			if len(runes) > 55 {
				desc = string(runes[:52]) + "..."
			}
		}

		platforms := platformIcons(t)

		fmt.Fprintf(os.Stderr, "  %s %2d. %-18s %s  %-8s  %s  %s\n",
			installed, i+1, t.DisplayName, stars, t.Category, platforms, desc)
	}

	return nil
}

func platformIcons(t *registry.Tool) string {
	var icons []string
	if t.Packages.Brew != "" {
		icons = append(icons, "🍺")
	}
	if t.Packages.Winget != "" {
		icons = append(icons, "📦")
	}
	if t.Packages.Apt != "" {
		icons = append(icons, "🐧")
	}
	if t.Packages.Scoop != "" {
		icons = append(icons, "🪣")
	}
	if t.Packages.Choco != "" {
		icons = append(icons, "🍫")
	}
	if t.Packages.Snap != "" {
		icons = append(icons, "📌")
	}
	if t.Packages.NPM != "" {
		icons = append(icons, "📗")
	}
	return strings.Join(icons, "")
}

// searchJSONResult is the JSON shape for a single search hit.
type searchJSONResult struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name,omitempty"`
	Category    string   `json:"category,omitempty"`
	Description string   `json:"description,omitempty"`
	Stars       int      `json:"stars,omitempty"`
	Installed   bool     `json:"installed"`
	Sources     []string `json:"sources,omitempty"`
	Score       int      `json:"score,omitempty"`
}

type searchJSONOutput struct {
	Query        string             `json:"query"`
	TotalMatches int                `json:"total_matches"`
	Returned     int                `json:"returned"`
	Results      []searchJSONResult `json:"results"`
}

func printSearchStructured(format OutputFormat, query string, results []search.Result, totalMatches int) error {
	out := searchJSONOutput{
		Query:        query,
		TotalMatches: totalMatches,
		Returned:     len(results),
	}
	for _, r := range results {
		t := r.Tool
		entry := searchJSONResult{
			Name:        t.Name,
			DisplayName: t.DisplayName,
			Category:    t.Category,
			Installed:   t.IsInstalled(),
			Score:       r.Score,
		}
		if t.GitHubInfo != nil {
			entry.Description = t.GitHubInfo.Description
			entry.Stars = t.GitHubInfo.Stars
		}
		entry.Sources = availableSources(t)
		out.Results = append(out.Results, entry)
	}
	return printStructured(format, out)
}

func availableSources(t *registry.Tool) []string {
	var srcs []string
	if t.Packages.Brew != "" {
		srcs = append(srcs, "brew")
	}
	if t.Packages.Winget != "" {
		srcs = append(srcs, "winget")
	}
	if t.Packages.Choco != "" {
		srcs = append(srcs, "choco")
	}
	if t.Packages.Scoop != "" {
		srcs = append(srcs, "scoop")
	}
	if t.Packages.Apt != "" {
		srcs = append(srcs, "apt")
	}
	if t.Packages.Snap != "" {
		srcs = append(srcs, "snap")
	}
	if t.Packages.NPM != "" {
		srcs = append(srcs, "npm")
	}
	return srcs
}
