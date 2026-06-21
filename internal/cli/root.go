package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/nassiharel/klim/internal/build"
	"github.com/nassiharel/klim/internal/logging"
	"github.com/nassiharel/klim/internal/tui"
)

var verboseFlag bool

var rootCmd = &cobra.Command{
	Use:   "klim",
	Short: "productivity booster",
	Long: `klim is a productivity booster for dev tools: a cross-platform
layer for discovering, standardising, securing, and automating the tools every project depends on.

Run without arguments to launch the interactive TUI, or use subcommands
for non-interactive operation.`,
	Version: build.Version,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		c := cfgFrom(cmd)
		logging.Init(c.Logging.Level, c.Logging.File, verboseFlag || c.Logging.Verbose)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		c := cliCtxFrom(cmd.Context())
		if term.IsTerminal(int(os.Stdout.Fd())) {
			// On the very first interactive launch, show a short welcome that
			// points at the install win instead of dropping a brand-new user
			// straight into the multi-tab TUI. Printed before the TUI takes the
			// alternate screen, then we exit so the hint stays on screen.
			if showFirstRunWelcome(cmd.OutOrStdout()) {
				return nil
			}
			return tui.RunWithConfig(c.Cfg, c.ConfigWarnings)
		}
		return runList(cmd, args)
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

// ANSI color codes for help output are defined in help.go.

func init() {
	rootCmd.PersistentFlags().BoolVar(&verboseFlag, "verbose", false, "enable verbose logging to stderr")
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	// Wrap Cobra flag-parse errors in UsageError so they exit with code 2
	// (ExitUsage), matching the convention documented in
	// CLI-CONVENTIONS.md. Children inherit this from the root.
	rootCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &UsageError{Err: err}
	})

	// Cobra auto-registers --version in Execute; trigger it early to add -v shorthand.
	rootCmd.InitDefaultVersionFlag()
	if f := rootCmd.Flags().Lookup("version"); f != nil {
		f.Shorthand = "v"
	}

	// Colorized help output only when stdout is a terminal (not piped/redirected).
	if term.IsTerminal(int(os.Stdout.Fd())) { //nolint:gosec // G115: uintptr→int is the standard Go pattern for term.IsTerminal
		initColorHelp()
	}

	// Command groups for organized help output. These are display-only
	// sections shown by the colorized root help (see help.go); the actual
	// command nesting is wired below.
	rootCmd.AddGroup(
		&cobra.Group{ID: "tools", Title: "Tools & Projects:"},
		&cobra.Group{ID: "state", Title: "State & Environment:"},
		&cobra.Group{ID: "health", Title: "Security & Diagnostics:"},
		&cobra.Group{ID: "system", Title: "System:"},
	)

	// ----- Tools & Projects -----

	// tool: discover, install, inspect, and manage developer tools.
	toolCmd.GroupID = "tools"
	toolCmd.AddCommand(
		searchCmd,
		installCmd,
		upgradeCmd,
		removeCmd,
		infoCmd,
		whyCmd,
		graphCmd,
		listCmd,
		tryCmd,
		onboardCmd,
		watchCmd,
		catalogCmd,
	)
	rootCmd.AddCommand(toolCmd)

	// project: the .klim.yaml contract lifecycle.
	projectCmd.GroupID = "tools"
	projectCmd.AddCommand(initCmd, checkCmd, generateCmd)
	rootCmd.AddCommand(projectCmd)

	// plan: the declarative preview → apply → rollback workflow.
	planCmd.GroupID = "tools"
	planCmd.AddCommand(planShowCmd, applyCmd, diffCmd, rollbackCmd, checkpointCmd)
	rootCmd.AddCommand(planCmd)

	// agent: the agent-tooling ecosystem (its own subtree, wired in agents.go).
	agentsCmd.GroupID = "tools"
	rootCmd.AddCommand(agentsCmd)

	// ----- State & Environment -----

	// env: environment fingerprints and history (trail wired in env.go/root below).
	envCmd.GroupID = "state"
	envCmd.AddCommand(trailCmd)
	rootCmd.AddCommand(envCmd)

	// share: move a toolchain between machines and people.
	shareCmd.GroupID = "state"
	shareCmd.AddCommand(exportCmd, importCmd, shareLinkCmd, badgeCmd)
	rootCmd.AddCommand(shareCmd)

	// ----- Security & Diagnostics -----

	// security: audit, vulnerabilities, compliance, score.
	securityCmd.GroupID = "health"
	securityCmd.AddCommand(scoreCmd)
	rootCmd.AddCommand(securityCmd)

	// doctor: environment health diagnostics and PATH repair.
	// Uses the doctorCmd value defined in doctor.go; its `path` and
	// `path-backups` subcommands self-attach in health_path.go /
	// health_backups.go.
	doctorCmd.GroupID = "health"
	rootCmd.AddCommand(doctorCmd)

	// ----- System -----

	// shell: shell integration (completion/hook wired in shell.go) + proxy shims.
	shellCmd.GroupID = "system"
	shellCmd.AddCommand(proxyCmd)
	rootCmd.AddCommand(shellCmd)

	// config: klim configuration.
	configCmd.GroupID = "system"
	rootCmd.AddCommand(configCmd)

	// Manage the klim binary itself. These stay top-level (no group
	// wrapper): they're distinct enough from the tool verbs — note
	// `update` (the klim binary) vs `tool upgrade` (installed tools).
	updateCmd.GroupID = "system"
	rootCmd.AddCommand(updateCmd)
	versionCmd.GroupID = "system"
	rootCmd.AddCommand(versionCmd)
	browserCmd.GroupID = "system"
	rootCmd.AddCommand(browserCmd)
}

// Run executes the root command and returns a process exit code.
//
// Exit codes:
//
//	0 — success
//	1 — runtime error
//	2 — usage error (bad flags, args, or output format)
//	3 — partial failure (e.g. some imports failed)
//
// Cobra's own flag-parse errors are wrapped in UsageError via
// SetFlagErrorFunc on rootCmd, and "unknown command" / "unknown flag"
// errors that escape that hook are detected by message prefix here. All
// other errors map to ExitRuntime.
func Run() int {
	// Build the per-invocation context (config, service) and bind it to
	// rootCmd before execution so every subcommand can retrieve it via
	// cliCtxFrom(cmd.Context()).
	rootCmd.SetContext(withCLICtx(context.Background(), newCLICtx()))

	err := rootCmd.Execute()
	if err == nil {
		return ExitOK
	}
	fmt.Fprintln(os.Stderr, "Error:", err)

	var ue *UsageError
	if errors.As(err, &ue) {
		return ExitUsage
	}
	var pf *PartialFailureError
	if errors.As(err, &pf) {
		return ExitPartialFailure
	}
	var pce *PendingChangesError
	if errors.As(err, &pce) {
		return ExitPartialFailure
	}
	if isCobraUsageError(err) {
		return ExitUsage
	}
	return ExitRuntime
}

// isCobraUsageError reports whether err is a Cobra-emitted usage error
// (unknown command / unknown subcommand / unknown flag) that didn't go
// through SetFlagErrorFunc. Detection is by message prefix because Cobra
// returns plain `errors.New` instances for these cases.
func isCobraUsageError(err error) bool {
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "unknown command"),
		strings.HasPrefix(msg, "unknown flag"),
		strings.HasPrefix(msg, "unknown shorthand flag"),
		strings.HasPrefix(msg, "invalid argument"):
		return true
	}
	return false
}

// Execute is retained for callers that want a plain error. Prefer Run, which
// distinguishes usage / runtime / partial-failure exits.
//
// Deprecated: use Run.
func Execute() error {
	if Run() != ExitOK {
		return errCLIFailed
	}
	return nil
}

var errCLIFailed = errors.New("klim: command failed")

// requireArgs returns a Cobra Args validator that requires exactly n arguments
// and prints a helpful error message with usage hint when they're missing.
func requireArgs(n int, example string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < n {
			return usageErrorf("requires %d argument(s)\n\nUsage:\n  %s\n\nRun '%s --help' for more information", n, example, cmd.CommandPath())
		}
		if len(args) > n {
			return usageErrorf("accepts at most %d argument(s), received %d\n\nUsage:\n  %s\n\nRun '%s --help' for more information", n, len(args), example, cmd.CommandPath())
		}
		return nil
	}
}

// requireMinArgs returns a Cobra Args validator that requires at least n arguments.
func requireMinArgs(n int, example string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < n {
			return usageErrorf("requires at least %d argument(s)\n\nUsage:\n  %s\n\nRun '%s --help' for more information", n, example, cmd.CommandPath())
		}
		return nil
	}
}
