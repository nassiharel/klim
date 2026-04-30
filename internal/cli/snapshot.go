package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/progress"
	"github.com/nassiharel/clim/internal/snapshot"
)

var snapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Save and restore environment snapshots",
	Long: `Manage timestamped snapshots of your installed tools.

Subcommands:
  save      Save current tool state as a snapshot
  list      List saved snapshots
  show      Show tools from a snapshot (use with clim diff/import)
  delete    Delete a snapshot

Profiles (named snapshots):
  profile save <name>     Save current state as a named profile
  profile list            List profiles
  profile show <name>     Show a profile's tools
  profile delete <name>   Delete a profile`,
}

var snapshotSaveCmd = &cobra.Command{
	Use:   "save [label]",
	Short: "Save current tool state as a snapshot",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runSnapshotSave,
}

var snapshotListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved snapshots",
	RunE:  runSnapshotList,
}

var snapshotShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show tools in a snapshot",
	Args:  requireArgs(1, "clim snapshot show <name>"),
	RunE:  runSnapshotShow,
}

var snapshotDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a snapshot",
	Args:  requireArgs(1, "clim snapshot delete <name>"),
	RunE:  runSnapshotDelete,
}

// Profile subcommands.
var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage named profiles (work, personal, etc.)",
}

var profileSaveCmd = &cobra.Command{
	Use:   "save <name>",
	Short: "Save current tool state as a named profile",
	Args:  requireArgs(1, "clim snapshot profile save <name>"),
	RunE:  runProfileSave,
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved profiles",
	RunE:  runProfileList,
}

var profileShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show a profile's tools",
	Args:  requireArgs(1, "clim snapshot profile show <name>"),
	RunE:  runProfileShow,
}

var profileDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a profile",
	Args:  requireArgs(1, "clim snapshot profile delete <name>"),
	RunE:  runProfileDelete,
}

func init() {
	snapshotCmd.AddCommand(snapshotSaveCmd)
	snapshotCmd.AddCommand(snapshotListCmd)
	snapshotCmd.AddCommand(snapshotShowCmd)
	snapshotCmd.AddCommand(snapshotDeleteCmd)

	profileCmd.AddCommand(profileSaveCmd)
	profileCmd.AddCommand(profileListCmd)
	profileCmd.AddCommand(profileShowCmd)
	profileCmd.AddCommand(profileDeleteCmd)
	snapshotCmd.AddCommand(profileCmd)

	// Registered in root.go with command group.
}

func runSnapshotSave(cmd *cobra.Command, args []string) error {
	label := ""
	if len(args) > 0 {
		label = args[0]
	}

	sp := progress.New("Scanning tools...")
	tools, _, err := svc.ScanOnly(cmd.Context())
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	sp.Done("Done")

	path, err := snapshot.Save(tools, label)
	if err != nil {
		return err
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

func runSnapshotList(cmd *cobra.Command, args []string) error {
	entries, err := snapshot.List()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "No snapshots saved. Create one with: clim snapshot save")
		return nil
	}
	fmt.Fprintf(os.Stderr, "%d snapshot(s):\n\n", len(entries))
	for _, e := range entries {
		age := time.Since(e.CreatedAt).Truncate(time.Minute)
		fmt.Fprintf(os.Stderr, "  %-40s  %d tools  %s ago\n", e.Name, e.ToolCount, age)
	}
	return nil
}

func runSnapshotShow(cmd *cobra.Command, args []string) error {
	snap, err := snapshot.Load(args[0])
	if err != nil {
		return err
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

func runSnapshotDelete(cmd *cobra.Command, args []string) error {
	if err := snapshot.Delete(args[0]); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "✓ Snapshot deleted: %s\n", args[0])
	return nil
}

func runProfileSave(cmd *cobra.Command, args []string) error {
	name := args[0]
	sp := progress.New("Scanning tools...")
	tools, _, err := svc.ScanOnly(cmd.Context())
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	sp.Done("Done")

	path, err := snapshot.SaveProfile(tools, name)
	if err != nil {
		return err
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

func runProfileList(cmd *cobra.Command, args []string) error {
	entries, err := snapshot.ListProfiles()
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintln(os.Stderr, "No profiles saved. Create one with: clim snapshot profile save <name>")
		return nil
	}
	fmt.Fprintf(os.Stderr, "%d profile(s):\n\n", len(entries))
	for _, e := range entries {
		fmt.Fprintf(os.Stderr, "  %-20s  %d tools\n", e.Name, e.ToolCount)
	}
	return nil
}

func runProfileShow(cmd *cobra.Command, args []string) error {
	snap, err := snapshot.LoadProfile(args[0])
	if err != nil {
		return err
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

func runProfileDelete(cmd *cobra.Command, args []string) error {
	if err := snapshot.DeleteProfile(args[0]); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "✓ Profile %q deleted\n", args[0])
	return nil
}
