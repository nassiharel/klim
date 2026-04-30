package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/share"
)

var shareCmd = &cobra.Command{
	Use:   "share",
	Short: "Share your toolchain — generate tokens, install from tokens",
	Long: `Share your installed tools as a compact token, or install tools from
a token shared by a teammate.

Subcommands:
  generate   Generate a share token of your installed tools
  open       Install tools from a share token

Legacy usage (still works):
  clim share             # same as clim share generate
  clim share open <token>`,
	RunE: runShare,
}

func init() {
	shareCmd.AddCommand(openCmd)
	// rootCmd.AddCommand done in root.go via group assignment.
}

func runShare(cmd *cobra.Command, args []string) error {
	// If called with no subcommand, behave as "generate".
	return runShareGenerate(cmd, args)
}

func runShareGenerate(cmd *cobra.Command, args []string) error {
	fmt.Fprintln(os.Stderr, "Scanning installed tools...")
	tools, _, err := svc.ScanOnly(cmd.Context())
	if err != nil {
		return err
	}

	// Collect names of installed tools.
	var names []string
	for _, tool := range tools {
		if tool.IsInstalled() {
			names = append(names, tool.Name)
		}
	}

	if len(names) == 0 {
		fmt.Fprintln(os.Stderr, "No installed tools found.")
		return nil
	}

	token, err := share.Encode(names)
	if err != nil {
		return fmt.Errorf("encoding share token: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\nShare token (%d tools):\n\n", len(names))
	fmt.Println(token)
	fmt.Fprintf(os.Stderr, "\nRecipients can install with: clim share open <token>\n")
	return nil
}
