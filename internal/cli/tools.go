package cli

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/registry"
)

var toolsCmd = &cobra.Command{
	Use:   "tools",
	Short: "Manage the tool definitions",
}

var toolsPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the path to marketplace.yaml",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := registry.ToolsPath()
		if err != nil {
			return err
		}
		fmt.Println(path)
		return nil
	},
}

var toolsEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open marketplace.yaml in your editor",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := registry.ToolsPath()
		if err != nil {
			return err
		}

		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = os.Getenv("VISUAL")
		}
		if editor == "" {
			return fmt.Errorf("no $EDITOR set; edit %s manually", path)
		}

		c := exec.Command(editor, path)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}

func init() {
	toolsCmd.AddCommand(toolsPathCmd)
	toolsCmd.AddCommand(toolsEditCmd)
}
