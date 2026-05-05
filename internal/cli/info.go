package cli

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/audit"
	"github.com/nassiharel/clim/internal/githubfmt"
	"github.com/nassiharel/clim/internal/progress"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/security"
	"github.com/nassiharel/clim/internal/textwrap"
	"github.com/nassiharel/clim/internal/vuln"
)

var (
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
  clim info terraform --output json # machine-readable for scripts`,
	Args: requireArgs(1, "clim info <tool>"),
	RunE: runInfo,
}

func init() {
	// `clim info` does a fresh single-tool scan on every invocation
	// (ScanOnly + RefreshTool — see runInfo), so there is no scan
	// cache to bypass. We deliberately do NOT expose a --refresh
	// flag: it would have no effect, and accepting one as a no-op
	// would lie to scripts that pass it expecting a behavior change.
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
//
// Topics is intentionally NOT tagged omitempty so it always serializes
// as an array (`[]` when there are no topics). The PR-advertised
// contract says collection fields render as arrays so consumers can
// safely iterate without nil-checking. Other fields stay omitempty
// since they're scalars.
type infoGitHub struct {
	Slug        string   `json:"slug,omitempty"`
	URL         string   `json:"url,omitempty"`
	Stars       int      `json:"stars,omitempty"`
	Forks       int      `json:"forks,omitempty"`
	Description string   `json:"description,omitempty"`
	Homepage    string   `json:"homepage,omitempty"`
	License     string   `json:"license,omitempty"`
	Topics      []string `json:"topics"`
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
	Security        *infoSecurity  `json:"security,omitempty"`
	References      []Reference    `json:"references"`
	RelatedTools    []string       `json:"related_tools"`
	Warnings        []string       `json:"warnings"`
}

// infoSecurity surfaces the current security verdict in `clim info`.
// Vulnerability data comes from the local cache only — running
// `clim info` should never make a network call. Encourage the user
// to run `clim security vuln` for fresh data when Vulns is nil.
type infoSecurity struct {
	Status      string   `json:"status"`            // clean/watch/risk/unknown
	Reasons     []string `json:"reasons,omitempty"` // human-readable contributors
	VulnsLoaded bool     `json:"vulns_loaded"`      // true when cache had data for this tool
}

func runInfo(cmd *cobra.Command, args []string) error {
	out, err := infoOutputFmt()
	if err != nil {
		return err
	}
	toolName := args[0]

	// Three phases:
	//   1. LoadTools — catalog-only lookup, no PATH I/O at all. A typo
	//      now exits without scanning the user's environment.
	//   2. ScanOnly  — full PATH walk only after the name is valid;
	//      we still need it to populate r.RelatedTools (which compares
	//      every catalog tool's installed status by tag/category).
	//   3. RefreshTool — single-tool PATH check + version resolution
	//      for the requested tool only.
	svc := svcFrom(cmd)
	sp := progress.New("Resolving tool info...")
	tools, _, err := svc.Catalog.LoadTools(cmd.Context())
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	if _, ok := registry.ToolMap(tools)[toolName]; !ok {
		// Mark the spinner failed before returning the typo error so
		// users don't see a misleading "✓ Done" right above an
		// "Error: tool ... not found" line.
		sp.Fail(fmt.Sprintf("tool %q not found", toolName))
		return notFoundError(toolName, closestToolName(tools, toolName))
	}

	// Name is valid — do the PATH scan. ScanOnly populates IsInstalled
	// across the catalog so RelatedTools can filter to installed
	// neighbors; we need this even for the single-tool detail page.
	tools, _, err = svc.ScanOnly(cmd.Context())
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	// The catalog may have changed between LoadTools and ScanOnly
	// (auto-refresh, extra-marketplace fetch, etc.). If the requested
	// tool no longer appears, surface the same UsageError we'd return
	// on the typo path rather than dereferencing a nil pointer.
	t, ok := registry.ToolMap(tools)[toolName]
	if !ok {
		sp.Fail(fmt.Sprintf("tool %q not found", toolName))
		return notFoundError(toolName, closestToolName(tools, toolName))
	}

	// Resolve only the requested tool's versions. RefreshTool runs an
	// extra single-tool Finder.FindAll first; if the tool turns out
	// to be installed, it then spawns the package-manager queries
	// for THIS tool only. The big perf win versus LoadAndResolveCached
	// is skipping catalog-wide version resolution.
	resolved := svc.RefreshTool(cmd.Context(), *t)
	*t = resolved
	sp.Stop()

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
		// Use the shared githubfmt.RepoURL helper so the slug is
		// trimmed/normalized exactly the same way the TUI does it
		// (extra-marketplace YAML can carry padded slugs that the
		// runtime parser doesn't normalize). Slug stays as the raw
		// value so consumers can see what the catalog actually wrote.
		gh := &infoGitHub{
			Slug:   t.GitHubSlug,
			URL:    githubfmt.RepoURL(t.GitHubSlug),
			Topics: []string{},
		}
		if t.GitHubInfo != nil {
			gh.Stars = t.GitHubInfo.Stars
			gh.Forks = t.GitHubInfo.Forks
			gh.Description = t.GitHubInfo.Description
			gh.Homepage = t.GitHubInfo.Homepage
			gh.License = t.GitHubInfo.License
			gh.Topics = append(gh.Topics, t.GitHubInfo.Topics...)
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
	r.Security = computeInfoSecurity(*t, tools)
	return r
}

// computeInfoSecurity builds the per-tool security verdict for the
// info report. Audit signals are computed from the in-memory tool
// list (no I/O); vulnerability data is read from the cache only —
// `clim info` never makes a network call. Returns nil when the tool
// isn't installed (security verdict only makes sense for installed
// tools).
func computeInfoSecurity(t registry.Tool, allTools []registry.Tool) *infoSecurity {
	if !t.IsInstalled() {
		return nil
	}
	findings, _ := audit.Analyze(allTools)
	// Read the vuln cache passively — `clim info` never makes a
	// network call. The cache key (resolved via ResolveVulnSourceKey)
	// matches whatever URL `clim security vuln` writes under, so the
	// CLI, web view, and TUI all see the same data. If the cache is
	// empty, the verdict still works (just with no vuln signal) and
	// the caller renders a "run clim security vuln" hint.
	var match *vuln.Match
	loaded := false
	if rep, ok := vuln.ReadCache(ResolveVulnSourceKey()); ok {
		loaded = true
		for i := range rep.Matches {
			if rep.Matches[i].Tool == t.Name {
				match = &rep.Matches[i]
				break
			}
		}
	}
	verdict := security.Score(t, findings, match)
	return &infoSecurity{
		Status:      verdict.Status.String(),
		Reasons:     verdict.Reasons,
		VulnsLoaded: loaded,
	}
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

	// Security verdict (computed from audit + cached vuln data; no
	// network I/O at info time).
	if r.Security != nil {
		_, _ = fmt.Fprintln(w, "  Security:")
		_, _ = fmt.Fprintf(w, "    Status: %s\n", r.Security.Status)
		for _, reason := range r.Security.Reasons {
			_, _ = fmt.Fprintf(w, "    · %s\n", reason)
		}
		if !r.Security.VulnsLoaded {
			_, _ = fmt.Fprintf(w, "    Note: vulnerability cache empty — run `clim security vuln` for CVE/GHSA data\n")
		}
		_, _ = fmt.Fprintln(w, "")
	}

	// References.
	if len(r.References) > 0 {
		_, _ = fmt.Fprintln(w, "  Referenced by:")
		for _, ref := range r.References {
			_, _ = fmt.Fprintf(w, "    • %s\n", FormatReference(ref))
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

// formatInfoRef and formatWhyRef have been replaced by the shared
// cli.FormatReference helper in refscan.go. Both `clim info` and
// `clim why` call FormatReference directly so any new Reference.Kind
// or wording change is a one-place edit.

// Star-count and date formatting live in internal/githubfmt so the TUI
// detail view and `clim info` share one implementation. Tests for the
// contract live in internal/githubfmt/githubfmt_test.go.

// wordWrapStr delegates to internal/textwrap.Wrap so `clim info` and
// the TUI detail view wrap GitHub descriptions identically with full
// display-width awareness (CJK / emoji / combining characters). The
// previous local implementation measured raw bytes and would mis-wrap
// non-ASCII content.
func wordWrapStr(s string, width int) []string {
	return textwrap.Wrap(s, width)
}

// dashIfEmpty returns "—" when s is empty so tables don't show empty cells.
func dashIfEmpty(s string) string {
	if s == "" {
		return "—"
	}
	return s
}
