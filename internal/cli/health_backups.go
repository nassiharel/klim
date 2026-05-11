package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/pathbackup"
)

var healthPathBackupsOutput func() (OutputFormat, error)

// healthPathBackupsCmd surfaces the PATH backups that the TUI's
// Health → Issues fix wizard captures automatically before any
// PATH-modifying command runs. CLI users get the same visibility
// the TUI does — list every backup, inspect one, copy the restore
// command, or delete an old file.
var healthPathBackupsCmd = &cobra.Command{
	Use:     "path-backups",
	Aliases: []string{"backups"},
	Short:   "List / show / restore-cmd PATH backups captured by the Health fix wizard",
	Long: `Every time the TUI Health → Issues fix wizard runs a command that
modifies $PATH (duplicate removal, missing-dir cleanup, reorder, …)
it writes a backup of the current PATH (and, on Windows, the
persistent User PATH from the registry) to:

  ~/.klim/backups/path/path-<UTC>.yaml

This command makes those backups inspectable from the CLI. Common
flows:

  klim health path-backups list                    # browse them
  klim health path-backups show <name>             # see the captured PATH
  klim health path-backups restore-cmd <name>      # print the restore command
                                                   # (review, then run in your shell)

The backup files are plain YAML and can also be opened with any
text editor.`,
	Args: cobra.MaximumNArgs(0),
	RunE: runHealthPathBackupsList,
}

var healthPathBackupsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List saved PATH backups, newest first",
	RunE:  runHealthPathBackupsList,
}

var healthPathBackupsShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show the contents of one PATH backup",
	Args:  cobra.ExactArgs(1),
	RunE:  runHealthPathBackupsShow,
}

var healthPathBackupsRestoreCmdCmd = &cobra.Command{
	Use:   "restore-cmd <name>",
	Short: "Print the platform-specific command that restores this backup",
	Long: `Emit (to stdout) the exact shell command that would restore the
captured PATH. We deliberately don't execute it — review the
command first, then paste it into your shell, or pipe it through
'powershell -NoProfile -Command' / 'sh -c'.`,
	Args: cobra.ExactArgs(1),
	RunE: runHealthPathBackupsRestoreCmd,
}

func init() {
	healthPathBackupsOutput = addOutputFlag(healthPathBackupsCmd, OutputText, OutputJSON)
	healthPathBackupsCmd.AddCommand(healthPathBackupsListCmd)
	healthPathBackupsCmd.AddCommand(healthPathBackupsShowCmd)
	healthPathBackupsCmd.AddCommand(healthPathBackupsRestoreCmdCmd)
	doctorCmd.AddCommand(healthPathBackupsCmd)
}

func runHealthPathBackupsList(cmd *cobra.Command, _ []string) error {
	out, err := healthPathBackupsOutput()
	if err != nil {
		return err
	}
	backups, err := pathbackup.List()
	if err != nil {
		return err
	}
	if out == OutputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(backups)
	}
	if len(backups) == 0 {
		fmt.Fprintln(os.Stderr, "No PATH backups yet.")
		fmt.Fprintln(os.Stderr, "Backups are created automatically by the TUI Health → Issues fix wizard")
		fmt.Fprintln(os.Stderr, "before any command modifies your $PATH.")
		return nil
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "FILE\tCAPTURED\tTRIGGER\tISSUE")
	for _, b := range backups {
		file := truncateBackupFile(b.File)
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			file,
			b.Timestamp.Local().Format("2006-01-02 15:04:05"),
			defaultString(b.Trigger, "—"),
			defaultString(b.Issue, "—"),
		)
	}
	return tw.Flush()
}

func runHealthPathBackupsShow(cmd *cobra.Command, args []string) error {
	out, err := healthPathBackupsOutput()
	if err != nil {
		return err
	}
	b, err := findPathBackup(args[0])
	if err != nil {
		return err
	}
	if out == OutputJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(b)
	}
	_, _ = fmt.Fprintf(os.Stdout, "Backup:   %s\n", b.File)
	_, _ = fmt.Fprintf(os.Stdout, "Captured: %s\n", b.Timestamp.Local().Format("2006-01-02 15:04:05"))
	_, _ = fmt.Fprintf(os.Stdout, "Trigger:  %s\n", defaultString(b.Trigger, "—"))
	_, _ = fmt.Fprintf(os.Stdout, "Issue:    %s\n", defaultString(b.Issue, "—"))
	_, _ = fmt.Fprintf(os.Stdout, "Platform: %s\n", b.GOOS)
	if b.Command != "" {
		_, _ = fmt.Fprintln(os.Stdout, "\nCommand that was about to run:")
		_, _ = fmt.Fprintln(os.Stdout, "  "+b.Command)
	}
	_, _ = fmt.Fprintln(os.Stdout, "\nCaptured $PATH:")
	_, _ = fmt.Fprintln(os.Stdout, "  "+b.PATH)
	if b.UserPATH != "" {
		_, _ = fmt.Fprintln(os.Stdout, "\nCaptured Windows User PATH (registry):")
		_, _ = fmt.Fprintln(os.Stdout, "  "+b.UserPATH)
	}
	_, _ = fmt.Fprintln(os.Stdout, "\nTo print the restore command:")
	_, _ = fmt.Fprintln(os.Stdout, "  klim health path-backups restore-cmd "+args[0])
	return nil
}

func runHealthPathBackupsRestoreCmd(cmd *cobra.Command, args []string) error {
	b, err := findPathBackup(args[0])
	if err != nil {
		return err
	}
	// Restore command goes to stdout so users can pipe it; the
	// reminder lines go to stderr.
	fmt.Fprintln(os.Stderr, "Restore command for "+b.File+":")
	fmt.Fprintln(os.Stderr, "(review carefully, then paste into your shell)")
	fmt.Fprintln(os.Stderr)
	_, _ = fmt.Fprintln(os.Stdout, pathbackup.RestoreCommand(b))
	return nil
}

// findPathBackup looks up a backup either by exact filename (with or
// without .yaml suffix) or by an unambiguous prefix of a filename.
// This keeps the CLI ergonomic — users don't have to copy a long
// timestamped name verbatim.
func findPathBackup(query string) (pathbackup.Backup, error) {
	backups, err := pathbackup.List()
	if err != nil {
		return pathbackup.Backup{}, err
	}
	var matches []pathbackup.Backup
	for _, b := range backups {
		name := fileBase(b.File)
		if name == query || name == query+".yaml" || startsWith(name, query) {
			matches = append(matches, b)
		}
	}
	switch len(matches) {
	case 0:
		return pathbackup.Backup{}, fmt.Errorf("no PATH backup matching %q (run `klim health path-backups list` to see what's available)", query)
	case 1:
		return matches[0], nil
	default:
		return pathbackup.Backup{}, fmt.Errorf("query %q matches %d backups — be more specific", query, len(matches))
	}
}

func fileBase(path string) string {
	// Lightweight filepath.Base — avoids importing filepath here
	// for one call (we already pull it in via pathbackup).
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[i+1:]
		}
	}
	return path
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func truncateBackupFile(path string) string {
	const maxLen = 60
	if len(path) <= maxLen {
		return path
	}
	return "…" + path[len(path)-maxLen+1:]
}

func defaultString(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
