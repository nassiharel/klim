package cli

import (
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/manifest"
	"github.com/nassiharel/clim/internal/progress"
	"github.com/nassiharel/clim/internal/service"
)

var exportRefreshFlag bool

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export installed tools to a portable YAML manifest",
	Long: `Export all detected tools to YAML, suitable for reinstalling on a new machine.

Usage:
  clim export > my-tools.yaml        # save to file
  clim export                         # print to stdout
  clim export --refresh               # force a fresh scan (skip cache)

On the new machine, install them with:
  clim import my-tools.yaml`,
	RunE: runExport,
}

func init() {
	exportCmd.Flags().BoolVar(&exportRefreshFlag, "refresh", false, "Force a fresh scan, ignoring the on-disk cache")
	rootCmd.AddCommand(exportCmd)
}

func runExport(cmd *cobra.Command, args []string) error {
	sp := progress.New("Scanning installed tools...")
	tools, _, scanInfo, err := svc.LoadAndResolveCached(cmd.Context(), exportRefreshFlag)
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	if scanInfo != nil && scanInfo.Source == service.ScanSourceCache {
		sp.Done("Loaded from cache (use --refresh to rescan)")
	} else {
		sp.Done("Tools scanned")
	}

	var exported []manifest.Tool
	for _, tool := range tools {
		if !tool.IsInstalled() {
			continue
		}
		exported = append(exported, manifest.FromRegistryTool(tool))
	}

	m := manifest.Manifest{
		GeneratedBy: "clim export",
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		Tools:       exported,
	}

	data, err := yaml.Marshal(&m)
	if err != nil {
		return fmt.Errorf("marshalling export: %w", err)
	}

	header := "# clim — Installed Tools Manifest\n# Generated on " + runtime.GOOS + "/" + runtime.GOARCH + "\n#\n# Reinstall on a new machine:\n#   clim import my-tools.yaml\n#\n\n"
	fmt.Print(header + string(data))

	fmt.Fprintf(os.Stderr, "\n%d tools exported.\n", len(exported))
	return nil
}
