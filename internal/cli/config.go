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

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print current configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, warnings, err := config.LoadWithWarnings()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		if path, pathErr := config.Path(); pathErr == nil {
			fmt.Fprintf(os.Stderr, "Config: %s\n\n", path)
		} else {
			fmt.Fprintf(os.Stderr, "Config: (unknown: %v)\n\n", pathErr)
		}

		fmt.Printf("logging:\n")
		fmt.Printf("  level: %s\n", c.Logging.Level)
		fmt.Printf("  file: %v\n", c.Logging.File)
		fmt.Printf("  verbose: %v\n", c.Logging.Verbose)
		fmt.Printf("\nmarketplace:\n")
		url := c.Marketplace.URL
		if url == "" {
			url = config.DefaultMarketplaceURL
		}
		fmt.Printf("  url: %s\n", url)
		fmt.Printf("  auto_refresh: %v\n", c.Marketplace.AutoRefresh)
		fmt.Printf("  refresh_interval: %s\n", c.Marketplace.RefreshInterval.Duration)
		if len(c.Marketplace.ExtraURLs) > 0 {
			fmt.Printf("  extra_urls:\n")
			for _, u := range c.Marketplace.ExtraURLs {
				fmt.Printf("    - %s\n", u)
			}
		}
		fmt.Printf("\nperformance:\n")
		fmt.Printf("  concurrency: %d\n", c.Performance.Concurrency)
		fmt.Printf("  command_timeout: %s\n", c.Performance.CommandTimeout.Duration)
		fmt.Printf("\nui:\n")
		fmt.Printf("  default_tab: %s\n", c.UI.DefaultTab)
		fmt.Printf("  show_path: %v\n", c.UI.ShowPath)
		fmt.Printf("  sidebar_right: %v\n", c.UI.SidebarRight)
		if c.Compliance.Policy != "" {
			fmt.Printf("\ncompliance:\n")
			fmt.Printf("  policy: %s\n", c.Compliance.Policy)
		}

		if len(warnings) > 0 {
			fmt.Fprintf(os.Stderr, "\nWarnings:\n")
			for _, w := range warnings {
				fmt.Fprintf(os.Stderr, "  ⚠ %s\n", w)
			}
		}
		return nil
	},
}

func init() {
	configCmd.AddCommand(configPathCmd)
	configCmd.AddCommand(configEditCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(marketplaceCmd)
	// rootCmd.AddCommand done in root.go via group assignment.
}
