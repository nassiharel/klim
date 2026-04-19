package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/catalog"
	"github.com/nassiharel/clim/internal/progress"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/service"
)

var (
	listCategoryFlag   string
	listSourceFlag     string
	listCategoriesFlag bool
	listRefreshFlag    bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List developer tools with versions, sources, and update status",
	Long: `List installed developer tools with version info, install sources, and update status.

Flags:
  --category <name>   Show only tools in a specific category (e.g. Cloud, CLI, Containers)
  --source <name>     Show only tools from a specific install source (e.g. brew, winget, apt)
  --categories        Print available category names and exit
  --refresh           Force a fresh scan, ignoring the on-disk cache

Examples:
  clim list                          # all installed tools
  clim list --category Cloud         # only Cloud tools
  clim list --source brew            # only Homebrew-installed tools
  clim list --category IaC --source brew  # combine filters
  clim list --categories             # list available categories
  clim list --refresh                # bypass cache and rescan`,
	RunE: runList,
}

func init() {
	listCmd.Flags().StringVarP(&listCategoryFlag, "category", "c", "", "Filter by category (e.g. Cloud, CLI, Containers)")
	listCmd.Flags().StringVar(&listSourceFlag, "source", "", "Filter by install source (e.g. brew, winget, apt)")
	listCmd.Flags().BoolVar(&listCategoriesFlag, "categories", false, "Print available category names and exit")
	listCmd.Flags().BoolVar(&listRefreshFlag, "refresh", false, "Force a fresh scan, ignoring the on-disk cache")
}

func runList(cmd *cobra.Command, args []string) error {
	sp := progress.New("Loading marketplace catalog...")
	tools, info, scanInfo, err := svc.LoadAndResolveCached(cmd.Context(), listRefreshFlag)
	if err != nil {
		sp.Fail(err.Error())
		return err
	}

	switch {
	case scanInfo != nil && scanInfo.Source == service.ScanSourceCache:
		sp.Done(fmt.Sprintf("Loaded %d tools from scan cache (use --refresh to rescan)", len(tools)))
	case info != nil:
		switch info.Source {
		case catalog.SourceCache:
			sp.Done(fmt.Sprintf("Loaded %d tools from cache", info.Tools))
		case catalog.SourceRemote:
			sp.Done(fmt.Sprintf("Fetched marketplace catalog (%d tools)", info.Tools))
		}
	default:
		sp.Done("Catalog loaded")
	}

	installed := 0
	updates := 0
	for _, t := range tools {
		if t.IsInstalled() {
			installed++
			if t.HasUpdate() {
				updates++
			}
		}
	}
	fmt.Fprintf(os.Stderr, "  ✓ Found %d installed tools", installed)
	if updates > 0 {
		fmt.Fprintf(os.Stderr, ", %d updates available", updates)
	}
	fmt.Fprintln(os.Stderr)

	// --categories: print available categories and exit.
	if listCategoriesFlag {
		printCategories(tools)
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, "TOOL\tCATEGORY\tVERSION\tLATEST\tSOURCE\tSTATUS\tPATH")
	_, _ = fmt.Fprintln(w, "────\t────────\t───────\t──────\t──────\t──────\t────")

	shownInstalled := 0
	shown := 0
	for _, tool := range tools {
		if !tool.IsInstalled() {
			continue
		}
		shownInstalled++

		primary := tool.PrimaryInstance()

		// Apply --category filter.
		if listCategoryFlag != "" && !strings.EqualFold(tool.Category, listCategoryFlag) {
			continue
		}
		// Apply --source filter.
		if listSourceFlag != "" && !strings.EqualFold(string(primary.Source), listSourceFlag) {
			continue
		}

		shown++
		ver := or(primary.Version, "—")
		status := registry.StatusString(primary.Version, tool.Latest)

		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			tool.DisplayName,
			tool.Category,
			ver,
			tool.Latest,
			primary.Source,
			status,
			registry.TruncatePath(primary.Path, 45),
		)

		for _, inst := range tool.Instances[1:] {
			_, _ = fmt.Fprintf(w, "  └─ also:\t\t%s\t\t%s\t\t%s\n",
				or(inst.Version, "—"),
				inst.Source,
				registry.TruncatePath(inst.Path, 45),
			)
		}
	}

	_ = w.Flush()

	if listCategoryFlag != "" || listSourceFlag != "" {
		fmt.Fprintf(os.Stderr, "\n%d/%d installed tools shown", shown, shownInstalled)
		var filters []string
		if listCategoryFlag != "" {
			filters = append(filters, "category="+listCategoryFlag)
		}
		if listSourceFlag != "" {
			filters = append(filters, "source="+listSourceFlag)
		}
		fmt.Fprintf(os.Stderr, " (filtered by %s).\n", strings.Join(filters, ", "))
	} else {
		fmt.Fprintf(os.Stderr, "\n%d/%d tools installed.\n", shownInstalled, len(tools))
	}
	return nil
}

// printCategories collects and prints all unique categories from the tool catalog.
func printCategories(tools []registry.Tool) {
	seen := make(map[string]int) // category → installed count
	for _, tool := range tools {
		if tool.Category == "" {
			continue
		}
		if _, exists := seen[tool.Category]; !exists {
			seen[tool.Category] = 0
		}
		if tool.IsInstalled() {
			seen[tool.Category]++
		}
	}

	cats := make([]string, 0, len(seen))
	for cat := range seen {
		cats = append(cats, cat)
	}
	sort.Strings(cats)

	for _, cat := range cats {
		fmt.Printf("%-14s (%d installed)\n", cat, seen[cat])
	}
}

func or(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}
