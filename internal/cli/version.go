package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/build"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print clim version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(build.Info())
	},
}
