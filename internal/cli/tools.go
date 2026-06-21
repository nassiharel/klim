package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/catalog"
)

var catalogCmd = &cobra.Command{
	Use:   "catalog",
	Short: "Inspect and manage the tool catalog",
	Long: `Inspect and manage the cached tool catalog (marketplace.yaml).

The catalog is fetched from the marketplace branch on first use and cached
locally. Use these subcommands to inspect or operate on the cache.`,
}

var toolsPathOutputFmt func() (OutputFormat, error)

var toolsPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the path to the local catalog cache",
	Long: `Print the absolute path to the locally cached marketplace.yaml file.

Useful for piping into other tools or for inspection:

  klim tool catalog path
  cat "$(klim tool catalog path)"
  klim tool catalog path --output json    # {"cache_path": "..."}`,
	Args: requireArgs(0, "klim tool catalog path"),
	RunE: runToolsPath,
}

func init() {
	toolsPathOutputFmt = addOutputFlag(toolsPathCmd, OutputText, OutputJSON, OutputYAML)
	catalogCmd.AddCommand(toolsPathCmd)
}

type toolsPathReport struct {
	CachePath string `json:"cache_path"`
}

func runToolsPath(cmd *cobra.Command, args []string) error {
	out, err := toolsPathOutputFmt()
	if err != nil {
		return err
	}
	path, err := catalog.CachePath()
	if err != nil {
		return fmt.Errorf("resolving catalog cache path: %w", err)
	}
	if out == OutputJSON || out == OutputYAML {
		return printStructured(out, toolsPathReport{CachePath: path})
	}
	fmt.Println(path)
	return nil
}
