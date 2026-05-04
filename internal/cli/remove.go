package cli

import (
	"github.com/spf13/cobra"
)

var (
	removeFlags     *actionFlags
	removeOutputFmt func() (OutputFormat, error)
)

var removeCmd = &cobra.Command{
	Use:   "remove [tool...]",
	Short: "Remove installed tools via the system package manager",
	Long: `Remove tools listed positionally and/or via --pack expansions.

For each target:
  · installed   → remove
  · not installed → skipped silently
  · "clim" itself → refused (use the OS uninstaller for clim)

Source precedence is the same as 'clim install'.

Examples:
  clim remove jq
  clim remove --pack go-dev --yes
  clim remove jq fzf --source brew --dry-run`,
	GroupID: "tools",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAction(cmd, args, ActionRemove, removeFlags, removeOutputFmt)
	},
}

func init() {
	removeFlags = addActionFlags(removeCmd)
	removeOutputFmt = addOutputFlag(removeCmd, OutputText, OutputJSON)
}
