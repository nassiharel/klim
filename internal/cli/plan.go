package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/plan"
	"github.com/nassiharel/klim/internal/teamfile"
)

var (
	planRefreshFlag      bool
	planFileFlag         string
	planDetailedExitFlag bool
	planOutput           func() (OutputFormat, error)
)

// planCmd computes a Terraform-plan-style preview of the changes klim
// would make. With no arguments it plans upgrades for every installed
// tool that has a newer version available; with positional args it
// scopes the plan to those tools; with --file it diffs against a
// .klim.yaml manifest (install missing, upgrade where required).
var planCmd = &cobra.Command{
	Use:   "plan [tool...]",
	Short: "Preview what klim would change (upgrades, installs, removes)",
	Long: `Terraform-plan-style preview of toolchain changes klim would make.

Sections:
  Planned changes     grouped by package manager
  Upgrade confidence  0-100% per upgrade, with the factors that produced it
  Risk analysis       heuristic warnings (breaking changes, plugin
                      compatibility, native modules, etc.)
  Disk impact         estimated cache footprint and reclaimable space
  Estimated time      pessimistic wall-clock estimate based on PM speed

Use --file path/to/.klim.yaml to plan a diff against a project's
target state (installs missing tools, upgrades where required).

Exit codes:
  0  Default behaviour — succeeds whether or not there are changes
  3  When --detailed-exitcode is set and at least one change is pending`,
	GroupID: "tools",
	RunE:    runPlan,
}

func init() {
	planOutput = addOutputFlag(planCmd, OutputText, OutputJSON, OutputYAML)
	planCmd.Flags().BoolVar(&planRefreshFlag, "refresh", false, "Force fresh scan (ignore cache)")
	planCmd.Flags().StringVarP(&planFileFlag, "file", "f", "", "Plan against a .klim.yaml target manifest")
	planCmd.Flags().BoolVar(&planDetailedExitFlag, "detailed-exitcode", false, "Exit 3 when changes are pending (mirrors 'terraform plan -detailed-exitcode')")
	rootCmd.AddCommand(planCmd)
}

func runPlan(cmd *cobra.Command, args []string) error {
	out, err := planOutput()
	if err != nil {
		return err
	}

	sp := spinnerFor(out, "Computing plan...")
	tools, _, _, err := svcFrom(cmd).LoadAndResolveCached(cmd.Context(), planRefreshFlag)
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	sp.Done("Plan ready")

	opts := plan.Options{IncludeUpgrades: true}
	if len(args) > 0 {
		opts.OnlyTools = make(map[string]bool, len(args))
		for _, a := range args {
			opts.OnlyTools[a] = true
		}
	}
	if planFileFlag != "" {
		opts.IncludeInstalls = true
		opts.IncludeUpgrades = true
		desired, derr := loadDesiredFromTeamFile(planFileFlag)
		if derr != nil {
			return fmt.Errorf("loading %s: %w", planFileFlag, derr)
		}
		opts.Desired = desired
	}

	p := plan.Build(tools, opts)

	if out == OutputJSON || out == OutputYAML {
		if err := printStructured(out, p); err != nil {
			return err
		}
		return planExitCode(p)
	}

	_, _ = fmt.Fprint(os.Stdout, plan.RenderText(p))
	return planExitCode(p)
}

// planExitCode returns the appropriate cobra.RunE error for the
// plan's outcome. Default behaviour is exit 0 regardless of pending
// changes — CI consumers that want to gate on the diff opt in via
// `--detailed-exitcode`, in which case a non-empty plan exits 3
// (PendingChangesError — keeps the "N changes pending" framing
// distinct from PartialFailureError's "X succeeded, Y failed").
func planExitCode(p plan.Plan) error {
	if !planDetailedExitFlag || len(p.Changes) == 0 {
		return nil
	}
	return &PendingChangesError{Op: "plan", Pending: len(p.Changes)}
}

// loadDesiredFromTeamFile turns a .klim.yaml's required + optional
// tool lists into the DesiredState map plan.Build expects. Pinned
// versions in the manifest become the target; unpinned ones imply
// "latest" (the desired-state map carries an empty Version).
func loadDesiredFromTeamFile(path string) (map[string]plan.DesiredState, error) {
	tf, err := teamfile.Parse(path)
	if err != nil {
		return nil, err
	}
	out := make(map[string]plan.DesiredState, len(tf.Tools)+len(tf.Optional))
	for _, t := range tf.Tools {
		out[t.Name] = plan.DesiredState{Version: t.Version}
	}
	for _, t := range tf.Optional {
		// Optional entries still appear in the plan when missing,
		// but only if the user explicitly asks. We add them so
		// --file produces the full set; callers that want only
		// "required" can filter on the manifest themselves.
		out[t.Name] = plan.DesiredState{Version: t.Version}
	}
	return out, nil
}
