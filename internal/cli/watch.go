package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var watchOutputFmt func() (OutputFormat, error)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Check for available tool updates",
	Long: `Check all installed tools for available updates and report results.

Designed to run periodically (cron, Task Scheduler) or on-demand.
Use --output=json for machine-readable output suitable for notifications.

Examples:
  klim tool watch                         # human-readable check
  klim tool watch --output json           # JSON output for scripts

  # Cron job (daily at 9am):
  0 9 * * * klim tool watch --output json >> ~/.klim/watch.log

  # Windows Task Scheduler:
  schtasks /create /tn "klim-watch" /tr "klim tool watch" /sc daily /st 09:00`,
	RunE: runWatch,
}

func init() {
	watchOutputFmt = addOutputFlag(watchCmd, OutputText, OutputJSON, OutputYAML)
	// Registered in root.go with command group.
}

type watchResult struct {
	Tool      string `json:"tool"`
	Installed string `json:"installed"`
	Latest    string `json:"latest"`
	Source    string `json:"source"`
}

type watchReport struct {
	Updates    []watchResult `json:"updates"`
	TotalTools int           `json:"total_tools"`
	UpToDate   int           `json:"up_to_date"`
}

func runWatch(cmd *cobra.Command, args []string) error {
	out, err := watchOutputFmt()
	if err != nil {
		return err
	}

	sp := spinnerFor(out, "Checking for updates...")
	tools, _, _, err := svcFrom(cmd).LoadAndResolveCached(cmd.Context(), true) // always fresh
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	sp.Done("Scan complete")

	var updates []watchResult
	var installed int
	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		installed++
		if t.HasUpdate() {
			primary := t.PrimaryInstance()
			updates = append(updates, watchResult{
				Tool:      t.Name,
				Installed: primary.Version,
				Latest:    t.Latest,
				Source:    string(primary.Source),
			})
		}
	}

	if out == OutputJSON || out == OutputYAML {
		return printStructured(out, watchReport{
			Updates:    updates,
			TotalTools: installed,
			UpToDate:   installed - len(updates),
		})
	}

	// Human output.
	if len(updates) == 0 {
		fmt.Fprintf(os.Stderr, "\n✓ All %d tools are up to date!\n", installed)
		return nil
	}

	fmt.Fprintf(os.Stderr, "\n%d update(s) available:\n\n", len(updates))
	for _, u := range updates {
		fmt.Fprintf(os.Stderr, "  ⬆ %-20s %s → %s  (%s)\n", u.Tool, u.Installed, u.Latest, u.Source)
	}
	fmt.Fprintf(os.Stderr, "\nRun 'klim' to upgrade interactively, or upgrade individually with your package manager.\n")
	return nil
}
