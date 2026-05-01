package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/search"
)

var searchCategoryFlag string
var searchLimitFlag int

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search the tool marketplace",
	Long: `Search the tool marketplace by name, description, category, or tags.

Results are ranked by relevance and GitHub stars, with platform badges
showing which package managers support each tool.

Examples:
  clim search json                   # find JSON tools
  clim search "kubernetes dashboard" # multi-word search
  clim search cloud --category Cloud # filter by category`,
	Args: requireMinArgs(1, "clim search <query>"),
	RunE: runSearch,
}

func init() {
	searchCmd.Flags().StringVarP(&searchCategoryFlag, "category", "c", "", "Filter by category")
	searchCmd.Flags().IntVarP(&searchLimitFlag, "limit", "n", 15, "Max results to show")
	// Registered in root.go with command group.
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := strings.Join(args, " ")

	// Load catalog.
	tools, _, catErr := svc.Catalog.LoadTools(cmd.Context())
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

	if len(results) == 0 {
		fmt.Fprintf(os.Stderr, "No tools found matching %q\n", query)
		return nil
	}

	// Limit results.
	totalMatches := len(results)
	if searchLimitFlag > 0 && len(results) > searchLimitFlag {
		results = results[:searchLimitFlag]
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
