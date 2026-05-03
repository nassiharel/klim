package cli

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/manifest"
	"github.com/nassiharel/clim/internal/progress"
	"github.com/nassiharel/clim/internal/service"
	"github.com/nassiharel/clim/internal/snapshot"
)

var exportRefreshFlag bool

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export installed tools — to stdout, snapshots, or profiles",
	Long: `Export detected tools to YAML. Without a subcommand, prints to stdout.

  clim export                       # print manifest to stdout
  clim export > my-tools.yaml       # redirect to file
  clim export --refresh             # force fresh scan

Snapshots (saved exports):
  clim export save [label]          # save timestamped snapshot
  clim export list                  # list saved snapshots
  clim export show <name>           # show a snapshot
  clim export delete <name>         # delete a snapshot

Profiles (named snapshots):
  clim export profile save <name>   # save as named profile
  clim export profile list          # list profiles
  clim export profile show <name>   # show a profile
  clim export profile delete <name> # delete a profile

Import on another machine:
  clim import my-tools.yaml`,
	RunE: runExport,
}

// Snapshot subcommands under export.
var exportSaveCmd = &cobra.Command{
	Use:   "save [label]",
	Short: "Save current tool state as a timestamped snapshot",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runExportSave,
}

var exportListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved snapshots",
	RunE:  runExportList,
}

var exportShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show tools in a saved snapshot",
	Args:  requireArgs(1, "clim export show <name>"),
	RunE:  runExportShow,
}

var exportDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a saved snapshot",
	Args:  requireArgs(1, "clim export delete <name>"),
	RunE:  runExportDelete,
}

// Profile subcommands under export.
var exportProfileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage named profiles (work, personal, etc.)",
}

var exportProfileSaveCmd = &cobra.Command{
	Use:   "save <name>",
	Short: "Save current tool state as a named profile",
	Args:  requireArgs(1, "clim export profile save <name>"),
	RunE:  runExportProfileSave,
}

var exportProfileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved profiles",
	RunE:  runExportProfileList,
}

var exportProfileShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show a profile's tools",
	Args:  requireArgs(1, "clim export profile show <name>"),
	RunE:  runExportProfileShow,
}

var exportProfileDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a profile",
	Args:  requireArgs(1, "clim export profile delete <name>"),
	RunE:  runExportProfileDelete,
}

func init() {
	exportCmd.Flags().BoolVar(&exportRefreshFlag, "refresh", false, "Force a fresh scan, ignoring the on-disk cache")

	exportCmd.AddCommand(exportSaveCmd)
	exportCmd.AddCommand(exportListCmd)
	exportCmd.AddCommand(exportShowCmd)
	exportCmd.AddCommand(exportDeleteCmd)

	exportProfileCmd.AddCommand(exportProfileSaveCmd)
	exportProfileCmd.AddCommand(exportProfileListCmd)
	exportProfileCmd.AddCommand(exportProfileShowCmd)
	exportProfileCmd.AddCommand(exportProfileDeleteCmd)
	exportCmd.AddCommand(exportProfileCmd)

	// Registered in root.go with command group.
}

func runExport(cmd *cobra.Command, args []string) error {
	sp := progress.New("Scanning installed tools...")
	tools, _, scanInfo, err := svcFrom(cmd).LoadAndResolveCached(cmd.Context(), exportRefreshFlag)
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

// --- Snapshot subcommands ---

func runExportSave(cmd *cobra.Command, args []string) error {
	label := ""
	if len(args) > 0 {
		label = args[0]
	}

	sp := progress.New("Scanning tools...")
	tools, _, err := svcFrom(cmd).ScanOnly(cmd.Context())
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	sp.Done("Done")

	path, err := snapshot.Save(tools, label)
	if err != nil {
		return fmt.Errorf("saving snapshot: %w", err)
	}

	var installed int
	for _, t := range tools {
		if t.IsInstalled() {
			installed++
		}
	}
	fmt.Fprintf(os.Stderr, "✓ Snapshot saved (%d tools): %s\n", installed, path)
	return nil
}

func runExportList(cmd *cobra.Command, args []string) error {
	entries, err := snapshot.List()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "No snapshots saved. Create one with: clim export save")
		return nil
	}
	fmt.Fprintf(os.Stderr, "%d snapshot(s):\n\n", len(entries))
	for _, e := range entries {
		age := time.Since(e.CreatedAt).Truncate(time.Minute)
		fmt.Fprintf(os.Stderr, "  %-40s  %d tools  %s ago\n", e.Name, e.ToolCount, age)
	}
	return nil
}

func runExportShow(cmd *cobra.Command, args []string) error {
	snap, err := snapshot.Load(args[0])
	if err != nil {
		return fmt.Errorf("loading snapshot %q: %w", args[0], err)
	}
	if snap.Name != "" {
		fmt.Fprintf(os.Stderr, "Snapshot: %s\n", snap.Name)
	}
	fmt.Fprintf(os.Stderr, "Created: %s\n", snap.CreatedAt)
	fmt.Fprintf(os.Stderr, "OS/Arch: %s/%s\n", snap.OS, snap.Arch)
	fmt.Fprintf(os.Stderr, "Tools:   %d\n\n", len(snap.Tools))
	for _, t := range snap.Tools {
		ver := t.Version
		if ver == "" {
			ver = "(unknown)"
		}
		fmt.Fprintf(os.Stderr, "  %-20s %s (%s)\n", t.Name, ver, t.Source)
	}
	return nil
}

func runExportDelete(cmd *cobra.Command, args []string) error {
	if err := snapshot.Delete(args[0]); err != nil {
		return fmt.Errorf("deleting snapshot %q: %w", args[0], err)
	}
	fmt.Fprintf(os.Stderr, "✓ Snapshot deleted: %s\n", args[0])
	return nil
}

// --- Profile subcommands ---

func runExportProfileSave(cmd *cobra.Command, args []string) error {
	name := args[0]
	sp := progress.New("Scanning tools...")
	tools, _, err := svcFrom(cmd).ScanOnly(cmd.Context())
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	sp.Done("Done")

	path, err := snapshot.SaveProfile(tools, name)
	if err != nil {
		return fmt.Errorf("saving profile %q: %w", name, err)
	}

	var installed int
	for _, t := range tools {
		if t.IsInstalled() {
			installed++
		}
	}
	fmt.Fprintf(os.Stderr, "✓ Profile %q saved (%d tools): %s\n", name, installed, path)
	return nil
}

func runExportProfileList(cmd *cobra.Command, args []string) error {
	entries, err := snapshot.ListProfiles()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "No profiles saved. Create one with: clim export profile save <name>")
		return nil
	}
	fmt.Fprintf(os.Stderr, "%d profile(s):\n\n", len(entries))
	for _, e := range entries {
		fmt.Fprintf(os.Stderr, "  %-20s  %d tools\n", e.Name, e.ToolCount)
	}
	return nil
}

func runExportProfileShow(cmd *cobra.Command, args []string) error {
	snap, err := snapshot.LoadProfile(args[0])
	if err != nil {
		return fmt.Errorf("loading profile %q: %w", args[0], err)
	}
	fmt.Fprintf(os.Stderr, "Profile: %s\n", snap.Name)
	fmt.Fprintf(os.Stderr, "OS/Arch: %s/%s\n", snap.OS, snap.Arch)
	fmt.Fprintf(os.Stderr, "Tools:   %d\n\n", len(snap.Tools))
	for _, t := range snap.Tools {
		ver := t.Version
		if ver == "" {
			ver = "(unknown)"
		}
		fmt.Fprintf(os.Stderr, "  %-20s %s (%s)\n", t.Name, ver, t.Source)
	}
	return nil
}

func runExportProfileDelete(cmd *cobra.Command, args []string) error {
	if err := snapshot.DeleteProfile(args[0]); err != nil {
		return fmt.Errorf("deleting profile %q: %w", args[0], err)
	}
	fmt.Fprintf(os.Stderr, "✓ Profile %q deleted\n", args[0])
	return nil
}
