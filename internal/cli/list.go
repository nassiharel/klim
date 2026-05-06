package cli

import (
	"cmp"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/catalog"
	"github.com/nassiharel/klim/internal/progress"
	"github.com/nassiharel/klim/internal/registry"
	"github.com/nassiharel/klim/internal/service"
)

var (
	listCategoryFlag   string
	listSourceFlag     string
	listCategoriesFlag bool
	listRefreshFlag    bool
	listOutput         func() (OutputFormat, error)
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List developer tools with versions, sources, and update status",
	Long: `List installed developer tools with version info, install sources, and update status.

Flags:
  --category <name>    Show only tools in a specific category (e.g. Cloud, CLI, Containers)
  --source <name>      Show only tools from a specific install source (e.g. brew, winget, apt)
  --categories         Print available category names and exit
  --refresh            Force a fresh scan, ignoring the on-disk cache
  --output text|json   Output format (default: text)

Examples:
  klim list                          # all installed tools
  klim list --category Cloud         # only Cloud tools
  klim list --source brew            # only Homebrew-installed tools
  klim list --category IaC --source brew  # combine filters
  klim list --categories             # list available categories
  klim list --refresh                # bypass cache and rescan
  klim list --output json            # machine-readable output`,
	RunE: runList,
}

func init() {
	listCmd.Flags().StringVarP(&listCategoryFlag, "category", "c", "", "Filter by category (e.g. Cloud, CLI, Containers)")
	listCmd.Flags().StringVar(&listSourceFlag, "source", "", "Filter by install source (e.g. brew, winget, apt)")
	listCmd.Flags().BoolVar(&listCategoriesFlag, "categories", false, "Print available category names and exit")
	listCmd.Flags().BoolVar(&listRefreshFlag, "refresh", false, "Force a fresh scan, ignoring the on-disk cache")
	listOutput = addOutputFlag(listCmd, OutputText, OutputJSON)
}

func runList(cmd *cobra.Command, args []string) error {
	out, err := listOutput()
	if err != nil {
		return err
	}

	sp := progress.New("Loading marketplace catalog...")
	tools, info, scanInfo, err := svcFrom(cmd).LoadAndResolveCached(cmd.Context(), listRefreshFlag)
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
		if out == OutputJSON {
			return printJSON(collectListCategories(tools))
		}
		printCategories(tools)
		return nil
	}

	// JSON output: emit structured tool list and skip the human table.
	if out == OutputJSON {
		return printListJSON(tools)
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
		ver := cmp.Or(primary.Version, "—")
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
				cmp.Or(inst.Version, "—"),
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

// or() and the local helper have been replaced by stdlib cmp.Or.

// listJSONTool is the JSON shape for `klim list --output json`.
type listJSONTool struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Category    string `json:"category,omitempty"`
	Installed   bool   `json:"installed"`
	Version     string `json:"version,omitempty"`
	Latest      string `json:"latest,omitempty"`
	Source      string `json:"source,omitempty"`
	Path        string `json:"path,omitempty"`
	Status      string `json:"status,omitempty"`
	Update      bool   `json:"update_available,omitempty"`
}

type listJSONOutput struct {
	OS    string         `json:"os"`
	Arch  string         `json:"arch"`
	Tools []listJSONTool `json:"tools"`
}

func printListJSON(tools []registry.Tool) error {
	out := listJSONOutput{
		OS:   runtime.GOOS,
		Arch: runtime.GOARCH,
	}
	for _, tool := range tools {
		if !tool.IsInstalled() {
			continue
		}
		primary := tool.PrimaryInstance()
		// Apply filters.
		if listCategoryFlag != "" && !strings.EqualFold(tool.Category, listCategoryFlag) {
			continue
		}
		if listSourceFlag != "" && primary != nil && !strings.EqualFold(string(primary.Source), listSourceFlag) {
			continue
		}
		entry := listJSONTool{
			Name:        tool.Name,
			DisplayName: tool.DisplayName,
			Category:    tool.Category,
			Installed:   true,
			Latest:      tool.Latest,
			Update:      tool.HasUpdate(),
		}
		if primary != nil {
			entry.Version = primary.Version
			entry.Source = string(primary.Source)
			entry.Path = primary.Path
			entry.Status = registry.StatusString(primary.Version, tool.Latest)
		}
		out.Tools = append(out.Tools, entry)
	}
	return printJSON(out)
}

// listJSONCategory is the per-category JSON shape for --categories --output json.
type listJSONCategory struct {
	Name      string `json:"name"`
	Installed int    `json:"installed"`
}

func collectListCategories(tools []registry.Tool) []listJSONCategory {
	seen := make(map[string]int)
	for _, t := range tools {
		if t.Category == "" {
			continue
		}
		if _, ok := seen[t.Category]; !ok {
			seen[t.Category] = 0
		}
		if t.IsInstalled() {
			seen[t.Category]++
		}
	}
	cats := make([]string, 0, len(seen))
	for cat := range seen {
		cats = append(cats, cat)
	}
	sort.Strings(cats)
	out := make([]listJSONCategory, 0, len(cats))
	for _, cat := range cats {
		out = append(out, listJSONCategory{Name: cat, Installed: seen[cat]})
	}
	return out
}
