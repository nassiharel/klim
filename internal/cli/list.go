package cli

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/detector"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/version"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List detected CLI tools with versions and paths",
	RunE:  runList,
}

func runList(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tools := registry.DefaultTools()
	cache := version.LoadCache()
	checker := version.NewHTTPChecker(version.TokenFromEnv())

	// Phase A: detect all local tools (parallel).
	fmt.Fprintln(os.Stderr, "Scanning installed tools...")
	detections := detector.DetectAll(ctx, tools)

	// Phase B: check latest versions (parallel, with cache).
	fmt.Fprintln(os.Stderr, "Checking for updates...")
	latest := version.CheckAll(ctx, checker, cache, tools)

	// Phase C: merge, compare, and render.
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "TOOL\tVERSION\tLATEST\tPATH\tSTATUS")
	fmt.Fprintln(w, "────\t───────\t──────\t────\t──────")

	for i, tool := range tools {
		det := detections[i]
		lat := latest[i]

		installed := "—"
		latestVer := "—"
		path := "—"

		if det.Found {
			path = det.Path
			if det.Version != "" {
				installed = det.Version
			}
		}

		if lat.Version != "" {
			latestVer = lat.Version
		} else if lat.Error != nil {
			latestVer = "err"
		}

		// Determine status.
		var status string
		if !det.Found {
			status = "✗ not found"
		} else if det.Version == "" {
			status = "? version unknown"
		} else if lat.Error != nil {
			status = "? check failed"
		} else {
			s, _ := version.CompareVersions(det.Version, lat.Version)
			status = version.StatusString(s)
		}

		// Truncate long paths.
		if len(path) > 40 {
			path = "..." + path[len(path)-37:]
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			tool.DisplayName,
			installed,
			latestVer,
			path,
			status,
		)
	}

	w.Flush()

	// Persist cache for next run.
	cache.Save()

	return nil
}
