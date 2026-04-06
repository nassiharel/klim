package cli

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/detector"
	"github.com/nassiharel/clim/internal/finder"
	"github.com/nassiharel/clim/internal/pkgmgr"
	"github.com/nassiharel/clim/internal/registry"
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export installed tools to a portable YAML manifest",
	Long: `Export all detected tools to YAML, suitable for reinstalling on a new machine.

Usage:
  clim export > my-tools.yaml        # save to file
  clim export                         # print to stdout

On the new machine, install them with:
  clim import my-tools.yaml`,
	RunE: runExport,
}

func init() {
	rootCmd.AddCommand(exportCmd)
}

func runExport(cmd *cobra.Command, args []string) error {
	tools := registry.DefaultTools()

	fmt.Fprintln(os.Stderr, "Scanning installed tools...")
	if err := finder.FindAll(tools); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "Resolving versions...")
	pkgmgr.ResolveVersions(tools, runtime.NumCPU())
	detector.EnrichFallback(tools)

	sort.Slice(tools, func(i, j int) bool {
		return strings.ToLower(tools[i].Name) < strings.ToLower(tools[j].Name)
	})

	var exported []exportTool
	for _, tool := range tools {
		if !tool.IsInstalled() || tool.Disabled {
			continue
		}
		primary := tool.PrimaryInstance()

		exported = append(exported, exportTool{
			Name:        tool.Name,
			DisplayName: tool.DisplayName,
			Version:     primary.Version,
			Source:      string(primary.Source),
			Category:    tool.Category,
			Packages: exportPackages{
				Winget: tool.Packages.Winget,
				Choco:  tool.Packages.Choco,
				Brew:   tool.Packages.Brew,
				Apt:    tool.Packages.Apt,
				Snap:   tool.Packages.Snap,
				NPM:    tool.Packages.NPM,
			},
		})
	}

	manifest := exportManifest{
		GeneratedBy: "clim export",
		OS:          runtime.GOOS,
		Arch:        runtime.GOARCH,
		Tools:       exported,
	}

	data, err := yaml.Marshal(&manifest)
	if err != nil {
		return fmt.Errorf("marshalling export: %w", err)
	}

	header := "# clim — Installed Tools Manifest\n# Generated on " + runtime.GOOS + "/" + runtime.GOARCH + "\n#\n# Reinstall on a new machine:\n#   clim import my-tools.yaml\n#\n\n"
	fmt.Print(header + string(data))

	fmt.Fprintf(os.Stderr, "\n%d tools exported.\n", len(exported))
	return nil
}
