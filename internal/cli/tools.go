package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

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

		// Ensure the file exists by loading the catalog (which creates it on first run).
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			fmt.Fprintln(os.Stderr, "Fetching marketplace catalog...")
			if _, loadErr := svc.Catalog.LoadTools(cmd.Context()); loadErr != nil {
				return fmt.Errorf("could not create marketplace file: %w", loadErr)
			}
		}

		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = os.Getenv("VISUAL")
		}
		if editor == "" {
			return fmt.Errorf("no $EDITOR set; edit %s manually", path)
		}

		// Support editors with args like "code --wait" or "vim -u NONE".
		parts := strings.Fields(editor)
		editorArgs := append(parts[1:], path)
		c := exec.Command(parts[0], editorArgs...)
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
