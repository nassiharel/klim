package cli

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

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
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return fmt.Errorf("parsing manifest: %w", err)
	}

	// Validate manifest has usable content.
	if err := validateManifest(&m); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}

	if len(m.Tools) == 0 {
		fmt.Println("No tools in manifest.")
		return nil
	}

	fmt.Fprintf(os.Stderr, "Manifest: %d tools (from %s/%s)\n", len(m.Tools), m.OS, m.Arch)

	// Load registry and scan PATH to know what's already installed.
	regTools, _, err := svc.ScanOnly(cmd.Context())
	if err != nil {
		return err
	}

	regMap := registry.ToolMap(regTools)

	// Build the install plan.
	ps := buildImportPlan(m.Tools, regMap)

	printPlanSummary(fmt.Sprintf("Import Summary — %d tools from %s/%s", len(m.Tools), m.OS, m.Arch), ps)

	if len(ps.toInstall) == 0 {
		fmt.Fprintln(os.Stderr, "  Nothing to install — all tools are present!")
		return nil
	}

	if !confirmInstall(yesFlag) {
		fmt.Fprintln(os.Stderr, "  Cancelled.")
		return nil
	}

	succeeded, failed := executeInstalls(ps.toInstall)
	// Any install attempt may have changed what's on PATH, so invalidate
	// the scan cache. Subsequent `clim list` / `clim export` runs will
	// rescan and rewrite the cache instead of serving stale data.
	if err := svc.InvalidateScanCache(); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ Failed to invalidate scan cache: %v\n", err)
	}
	fmt.Fprintf(os.Stderr, "\n──── Done: %d installed, %d failed, %d already present ────\n",
		succeeded, failed, len(ps.alreadyInstalled))
	if failed > 0 {
		return fmt.Errorf("%d tool(s) failed to install", failed)
	}
	return nil
}

// buildImportPlan classifies manifest tools into install/skip/error categories.
func buildImportPlan(manifestTools []manifest.Tool, regMap map[string]*registry.Tool) planSummary {
	var ps planSummary

	for _, mt := range manifestTools {
		rt, exists := regMap[mt.Name]
		if !exists {
			// Tool not in registry — try to use manifest's package IDs directly.
			pkgs := registry.PackageIDs{
				Winget: mt.Packages.Winget,
				Choco:  mt.Packages.Choco,
				Scoop:  mt.Packages.Scoop,
				Brew:   mt.Packages.Brew,
				Apt:    mt.Packages.Apt,
				Snap:   mt.Packages.Snap,
				NPM:    mt.Packages.NPM,
			}
			src := pkgs.BestInstallSource()
			if mt.Source != "" {
				preferred := registry.InstallSource(mt.Source)
				if args := pkgs.InstallArgs(preferred); args != nil {
					src = preferred
				}
			}
			installArgs := pkgs.InstallArgs(src)
			if installArgs == nil {
				if pkgs.HasAnyPackageForOS() {
					ps.noPkgMgr = append(ps.noPkgMgr, mt.Name)
				} else {
					ps.noPackage = append(ps.noPackage, mt.Name)
				}
				continue
			}
			ps.toInstall = append(ps.toInstall, installPlan{
				name: mt.Name, display: mt.DisplayName,
				cmdArgs: installArgs, source: string(src),
			})
			continue
		}

		if rt.IsInstalled() {
			ps.alreadyInstalled = append(ps.alreadyInstalled, mt.DisplayName)
			continue
		}

		src := rt.Packages.BestInstallSource()
		if mt.Source != "" {
			preferred := registry.InstallSource(mt.Source)
			if args := rt.Packages.InstallArgs(preferred); args != nil {
				src = preferred
			}
		}
		installArgs := rt.Packages.InstallArgs(src)
		if installArgs == nil {
			if rt.Packages.HasAnyPackageForOS() {
				ps.noPkgMgr = append(ps.noPkgMgr, mt.Name)
			} else {
				ps.noPackage = append(ps.noPackage, mt.Name)
			}
			continue
		}

		ps.toInstall = append(ps.toInstall, installPlan{
			name: mt.Name, display: mt.DisplayName,
			cmdArgs: installArgs, source: string(src),
		})
	}

	return ps
}

// validateManifest checks that a parsed manifest has the minimum required
// structure — tools must have a non-empty name. This catches cases where
// a valid YAML file (e.g. a random config) is passed but isn't a clim manifest.
func validateManifest(m *manifest.Manifest) error {
	for i, t := range m.Tools {
		if strings.TrimSpace(t.Name) == "" {
			return fmt.Errorf("tool at index %d has no name", i)
		}
	}
	return nil
}
