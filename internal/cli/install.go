package cli

import (
	"github.com/spf13/cobra"
)

var (
	installFlags     *actionFlags
	installOutputFmt func() (OutputFormat, error)
)

var installCmd = &cobra.Command{
	Use:   "install [tool...]",
	Short: "Install one or more tools or packs via the system package manager",
	Long: `Install tools listed positionally and/or via --pack expansions.

klim picks the package manager for each tool using this precedence:

  1. --source flag        (per invocation)
  2. defaults.preferred_source in config.yaml (global default)
  3. OS-priority fallback (e.g. brew on macOS, winget on Windows)

Tools that are already installed are skipped (listed under "Already
installed" in the plan summary). Tools missing from the catalog or
without a package on the current OS are reported but do not stop the
run.

Examples:
  klim install jq fzf
  klim install --pack go-developer
  klim install jq --source brew --yes
  klim install --pack rust-dev --pack web-dev --dry-run
  klim install jq --output json`,
	GroupID: "tools",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runAction(cmd, args, ActionInstall, installFlags, installOutputFmt)
	},
}

func init() {
	installFlags = addActionFlags(installCmd)
	installOutputFmt = addOutputFlag(installCmd, OutputText, OutputJSON)
}
