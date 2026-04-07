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
	"github.com/nassiharel/clim/internal/manifest"
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

	var m manifest.Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("parsing manifest: %w", err)
	}

	if len(m.Tools) == 0 {
		fmt.Println("No tools in manifest.")
		return nil
	}

	fmt.Fprintf(os.Stderr, "Manifest: %d tools (from %s/%s)\n", len(m.Tools), m.OS, m.Arch)

	// Load registry and scan PATH to know what's already installed.
	regTools := registry.DefaultTools()
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
		cmdArgs []string
		source  string
	}

	var toInstall []installPlan
	var alreadyInstalled []string
	var noPackage []string
	var noPkgMgr []string

	for _, mt := range m.Tools {
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
			// Prefer the source recorded in the manifest if it's available.
			if mt.Source != "" {
				preferred := registry.InstallSource(mt.Source)
				if args := pkgs.InstallArgs(preferred); args != nil {
					src = preferred
				}
			}
			installArgs := pkgs.InstallArgs(src)
			if installArgs == nil {
				if pkgs.HasAnyPackageForOS() {
					noPkgMgr = append(noPkgMgr, mt.Name)
				} else {
					noPackage = append(noPackage, mt.Name)
				}
				continue
			}
			toInstall = append(toInstall, installPlan{
				name:    mt.Name,
				display: mt.DisplayName,
				cmdArgs: installArgs,
				source:  string(src),
			})
			continue
		}

		if rt.IsInstalled() {
			alreadyInstalled = append(alreadyInstalled, mt.DisplayName)
			continue
		}

		src := rt.Packages.BestInstallSource()
		// Prefer the source recorded in the manifest if it's available.
		if mt.Source != "" {
			preferred := registry.InstallSource(mt.Source)
			if args := rt.Packages.InstallArgs(preferred); args != nil {
				src = preferred
			}
		}
		installArgs := rt.Packages.InstallArgs(src)
		if installArgs == nil {
			if rt.Packages.HasAnyPackageForOS() {
				noPkgMgr = append(noPkgMgr, mt.Name)
			} else {
				noPackage = append(noPackage, mt.Name)
			}
			continue
		}

		toInstall = append(toInstall, installPlan{
			name:    mt.Name,
			display: mt.DisplayName,
			cmdArgs: installArgs,
			source:  string(src),
		})
	}

	fmt.Fprintf(os.Stderr, "\n──── Import Summary ────\n\n")
	fmt.Fprintf(os.Stderr, "  Manifest:  %d tools (from %s/%s)\n\n", len(m.Tools), m.OS, m.Arch)

	if len(alreadyInstalled) > 0 {
		fmt.Fprintf(os.Stderr, "  ✓ Already installed (%d):\n", len(alreadyInstalled))
		for _, name := range alreadyInstalled {
			fmt.Fprintf(os.Stderr, "    · %s\n", name)
		}
		fmt.Fprintln(os.Stderr)
	}
	if len(noPackage) > 0 {
		fmt.Fprintf(os.Stderr, "  ⚠ No package for %s (%d):\n", runtime.GOOS, len(noPackage))
		for _, name := range noPackage {
			fmt.Fprintf(os.Stderr, "    · %s\n", name)
		}
		fmt.Fprintln(os.Stderr)
	}
	if len(noPkgMgr) > 0 {
		fmt.Fprintf(os.Stderr, "  ⚠ No supported package manager (%d):\n", len(noPkgMgr))
		for _, name := range noPkgMgr {
			fmt.Fprintf(os.Stderr, "    · %s\n", name)
		}
		fmt.Fprintln(os.Stderr)
	}
	if len(toInstall) == 0 {
		fmt.Fprintln(os.Stderr, "  Nothing to install — all tools are present!")
		fmt.Fprintln(os.Stderr, "\n────────────────────────")
		return nil
	}

	fmt.Fprintf(os.Stderr, "  To install (%d):\n", len(toInstall))
	for _, p := range toInstall {
		fmt.Fprintf(os.Stderr, "    · %-20s  via %s\n", p.display, p.source)
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

		c := exec.Command(p.cmdArgs[0], p.cmdArgs[1:]...)
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
