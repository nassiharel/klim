package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/updater"
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade <tool>",
	Short: "Upgrade a tool using the native package manager",
	Args:  cobra.ExactArgs(1),
	RunE:  runUpgrade,
}

func init() {
	rootCmd.AddCommand(upgradeCmd)
}

func runUpgrade(cmd *cobra.Command, args []string) error {
	toolName := args[0]

	// Find the tool in the registry.
	tools := registry.DefaultTools()
	var tool *registry.Tool
	for i := range tools {
		if tools[i].Name == toolName {
			tool = &tools[i]
			break
		}
	}
	if tool == nil {
		for i := range tools {
			if contains(tools[i].DisplayName, toolName) || contains(tools[i].Name, toolName) {
				tool = &tools[i]
				break
			}
		}
	}
	if tool == nil {
		fmt.Fprintf(os.Stderr, "Unknown tool: %s\n\nAvailable tools:\n", toolName)
		for _, t := range tools {
			fmt.Fprintf(os.Stderr, "  %-12s  %s\n", t.Name, t.DisplayName)
		}
		return fmt.Errorf("tool %q not found in registry", toolName)
	}

	// Show what we'll do.
	upgradeArgs, err := updater.UpgradeCmd(*tool)
	if err != nil {
		return err
	}
	fmt.Printf("Upgrading %s via: %s\n\n", tool.DisplayName, strings.Join(upgradeArgs, " "))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	return updater.Upgrade(ctx, *tool)
}
