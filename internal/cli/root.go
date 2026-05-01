package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/nassiharel/clim/internal/build"
	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/logging"
	"github.com/nassiharel/clim/internal/service"
	"github.com/nassiharel/clim/internal/tui"
)

// cfg is the global configuration loaded once on startup.
// configWarnings holds any warnings about unknown/invalid config fields.
var (
	cfg, configWarnings = loadConfig()
	svc                 = service.NewWithConfig(cfg)
)

func loadConfig() (*config.Config, []string) {
	c, w, err := config.LoadWithWarnings()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		return config.Default(), nil
	}
	return c, w
}

var verboseFlag bool

var rootCmd = &cobra.Command{
	Use:   "clim",
	Short: "Interactive CLI manager — detect, check, and manage your dev tools",
	Long: `clim is a developer tool manager that detects installed CLI tools,
shows their versions and install sources, checks for updates,
and helps you keep everything current.

Run without arguments to launch the interactive TUI, or use subcommands
for non-interactive operation.`,
	Version: build.Version,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		logging.Init(cfg.Logging.Level, cfg.Logging.File, verboseFlag || cfg.Logging.Verbose)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		if term.IsTerminal(int(os.Stdout.Fd())) {
			return tui.RunWithConfig(cfg, configWarnings)
		}
		return runList(cmd, args)
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verboseFlag, "verbose", "v", false, "enable verbose logging to stderr")
	rootCmd.CompletionOptions.DisableDefaultCmd = true

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

	// Tool discovery & management.
	searchCmd.GroupID = "tools"
	rootCmd.AddCommand(searchCmd)
	onboardCmd.GroupID = "tools"
	rootCmd.AddCommand(onboardCmd)
	whyCmd.GroupID = "tools"
	rootCmd.AddCommand(whyCmd)
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
	snapshotCmd.GroupID = "data"
	rootCmd.AddCommand(snapshotCmd)

	// Health & security.
	doctorCmd.GroupID = "health"
	rootCmd.AddCommand(doctorCmd)
	auditCmd.GroupID = "health"
	rootCmd.AddCommand(auditCmd)
	complianceCmd.GroupID = "health"
	rootCmd.AddCommand(complianceCmd)
	scoreCmd.GroupID = "health"
	rootCmd.AddCommand(scoreCmd)

	// Shell integration.
	shellCmd.GroupID = "shell"
	rootCmd.AddCommand(shellCmd)
	proxyCmd.GroupID = "shell"
	rootCmd.AddCommand(proxyCmd)

	// Configuration.
	configCmd.GroupID = "config"
	rootCmd.AddCommand(configCmd)
}

// Execute runs the root command.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	return nil
}

// requireArgs returns a Cobra Args validator that requires exactly n arguments
// and prints a helpful error message with usage hint when they're missing.
func requireArgs(n int, example string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < n {
			return fmt.Errorf("requires %d argument(s)\n\nUsage:\n  %s\n\nRun '%s --help' for more information", n, example, cmd.CommandPath())
		}
		if len(args) > n {
			return fmt.Errorf("accepts at most %d argument(s), received %d\n\nUsage:\n  %s\n\nRun '%s --help' for more information", n, len(args), example, cmd.CommandPath())
		}
		return nil
	}
}

// requireMinArgs returns a Cobra Args validator that requires at least n arguments.
func requireMinArgs(n int, example string) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < n {
			return fmt.Errorf("requires at least %d argument(s)\n\nUsage:\n  %s\n\nRun '%s --help' for more information", n, example, cmd.CommandPath())
		}
		return nil
	}
}
