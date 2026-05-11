package cli

import (
	"github.com/spf13/cobra"
)

var (
	applyFlags     *actionFlags
	applyOutputFmt func() (OutputFormat, error)
)

// applyCmd is the post-plan execution verb. Semantically:
//
//	klim plan       — preview the pending changes
//	klim apply      — execute them
//
// Today the body is identical to `klim upgrade` because every change
// `klim plan` emits today is an upgrade. The separate command exists
// so the verb stays available as future plan kinds (installs from a
// manifest, removes against a checkpoint diff, etc.) grow into it
// without users having to re-learn a new command name.
var applyCmd = &cobra.Command{
	Use:   "apply [tool...]",
	Short: "Execute the changes klim plan proposes",
	Long: `Run the changes klim plan would output. Today this is equivalent
to "klim upgrade" — every plan kind currently is an upgrade — but
the verb stays available as klim plan grows to cover installs and
removes too.

Examples:
  klim apply                Apply every pending upgrade.
  klim apply jq fzf         Apply specific tools only.
  klim apply --pack go-dev  Apply a pack's worth of upgrades.
  klim apply --yes          Skip the per-tool confirmation prompt.

For full flag documentation see "klim upgrade --help".`,
	GroupID: "tools",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAction(cmd, args, ActionUpgrade, applyFlags, applyOutputFmt)
	},
}

func init() {
	applyFlags = addActionFlags(applyCmd)
	applyOutputFmt = addOutputFlag(applyCmd, OutputText, OutputJSON)
	rootCmd.AddCommand(applyCmd)
}
