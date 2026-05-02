package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/catalog"
)

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "Manage the tool catalog",
	Long: `Inspect and manage the cached tool catalog (marketplace.yaml).

The catalog is fetched from the marketplace branch on first use and cached
locally. Use these subcommands to inspect or operate on the cache.`,
}

var toolsPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the path to the local catalog cache",
	Long: `Print the absolute path to the locally cached marketplace.yaml file.

Useful for piping into other tools or for inspection:

  clim tools path
  cat "$(clim tools path)"`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := catalog.CachePath()
		if err != nil {
			return fmt.Errorf("resolving catalog cache path: %w", err)
		}
		fmt.Println(path)
		return nil
	},
}

func init() {
	toolsCmd.AddCommand(toolsPathCmd)
}
