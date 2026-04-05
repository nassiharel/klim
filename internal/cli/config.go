package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage clim configuration (include/exclude lists, timeouts)",
	Long: `Manage the clim configuration file. The config controls which binaries
are shown or hidden and the version detection timeout.

Config file location: ~/.config/clim/config.json (or %APPDATA%\clim\config.json on Windows)`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the current configuration",
	RunE:  runConfigShow,
}

var configEditCmd = &cobra.Command{
	Use:   "edit",
	Short: "Open the config file in your $EDITOR",
	RunE:  runConfigEdit,
}

var configResetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Delete the config file and reset to defaults",
	RunE:  runConfigReset,
}

var configExcludeCmd = &cobra.Command{
	Use:   "exclude",
	Short: "Manage the exclude list",
}

var configExcludeAddCmd = &cobra.Command{
	Use:   "add [names...]",
	Short: "Add binary names to the exclude list",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runConfigExcludeAdd,
}

var configExcludeRemoveCmd = &cobra.Command{
	Use:   "remove [names...]",
	Short: "Remove binary names from the exclude list",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runConfigExcludeRemove,
}

var configIncludeCmd = &cobra.Command{
	Use:   "include",
	Short: "Manage the include list (allowlist mode)",
}

var configIncludeAddCmd = &cobra.Command{
	Use:   "add [names...]",
	Short: "Add binary names to the include list",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runConfigIncludeAdd,
}

var configIncludeRemoveCmd = &cobra.Command{
	Use:   "remove [names...]",
	Short: "Remove binary names from the include list",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runConfigIncludeRemove,
}

func init() {
	configExcludeCmd.AddCommand(configExcludeAddCmd)
	configExcludeCmd.AddCommand(configExcludeRemoveCmd)

	configIncludeCmd.AddCommand(configIncludeAddCmd)
	configIncludeCmd.AddCommand(configIncludeRemoveCmd)

	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configEditCmd)
	configCmd.AddCommand(configResetCmd)
	configCmd.AddCommand(configExcludeCmd)
	configCmd.AddCommand(configIncludeCmd)
}

func runConfigShow(_ *cobra.Command, _ []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	fmt.Println(string(data))
	return nil
}

func runConfigEdit(_ *cobra.Command, _ []string) error {
	path, err := resolveConfigPath()
	if err != nil {
		return err
	}

	// Ensure the file exists so the editor has something to open.
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	if err := config.Save(path, cfg); err != nil {
		return err
	}

	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		return fmt.Errorf("no $EDITOR or $VISUAL set; edit %s manually", path)
	}

	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runConfigReset(_ *cobra.Command, _ []string) error {
	path, err := resolveConfigPath()
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}

	fmt.Fprintln(os.Stderr, "Config reset to defaults.")
	return nil
}

func runConfigExcludeAdd(_ *cobra.Command, args []string) error {
	return modifyConfig(func(cfg *config.Config) {
		for _, name := range args {
			if !slices.Contains(cfg.Exclude, name) {
				cfg.Exclude = append(cfg.Exclude, name)
			}
		}
	})
}

func runConfigExcludeRemove(_ *cobra.Command, args []string) error {
	return modifyConfig(func(cfg *config.Config) {
		cfg.Exclude = removeAll(cfg.Exclude, args)
	})
}

func runConfigIncludeAdd(_ *cobra.Command, args []string) error {
	return modifyConfig(func(cfg *config.Config) {
		for _, name := range args {
			if !slices.Contains(cfg.Include, name) {
				cfg.Include = append(cfg.Include, name)
			}
		}
	})
}

func runConfigIncludeRemove(_ *cobra.Command, args []string) error {
	return modifyConfig(func(cfg *config.Config) {
		cfg.Include = removeAll(cfg.Include, args)
	})
}

// --- helpers ---

func resolveConfigPath() (string, error) {
	if configPath != "" {
		return configPath, nil
	}
	return config.DefaultPath()
}

func modifyConfig(fn func(cfg *config.Config)) error {
	path, err := resolveConfigPath()
	if err != nil {
		return err
	}

	cfg, err := config.Load(path)
	if err != nil {
		return err
	}

	fn(&cfg)

	if err := config.Save(path, cfg); err != nil {
		return err
	}

	// Print updated config for confirmation.
	data, _ := json.MarshalIndent(cfg, "", "  ")
	fmt.Println(string(data))
	return nil
}

func removeAll(slice []string, toRemove []string) []string {
	removeSet := make(map[string]bool, len(toRemove))
	for _, r := range toRemove {
		removeSet[r] = true
	}

	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if !removeSet[s] {
			result = append(result, s)
		}
	}
	return result
}
