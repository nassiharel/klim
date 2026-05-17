package cli

import (
	"github.com/spf13/cobra"
)

var (
	upgradeFlags     *actionFlags
	upgradeOutputFmt func() (OutputFormat, error)
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade [tool...]",
	Short: "Upgrade installed tools to the latest version",
	Long: `Upgrade tools listed positionally and/or via --pack expansions.

For each target:
  · installed with an update available → upgrade
  · installed and already at the latest → skipped (listed under "Up to date")
  · not installed                       → skipped (use 'klim install' for those)

Source precedence is the same as 'klim install':
  1. --source flag
  2. defaults.preferred_source in config.yaml
  3. OS-priority fallback

Examples:
  klim upgrade jq
  klim upgrade --pack go-developer
  klim upgrade jq fzf --source brew --yes
  klim upgrade --pack rust-dev --dry-run`,
	GroupID: "tools",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAction(cmd, args, ActionUpgrade, upgradeFlags, upgradeOutputFmt)
	},
}

func init() {
	upgradeFlags = addActionFlags(upgradeCmd)
	upgradeOutputFmt = addOutputFlag(upgradeCmd, OutputText, OutputJSON, OutputYAML)
}
