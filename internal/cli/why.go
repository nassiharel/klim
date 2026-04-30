package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/progress"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/teamfile"
)

var whyCmd = &cobra.Command{
	Use:   "why <tool>",
	Short: "Show why a tool is needed and where it's referenced",
	Long: `Show all references to a tool across projects, packs, and your system.

Examples:
  clim why kubectl
  clim why terraform`,
	Args: cobra.ExactArgs(1),
	RunE: runWhy,
}

func init() {
	rootCmd.AddCommand(whyCmd)
}

func runWhy(cmd *cobra.Command, args []string) error {
	toolName := args[0]

	sp := progress.New("Scanning...")
	tools, _, _, err := svc.LoadAndResolveCached(cmd.Context(), false)
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	sp.Done("Done")

	// Find the tool.
	toolMap := registry.ToolMap(tools)
	t, found := toolMap[toolName]
	if !found {
		return fmt.Errorf("%s is not in the catalog", toolName)
	}

	fmt.Fprintf(os.Stderr, "\n%s (%s)\n", t.DisplayName, t.Category)
	if t.GitHubInfo != nil && t.GitHubInfo.Description != "" {
		fmt.Fprintf(os.Stderr, "  %s\n", t.GitHubInfo.Description)
	}
	fmt.Fprintln(os.Stderr)

	// Installation status.
	if t.IsInstalled() {
		primary := t.PrimaryInstance()
		fmt.Fprintf(os.Stderr, "  ✓ Installed: %s (%s) at %s\n", primary.Version, primary.Source, primary.Path)
		if len(t.Instances) > 1 {
			fmt.Fprintf(os.Stderr, "    + %d additional installation(s)\n", len(t.Instances)-1)
		}
		if t.HasUpdate() {
			fmt.Fprintf(os.Stderr, "    ⬆ Update available: %s → %s\n", primary.Version, t.Latest)
		}
	} else {
		fmt.Fprintf(os.Stderr, "  ✗ Not installed\n")
	}
	fmt.Fprintln(os.Stderr)

	// References.
	var refs []string

	// 1. Check .clim.yaml projects.
	cwd, _ := os.Getwd()
	if cwd != "" {
		path := teamfile.Find(cwd)
		if path != "" {
			tf, err := teamfile.Parse(path)
			if err == nil {
				for _, req := range tf.Tools {
					if req.Name == toolName {
						constraint := ""
						if req.Version != "" {
							constraint = " " + req.Version
						}
						refs = append(refs, fmt.Sprintf(".clim.yaml (required%s) — %s", constraint, path))
					}
				}
				for _, opt := range tf.Optional {
					if opt.Name == toolName {
						refs = append(refs, fmt.Sprintf(".clim.yaml (optional) — %s", path))
					}
				}
			}
		}
	}

	// 2. Check all registered projects.
	projects, _ := teamfile.LoadProjects()
	for _, proj := range projects {
		climPath := filepath.Join(proj.Path, ".clim.yaml")
		if cwd != "" && filepath.Clean(climPath) == filepath.Clean(teamfile.Find(cwd)) {
			continue // already handled above
		}
		tf, err := teamfile.Parse(climPath)
		if err != nil {
			continue
		}
		for _, req := range tf.Tools {
			if req.Name == toolName {
				refs = append(refs, fmt.Sprintf("Project %q (required) — %s", proj.Name, climPath))
			}
		}
		for _, opt := range tf.Optional {
			if opt.Name == toolName {
				refs = append(refs, fmt.Sprintf("Project %q (optional) — %s", proj.Name, climPath))
			}
		}
	}

	// 3. Check packs.
	packs, _ := svc.LoadPacks(cmd.Context())
	for _, pack := range packs {
		for _, pToolName := range pack.ToolNames {
			if pToolName == toolName {
				refs = append(refs, fmt.Sprintf("Pack %q (%s)", pack.DisplayName, pack.Name))
			}
		}
	}

	// 4. Check custom packs.
	// Already included in packs if loaded correctly, but let's be safe.

	if len(refs) > 0 {
		fmt.Fprintf(os.Stderr, "  Referenced by:\n")
		for _, ref := range refs {
			fmt.Fprintf(os.Stderr, "    • %s\n", ref)
		}
	} else {
		fmt.Fprintf(os.Stderr, "  No project or pack references found.\n")
	}
	fmt.Fprintln(os.Stderr)

	// Available packages.
	var pkgs []string
	if t.Packages.Winget != "" {
		pkgs = append(pkgs, fmt.Sprintf("winget: %s", t.Packages.Winget))
	}
	if t.Packages.Choco != "" {
		pkgs = append(pkgs, fmt.Sprintf("choco: %s", t.Packages.Choco))
	}
	if t.Packages.Scoop != "" {
		pkgs = append(pkgs, fmt.Sprintf("scoop: %s", t.Packages.Scoop))
	}
	if t.Packages.Brew != "" {
		pkgs = append(pkgs, fmt.Sprintf("brew: %s", t.Packages.Brew))
	}
	if t.Packages.Apt != "" {
		pkgs = append(pkgs, fmt.Sprintf("apt: %s", t.Packages.Apt))
	}
	if t.Packages.Snap != "" {
		pkgs = append(pkgs, fmt.Sprintf("snap: %s", t.Packages.Snap))
	}
	if t.Packages.NPM != "" {
		pkgs = append(pkgs, fmt.Sprintf("npm: %s", t.Packages.NPM))
	}
	if len(pkgs) > 0 {
		fmt.Fprintf(os.Stderr, "  Available via: %s\n", strings.Join(pkgs, ", "))
	}

	// Dependents — tools commonly used with this one (same category/tags).
	var related []string
	toolTags := make(map[string]bool)
	for _, tag := range t.Tags {
		toolTags[strings.ToLower(tag)] = true
	}
	for _, other := range tools {
		if other.Name == toolName || !other.IsInstalled() {
			continue
		}
		overlap := 0
		if strings.EqualFold(other.Category, t.Category) {
			overlap += 2
		}
		for _, tag := range other.Tags {
			if toolTags[strings.ToLower(tag)] {
				overlap++
			}
		}
		if overlap >= 2 {
			related = append(related, other.Name)
		}
	}
	sort.Strings(related)
	if len(related) > 5 {
		related = related[:5]
	}
	if len(related) > 0 {
		fmt.Fprintf(os.Stderr, "  Related installed tools: %s\n", strings.Join(related, ", "))
	}

	return nil
}
