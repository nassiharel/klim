package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/build"
	"github.com/nassiharel/clim/internal/selfupdate"
)

var checkFlag bool

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update clim to the latest version",
	Long: `Check GitHub Releases for a newer version of clim and, if one exists,
download and install it in-place.

This command requires an internet connection and write permission to the
directory containing the clim binary.

Use --check to see if an update is available without installing it.

If you installed clim via Homebrew, you may prefer 'brew upgrade clim'.`,
	RunE: runUpdate,
}

func init() {
	updateCmd.Flags().BoolVar(&checkFlag, "check", false, "Check for updates without installing")
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	currentVersion := build.VersionOnly()

	fmt.Fprintf(os.Stderr, "Current version: %s\n", currentVersion)
	fmt.Fprintf(os.Stderr, "Checking for updates...\n")

	ctx, cancel := context.WithTimeout(cmd.Context(), 60*time.Second)
	defer cancel()

	result, err := selfupdate.Update(ctx, currentVersion, &selfupdate.Options{
		CheckOnly: checkFlag,
	})
	if err != nil {
		return fmt.Errorf("update failed: %w", err)
	}

	if checkFlag {
		if result.UpdateAvailable() {
			fmt.Fprintf(os.Stderr, "Update available: %s → %s\n",
				result.CurrentVersion, result.LatestVersion)
			fmt.Fprintf(os.Stderr, "Run 'clim update' to install it.\n")
		} else {
			fmt.Fprintf(os.Stderr, "Already up to date!\n")
		}
		return nil
	}

	if result.Updated {
		fmt.Fprintf(os.Stderr, "Successfully updated from %s to version %s\n",
			result.CurrentVersion, result.LatestVersion)
	} else {
		fmt.Fprintf(os.Stderr, "Already up to date!\n")
	}
	return nil
}
