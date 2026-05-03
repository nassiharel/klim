package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/githubfmt"
	"github.com/nassiharel/clim/internal/progress"
	"github.com/nassiharel/clim/internal/registry"
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

// infoReference is now an alias for the shared cli.Reference type to keep
// the existing JSON shape stable while the underlying scanner lives in
// refscan.go (shared with `clim why`).
//
//nolint:unused // kept as a documentation anchor; future code may reference it
type infoReference = Reference

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
	Name            string         `json:"name"`
	DisplayName     string         `json:"display_name,omitempty"`
	Category        string         `json:"category,omitempty"`
	Tags            []string       `json:"tags"`
	Installed       bool           `json:"installed"`
	UpdateAvailable bool           `json:"update_available"`
	Latest          string         `json:"latest,omitempty"`
	LatestSource    string         `json:"latest_source,omitempty"`
	Instances       []infoInstance `json:"instances"`
	Packages        []infoPackage  `json:"packages"`
	GitHub          *infoGitHub    `json:"github,omitempty"`
	References      []Reference    `json:"references"`
	RelatedTools    []string       `json:"related_tools"`
	Warnings        []string       `json:"warnings"`
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
		return notFoundError(toolName, closestToolName(tools, toolName))
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

	refs, warnings := CollectReferences(cmd, t.Name)
	if refs == nil {
		refs = []Reference{}
	}
	r.References = refs
	r.Warnings = append(r.Warnings, warnings...)
	r.RelatedTools = relatedInstalledTools(t.Name, t, tools)
	return r
}

// infoPackage is now an alias for cli.PackageEntry to keep the JSON
// shape stable while the canonical helper lives in refscan.go (shared
// with `clim why`).
type infoPackage = PackageEntry

// catalogPackagesFor returns the populated package IDs in display order.
// Delegates to the shared helper so `clim info` and `clim why` cannot
// drift out of sync.
func catalogPackagesFor(p registry.PackageIDs) []infoPackage {
	return CollectPackageEntries(p)
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

// notFoundError builds the error returned when `clim info <name>` is
// given a tool name that isn't in the catalog. Wrapped in UsageError
// so the CLI exits with code 2 (malformed user input) instead of 1
// (runtime failure) — scripts can distinguish a typo from a genuine
// failure. If suggestion is non-empty, it is appended as a "did you
// mean" hint.
func notFoundError(name, suggestion string) error {
	if suggestion != "" {
		return &UsageError{Err: fmt.Errorf("tool %q not found in catalog (did you mean %q?)", name, suggestion)}
	}
	return &UsageError{Err: fmt.Errorf("tool %q not found in catalog", name)}
}

// levenshtein computes the full edit distance between a and b using a
// rolling two-row dynamic-programming table. It does no early-exit; the
// inputs are tool names (≤32 chars in practice) so the cost is trivial.
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

// renderInfoText writes the human-readable view of report to stderr.
// Warnings are appended at the end of the output (after the structured
// detail blocks) so they don't push the headline metadata off-screen
// in a small terminal — the layout matches the TUI detail page.
func renderInfoText(r infoReport, t *registry.Tool) {
	// Per docs/cli-conventions.md, human-readable prose belongs on
	// stderr so that stdout stays free for pipe-friendly machine output
	// (e.g. `clim info foo --output json | jq`). `clim info` is the
	// only command in the codebase that previously wrote rendered text
	// to stdout — this aligns it with `clim list`, `clim why`, etc.
	w := os.Stderr

	// Header line.
	header := r.DisplayName
	if header == "" {
		header = r.Name
	}
	if r.Category != "" {
		header += "  (" + r.Category + ")"
	}
	if r.GitHub != nil && r.GitHub.Stars > 0 {
		header += "  ★ " + githubfmt.FormatStars(r.GitHub.Stars)
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
			stats = append(stats, "★ "+githubfmt.FormatStars(r.GitHub.Stars)+" stars")
		}
		if r.GitHub.Forks > 0 {
			stats = append(stats, "⑂ "+githubfmt.FormatStars(r.GitHub.Forks)+" forks")
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
			if d := githubfmt.FormatDate(r.GitHub.LastPush); d != "" {
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

// formatInfoRef renders a Reference as a single human-readable line for
// the text output of `clim info`. Both required and optional refs may
// carry a version constraint — neither role drops it.
func formatInfoRef(ref infoReference) string {
	switch ref.Kind {
	case "teamfile":
		role := "optional"
		if ref.Required {
			role = "required"
		}
		if ref.Constraint != "" {
			role += " " + ref.Constraint
		}
		return fmt.Sprintf(".clim.yaml (%s) — %s", role, ref.Path)
	case "project":
		role := "optional"
		if ref.Required {
			role = "required"
		}
		if ref.Constraint != "" {
			role += " " + ref.Constraint
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

// formatStarCount and formatGitHubDate moved to internal/githubfmt so the
// TUI detail view and `clim info` share one implementation. Tests for
// the contract live in `internal/githubfmt/githubfmt_test.go`.

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
