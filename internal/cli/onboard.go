package cli

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/progress"
	"github.com/nassiharel/clim/internal/registry"
)

// Dev role definitions for the onboard wizard.
type devRole struct {
	name        string
	description string
	categories  []string // marketplace categories to recommend
	tags        []string // tool tags to boost
}

var roles = []devRole{
	{
		name:        "web",
		description: "Web Development (Frontend/Backend)",
		categories:  []string{"JavaScript", "Web", "API"},
		tags:        []string{"javascript", "typescript", "node", "web", "frontend", "backend", "api", "http"},
	},
	{
		name:        "devops",
		description: "DevOps / Cloud / Infrastructure",
		categories:  []string{"Cloud", "IaC", "Containers", "K8s", "CI/CD"},
		tags:        []string{"cloud", "aws", "azure", "gcp", "kubernetes", "docker", "terraform", "ci", "cd", "devops", "infrastructure"},
	},
	{
		name:        "data",
		description: "Data / ML / AI",
		categories:  []string{"Data", "ML", "Python"},
		tags:        []string{"data", "ml", "ai", "python", "jupyter", "analytics"},
	},
	{
		name:        "mobile",
		description: "Mobile Development (iOS/Android)",
		categories:  []string{"Mobile", "JavaScript"},
		tags:        []string{"mobile", "ios", "android", "flutter", "react-native"},
	},
	{
		name:        "systems",
		description: "Systems / Embedded / Low-level",
		categories:  []string{"Systems", "Compilers", "Debug"},
		tags:        []string{"systems", "c", "c++", "rust", "embedded", "compiler", "debug", "performance"},
	},
	{
		name:        "security",
		description: "Security / Pen-testing",
		categories:  []string{"Security", "Network"},
		tags:        []string{"security", "pentest", "crypto", "network", "vulnerability"},
	},
}

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
  clim onboard              # interactive — shows role picker
  clim onboard devops       # recommend tools for DevOps
  clim onboard web --list   # just list, don't install`,
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
	var role *devRole
	if len(args) > 0 {
		for i := range roles {
			if strings.EqualFold(roles[i].name, args[0]) {
				role = &roles[i]
				break
			}
		}
		if role == nil {
			fmt.Fprintf(os.Stderr, "Unknown role %q. Available roles:\n", args[0])
			for _, r := range roles {
				fmt.Fprintf(os.Stderr, "  %-12s %s\n", r.name, r.description)
			}
			return fmt.Errorf("unknown role: %s", args[0])
		}
	} else {
		// Show role picker.
		fmt.Fprintln(os.Stderr, "What kind of development do you do?")
		for i, r := range roles {
			fmt.Fprintf(os.Stderr, "  %d. %-12s %s\n", i+1, r.name, r.description)
		}
		fmt.Fprintln(os.Stderr)
		fmt.Fprint(os.Stderr, "Enter number (1-6): ")
		var choice int
		if _, err := fmt.Fscan(os.Stdin, &choice); err != nil || choice < 1 || choice > len(roles) {
			return fmt.Errorf("invalid choice")
		}
		role = &roles[choice-1]
	}

	// Load catalog + scan PATH (no version resolution needed).
	sp := progress.New("Loading catalog...")
	tools, _, err := svc.ScanOnly(cmd.Context())
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	sp.Done("Catalog loaded")

	// Score tools for this role.
	type scored struct {
		tool  registry.Tool
		score int
	}

	catSet := make(map[string]bool)
	for _, c := range role.categories {
		catSet[strings.ToLower(c)] = true
	}
	tagSet := make(map[string]bool)
	for _, t := range role.tags {
		tagSet[strings.ToLower(t)] = true
	}

	var recommended []scored
	for _, t := range tools {
		// Skip already installed tools.
		if t.IsInstalled() {
			continue
		}
		// Skip tools without packages for this OS.
		if !t.Packages.HasAnyPackageForOS() {
			continue
		}

		score := 0
		if catSet[strings.ToLower(t.Category)] {
			score += 10
		}
		for _, tag := range t.Tags {
			if tagSet[strings.ToLower(tag)] {
				score += 5
			}
		}
		// Boost by GitHub stars.
		if t.GitHubInfo != nil && t.GitHubInfo.Stars > 1000 {
			score += 2
		}
		if t.GitHubInfo != nil && t.GitHubInfo.Stars > 10000 {
			score += 3
		}

		if score > 0 {
			recommended = append(recommended, scored{tool: t, score: score})
		}
	}

	sort.Slice(recommended, func(i, j int) bool {
		return recommended[i].score > recommended[j].score
	})

	// Cap at 15.
	if len(recommended) > 15 {
		recommended = recommended[:15]
	}

	if len(recommended) == 0 {
		fmt.Fprintf(os.Stderr, "\nNo additional tools found for %s role — you may already have everything!\n", role.name)
		return nil
	}

	// Display.
	fmt.Fprintf(os.Stderr, "\nRecommended tools for %s (%s):\n\n", role.name, role.description)

	installedCount := 0
	for _, t := range tools {
		if t.IsInstalled() {
			installedCount++
		}
	}
	fmt.Fprintf(os.Stderr, "  You have %d tools installed. Here are %d more you might need:\n\n", installedCount, len(recommended))

	for i, s := range recommended {
		desc := ""
		if s.tool.GitHubInfo != nil && s.tool.GitHubInfo.Description != "" {
			desc = " — " + s.tool.GitHubInfo.Description
		}
		stars := ""
		if s.tool.GitHubInfo != nil && s.tool.GitHubInfo.Stars > 0 {
			stars = fmt.Sprintf(" (★%d)", s.tool.GitHubInfo.Stars)
		}
		fmt.Fprintf(os.Stderr, "  %2d. %-20s %s%s%s\n", i+1, s.tool.DisplayName, s.tool.Category, stars, desc)
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
			if ia := s.tool.Packages.InstallArgs(src); ia != nil {
				installArgs = ia
				break
			}
		}
		if installArgs == nil {
			continue
		}
		fmt.Fprintf(os.Stderr, "  Installing %s...\n", s.tool.DisplayName)
		c := exec.CommandContext(cmd.Context(), installArgs[0], installArgs[1:]...)
		c.Stdin = os.Stdin
		c.Stdout = os.Stderr
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "  ⚠ %s failed: %v\n", s.tool.Name, err)
			failed++
		} else {
			fmt.Fprintf(os.Stderr, "  ✓ %s installed\n", s.tool.DisplayName)
			installed++
		}
	}

	_ = svc.InvalidateScanCache()
	fmt.Fprintf(os.Stderr, "\n%d installed, %d failed\n", installed, failed)
	return nil
}
