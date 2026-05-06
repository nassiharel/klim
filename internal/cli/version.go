package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/build"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print klim version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(build.Info())
	},
}
