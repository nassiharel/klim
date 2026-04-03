package updater

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/nassiharel/clim/internal/registry"
)

// Upgrade runs the platform-specific upgrade command for a tool.
// It streams stdout/stderr to the caller's terminal.
func Upgrade(ctx context.Context, tool registry.Tool) error {
	platform := runtime.GOOS
	installArgs, ok := tool.InstallCmds[platform]
	if !ok {
		return fmt.Errorf("no install command defined for %s on %s", tool.Name, platform)
	}

	// Transform install → upgrade where applicable.
	upgradeArgs := toUpgradeCmd(platform, installArgs)

	fmt.Fprintf(os.Stderr, "Running: %s\n\n", strings.Join(upgradeArgs, " "))

	cmd := exec.CommandContext(ctx, upgradeArgs[0], upgradeArgs[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("upgrade failed: %w", err)
	}
	return nil
}

// toUpgradeCmd transforms an install command to an upgrade command.
func toUpgradeCmd(platform string, args []string) []string {
	if len(args) == 0 {
		return args
	}

	result := make([]string, len(args))
	copy(result, args)

	switch args[0] {
	case "brew":
		// brew install X → brew upgrade X
		for i, a := range result {
			if a == "install" {
				result[i] = "upgrade"
				break
			}
		}
	case "winget":
		// winget install X → winget upgrade X
		for i, a := range result {
			if a == "install" {
				result[i] = "upgrade"
				break
			}
		}
	case "apt", "apt-get":
		// apt install X → apt upgrade X
		for i, a := range result {
			if a == "install" {
				result[i] = "upgrade"
				break
			}
		}
	case "snap":
		// snap install X → snap refresh X
		for i, a := range result {
			if a == "install" {
				result[i] = "refresh"
				break
			}
		}
	case "choco":
		// choco install X → choco upgrade X
		for i, a := range result {
			if a == "install" {
				result[i] = "upgrade"
				break
			}
		}
	case "npm":
		// npm install -g X → npm update -g X
		for i, a := range result {
			if a == "install" {
				result[i] = "update"
				break
			}
		}
	}

	return result
}

// InstallCmd returns the platform-specific install command for a tool.
func InstallCmd(tool registry.Tool) ([]string, error) {
	platform := runtime.GOOS
	args, ok := tool.InstallCmds[platform]
	if !ok {
		return nil, fmt.Errorf("no install command for %s on %s", tool.Name, platform)
	}
	return args, nil
}

// UpgradeCmd returns the platform-specific upgrade command for a tool.
func UpgradeCmd(tool registry.Tool) ([]string, error) {
	platform := runtime.GOOS
	args, ok := tool.InstallCmds[platform]
	if !ok {
		return nil, fmt.Errorf("no upgrade command for %s on %s", tool.Name, platform)
	}
	return toUpgradeCmd(platform, args), nil
}
