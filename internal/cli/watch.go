package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/progress"
)

var watchOutputFmt func() OutputFormat

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Check for available tool updates",
	Long: `Check all installed tools for available updates and report results.

Designed to run periodically (cron, Task Scheduler) or on-demand.
Use --output=json for machine-readable output suitable for notifications.

Examples:
  clim watch                         # human-readable check
  clim watch --output json           # JSON output for scripts

  # Cron job (daily at 9am):
  0 9 * * * clim watch --output json >> ~/.config/clim/watch.log

  # Windows Task Scheduler:
  schtasks /create /tn "clim-watch" /tr "clim watch" /sc daily /st 09:00`,
	RunE: runWatch,
}

func init() {
	watchOutputFmt = addOutputFlag(watchCmd, OutputText, OutputJSON)
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
	sp := progress.New("Checking for updates...")
	tools, _, _, err := svc.LoadAndResolveCached(cmd.Context(), true) // always fresh
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

	if watchOutputFmt() == OutputJSON {
		return printJSON(watchReport{
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
	fmt.Fprintf(os.Stderr, "\nRun 'clim' to upgrade interactively, or upgrade individually with your package manager.\n")
	return nil
}
