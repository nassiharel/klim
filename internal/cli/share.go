package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/finder"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/share"
)

var shareCmd = &cobra.Command{
	Use:   "share",
	Short: "Generate a share token of your installed tools",
	Long: `Generate a compact token that encodes your installed tools. The token can
be pasted in Slack, Teams, or any chat to share your toolchain with others.

Recipients install the tools with:
  clim open <token>

Example:
  clim share
  # → Share token (21 tools):
  # → clim:v1:H4sIAAAA...
  # → Recipients can install with: clim open <token>`,
	RunE: runShare,
}

func init() {
	rootCmd.AddCommand(shareCmd)
}

func runShare(cmd *cobra.Command, args []string) error {
	tools := registry.DefaultTools()

	fmt.Fprintln(os.Stderr, "Scanning installed tools...")
	if err := finder.FindAll(tools); err != nil {
		return fmt.Errorf("scanning PATH: %w", err)
	}

	sort.Slice(tools, func(i, j int) bool {
		return strings.ToLower(tools[i].Name) < strings.ToLower(tools[j].Name)
	})

	// Collect names of installed, enabled tools.
	var names []string
	for _, tool := range tools {
		if tool.IsInstalled() && !tool.Disabled {
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
	fmt.Fprintf(os.Stderr, "\nRecipients can install with: clim open <token>\n")
	return nil
}
