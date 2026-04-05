package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/nassiharel/clim/internal/build"
	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/tui"
)

var configPath string

var rootCmd = &cobra.Command{
	Use:   "clim",
	Short: "Interactive CLI manager — discover all executables on your PATH",
	Long: `clim discovers all executable binaries on your system PATH,
detects their versions, and presents them in an interactive TUI.

Run without arguments to launch the interactive TUI, or use subcommands
for non-interactive operation.`,
	Version: build.Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		// If stdout is a terminal, launch the interactive TUI.
		if term.IsTerminal(int(os.Stdout.Fd())) {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			return tui.Run(cfg)
		}
		// Otherwise fall back to non-interactive list (for piping).
		return runList(cmd, args)
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(configCmd)

	// Global flags.
	rootCmd.PersistentFlags().Bool("no-color", false, "disable color output")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "path to config file (default: ~/.config/clim/config.json)")
}

// Execute runs the root command.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	return nil
}

// loadConfig loads the config from the specified path or the default location.
func loadConfig() (config.Config, error) {
	path := configPath
	if path == "" {
		var err error
		path, err = config.DefaultPath()
		if err != nil {
			return config.Config{}, fmt.Errorf("determining config path: %w", err)
		}
	}
	return config.Load(path)
}
