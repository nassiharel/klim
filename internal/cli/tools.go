package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/catalog"
)

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "Manage the tool catalog",
}

var toolsPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the path to the local catalog cache",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := catalog.CachePath()
		if err != nil {
			return err
		}
		fmt.Println(path)
		return nil
	},
}

func init() {
	toolsCmd.AddCommand(toolsPathCmd)
}
