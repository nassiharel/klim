package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage clim configuration",
}

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the path to config.yaml",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := config.Path()
		if err != nil {
			return err
		}
		fmt.Println(path)
		return nil
	},
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open config.yaml in your editor",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := config.Path()
		if err != nil {
			return err
		}

		// Ensure the file exists with defaults before opening.
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			_, _ = config.Load() // creates the default file
		}

		editor := strings.TrimSpace(os.Getenv("EDITOR"))
		if editor == "" {
			editor = strings.TrimSpace(os.Getenv("VISUAL"))
		}
		if editor == "" {
			return fmt.Errorf("no $EDITOR set; edit %s manually", path)
		}

		// Support editors with args like "code --wait" or "vim -u NONE".
		parts := strings.Fields(editor)
		if len(parts) == 0 {
			return fmt.Errorf("$EDITOR is empty; edit %s manually", path)
		}
		editorArgs := append(parts[1:], path)
		c := exec.Command(parts[0], editorArgs...)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		return c.Run()
	},
}

func init() {
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configEditCmd)
	configCmd.AddCommand(marketplaceCmd)
	// rootCmd.AddCommand done in root.go via group assignment.
}
