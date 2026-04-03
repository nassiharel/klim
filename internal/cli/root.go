package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/nassiharel/clim/internal/build"
	"github.com/nassiharel/clim/internal/tui"
)

var rootCmd = &cobra.Command{
	Use:   "clim",
	Short: "Interactive CLI manager — detect, check, and upgrade your dev tools",
	Long: `clim is an interactive CLI manager that detects installed developer tools,
shows their versions and install locations, checks for newer versions,
and helps you upgrade them.

Run without arguments to launch the interactive TUI, or use subcommands
for non-interactive operation.`,
	Version: build.Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		// If stdout is a terminal, launch the interactive TUI.
		if term.IsTerminal(int(os.Stdout.Fd())) {
			return tui.Run()
		}
		// Otherwise fall back to non-interactive list (for piping).
		return runList(cmd, args)
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(versionCmd)

	// Global flags
	rootCmd.PersistentFlags().Bool("no-color", false, "disable color output")
}

// Execute runs the root command.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	return nil
}
