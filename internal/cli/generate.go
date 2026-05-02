package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/generate"
	"github.com/nassiharel/clim/internal/teamfile"
)

var generateCmd = &cobra.Command{
	Use:   "generate <github-action|dockerfile|devcontainer>",
	Short: "Generate CI/container configs from .clim.yaml",
	Long: `Auto-generate CI and container configuration files from your .clim.yaml
tool requirements.

Generators:
  github-action   GitHub Actions workflow step
  dockerfile      Dockerfile with tool installations
  devcontainer    devcontainer.json for VS Code / GitHub Codespaces

The generated files use the package IDs from the clim marketplace
to produce install commands for each tool.`,
	Args:      requireArgs(1, "clim generate <github-action|dockerfile|devcontainer>"),
	ValidArgs: []string{"github-action", "dockerfile", "devcontainer"},
	RunE:      runGenerate,
}

var generateFileFlag string
var generateOutputFlag string
var generateBaseFlag string
var generateOSFlag string

func init() {
	generateCmd.Flags().StringVarP(&generateFileFlag, "file", "f", "", "Path to .clim.yaml (default: auto-detect)")
	generateCmd.Flags().StringVarP(&generateOutputFlag, "output", "o", "", "Write to file instead of stdout")
	generateCmd.Flags().StringVar(&generateBaseFlag, "base", "", "Base image for Dockerfile (default: ubuntu:24.04)")
	generateCmd.Flags().StringVar(&generateOSFlag, "os", "ubuntu", "Target OS: ubuntu, debian, alpine, fedora, macos, windows")
	// Registered in root.go with command group.
}

func runGenerate(cmd *cobra.Command, args []string) error {
	format := args[0]

	// Find and parse .clim.yaml.
	path := generateFileFlag
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
		path = teamfile.Find(cwd)
		if path == "" {
			return fmt.Errorf("no .clim.yaml found (searched from %s to root)\n\nCreate one with: clim init", cwd)
		}
	}

	tf, err := teamfile.Parse(path)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}

	if len(tf.Tools) == 0 && len(tf.Optional) == 0 {
		return errors.New(".clim.yaml has no tools defined")
	}

	// Load catalog for package IDs.
	tools, _, catErr := svcFrom(cmd).Catalog.LoadTools(cmd.Context())
	if catErr != nil {
		return fmt.Errorf("loading catalog: %w", catErr)
	}

	installs := generate.ResolveInstalls(tf, tools)

	projectName := tf.Name
	if projectName == "" {
		projectName = filepath.Base(filepath.Dir(path))
	}

	// Generate.
	opts := generate.Options{
		OS:          generateOSFlag,
		BaseImage:   generateBaseFlag,
		ProjectName: projectName,
	}

	var output string
	switch format {
	case "github-action":
		output = generate.GitHubAction(installs, opts)
	case "dockerfile":
		output = generate.Dockerfile(installs, opts)
	case "devcontainer":
		output = generate.DevContainer(installs, opts)
	default:
		return fmt.Errorf("unknown format %q — use github-action, dockerfile, or devcontainer", format)
	}

	// Write output.
	if generateOutputFlag != "" {
		if err := os.WriteFile(generateOutputFlag, []byte(output), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", generateOutputFlag, err)
		}
		fmt.Fprintf(os.Stderr, "✓ Generated %s → %s (%d tools)\n", format, generateOutputFlag, len(installs))
		return nil
	}

	fmt.Print(output)
	fmt.Fprintf(os.Stderr, "\n%d tools resolved from %s\n", len(installs), path)
	return nil
}
