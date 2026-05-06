package cli

import (
	"os"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion <bash|zsh|fish|powershell>",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for klim.

To load completions:

  bash:
    source <(klim shell completion bash)
    # To load on every session, add to your ~/.bashrc:
    echo 'source <(klim shell completion bash)' >> ~/.bashrc

  zsh:
    source <(klim shell completion zsh)
    # To load on every session, add to your ~/.zshrc:
    echo 'source <(klim shell completion zsh)' >> ~/.zshrc

  fish:
    klim shell completion fish | source
    # To load on every session:
    klim shell completion fish > ~/.config/fish/completions/klim.fish

  powershell:
    klim shell completion powershell | Out-String | Invoke-Expression
    # To load on every session, add to your $PROFILE:
    Add-Content $PROFILE 'klim shell completion powershell | Out-String | Invoke-Expression'`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(requireArgs(1, "klim shell completion <bash|zsh|fish|powershell>"), cobra.OnlyValidArgs),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
		}
		return nil
	},
}

func init() {
	// Registered under shellCmd in shell.go.
}
