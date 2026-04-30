package cli

import (
	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Shell integration — completions, hooks, and proxies",
	Long: `Configure shell integration for clim.

Subcommands:
  completion   Generate tab completion scripts
  hook         Generate auto-check hooks for .clim.yaml`,
}

func init() {
	shellCmd.AddCommand(completionCmd)
	shellCmd.AddCommand(hookCmd)
}
