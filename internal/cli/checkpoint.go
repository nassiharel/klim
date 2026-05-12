package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/checkpoint"
)

var (
	checkpointDescriptionFlag string
	checkpointOutput          func() (OutputFormat, error)
)

// checkpointCmd is the parent: `klim checkpoint <name>` captures a
// snapshot, while `list / show / delete` manage them. Mirrors the
// terraform-style "named workspace state" model so users have an
// intuitive home for the verbs.
var checkpointCmd = &cobra.Command{
	Use:   "checkpoint",
	Short: "Save / list / show / delete named toolchain snapshots",
	Long: `Capture the currently installed tool versions (and PATH) under a
named checkpoint so a future ` + "`klim rollback <name>`" + ` can restore them.

Usage:
  klim checkpoint <name>             Capture a new checkpoint.
  klim checkpoint list               List every checkpoint.
  klim checkpoint show <name>        Print a checkpoint's tools/versions.
  klim checkpoint delete <name>      Remove a checkpoint.

Checkpoints are stored under ~/.klim/checkpoints/<name>.yaml as
human-readable YAML — you can review them with any text editor.`,
	GroupID: "tools",
	Args:    cobra.MaximumNArgs(1),
	RunE:    runCheckpointCapture,
}

var checkpointListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved checkpoints",
	RunE:  runCheckpointList,
}

var checkpointShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show a checkpoint's contents",
	Args:  cobra.ExactArgs(1),
	RunE:  runCheckpointShow,
}

var checkpointDeleteCmd = &cobra.Command{
	Use:     "delete <name>",
	Aliases: []string{"rm"},
	Short:   "Delete a saved checkpoint",
	Args:    cobra.ExactArgs(1),
	RunE:    runCheckpointDelete,
}

func init() {
	checkpointOutput = addPersistentOutputFlag(checkpointCmd, OutputText, OutputJSON)
	checkpointCmd.Flags().StringVarP(&checkpointDescriptionFlag, "description", "d", "", "Free-text description stored with the snapshot")

	checkpointCmd.AddCommand(checkpointListCmd)
	checkpointCmd.AddCommand(checkpointShowCmd)
	checkpointCmd.AddCommand(checkpointDeleteCmd)

	rootCmd.AddCommand(checkpointCmd)
}

func runCheckpointCapture(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		// Empty args means "run the subcommand-less default" which
		// is "list". We don't error: matches `klim` (no args) UX.
		return runCheckpointList(cmd, args)
	}
	name := args[0]
	out, err := checkpointOutput()
	if err != nil {
		return err
	}

	sp := spinnerFor(out, "Capturing checkpoint...")
	tools, _, _, err := svcFrom(cmd).LoadAndResolveCached(cmd.Context(), false)
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	sp.Done("Scan complete")

	cp := checkpoint.Capture(name, checkpointDescriptionFlag, tools)
	path, err := checkpoint.Save(cp)
	if err != nil {
		return err
	}

	if out == OutputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Name  string `json:"name"`
			Tools int    `json:"tools"`
			Path  string `json:"path"`
		}{Name: cp.Name, Tools: len(cp.Tools), Path: path})
	}

	fmt.Fprintf(os.Stderr, "\n✓ Captured checkpoint %q with %d tool(s)\n", cp.Name, len(cp.Tools))
	fmt.Fprintf(os.Stderr, "  Saved to %s\n", path)
	fmt.Fprintf(os.Stderr, "  Roll back any time with: klim rollback %s\n", cp.Name)
	return nil
}

func runCheckpointList(cmd *cobra.Command, _ []string) error {
	out, err := checkpointOutput()
	if err != nil {
		return err
	}
	list, err := checkpoint.List()
	if err != nil {
		return err
	}
	if out == OutputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(list)
	}
	if len(list) == 0 {
		fmt.Fprintln(os.Stderr, "No checkpoints saved. Capture one with:")
		fmt.Fprintln(os.Stderr, "  klim checkpoint <name>")
		return nil
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "NAME\tCREATED\tTOOLS\tDESCRIPTION")
	for _, c := range list {
		desc := c.Description
		if desc == "" {
			desc = "—"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n",
			c.Name,
			c.CreatedAt.Local().Format("2006-01-02 15:04"),
			len(c.Tools),
			desc,
		)
	}
	return tw.Flush()
}

func runCheckpointShow(cmd *cobra.Command, args []string) error {
	out, err := checkpointOutput()
	if err != nil {
		return err
	}
	c, err := checkpoint.Load(args[0])
	if err != nil {
		return err
	}
	if out == OutputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(c)
	}
	_, _ = fmt.Fprintf(os.Stdout, "Checkpoint: %s\n", c.Name)
	if c.Description != "" {
		_, _ = fmt.Fprintf(os.Stdout, "Description: %s\n", c.Description)
	}
	_, _ = fmt.Fprintf(os.Stdout, "Created:     %s\n", c.CreatedAt.Local().Format("2006-01-02 15:04:05"))
	_, _ = fmt.Fprintf(os.Stdout, "Platform:    %s\n", c.GOOS)
	_, _ = fmt.Fprintf(os.Stdout, "Tools:       %d\n\n", len(c.Tools))
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "TOOL\tVERSION\tSOURCE")
	for _, t := range c.Tools {
		v := t.Version
		if v == "" {
			v = "—"
		}
		s := string(t.Source)
		if s == "" {
			s = "—"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", t.Name, v, s)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if c.PATH != "" {
		_, _ = fmt.Fprintln(os.Stdout, "\nCaptured PATH (truncated to first 200 chars):")
		path := c.PATH
		if len(path) > 200 {
			path = path[:200] + "…"
		}
		_, _ = fmt.Fprintln(os.Stdout, "  "+path)
	}
	return nil
}

func runCheckpointDelete(cmd *cobra.Command, args []string) error {
	out, err := checkpointOutput()
	if err != nil {
		return err
	}
	name := strings.TrimSpace(args[0])
	if err := checkpoint.Delete(name); err != nil {
		return err
	}
	if out == OutputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(struct {
			Deleted string `json:"deleted"`
		}{Deleted: name})
	}
	fmt.Fprintf(os.Stderr, "✓ Deleted checkpoint %q\n", name)
	return nil
}
