package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/custompacks"
	"github.com/nassiharel/clim/internal/progress"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/teamfile"
)

var (
	infoRefresh   bool
	infoOutputFmt func() (OutputFormat, error)
)

var infoCmd = &cobra.Command{
	Use:   "info <tool>",
	Short: "Show everything about a tool — versions, packages, references, GitHub info",
	Long: `Display a comprehensive view of a tool: install status across every
detected instance, available package managers, GitHub project metadata,
project / pack references, and related installed tools.

This is the CLI counterpart to the TUI's tool detail page.

Examples:
  clim info kubectl                 # human-readable table
  clim info terraform --output json # machine-readable for scripts
  clim info bat --refresh           # force a fresh PATH scan first`,
	Args: requireArgs(1, "clim info <tool>"),
	RunE: runInfo,
}

func init() {
	infoCmd.Flags().BoolVar(&infoRefresh, "refresh", false, "Force fresh scan (ignore cache)")
	infoOutputFmt = addOutputFlag(infoCmd, OutputText, OutputJSON)
	// Registered in root.go with command group.
}

// infoInstance is the JSON shape for one detected installation.
type infoInstance struct {
	Path    string `json:"path,omitempty"`
	Version string `json:"version,omitempty"`
	Source  string `json:"source,omitempty"`
}

// infoPackage represents one available package-manager ID.
type infoPackage struct {
	Source string `json:"source"`
	ID     string `json:"id"`
}

// infoReference is where the tool is referenced (teamfile / project / pack).
type infoReference struct {
	Kind        string `json:"kind"`
	Name        string `json:"name,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Path        string `json:"path,omitempty"`
	Required    bool   `json:"required,omitempty"`
	Constraint  string `json:"version_constraint,omitempty"`
}

// infoGitHub mirrors the catalog's GitHub metadata in CLI-shape.
type infoGitHub struct {
	Slug        string   `json:"slug,omitempty"`
	URL         string   `json:"url,omitempty"`
	Stars       int      `json:"stars,omitempty"`
	Forks       int      `json:"forks,omitempty"`
	Description string   `json:"description,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
	License     string   `json:"license,omitempty"`
	Topics      []string `json:"topics,omitempty"`
	Archived    bool     `json:"archived,omitempty"`
	LastPush    string   `json:"last_push,omitempty"`
}

// infoReport is the full JSON shape returned by `clim info <tool> --output json`.
type infoReport struct {
	Name            string          `json:"name"`
	DisplayName     string          `json:"display_name,omitempty"`
	Category        string          `json:"category,omitempty"`
	Tags            []string        `json:"tags"`
	Installed       bool            `json:"installed"`
	UpdateAvailable bool            `json:"update_available"`
	Latest          string          `json:"latest,omitempty"`
	LatestSource    string          `json:"latest_source,omitempty"`
	Instances       []infoInstance  `json:"instances"`
	Packages        []infoPackage   `json:"packages"`
	GitHub          *infoGitHub     `json:"github,omitempty"`
	References      []infoReference `json:"references"`
	RelatedTools    []string        `json:"related_tools"`
	Warnings        []string        `json:"warnings"`
}

func runInfo(cmd *cobra.Command, args []string) error {
	out, err := infoOutputFmt()
	if err != nil {
		return err
	}
	toolName := args[0]

	sp := progress.New("Resolving tool info...")
	tools, _, _, err := svcFrom(cmd).LoadAndResolveCached(cmd.Context(), infoRefresh)
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	sp.Done("Done")

	toolMap := registry.ToolMap(tools)
	t, ok := toolMap[toolName]
	if !ok {
		// Suggest the closest match if the user fat-fingered.
		if suggestion := closestToolName(tools, toolName); suggestion != "" {
			return fmt.Errorf("tool %q not found in catalog (did you mean %q?)", toolName, suggestion)
		}
		return fmt.Errorf("tool %q not found in catalog", toolName)
	}

	report := buildInfoReport(cmd, t, tools)

	if out == OutputJSON {
		return printJSON(report)
	}
	renderInfoText(report, t)
	return nil
}

// buildInfoReport assembles the full report from the tool, its instances,
// the catalog, project references, and pack references. Errors that don't
// fully prevent rendering are pushed into report.Warnings instead of
// aborting (matches the convention used by `clim why`).
func buildInfoReport(cmd *cobra.Command, t *registry.Tool, tools []registry.Tool) infoReport {
	r := infoReport{
		Name:            t.Name,
		DisplayName:     t.DisplayName,
		Category:        t.Category,
		Tags:            append([]string{}, t.Tags...),
		Installed:       t.IsInstalled(),
		UpdateAvailable: t.HasUpdate(),
		Latest:          t.Latest,
		LatestSource:    t.LatestFrom,
		Instances:       []infoInstance{},
		Packages:        []infoPackage{},
		References:      []infoReference{},
		RelatedTools:    []string{},
		Warnings:        []string{},
	}
	if r.Tags == nil {
		r.Tags = []string{}
	}

	for _, inst := range t.Instances {
		r.Instances = append(r.Instances, infoInstance{
			Path:    inst.Path,
			Version: inst.Version,
			Source:  string(inst.Source),
		})
	}

	r.Packages = append(r.Packages, catalogPackagesFor(t.Packages)...)
	if t.GitHubSlug != "" || t.GitHubInfo != nil {
		gh := &infoGitHub{Slug: t.GitHubSlug}
		if t.GitHubSlug != "" {
			gh.URL = "https://github.com/" + t.GitHubSlug
		}
		if t.GitHubInfo != nil {
			gh.Stars = t.GitHubInfo.Stars
			gh.Forks = t.GitHubInfo.Forks
			gh.Description = t.GitHubInfo.Description
			gh.Homepage = t.GitHubInfo.Homepage
			gh.License = t.GitHubInfo.License
			gh.Topics = append([]string{}, t.GitHubInfo.Topics...)
			gh.Archived = t.GitHubInfo.Archived
			gh.LastPush = t.GitHubInfo.PushedAt
		}
		r.GitHub = gh
	}

	r.References = collectInfoReferences(cmd, t.Name, &r)
	r.RelatedTools = relatedInstalledTools(t.Name, t, tools)
	return r
}

// catalogPackagesFor returns the populated package IDs in display order.
func catalogPackagesFor(p registry.PackageIDs) []infoPackage {
	all := []infoPackage{
		{Source: "winget", ID: p.Winget},
		{Source: "choco", ID: p.Choco},
		{Source: "scoop", ID: p.Scoop},
		{Source: "brew", ID: p.Brew},
		{Source: "apt", ID: p.Apt},
		{Source: "snap", ID: p.Snap},
		{Source: "npm", ID: p.NPM},
	}
	out := make([]infoPackage, 0, len(all))
	for _, e := range all {
		if e.ID != "" {
			out = append(out, e)
		}
	}
	return out
}

// collectInfoReferences mirrors `clim why`'s reference scan. Warnings are
// appended to r.Warnings rather than printed to stderr so JSON consumers
// see the same signal.
func collectInfoReferences(cmd *cobra.Command, toolName string, r *infoReport) []infoReference {
	var refs []infoReference

	// 1) Local .clim.yaml in or above CWD.
	cwd, _ := os.Getwd()
	var seenTeamPath string
	if cwd != "" {
		path := teamfile.Find(cwd)
		if path != "" {
			seenTeamPath = path
			tf, err := teamfile.Parse(path)
			if err != nil {
				r.Warnings = append(r.Warnings, fmt.Sprintf("could not parse %s: %v", path, err))
			} else {
				for _, req := range tf.Tools {
					if req.Name == toolName {
						refs = append(refs, infoReference{
							Kind: "teamfile", Path: path, Required: true, Constraint: req.Version,
						})
					}
				}
				for _, opt := range tf.Optional {
					if opt.Name == toolName {
						refs = append(refs, infoReference{
							Kind: "teamfile", Path: path, Required: false, Constraint: opt.Version,
						})
					}
				}
			}
		}
	}

	// 2) Registered projects.
	projects, projErr := teamfile.LoadProjects()
	if projErr != nil {
		r.Warnings = append(r.Warnings, fmt.Sprintf("could not load project registry: %v", projErr))
	}
	for _, proj := range projects {
		climPath := filepath.Join(proj.Path, ".clim.yaml")
		if seenTeamPath != "" && filepath.Clean(climPath) == filepath.Clean(seenTeamPath) {
			continue
		}
		tf, err := teamfile.Parse(climPath)
		if err != nil {
			r.Warnings = append(r.Warnings, fmt.Sprintf("could not parse %s: %v", climPath, err))
			continue
		}
		for _, req := range tf.Tools {
			if req.Name == toolName {
				refs = append(refs, infoReference{
					Kind: "project", Name: proj.Name, Path: climPath, Required: true, Constraint: req.Version,
				})
			}
		}
		for _, opt := range tf.Optional {
			if opt.Name == toolName {
				refs = append(refs, infoReference{
					Kind: "project", Name: proj.Name, Path: climPath, Required: false, Constraint: opt.Version,
				})
			}
		}
	}

	// 3) Marketplace packs.
	packs, packErr := svcFrom(cmd).LoadPacks(cmd.Context())
	if packErr != nil {
		r.Warnings = append(r.Warnings, fmt.Sprintf("could not load packs: %v", packErr))
	}
	for _, pack := range packs {
		for _, pToolName := range pack.ToolNames {
			if pToolName == toolName {
				refs = append(refs, infoReference{
					Kind: "pack", Name: pack.Name, DisplayName: pack.DisplayName,
				})
			}
		}
	}

	// 4) Custom packs.
	if cp, cpErr := custompacks.Load(); cpErr != nil {
		r.Warnings = append(r.Warnings, fmt.Sprintf("could not load custom packs: %v", cpErr))
	} else {
		for _, pack := range cp {
			for _, pToolName := range pack.ToolNames {
				if pToolName == toolName {
					refs = append(refs, infoReference{
						Kind: "custom_pack", Name: pack.Name, DisplayName: pack.DisplayName,
					})
				}
			}
		}
	}

	return refs
}

// closestToolName returns a single suggestion for a misspelled tool name,
// or "" if no candidate is similar enough. Levenshtein distance ≤ 3.
func closestToolName(tools []registry.Tool, q string) string {
	q = strings.ToLower(q)
	bestName := ""
	bestDist := 4
	for _, t := range tools {
		d := levenshtein(strings.ToLower(t.Name), q)
		if d < bestDist {
			bestDist = d
			bestName = t.Name
		}
	}
	return bestName
}

// levenshtein computes the edit distance between a and b. Caps at len(b)+1
// for early termination on very different strings.
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = minInt(curr[j-1]+1, minInt(prev[j]+1, prev[j-1]+cost))
		}
		copy(prev, curr)
	}
	return prev[lb]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// renderInfoText writes the human-readable view of report to stdout.
// Warnings (if any) lead so the user can't miss them; the rest is
// structured to match the TUI detail page.
func renderInfoText(r infoReport, t *registry.Tool) {
	w := os.Stdout

	// Header line.
	header := r.DisplayName
	if header == "" {
		header = r.Name
	}
	if r.Category != "" {
		header += "  (" + r.Category + ")"
	}
	if r.GitHub != nil && r.GitHub.Stars > 0 {
		header += "  ★ " + formatStarCount(r.GitHub.Stars)
	}
	if r.UpdateAvailable {
		header += "  ⬆ Update available"
	}
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, header)

	// Description.
	if r.GitHub != nil && r.GitHub.Description != "" {
		for _, line := range wordWrapStr(r.GitHub.Description, 78) {
			_, _ = fmt.Fprintln(w, "  "+line)
		}
	}
	_, _ = fmt.Fprintln(w, "")

	// Install status — all instances.
	if r.Installed {
		switch len(r.Instances) {
		case 0:
			// shouldn't happen; defensive
		case 1:
			inst := r.Instances[0]
			_, _ = fmt.Fprintf(w, "  ✓ Installed: %s (%s) at %s\n", dashIfEmpty(inst.Version), dashIfEmpty(inst.Source), inst.Path)
		default:
			_, _ = fmt.Fprintln(w, "  ✓ Installed (multiple instances):")
			for _, inst := range r.Instances {
				_, _ = fmt.Fprintf(w, "      %s (%s) at %s\n", dashIfEmpty(inst.Version), dashIfEmpty(inst.Source), inst.Path)
			}
		}
		if r.UpdateAvailable && r.Latest != "" {
			_, _ = fmt.Fprintf(w, "  ⬆ Update available: %s → %s\n", dashIfEmpty(t.InstalledVersion()), r.Latest)
		}
	} else {
		_, _ = fmt.Fprintln(w, "  ✗ Not installed")
	}
	_, _ = fmt.Fprintln(w, "")

	// Available packages.
	if len(r.Packages) > 0 {
		_, _ = fmt.Fprintln(w, "  Available via:")
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, p := range r.Packages {
			_, _ = fmt.Fprintf(tw, "    %s\t%s\n", p.Source, p.ID)
		}
		_ = tw.Flush()
		_, _ = fmt.Fprintln(w, "")
	}

	// GitHub block.
	if r.GitHub != nil {
		_, _ = fmt.Fprintln(w, "  GitHub:")
		if r.GitHub.Slug != "" {
			_, _ = fmt.Fprintf(w, "    Repo:      %s\n", r.GitHub.URL)
		}
		if r.GitHub.Archived {
			_, _ = fmt.Fprintln(w, "    ⚠ Repository is archived (no longer maintained)")
		}
		stats := []string{}
		if r.GitHub.Stars > 0 {
			stats = append(stats, "★ "+formatStarCount(r.GitHub.Stars)+" stars")
		}
		if r.GitHub.Forks > 0 {
			stats = append(stats, "⑂ "+formatStarCount(r.GitHub.Forks)+" forks")
		}
		if len(stats) > 0 {
			_, _ = fmt.Fprintf(w, "    Stats:     %s\n", strings.Join(stats, "   "))
		}
		if r.GitHub.License != "" {
			_, _ = fmt.Fprintf(w, "    License:   %s\n", r.GitHub.License)
		}
		if r.GitHub.Homepage != "" {
			_, _ = fmt.Fprintf(w, "    Homepage:  %s\n", r.GitHub.Homepage)
		}
		if len(r.GitHub.Topics) > 0 {
			_, _ = fmt.Fprintf(w, "    Topics:    %s\n", strings.Join(r.GitHub.Topics, ", "))
		}
		if r.GitHub.LastPush != "" {
			if d := formatGitHubDate(r.GitHub.LastPush); d != "" {
				_, _ = fmt.Fprintf(w, "    Last push: %s\n", d)
			}
		}
		_, _ = fmt.Fprintln(w, "")
	}

	// Tags.
	if len(r.Tags) > 0 {
		_, _ = fmt.Fprintf(w, "  Tags: %s\n\n", strings.Join(r.Tags, ", "))
	}

	// References.
	if len(r.References) > 0 {
		_, _ = fmt.Fprintln(w, "  Referenced by:")
		for _, ref := range r.References {
			_, _ = fmt.Fprintf(w, "    • %s\n", formatInfoRef(ref))
		}
		_, _ = fmt.Fprintln(w, "")
	}

	// Related installed tools.
	if len(r.RelatedTools) > 0 {
		_, _ = fmt.Fprintf(w, "  Related installed tools: %s\n\n", strings.Join(r.RelatedTools, ", "))
	}

	// Warnings last so they don't blow away the heading.
	for _, msg := range r.Warnings {
		_, _ = fmt.Fprintf(os.Stderr, "  ⚠ %s\n", msg)
	}
}

func formatInfoRef(ref infoReference) string {
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

// formatStarCount renders 1234 → "1.2k", 12345 → "12.3k", under 1000 unchanged.
func formatStarCount(n int) string {
	switch {
	case n >= 100000:
		return strconv.Itoa(n/1000) + "k"
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return strconv.Itoa(n)
	}
}

// formatGitHubDate renders an RFC3339 timestamp as a human-friendly delta
// (e.g. "3 days ago"), falling back to the raw date if parsing fails.
func formatGitHubDate(s string) string {
	if s == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	d := time.Since(t)
	switch {
	case d < 24*time.Hour:
		return "today"
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%d day(s) ago", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%d week(s) ago", int(d.Hours()/(24*7)))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%d month(s) ago", int(d.Hours()/(24*30)))
	default:
		return fmt.Sprintf("%d year(s) ago", int(d.Hours()/(24*365)))
	}
}

// wordWrapStr wraps s into lines no wider than width characters. Pure-byte
// width is fine for descriptions in the catalog (ASCII-dominant).
func wordWrapStr(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	current := words[0]
	for _, word := range words[1:] {
		if len(current)+1+len(word) > width {
			lines = append(lines, current)
			current = word
		} else {
			current += " " + word
		}
	}
	lines = append(lines, current)
	return lines
}

// dashIfEmpty returns "—" when s is empty so tables don't show empty cells.
func dashIfEmpty(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
