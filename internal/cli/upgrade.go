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
  · installed and already at the latest → skipped silently
  · not installed                       → skipped (use 'clim install' for those)

Source precedence is the same as 'clim install':
  1. --source flag
  2. defaults.preferred_source in config.yaml
  3. OS-priority fallback

Examples:
  clim upgrade jq
  clim upgrade --pack go-dev
  clim upgrade jq fzf --source brew --yes
  clim upgrade --pack rust-dev --dry-run`,
	GroupID: "data",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAction(cmd, args, ActionUpgrade, upgradeFlags, upgradeOutputFmt)
	},
}

func init() {
	upgradeFlags = addActionFlags(upgradeCmd)
	upgradeOutputFmt = addOutputFlag(upgradeCmd, OutputText, OutputJSON)
}
