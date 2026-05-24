package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/checkpoint"
	"github.com/nassiharel/klim/internal/plan"
)

var (
	rollbackRefreshFlag       bool
	rollbackIncludeRemoveFlag bool
	rollbackOutput            func() (OutputFormat, error)
)

// rollbackCmd renders a plan that, when applied, would bring the
// currently installed toolchain back to the named checkpoint. It
// never executes anything itself — the user pipes through `klim
// apply` (or the recommended commands in the plan output) to do the
// work, so a rollback is always a deliberate two-step.
var rollbackCmd = &cobra.Command{
	Use:   "rollback <checkpoint>",
	Short: "Produce a plan that rolls back to a saved checkpoint",
	Long: `Compare the current toolchain to a saved checkpoint and emit a
plan that would restore it. Read-only by default — review the diff
first, then run the commands the plan recommends (or ` + "`klim apply`" + `
when applicable).

Sections shown are the same as klim plan: planned changes, upgrade
confidence (with downgrade caveats), risk analysis, disk impact, and
an estimated time.

Use --remove-extras to also propose removing tools that were
installed after the checkpoint (i.e. tools not in the snapshot).`,
	GroupID: "tools",
	Args:    cobra.ExactArgs(1),
	RunE:    runRollback,
}

func init() {
	rollbackOutput = addOutputFlag(rollbackCmd, OutputText, OutputJSON, OutputYAML)
	rollbackCmd.Flags().BoolVar(&rollbackRefreshFlag, "refresh", false, "Force fresh scan (ignore cache)")
	rollbackCmd.Flags().BoolVar(&rollbackIncludeRemoveFlag, "remove-extras", false, "Also propose removing tools added after the checkpoint")
	rootCmd.AddCommand(rollbackCmd)
}

func runRollback(cmd *cobra.Command, args []string) error {
	out, err := rollbackOutput()
	if err != nil {
		return err
	}
	cpName := args[0]

	cp, err := checkpoint.Load(cpName)
	if err != nil {
		return err
	}

	sp := spinnerFor(out, "Building rollback plan...")
	tools, _, _, err := svcFrom(cmd).LoadAndResolveCached(cmd.Context(), rollbackRefreshFlag)
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	sp.Done("Plan ready")

	desired := make(map[string]plan.DesiredState, len(cp.Tools))
	checkpointed := make(map[string]bool, len(cp.Tools))
	for _, t := range cp.Tools {
		desired[t.Name] = plan.DesiredState{Version: t.Version}
		checkpointed[t.Name] = true
	}

	// "Remove extras" path: any tool currently installed but not in
	// the checkpoint becomes a planned remove. Opt-in so the
	// default rollback only restores known tools without touching
	// new arrivals.
	if rollbackIncludeRemoveFlag {
		for _, t := range tools {
			if !t.IsInstalled() {
				continue
			}
			if !checkpointed[t.Name] {
				desired[t.Name] = plan.DesiredState{Remove: true}
			}
		}
	}

	opts := plan.Options{
		IncludeInstalls: true,
		IncludeUpgrades: true,
		IncludeRemoves:  rollbackIncludeRemoveFlag,
		Desired:         desired,
	}
	p := plan.Build(tools, opts)

	if out == OutputJSON || out == OutputYAML {
		return printStructured(out, struct {
			Checkpoint string    `json:"checkpoint"`
			Plan       plan.Plan `json:"plan"`
		}{Checkpoint: cp.Name, Plan: p})
	}

	_, _ = fmt.Fprintf(os.Stdout, "Rollback plan to checkpoint %q (captured %s)\n\n",
		cp.Name, cp.CreatedAt.Local().Format("2006-01-02 15:04"))
	_, _ = fmt.Fprint(os.Stdout, plan.RenderText(p))
	if len(p.Changes) > 0 {
		fmt.Fprintln(os.Stderr, "\nReview the plan above, then apply the suggested commands manually")
		fmt.Fprintln(os.Stderr, "(or run `klim apply` for upgrade-only rollbacks).")
	}
	return nil
}
