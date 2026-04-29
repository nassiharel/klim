package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/progress"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/teamfile"
)

var initMinVersionFlag bool
var initNameFlag string
var initAllFlag bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate a .clim.yaml from project files",
	Long: `Scan your project directory to detect which CLI tools it needs,
then generate a .clim.yaml team manifest. Detection reads Dockerfile,
package.json, go.mod, CI workflows, Helm charts, Terraform files, and more.

Only tools that are both detected AND installed are included (so versions
can be pinned). Tools detected but not installed are listed as suggestions.

Usage:
  clim init                        # auto-detect from project files
  clim init --all                  # include ALL installed tools (no detection)
  clim init --min-version          # include >=X.Y version constraints
  clim init --name my-project      # set project name`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().BoolVar(&initMinVersionFlag, "min-version", false, "Include minimum version constraints (>=X.Y)")
	initCmd.Flags().StringVar(&initNameFlag, "name", "", "Project name for the manifest")
	initCmd.Flags().BoolVar(&initAllFlag, "all", false, "Include all installed tools (skip project detection)")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	outPath := filepath.Join(".", teamfile.FileName)

	// Check if file already exists.
	if _, err := os.Stat(outPath); err == nil {
		return fmt.Errorf("%s already exists (delete it first or edit manually)", outPath)
	}

	// Scan installed tools. Use full resolution when --min-version needs versions.
	sp := progress.New("Scanning installed tools...")
	var tools []registry.Tool
	var scanErr error
	if initMinVersionFlag {
		tools, _, _, scanErr = svc.LoadAndResolveCached(cmd.Context(), false)
	} else {
		tools, _, scanErr = svc.ScanOnly(cmd.Context())
	}
	if scanErr != nil {
		sp.Fail(scanErr.Error())
		return scanErr
	}
	sp.Done("Tools scanned")

	sort.Slice(tools, func(i, j int) bool {
		return strings.ToLower(tools[i].Name) < strings.ToLower(tools[j].Name)
	})

	var tf *teamfile.TeamFile

	if initAllFlag {
		// --all: include every installed tool.
		tf = teamfile.Generate(tools, initMinVersionFlag)
	} else {
		// Project-aware detection.
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}

		fmt.Fprintln(os.Stderr, "Detecting tools from project files...")
		result := teamfile.DetectFromProject(cwd)
		detected := result.Tools

		fmt.Fprintf(os.Stderr, "  Scanned %d files in %d directories\n", result.FilesScanned, result.DirsScanned)

		if len(detected) == 0 {
			fmt.Fprintln(os.Stderr, "No project files detected. Use --all to include all installed tools.")
			return nil
		}

		// Build installed tool map.
		installedMap := make(map[string]*registry.Tool, len(tools))
		for i := range tools {
			if tools[i].IsInstalled() {
				installedMap[tools[i].Name] = &tools[i]
			}
		}

		// Split into installed (include) and not-installed (suggest).
		tf = &teamfile.TeamFile{}
		var notInstalled []teamfile.DetectedTool

		for _, d := range detected {
			rt, ok := installedMap[d.Name]
			if !ok {
				notInstalled = append(notInstalled, d)
				continue
			}
			req := teamfile.RequiredTool{Name: d.Name}
			if initMinVersionFlag {
				ver := rt.InstalledVersion()
				if ver != "" {
					req.Version = ">=" + ver
				}
			}
			tf.Tools = append(tf.Tools, req)
		}

		// Print detection summary.
		fmt.Fprintf(os.Stderr, "\nDetected %d tools from project files:\n\n", len(detected))
		for _, d := range detected {
			icon := "✓"
			if _, ok := installedMap[d.Name]; !ok {
				icon = "✗"
			}
			fmt.Fprintf(os.Stderr, "  %s %-20s  (from %s)\n", icon, d.Name, d.Source)
		}

		if len(notInstalled) > 0 {
			fmt.Fprintf(os.Stderr, "\n⚠ %d detected tool(s) not installed:\n", len(notInstalled))
			for _, d := range notInstalled {
				fmt.Fprintf(os.Stderr, "    · %s  (from %s)\n", d.Name, d.Source)
			}
			fmt.Fprintln(os.Stderr, "  Install them first, then re-run clim init to pin versions.")
		}

		// Ecosystem suggestions.
		if len(result.Suggestions) > 0 {
			fmt.Fprintf(os.Stderr, "\n💡 Suggested tools for this project:\n")
			for _, s := range result.Suggestions {
				icon := "○"
				if _, ok := installedMap[s.Name]; ok {
					icon = "●"
				}
				fmt.Fprintf(os.Stderr, "  %s %-20s  (%s)\n", icon, s.Name, s.Source)
			}
		}
	}

	if initNameFlag != "" {
		tf.Name = initNameFlag
	}

	if len(tf.Tools) == 0 {
		fmt.Fprintln(os.Stderr, "No tools to include in manifest.")
		return nil
	}

	// Write.
	if err := teamfile.Write(tf, outPath); err != nil {
		return err
	}

	abs, _ := filepath.Abs(outPath)
	fmt.Fprintf(os.Stderr, "\n✓ Generated %s (%d tools)\n", abs, len(tf.Tools))

	// Auto-register project.
	name := tf.Name
	if name == "" {
		name = filepath.Base(filepath.Dir(abs))
	}
	_ = teamfile.AddProject(filepath.Dir(abs), name, len(tf.Tools))

	fmt.Fprintln(os.Stderr, "\nTeammates can now run:")
	fmt.Fprintln(os.Stderr, "  clim check    # validate their environment")
	return nil
}
