package cli

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/detector"
	"github.com/nassiharel/clim/internal/finder"
	"github.com/nassiharel/clim/internal/pkgmgr"
	"github.com/nassiharel/clim/internal/registry"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List developer tools with versions, sources, and update status",
	RunE:  runList,
}

func runList(cmd *cobra.Command, args []string) error {
	tools := registry.DefaultTools()

	fmt.Fprintln(os.Stderr, "Finding tools...")
	if err := finder.FindAll(tools); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "Checking versions...")
	pkgmgr.ResolveVersions(tools, runtime.NumCPU())
	detector.EnrichFallback(tools)

	sort.Slice(tools, func(i, j int) bool {
		return strings.ToLower(tools[i].Name) < strings.ToLower(tools[j].Name)
	})

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "TOOL\tVERSION\tLATEST\tSOURCE\tSTATUS\tPATH")
	fmt.Fprintln(w, "────\t───────\t──────\t──────\t──────\t────")

	installed := 0
	for _, tool := range tools {
		if !tool.IsInstalled() {
			continue
		}
		installed++

		primary := tool.PrimaryInstance()
		ver := or(primary.Version, "—")
		status := registry.StatusString(primary.Version, tool.Latest)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			tool.DisplayName,
			ver,
			tool.Latest,
			primary.Source,
			status,
			registry.TruncatePath(primary.Path, 45),
		)

		for _, inst := range tool.Instances[1:] {
			fmt.Fprintf(w, "  └─ also:\t%s\t\t%s\t\t%s\n",
				or(inst.Version, "—"),
				inst.Source,
				registry.TruncatePath(inst.Path, 45),
			)
		}
	}

	w.Flush()
	fmt.Fprintf(os.Stderr, "\n%d/%d tools installed.\n", installed, len(tools))
	return nil
}

func or(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}
