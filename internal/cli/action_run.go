package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/registry"
)

// actionFlags holds the shared flag state for clim install/upgrade/remove.
type actionFlags struct {
	source  string
	packs   []string
	dryRun  bool
	yes     bool
	refresh bool
}

// actionResult is the JSON-output shape for action commands.
type actionResult struct {
	Action    string               `json:"action"`
	DryRun    bool                 `json:"dry_run,omitempty"`
	Planned   []actionPlannedEntry `json:"planned"`
	Succeeded []string             `json:"succeeded"`
	Failed    []actionFailedEntry  `json:"failed,omitempty"`
	Skipped   []actionSkippedEntry `json:"skipped,omitempty"`
	Errors    []string             `json:"errors,omitempty"`
}

type actionPlannedEntry struct {
	Name    string   `json:"name"`
	Display string   `json:"display"`
	Source  string   `json:"source"`
	Cmd     []string `json:"cmd"`
}

type actionFailedEntry struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

type actionSkippedEntry struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// addActionFlags registers the shared install/upgrade/remove flag set on
// cmd and returns the populated state. The returned getter for --output
// is bound by the caller via addOutputFlag.
func addActionFlags(cmd *cobra.Command) *actionFlags {
	f := &actionFlags{}
	cmd.Flags().StringVar(&f.source, "source", "",
		"package manager (winget|choco|scoop|brew|apt|snap|npm); overrides config defaults.preferred_source")
	cmd.Flags().StringSliceVar(&f.packs, "pack", nil,
		"pack name to expand into a tool list (repeatable)")
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "print the plan without executing")
	cmd.Flags().BoolVarP(&f.yes, "yes", "y", false, "skip confirmation prompt")
	cmd.Flags().BoolVar(&f.refresh, "refresh", false, "ignore the scan cache and rescan PATH")
	return f
}

// runAction is the shared body for clim install/upgrade/remove. It
// resolves the catalog + scan, expands targets, builds the plan, and —
// unless --dry-run — executes plans with confirmation. Output is text
// on stderr by default; --output=json emits a machine-readable result
// on stdout and skips the confirmation prompt.
func runAction(cmd *cobra.Command, args []string, action Action, flags *actionFlags, getOutput func() (OutputFormat, error)) error {
	if err := validateSource(flags.source); err != nil {
		return err
	}
	if len(args) == 0 && len(flags.packs) == 0 {
		return usageErrorf(
			"requires at least one tool name or --pack <name>\n\nExamples:\n  clim %s jq fzf\n  clim %s --pack go-dev",
			action, action,
		)
	}

	output, err := getOutput()
	if err != nil {
		return err
	}

	cfg := cfgFrom(cmd)
	svc := svcFrom(cmd)

	tools, _, _, err := svc.LoadAndResolveCached(cmd.Context(), flags.refresh)
	if err != nil {
		return fmt.Errorf("scanning installed tools: %w", err)
	}
	regMap := registry.ToolMap(tools)

	var packs []registry.Pack
	if len(flags.packs) > 0 {
		packs, err = svc.LoadPacks(cmd.Context())
		if err != nil {
			return fmt.Errorf("loading packs: %w", err)
		}
	}

	targets, unknownPacks := expandTargets(args, flags.packs, packs)
	if len(unknownPacks) > 0 {
		return usageErrorf("unknown pack(s): %s", strings.Join(unknownPacks, ", "))
	}

	sourceHint := resolveSource(flags.source, cfg)
	plan := buildActionPlan(action, targets, regMap, sourceHint)

	if output == OutputJSON {
		return runActionJSON(cmd, action, plan, flags)
	}
	return runActionText(cmd, action, plan, flags)
}

// runActionText renders the human-readable flow on stderr.
func runActionText(cmd *cobra.Command, action Action, plan actionSummary, flags *actionFlags) error {
	printActionSummary(plan)

	if len(plan.toExec) == 0 {
		fmt.Fprintln(os.Stderr, "  Nothing to do.")
		return nil
	}
	if flags.dryRun {
		fmt.Fprintln(os.Stderr, "  --dry-run: no commands executed.")
		return nil
	}
	if !confirmInstall(flags.yes) {
		fmt.Fprintln(os.Stderr, "  Cancelled.")
		return nil
	}

	results := executeActionPlans(cmd.Context(), plan, true)
	succeeded, failed := countResults(results)

	svc := svcFrom(cmd)
	if err := svc.InvalidateScanCache(); err != nil {
		fmt.Fprintf(os.Stderr, "  ⚠ Failed to invalidate scan cache: %v\n", err)
	}

	fmt.Fprintf(os.Stderr, "\n──── Done: %d %s, %d failed ────\n",
		succeeded, pastTense(action), failed)

	if failed > 0 {
		return &PartialFailureError{Op: string(action), Succeeded: succeeded, Failed: failed}
	}
	return nil
}

// runActionJSON emits a structured result on stdout and (unless dry-run)
// executes plans without an interactive prompt. A human-readable plan
// summary is also written to stderr so the user can see what is about
// to happen even when stdout is piped to a file or jq.
func runActionJSON(cmd *cobra.Command, action Action, plan actionSummary, flags *actionFlags) error {
	// Plan summary on stderr — keeps stdout pristine for the final
	// JSON object that scripts will parse.
	printActionSummary(plan)

	res := actionResult{
		Action:    string(action),
		DryRun:    flags.dryRun,
		Planned:   make([]actionPlannedEntry, 0, len(plan.toExec)),
		Succeeded: []string{},
	}
	for _, p := range plan.toExec {
		res.Planned = append(res.Planned, actionPlannedEntry{
			Name:    p.name,
			Display: p.display,
			Source:  p.source,
			Cmd:     append([]string(nil), p.cmdArgs...),
		})
	}
	addSkipNames := func(reason string, names []string) {
		for _, n := range names {
			res.Skipped = append(res.Skipped, actionSkippedEntry{Name: n, Reason: reason})
		}
	}
	addSkipEntries := func(reason string, items []bucketEntry) {
		for _, e := range items {
			res.Skipped = append(res.Skipped, actionSkippedEntry{Name: e.Name, Reason: reason})
		}
	}
	addSkipEntries("already_installed", plan.alreadyInstalled)
	addSkipEntries("not_installed", plan.notInstalled)
	addSkipEntries("up_to_date", plan.upToDate)
	addSkipNames("self_protected", plan.selfProtected)
	addSkipNames("unknown_tool", plan.unknown)
	addSkipNames("no_package_for_os", plan.noPackage)
	addSkipNames("no_package_manager", plan.noPkgMgr)

	if flags.dryRun || len(plan.toExec) == 0 {
		return printJSON(res)
	}

	// JSON mode: subprocess stdout MUST NOT touch our stdout — the
	// final printJSON is the sole stdout writer for scripts.
	results := executeActionPlans(cmd.Context(), plan, false)
	for _, r := range results {
		if r.Err == nil {
			res.Succeeded = append(res.Succeeded, r.Plan.name)
		} else {
			res.Failed = append(res.Failed, actionFailedEntry{
				Name:  r.Plan.name,
				Error: r.Err.Error(),
			})
		}
	}

	svc := svcFrom(cmd)
	if err := svc.InvalidateScanCache(); err != nil {
		res.Errors = append(res.Errors, fmt.Sprintf("invalidate scan cache: %v", err))
	}

	if err := printJSON(res); err != nil {
		return err
	}
	if failed := len(res.Failed); failed > 0 {
		return &PartialFailureError{Op: string(action), Succeeded: len(res.Succeeded), Failed: failed}
	}
	return nil
}
