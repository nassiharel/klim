package cli

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/share"
)

var shareOutputFmt func() (OutputFormat, error)

var shareCmd = &cobra.Command{
	Use:   "share",
	Short: "Share your toolchain — generate tokens, install from tokens",
	Long: `Share your installed tools as a compact token, or install tools from
a token shared by a teammate.

Usage:
  clim share                         # generate a share token (text)
  clim share --output json           # generate a share token (JSON)
  clim share open <token>            # install from a share token`,
	Args: cobra.NoArgs,
	RunE: runShare,
}

func init() {
	shareOutputFmt = addOutputFlag(shareCmd, OutputText, OutputJSON)
	shareCmd.AddCommand(openCmd)
	// rootCmd.AddCommand done in root.go via group assignment.
}

func runShare(cmd *cobra.Command, args []string) error {
	// If called with no subcommand, behave as "generate".
	return runShareGenerate(cmd, args)
}

type shareReport struct {
	Token     string   `json:"token,omitempty"`
	ToolCount int      `json:"tool_count"`
	Tools     []string `json:"tools"`
}

func runShareGenerate(cmd *cobra.Command, args []string) error {
	out, err := shareOutputFmt()
	if err != nil {
		return err
	}

	if out == OutputText {
		fmt.Fprintln(os.Stderr, "Scanning installed tools...")
	}
	tools, _, err := svcFrom(cmd).ScanOnly(cmd.Context())
	if err != nil {
		return fmt.Errorf("scanning installed tools: %w", err)
	}

	// Collect names of installed tools in deterministic (sorted) order so
	// the same installed set always produces the same token, regardless of
	// catalog ordering. This keeps the share output stable for scripts
	// and caching.
	names := []string{}
	for _, tool := range tools {
		if tool.IsInstalled() {
			names = append(names, tool.Name)
		}
	}
	sort.Strings(names)

	if len(names) == 0 {
		if out == OutputJSON {
			// Token is intentionally omitted (omitempty) so callers can
			// distinguish "no tools" from a real share payload.
			return printJSON(shareReport{Tools: names})
		}
		fmt.Fprintln(os.Stderr, "No installed tools found.")
		return nil
	}

	token, err := share.Encode(names)
	if err != nil {
		return fmt.Errorf("encoding share token: %w", err)
	}

	if out == OutputJSON {
		return printJSON(shareReport{Token: token, ToolCount: len(names), Tools: names})
	}

	fmt.Fprintf(os.Stderr, "\nShare token (%d tools):\n\n", len(names))
	fmt.Println(token)
	fmt.Fprintf(os.Stderr, "\nRecipients can install with: clim share open <token>\n")
	return nil
}
