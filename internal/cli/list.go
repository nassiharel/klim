package cli

import (
	"fmt"
	"os"
	"runtime"
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

	// Phase A: find all installations on PATH.
	fmt.Fprintln(os.Stderr, "Finding tools...")
	finder.FindAll(tools)

	// Phase B: get versions from package managers.
	fmt.Fprintln(os.Stderr, "Checking versions...")
	pkgmgr.ResolveVersions(tools, runtime.NumCPU())

	// Phase C: fallback version detection (PE metadata, Go buildinfo).
	detector.EnrichFallback(tools)

	// Phase D: render table.
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
		ver := valueOr(primary.Version, "—")
		lat := valueOr(tool.Latest, "")
		source := valueOr(primary.Source, "")
		status := computeStatus(primary.Version, tool.Latest)
		path := primary.Path
		if len(path) > 45 {
			path = "..." + path[len(path)-42:]
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			tool.DisplayName, ver, lat, source, status, path)

		// Show additional instances.
		for _, inst := range tool.Instances {
			if inst.IsPrimary {
				continue
			}
			instVer := valueOr(inst.Version, "—")
			instPath := inst.Path
			if len(instPath) > 45 {
				instPath = "..." + instPath[len(instPath)-42:]
			}
			fmt.Fprintf(w, "  └─ also:\t%s\t\t%s\t\t%s\n",
				instVer, inst.Source, instPath)
		}
	}

	w.Flush()
	fmt.Fprintf(os.Stderr, "\n%d/%d tools installed.\n", installed, len(tools))

	return nil
}

func computeStatus(installed, latest string) string {
	if installed == "" || installed == "—" {
		if latest != "" {
			return "?"
		}
		return ""
	}
	if latest == "" {
		return ""
	}
	if registry.VersionsMatch(installed, latest) {
		return "✓ up to date"
	}
	return "⬆ update"
}

func valueOr(val, fallback string) string {
	if val == "" {
		return fallback
	}
	return val
}
