package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/checkpoint"
	"github.com/nassiharel/klim/internal/postcheck"
	"github.com/nassiharel/klim/internal/registry"
)

var (
	applyFlags                *actionFlags
	applyOutputFmt            func() (OutputFormat, error)
	applyNoCheckpoint         bool
	applyNoPostcheck          bool
	applyPostcheckConcurrency int
	applyPostcheckBudget      time.Duration
)

// applyCmd is the post-plan execution verb with a safety wrapper.
//
// Lifecycle of `klim apply`:
//
//  1. Scan + capture an automatic checkpoint named pre-apply-<UTC>.
//  2. Run the upgrade pass — identical to klim upgrade.
//  3. Re-scan and feed BOTH the pre-apply state AND the post-apply
//     state into postcheck so true regressions can be distinguished
//     from pre-existing issues.
//  4. On regression: surface the exact `klim rollback <name>`
//     command and exit 3.
//
// We deliberately do NOT auto-execute a rollback. Downgrading is
// PM-specific, can race against running processes, and would obscure
// exactly which change broke things — the user gets one explicit
// command, with the same modal confirmation klim upgrade already
// wraps each invocation in.
var applyCmd = &cobra.Command{
	Use:   "apply [tool...]",
	Short: "Apply pending changes with checkpoint + postcheck safety net",
	Long: `Execute the changes klim plan proposes. Wrapped in a safety net so
you can trust the result:

  1. A checkpoint named pre-apply-<UTC> is captured BEFORE anything
     runs. Roll back any time with: klim rollback pre-apply-<UTC>.
  2. The upgrade pass runs — same logic as klim upgrade.
  3. Postcheck validates the resulting state:
       shell resolution     every installed tool resolves via PATH
       binary validation    each binary stats + responds to --version
       PATH consistency     no missing/duplicate PATH entries
       manager integrity    every PM (brew/winget/scoop/…) is healthy
     Regressions vs the pre-apply state are flagged Fail; pre-existing
     issues are flagged Warn so postcheck doesn't trip on them.
  4. On regression: klim apply prints the exact command to roll
     back the auto-checkpoint and exits 3.

Examples:
  klim apply                 Upgrade every tool with the full safety net.
  klim apply jq fzf          Apply specific tools only.
  klim apply --no-postcheck  Skip validation (faster, no auto-rollback hint).
  klim apply --no-checkpoint Skip the pre-apply checkpoint entirely.

Use 'klim upgrade' for the bare upgrade with none of this wrapper.

Exit codes:
  0  Apply succeeded and every postcheck passed.
  3  Apply succeeded but postcheck detected one or more regressions.
     A 'klim rollback <name>' command is printed; rerun the apply
     after rolling back, or use --no-postcheck to accept the warnings.`,
	GroupID: "tools",
	RunE:    runApplyWithSafety,
}

func init() {
	applyFlags = addActionFlags(applyCmd)
	applyOutputFmt = addOutputFlag(applyCmd, OutputText, OutputJSON, OutputYAML)
	applyCmd.Flags().BoolVar(&applyNoCheckpoint, "no-checkpoint", false, "Skip the pre-apply checkpoint")
	applyCmd.Flags().BoolVar(&applyNoPostcheck, "no-postcheck", false, "Skip the post-apply validation pass")
	applyCmd.Flags().IntVar(&applyPostcheckConcurrency, "postcheck-concurrency", 0, "Max parallel binary probes during postcheck (0 = NumCPU)")
	applyCmd.Flags().DurationVar(&applyPostcheckBudget, "postcheck-budget", 60*time.Second, "Wall-clock ceiling for the postcheck pass")
	rootCmd.AddCommand(applyCmd)
}

func runApplyWithSafety(cmd *cobra.Command, args []string) error {
	// 1. Pre-apply scan + checkpoint. Single scan, used both
	//    on disk and for postcheck's regression detection.
	preTools, scanErr := scanForApply(cmd, applyFlags.refresh)
	if scanErr != nil {
		fmt.Fprintln(os.Stderr, "⚠ Could not scan before apply:", scanErr)
	}

	var cpName string
	if !applyNoCheckpoint && preTools != nil {
		name := "pre-apply-" + time.Now().UTC().Format("20060102-150405")
		cp := checkpoint.Capture(name, "Automatic pre-apply snapshot", preTools)
		if path, err := checkpoint.Save(cp); err != nil {
			fmt.Fprintln(os.Stderr, "⚠ Could not capture pre-apply checkpoint:", err)
		} else {
			cpName = name
			fmt.Fprintln(os.Stderr, "💾 Pre-apply checkpoint saved:", cpName)
			fmt.Fprintln(os.Stderr, "   File:    "+path)
			fmt.Fprintln(os.Stderr, "   Restore: klim rollback "+cpName)
			fmt.Fprintln(os.Stderr)
		}
	}

	// 2. The upgrade pass itself.
	if err := runAction(cmd, args, ActionUpgrade, applyFlags, applyOutputFmt); err != nil {
		if cpName != "" {
			fmt.Fprintln(os.Stderr, "\nApply failed before completion. Roll back with:")
			fmt.Fprintln(os.Stderr, "  klim rollback "+cpName)
		}
		return err
	}

	if applyNoPostcheck {
		return nil
	}

	// 3. Postcheck against the freshly-rescanned state.
	postTools, _, _, err := svcFrom(cmd).LoadAndResolveCached(cmd.Context(), true)
	if err != nil {
		fmt.Fprintln(os.Stderr, "⚠ Postcheck skipped — could not rescan:", err)
		return nil
	}
	result := postcheck.Run(preTools, postTools, postcheck.Options{
		Concurrency:     applyPostcheckConcurrency,
		WallClockBudget: applyPostcheckBudget,
	})
	renderPostcheck(result)

	if !result.Failed {
		return nil
	}
	// 4. Failure path: surface the rollback affordance and exit 3.
	fmt.Fprintln(os.Stderr)
	if len(result.Regressions) > 0 {
		fmt.Fprintln(os.Stderr, "✗ Postcheck regressions:")
		for _, name := range result.Regressions {
			fmt.Fprintln(os.Stderr, "    "+name)
		}
	}
	if cpName != "" {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Restore the pre-apply state with:")
		fmt.Fprintln(os.Stderr, "  klim rollback "+cpName)
	}
	return &PartialFailureError{
		Op:        "apply postcheck",
		Succeeded: countCheckStatus(result, postcheck.StatusPass),
		Failed:    countFailedChecks(result),
	}
}

func scanForApply(cmd *cobra.Command, refresh bool) ([]registry.Tool, error) {
	// refresh comes from the shared --refresh action flag. When the
	// user asks for a fresh scan we must honour it for the pre-apply
	// snapshot too — otherwise the postcheck regression comparison
	// and the auto-checkpoint would be anchored to stale cached
	// state while the upgrade phase ran fresh.
	tools, _, _, err := svcFrom(cmd).LoadAndResolveCached(cmd.Context(), refresh)
	if err != nil {
		return nil, err
	}
	return tools, nil
}

func renderPostcheck(r postcheck.Result) {
	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "Postcheck (%s):\n", r.Took.Round(time.Millisecond))
	for _, c := range r.Checks {
		icon := "✓"
		switch c.Status {
		case postcheck.StatusWarn:
			icon = "⚠"
		case postcheck.StatusFail:
			icon = "✗"
		case postcheck.StatusSkip:
			icon = "○"
		}
		fmt.Fprintf(os.Stderr, "  %s %-20s %s  (%s)\n", icon, c.Name, c.Detail, c.Took.Round(time.Millisecond))
		if len(c.Items) > 0 && (c.Status == postcheck.StatusFail || c.Status == postcheck.StatusWarn) {
			limit := len(c.Items)
			if limit > 5 {
				limit = 5
			}
			for _, it := range c.Items[:limit] {
				fmt.Fprintln(os.Stderr, "        - "+it)
			}
			if len(c.Items) > limit {
				fmt.Fprintf(os.Stderr, "        (and %d more)\n", len(c.Items)-limit)
			}
		}
	}
}

func countFailedChecks(r postcheck.Result) int {
	return countCheckStatus(r, postcheck.StatusFail)
}

func countCheckStatus(r postcheck.Result, want postcheck.Status) int {
	n := 0
	for _, c := range r.Checks {
		if c.Status == want {
			n++
		}
	}
	return n
}
