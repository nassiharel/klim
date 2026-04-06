package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/finder"
	"github.com/nassiharel/clim/internal/registry"
)

var yesFlag bool

var importCmd = &cobra.Command{
	Use:   "import <file>",
	Short: "Install tools from an exported manifest",
	Long: `Install tools listed in a YAML manifest (created by clim export).

The manifest is cross-platform — package IDs for all managers are included,
and clim picks the best one for your current OS.

Usage:
  clim import my-tools.yaml          # interactive: confirm before installing
  clim import my-tools.yaml --yes    # non-interactive: install all without prompting`,
	Args: cobra.ExactArgs(1),
	RunE: runImport,
}

func init() {
	importCmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "Install all tools without prompting")
	rootCmd.AddCommand(importCmd)
}

func runImport(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	// Read and parse the manifest.
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading manifest: %w", err)
	}

	var manifest exportManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parsing manifest: %w", err)
	}

	if len(manifest.Tools) == 0 {
		fmt.Println("No tools in manifest.")
		return nil
	}

	fmt.Fprintf(os.Stderr, "Manifest: %d tools (from %s/%s)\n", len(manifest.Tools), manifest.OS, manifest.Arch)

	// Load registry and scan PATH to know what's already installed.
	regTools := registry.DefaultTools()
	fmt.Fprintln(os.Stderr, "Scanning installed tools...")
	if err := finder.FindAll(regTools); err != nil {
		return fmt.Errorf("scanning PATH: %w", err)
	}

	// Build a lookup by name.
	regMap := make(map[string]*registry.Tool, len(regTools))
	for i := range regTools {
		regMap[regTools[i].Name] = &regTools[i]
	}

	// Build the install plan.
	type installPlan struct {
		name    string
		display string
		cmd     string
		source  string
	}

	var toInstall []installPlan
	var alreadyInstalled []string
	var noPackage []string

	for _, mt := range manifest.Tools {
		rt, exists := regMap[mt.Name]
		if !exists {
			// Tool not in registry — try to use manifest's package IDs directly.
			pkgs := registry.PackageIDs{
				Winget: mt.Packages.Winget,
				Choco:  mt.Packages.Choco,
				Brew:   mt.Packages.Brew,
				Apt:    mt.Packages.Apt,
				Snap:   mt.Packages.Snap,
				NPM:    mt.Packages.NPM,
			}
			src := pkgs.BestInstallSource()
			installCmd := pkgs.InstallCmd(src)
			if installCmd == "" {
				noPackage = append(noPackage, mt.Name)
				continue
			}
			toInstall = append(toInstall, installPlan{
				name:    mt.Name,
				display: mt.DisplayName,
				cmd:     installCmd,
				source:  string(src),
			})
			continue
		}

		if rt.IsInstalled() {
			alreadyInstalled = append(alreadyInstalled, mt.DisplayName)
			continue
		}

		src := rt.Packages.BestInstallSource()
		installCmd := rt.Packages.InstallCmd(src)
		if installCmd == "" {
			noPackage = append(noPackage, mt.Name)
			continue
		}

		toInstall = append(toInstall, installPlan{
			name:    mt.Name,
			display: mt.DisplayName,
			cmd:     installCmd,
			source:  string(src),
		})
	}

	// Print summary.
	fmt.Fprintf(os.Stderr, "\n")
	if len(alreadyInstalled) > 0 {
		fmt.Fprintf(os.Stderr, "  ✓ Already installed (%d): %s\n", len(alreadyInstalled), strings.Join(alreadyInstalled, ", "))
	}
	if len(noPackage) > 0 {
		fmt.Fprintf(os.Stderr, "  ⚠ No package available for %s (%d): %s\n", runtime.GOOS, len(noPackage), strings.Join(noPackage, ", "))
	}
	if len(toInstall) == 0 {
		fmt.Fprintln(os.Stderr, "\n  Nothing to install — all tools are present!")
		return nil
	}

	fmt.Fprintf(os.Stderr, "\n  To install (%d):\n", len(toInstall))
	for _, p := range toInstall {
		fmt.Fprintf(os.Stderr, "    %s  →  %s\n", p.display, p.cmd)
	}
	fmt.Fprintln(os.Stderr)

	// Confirm unless --yes.
	if !yesFlag {
		fmt.Fprint(os.Stderr, "  Proceed? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(strings.ToLower(line))
		if line != "y" && line != "yes" {
			fmt.Fprintln(os.Stderr, "  Cancelled.")
			return nil
		}
	}

	// Execute installs sequentially with live terminal output.
	succeeded := 0
	failed := 0
	for _, p := range toInstall {
		fmt.Fprintf(os.Stderr, "\n──── Installing %s via %s ────\n", p.display, p.source)

		c := buildImportShellCmd(p.cmd)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr

		if err := c.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗ %s failed: %s\n", p.display, err)
			failed++
		} else {
			fmt.Fprintf(os.Stderr, "  ✓ %s installed\n", p.display)
			succeeded++
		}
	}

	fmt.Fprintf(os.Stderr, "\n──── Done: %d installed, %d failed, %d already present ────\n",
		succeeded, failed, len(alreadyInstalled))
	return nil
}

func buildImportShellCmd(cmdStr string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/C", cmdStr)
	}
	return exec.Command("sh", "-c", cmdStr)
}
