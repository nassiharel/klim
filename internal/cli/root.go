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

// ANSI color codes for help output.
const (
	cReset = "\033[0m"
	cBold  = "\033[1m"
	cDim   = "\033[2m"
	cTeal  = "\033[38;5;37m"
	cGreen = "\033[38;5;78m"
	cGold  = "\033[38;5;179m"
	cWhite = "\033[38;5;15m"
	cGray  = "\033[38;5;244m"
)

func init() {
	rootCmd.PersistentFlags().BoolVar(&verboseFlag, "verbose", false, "enable verbose logging to stderr")
	rootCmd.CompletionOptions.DisableDefaultCmd = true

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

// initColorHelp sets up colorized help output for the root command.
// Subcommands use Cobra's default help template.
func initColorHelp() {
	defaultHelp := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if cmd != rootCmd {
			defaultHelp(cmd, args)
			return
		}
		w := cmd.OutOrStdout()
		p := func(format string, a ...any) { //nolint:errcheck
			_, _ = fmt.Fprintf(w, format, a...)
		}

		// Brand header.
		brand := cBold + cWhite + "\033[48;5;37m" + " clim " + cReset
		p("\n  %s  %s\n\n", brand, cmd.Short)

		// Description.
		if cmd.Long != "" {
			p("%s%s%s\n\n", cGray, cmd.Long, cReset)
		}

		// Command groups.
		for _, g := range cmd.Groups() {
			p("%s%s%s%s\n", cBold, cTeal, g.Title, cReset)
			for _, c := range cmd.Commands() {
				if c.GroupID == g.ID && c.IsAvailableCommand() {
					p("  %s%-16s%s%s%s%s\n",
						cGreen, c.Name(), cReset,
						cGray, c.Short, cReset)
				}
			}
			p("\n")
		}

		// Ungrouped commands.
		var ungrouped []*cobra.Command
		for _, c := range cmd.Commands() {
			if c.IsAvailableCommand() && c.GroupID == "" {
				ungrouped = append(ungrouped, c)
			}
		}
		if len(ungrouped) > 0 {
			p("%s%sAdditional Commands:%s\n", cBold, cTeal, cReset)
			for _, c := range ungrouped {
				p("  %s%-16s%s%s%s%s\n",
					cGreen, c.Name(), cReset,
					cGray, c.Short, cReset)
			}
			p("\n")
		}

		// Flags.
		if cmd.HasAvailableLocalFlags() {
			p("%s%sFlags:%s\n", cBold, cTeal, cReset)
			p("%s\n", cmd.LocalFlags().FlagUsages())
		}

		// Usage hint.
		p("Use \"%sclim [command] --help%s\" for more information.\n\n",
			cGreen, cReset)
	})
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
