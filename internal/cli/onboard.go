package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/onboard"
	"github.com/nassiharel/klim/internal/progress"
	"github.com/nassiharel/klim/internal/registry"
)

var onboardCmd = &cobra.Command{
	Use:   "onboard [role]",
	Short: "Recommend and install tools based on your development role",
	Long: `Interactive onboarding wizard that recommends tools based on what you do.

Available roles:
  web       Web Development (Frontend/Backend)
  devops    DevOps / Cloud / Infrastructure
  data      Data / ML / AI
  mobile    Mobile Development
  systems   Systems / Embedded / Low-level
  security  Security / Pen-testing

Usage:
  klim tool onboard              # interactive — shows role picker
  klim tool onboard devops       # recommend tools for DevOps
  klim tool onboard web --list   # just list, don't install`,
	Args: cobra.MaximumNArgs(1),
	RunE: runOnboard,
}

var onboardListFlag bool

func init() {
	onboardCmd.Flags().BoolVar(&onboardListFlag, "list", false, "List recommended tools without installing")
	// Registered in root.go with command group.
}

func runOnboard(cmd *cobra.Command, args []string) error {
	// Determine role.
	var role *onboard.Role
	if len(args) > 0 {
		role = onboard.FindRole(args[0])
		if role == nil {
			fmt.Fprintf(os.Stderr, "Unknown role %q. Available roles:\n", args[0])
			for _, r := range onboard.Roles {
				fmt.Fprintf(os.Stderr, "  %-12s %s\n", r.Name, r.Description)
			}
			return fmt.Errorf("unknown role: %s", args[0])
		}
	} else {
		// Show role picker.
		fmt.Fprintln(os.Stderr, "What kind of development do you do?")
		for i, r := range onboard.Roles {
			fmt.Fprintf(os.Stderr, "  %d. %-12s %s\n", i+1, r.Name, r.Description)
		}
		fmt.Fprintln(os.Stderr)
		fmt.Fprintf(os.Stderr, "Enter number (1-%d): ", len(onboard.Roles))
		var choice int
		if _, err := fmt.Fscan(os.Stdin, &choice); err != nil || choice < 1 || choice > len(onboard.Roles) {
			return errors.New("invalid choice")
		}
		role = &onboard.Roles[choice-1]
	}

	// Load catalog + scan PATH (no version resolution needed).
	sp := progress.New("Loading catalog...")
	tools, _, err := svcFrom(cmd).ScanOnly(cmd.Context())
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	sp.Done("Catalog loaded")

	// Score tools for this role.
	recommended := onboard.Recommend(role, tools, 15)

	if len(recommended) == 0 {
		fmt.Fprintf(os.Stderr, "\nNo additional tools found for %s role — you may already have everything!\n", role.Name)
		return nil
	}

	// Display.
	fmt.Fprintf(os.Stderr, "\nRecommended tools for %s (%s):\n\n", role.Name, role.Description)

	installedCount := 0
	for _, t := range tools {
		if t.IsInstalled() {
			installedCount++
		}
	}
	fmt.Fprintf(os.Stderr, "  You have %d tools installed. Here are %d more you might need:\n\n", installedCount, len(recommended))

	for i, s := range recommended {
		desc := ""
		if s.Tool.GitHubInfo != nil && s.Tool.GitHubInfo.Description != "" {
			desc = " — " + s.Tool.GitHubInfo.Description
		}
		stars := ""
		if s.Tool.GitHubInfo != nil && s.Tool.GitHubInfo.Stars > 0 {
			stars = fmt.Sprintf(" (★%d)", s.Tool.GitHubInfo.Stars)
		}
		fmt.Fprintf(os.Stderr, "  %2d. %-20s %s%s%s\n", i+1, s.Tool.DisplayName, s.Tool.Category, stars, desc)
	}

	if onboardListFlag {
		return nil
	}

	fmt.Fprintf(os.Stderr, "\nInstall all? [y/N]: ")
	var answer string
	_, _ = fmt.Fscan(os.Stdin, &answer)
	if !strings.EqualFold(answer, "y") {
		fmt.Fprintln(os.Stderr, "Skipped.")
		return nil
	}

	// Install via best PM.
	sources := registry.SourcesForOS()
	var installed, failed int
	for _, s := range recommended {
		var installArgs []string
		for _, src := range sources {
			if ia := s.Tool.Packages.InstallArgs(src); ia != nil {
				installArgs = ia
				break
			}
		}
		if installArgs == nil {
			continue
		}
		fmt.Fprintf(os.Stderr, "  Installing %s...\n", s.Tool.DisplayName)
		c := exec.CommandContext(cmd.Context(), installArgs[0], installArgs[1:]...)
		c.Stdin = os.Stdin
		c.Stdout = os.Stderr
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠ %s failed: %v\n", s.Tool.Name, err)
			failed++
		} else {
			fmt.Fprintf(os.Stderr, "  ✓ %s installed\n", s.Tool.DisplayName)
			installed++
		}
	}

	_ = svcFrom(cmd).InvalidateScanCache()
	fmt.Fprintf(os.Stderr, "\n%d installed, %d failed\n", installed, failed)
	return nil
}
