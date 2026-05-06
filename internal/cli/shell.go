package cli

import (
	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Shell integration — completions and hooks",
	Long: `Configure shell integration for klim.

Subcommands:
  completion   Generate tab completion scripts
  hook         Generate auto-check hooks for .klim.yaml`,
}

func init() {
	shellCmd.AddCommand(completionCmd)
	shellCmd.AddCommand(hookCmd)
}
