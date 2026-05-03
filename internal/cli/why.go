package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/progress"
	"github.com/nassiharel/clim/internal/registry"
)

var whyOutputFmt func() (OutputFormat, error)

var whyCmd = &cobra.Command{
	Use:   "why <tool>",
	Short: "Show why a tool is needed and where it's referenced",
	Long: `Show all references to a tool across projects, packs, and your system.

Examples:
  clim why kubectl
  clim why terraform
  clim why kubectl --output json`,
	Args: requireArgs(1, "clim why <tool>"),
	RunE: runWhy,
}

func init() {
	whyOutputFmt = addOutputFlag(whyCmd, OutputText, OutputJSON)
	// Registered in root.go with command group.
}

// whyReference is preserved as a backwards-compatible alias for the
// shared cli.Reference type. The JSON shape is unchanged because the
// field tags are identical.
//
//nolint:unused // kept for documentation
type whyReference = Reference

type whyPackageEntry struct {
	Source string `json:"source"`
	ID     string `json:"id"`
}

type whyReport struct {
	Name         string            `json:"name"`
	DisplayName  string            `json:"display_name,omitempty"`
	Category     string            `json:"category,omitempty"`
	Description  string            `json:"description,omitempty"`
	Installed    bool              `json:"installed"`
	Version      string            `json:"version,omitempty"`
	Source       string            `json:"source,omitempty"`
	Path         string            `json:"path,omitempty"`
	Instances    int               `json:"instances,omitempty"`
	Latest       string            `json:"latest,omitempty"`
	UpdateAvail  bool              `json:"update_available,omitempty"`
	References   []whyReference    `json:"references"`
	AvailableVia []whyPackageEntry `json:"available_via"`
	RelatedTools []string          `json:"related_tools"`
	Warnings     []string          `json:"warnings"`
}

func runWhy(cmd *cobra.Command, args []string) error {
	out, err := whyOutputFmt()
	if err != nil {
		return err
	}
	toolName := args[0]

	sp := progress.New("Scanning...")
	tools, _, _, err := svcFrom(cmd).LoadAndResolveCached(cmd.Context(), false)
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

	report := buildWhyReport(cmd, toolName, t, tools)

	if out == OutputJSON {
		return printJSON(report)
	}

	renderWhyText(report)
	return nil
}

// buildWhyReport collects all referenced data without printing anything.
// Warnings (e.g. "could not load project registry") are accumulated on the
// report rather than printed to stderr so JSON callers see the same data.
//
// Collection fields are pre-allocated to empty slices so the JSON shape
// is stable: callers always see arrays, never null, regardless of whether
// the tool has any references / available packages / related tools.
func buildWhyReport(cmd *cobra.Command, toolName string, t *registry.Tool, tools []registry.Tool) whyReport {
	r := whyReport{
		Name:         t.Name,
		DisplayName:  t.DisplayName,
		Category:     t.Category,
		Installed:    t.IsInstalled(),
		Latest:       t.Latest,
		References:   []whyReference{},
		AvailableVia: []whyPackageEntry{},
		RelatedTools: []string{},
		Warnings:     []string{},
	}
	if t.GitHubInfo != nil {
		r.Description = t.GitHubInfo.Description
	}
	if r.Installed {
		primary := t.PrimaryInstance()
		if primary != nil {
			r.Version = primary.Version
			r.Source = string(primary.Source)
			r.Path = primary.Path
		}
		r.Instances = len(t.Instances)
		r.UpdateAvail = t.HasUpdate()
	}

	// References + warnings: shared scanner so `clim why` and `clim info`
	// cannot drift in their reference-discovery rules.
	refs, warnings := CollectReferences(cmd, toolName)
	if refs != nil {
		r.References = refs
	}
	r.Warnings = append(r.Warnings, warnings...)

	// Available packages.
	r.AvailableVia = append(r.AvailableVia, collectPackageEntriesForWhy(t.Packages)...)

	// Related installed tools (same category/tags).
	r.RelatedTools = relatedInstalledTools(toolName, t, tools)

	return r
}

func collectPackageEntriesForWhy(pkgs registry.PackageIDs) []whyPackageEntry {
	all := []whyPackageEntry{
		{Source: "winget", ID: pkgs.Winget},
		{Source: "choco", ID: pkgs.Choco},
		{Source: "scoop", ID: pkgs.Scoop},
		{Source: "brew", ID: pkgs.Brew},
		{Source: "apt", ID: pkgs.Apt},
		{Source: "snap", ID: pkgs.Snap},
		{Source: "npm", ID: pkgs.NPM},
	}
	out := make([]whyPackageEntry, 0, len(all))
	for _, e := range all {
		if e.ID != "" {
			out = append(out, e)
		}
	}
	return out
}

func relatedInstalledTools(toolName string, t *registry.Tool, tools []registry.Tool) []string {
	toolTags := make(map[string]bool)
	for _, tag := range t.Tags {
		toolTags[strings.ToLower(tag)] = true
	}
	related := []string{}
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
	return related
}

func renderWhyText(r whyReport) {
	fmt.Fprintf(os.Stderr, "\n%s (%s)\n", r.DisplayName, r.Category)
	if r.Description != "" {
		fmt.Fprintf(os.Stderr, "  %s\n", r.Description)
	}
	fmt.Fprintln(os.Stderr)

	if r.Installed {
		fmt.Fprintf(os.Stderr, "  ✓ Installed: %s (%s) at %s\n", r.Version, r.Source, r.Path)
		if r.Instances > 1 {
			fmt.Fprintf(os.Stderr, "    + %d additional installation(s)\n", r.Instances-1)
		}
		if r.UpdateAvail {
			fmt.Fprintf(os.Stderr, "    ⬆ Update available: %s → %s\n", r.Version, r.Latest)
		}
	} else {
		fmt.Fprintf(os.Stderr, "  ✗ Not installed\n")
	}
	fmt.Fprintln(os.Stderr)

	if len(r.References) > 0 {
		fmt.Fprintf(os.Stderr, "  Referenced by:\n")
		for _, ref := range r.References {
			fmt.Fprintf(os.Stderr, "    • %s\n", formatWhyRef(ref))
		}
	} else {
		fmt.Fprintf(os.Stderr, "  No project or pack references found.\n")
	}
	fmt.Fprintln(os.Stderr)

	if len(r.AvailableVia) > 0 {
		var pairs []string
		for _, p := range r.AvailableVia {
			pairs = append(pairs, p.Source+": "+p.ID)
		}
		fmt.Fprintf(os.Stderr, "  Available via: %s\n", strings.Join(pairs, ", "))
	}

	if len(r.RelatedTools) > 0 {
		fmt.Fprintf(os.Stderr, "  Related installed tools: %s\n", strings.Join(r.RelatedTools, ", "))
	}

	// Warnings last so they don't disrupt the tool heading / status block.
	for _, w := range r.Warnings {
		fmt.Fprintf(os.Stderr, "  ⚠ %s\n", w)
	}
}

func formatWhyRef(ref whyReference) string {
	switch ref.Kind {
	case "teamfile":
		if ref.Required {
			constraint := ""
			if ref.Constraint != "" {
				constraint = " " + ref.Constraint
			}
			return fmt.Sprintf(".clim.yaml (required%s) — %s", constraint, ref.Path)
		}
		return ".clim.yaml (optional) — " + ref.Path
	case "project":
		role := "optional"
		if ref.Required {
			role = "required"
		}
		return fmt.Sprintf("Project %q (%s) — %s", ref.Name, role, ref.Path)
	case "pack":
		if ref.DisplayName != "" {
			return fmt.Sprintf("Pack %q (%s)", ref.DisplayName, ref.Name)
		}
		return fmt.Sprintf("Pack %q", ref.Name)
	case "custom_pack":
		if ref.DisplayName != "" {
			return fmt.Sprintf("Custom pack %q (%s)", ref.DisplayName, ref.Name)
		}
		return fmt.Sprintf("Custom pack %q", ref.Name)
	}
	return ref.Kind + " " + ref.Name
}
