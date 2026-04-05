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
	Short: "Interactive CLI manager — detect, check, and manage your dev tools",
	Long: `clim is a developer tool manager that detects installed CLI tools,
shows their versions and install sources, checks for updates,
and helps you keep everything current.

Run without arguments to launch the interactive TUI, or use subcommands
for non-interactive operation.`,
	Version: build.Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		if term.IsTerminal(int(os.Stdout.Fd())) {
			return tui.Run()
		}
		return runList(cmd, args)
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(toolsCmd)
}

// Execute runs the root command.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	return nil
}
