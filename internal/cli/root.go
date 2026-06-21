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

	// Command groups for organized help output.
	rootCmd.AddGroup(
		&cobra.Group{ID: "core", Title: "Core Commands:"},
		&cobra.Group{ID: "project", Title: "Project Commands:"},
		&cobra.Group{ID: "tools", Title: "Tool Discovery & Management:"},
		&cobra.Group{ID: "data", Title: "Backup & Sharing:"},
		&cobra.Group{ID: "health", Title: "Health & Security:"},
		&cobra.Group{ID: "shell", Title: "Shell Integration:"},
		&cobra.Group{ID: "config", Title: "Configuration:"},
	)

	// Core commands.
	listCmd.GroupID = "core"
	rootCmd.AddCommand(listCmd)
	versionCmd.GroupID = "core"
	rootCmd.AddCommand(versionCmd)
	updateCmd.GroupID = "core"
	rootCmd.AddCommand(updateCmd)

	// Project commands.
	initCmd.GroupID = "project"
	rootCmd.AddCommand(initCmd)
	checkCmd.GroupID = "project"
	rootCmd.AddCommand(checkCmd)
	generateCmd.GroupID = "project"
	rootCmd.AddCommand(generateCmd)

	// Tool discovery & management. install/upgrade/remove already set
	// their own GroupID in the command definition — no override here,
	// to keep one source of truth per command.
	searchCmd.GroupID = "tools"
	rootCmd.AddCommand(searchCmd)
	onboardCmd.GroupID = "tools"
	rootCmd.AddCommand(onboardCmd)
	whyCmd.GroupID = "tools"
	rootCmd.AddCommand(whyCmd)
	infoCmd.GroupID = "tools"
	rootCmd.AddCommand(infoCmd)
	graphCmd.GroupID = "tools"
	rootCmd.AddCommand(graphCmd)
	badgeCmd.GroupID = "data"
	rootCmd.AddCommand(badgeCmd)
	rootCmd.AddCommand(installCmd)
	rootCmd.AddCommand(upgradeCmd)
	rootCmd.AddCommand(removeCmd)
	tryCmd.GroupID = "tools"
	rootCmd.AddCommand(tryCmd)
	watchCmd.GroupID = "tools"
	rootCmd.AddCommand(watchCmd)
	diffCmd.GroupID = "tools"
	rootCmd.AddCommand(diffCmd)
	toolsCmd.GroupID = "tools"
	rootCmd.AddCommand(toolsCmd)

	// Backup & sharing.
	exportCmd.GroupID = "data"
	rootCmd.AddCommand(exportCmd)
	importCmd.GroupID = "data"
	rootCmd.AddCommand(importCmd)
	shareCmd.GroupID = "data"
	rootCmd.AddCommand(shareCmd)
	envCmd.GroupID = "data"
	rootCmd.AddCommand(envCmd)
	trailCmd.GroupID = "data"
	rootCmd.AddCommand(trailCmd)

	// Health & security.
	securityCmd.GroupID = "health"
	rootCmd.AddCommand(securityCmd)
	scoreCmd.GroupID = "health"
	rootCmd.AddCommand(scoreCmd)
	// Environment health (PATH, multi-installs, missing PMs, cache).
	// Uses the doctorCmd value defined in doctor.go — kept under that
	// name internally because the existing function/variable names are
	// "doctor" everywhere. The user-facing Cobra Use string is "health".
	rootCmd.AddCommand(doctorCmd)

	// Shell integration.
	shellCmd.GroupID = "shell"
	rootCmd.AddCommand(shellCmd)
	proxyCmd.GroupID = "shell"
	rootCmd.AddCommand(proxyCmd)

	// Configuration.
	configCmd.GroupID = "config"
	rootCmd.AddCommand(configCmd)

	// Browser UI.
	browserCmd.GroupID = "core"
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
