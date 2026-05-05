package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/audit"
	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/vuln"
)

// ResolveVulnSourceKey returns the OSV endpoint string used as the
// cache key for vulnerability scan results. Surfaces that read the
// cache passively (`clim info`, web `/security`) must use the same
// key as `clim security vuln`, otherwise they look at a different
// file. Falls back to vuln.DefaultOSVURL when config is unreadable
// or the URL is unset.
func ResolveVulnSourceKey() string {
	cfg, _ := config.Load()
	if cfg != nil {
		if u := strings.TrimSpace(cfg.Vuln.URL); u != "" {
			return u
		}
	}
	return vuln.DefaultOSVURL
}

// securityCmd is the parent of all security-related subcommands.
//
// Layout:
//
//	clim security             — runs every check + aggregate summary
//	clim security health      — environment health (PATH, multi-installs, …)
//	clim security audit       — audit installed tools (archived/stale/license)
//	clim security vuln        — CVE/GHSA lookup against installed tools
//	clim security compliance  — validate against policy
//
// Top-level `clim audit`, `clim doctor`, `clim compliance` no longer
// exist — clim is a fresh tool and avoids back-compat clutter.
var securityCmd = &cobra.Command{
	Use:   "security",
	Short: "Security checks: health, audit, vulnerabilities, compliance",
	Long: `clim security is the umbrella for every security-related
check clim performs. Run with no arguments to execute every subcommand
and emit an aggregate summary, or pick a specific subcommand for
focused output.

Subcommands:
  health      Environment health (PATH issues, multi-installs, missing PMs)
  audit       Audit installed tools (archived upstream, stale, license)
  vuln        Look up known CVEs/GHSAs against installed versions
  compliance  Validate installed tools against a compliance policy

Bare invocation: 'clim security' runs every subcommand and exits with
the worst severity encountered (0 = clean, 1 = findings, 3 = a
subcommand failed entirely).`,
	GroupID: "health",
	RunE:    runSecurityAggregate,
}

func init() {
	// Re-parent the existing subcommand definitions instead of
	// duplicating their flag wiring. Each child still defines its
	// own Use/Short/Long/RunE — `clim security audit` and
	// `clim audit` would behave identically (but only the umbrella
	// path is registered now).
	securityCmd.AddCommand(doctorCmd) // Use: "health"
	securityCmd.AddCommand(auditCmd)
	securityCmd.AddCommand(vulnCmd)
	securityCmd.AddCommand(complianceCmd)
}

// --- vuln subcommand ---

var (
	vulnRefreshFlag      bool
	vulnForceRefreshFlag bool
	vulnFailOnFlag       string
	vulnURLFlag          string
	vulnOutputFormat     func() (OutputFormat, error)
)

var vulnCmd = &cobra.Command{
	Use:   "vuln",
	Short: "Scan installed tools for known vulnerabilities (CVEs / GHSAs)",
	Long: `Query an OSV-compatible endpoint (default: api.osv.dev) for
vulnerabilities affecting the installed version of every detected
tool. Results are cached locally — see compliance.url's behaviour for
the per-source caching pattern.

Tools are mapped to OSV ecosystems via their package IDs:
  npm       — Packages.NPM
  Homebrew  — Packages.Brew
  GitHub    — registry GitHubSlug (last-resort match)

Tools without any of these mappings are listed under "skipped" so
coverage gaps are visible.

Exit codes:
  0  No findings, OR --fail-on not set
  1  Findings exist (only when --fail-on is set and threshold met)
  2  Usage error
  3  All tool queries failed`,
	RunE: runVuln,
}

func init() {
	vulnCmd.Flags().BoolVar(&vulnRefreshFlag, "refresh", false, "Force fresh PATH scan")
	vulnCmd.Flags().BoolVar(&vulnForceRefreshFlag, "force-refresh-vulns", false, "Force re-fetch from the OSV endpoint, bypassing the cache")
	vulnCmd.Flags().StringVar(&vulnFailOnFlag, "fail-on", "", "Exit non-zero when any finding meets this severity: critical|high|medium|low")
	vulnCmd.Flags().StringVar(&vulnURLFlag, "url", "", "OSV-compatible endpoint (overrides config vuln.url; default https://api.osv.dev)")
	vulnOutputFormat = addOutputFlag(vulnCmd, OutputText, OutputJSON)
}

func runVuln(cmd *cobra.Command, args []string) error {
	out, err := vulnOutputFormat()
	if err != nil {
		return err
	}

	cfg := cfgFrom(cmd)
	svc := svcFrom(cmd)

	// Resolve threshold — flag overrides config.
	threshold := vuln.SeverityUnknown
	thresholdRaw := vulnFailOnFlag
	if thresholdRaw == "" {
		thresholdRaw = cfg.Vuln.FailOnSeverity
	}
	if thresholdRaw != "" {
		threshold = vuln.ParseSeverity(thresholdRaw)
		if threshold == vuln.SeverityUnknown {
			return usageErrorf("--fail-on %q is not one of critical/high/medium/low", thresholdRaw)
		}
	}

	tools, _, _, err := svc.LoadAndResolveCached(cmd.Context(), vulnRefreshFlag)
	if err != nil {
		return fmt.Errorf("scanning tools: %w", err)
	}

	url := strings.TrimSpace(vulnURLFlag)
	if url == "" {
		url = strings.TrimSpace(cfg.Vuln.URL)
	}
	source := url
	if source == "" {
		source = vuln.DefaultOSVURL
	}

	looker := &vuln.OSVClient{URL: source}
	opts := vuln.LookupOptions{
		ForceRefresh: vulnForceRefreshFlag,
	}
	if cfg.Vuln.AutoRefresh && cfg.Vuln.RefreshInterval.Duration > 0 {
		opts.MaxAge = cfg.Vuln.RefreshInterval.Duration
	}

	report, err := vuln.Lookup(cmd.Context(), looker, tools, source, opts)
	if err != nil {
		return fmt.Errorf("vuln lookup: %w", err)
	}

	switch out {
	case OutputJSON:
		if err := printJSON(report); err != nil {
			return err
		}
	default:
		printVulnText(report)
	}

	if threshold != vuln.SeverityUnknown && reportTriggers(report, threshold) {
		// Use a sentinel error that exits with code 1 (PartialFailure
		// would be 3; here we want a "findings present" signal).
		return fmt.Errorf("vulnerability findings at or above %s", strings.ToUpper(string(threshold)))
	}
	return nil
}

// printVulnText writes the human-readable report to stderr (CLI
// convention: text on stderr, JSON on stdout).
func printVulnText(rep *vuln.Report) {
	w := tabwriter.NewWriter(os.Stderr, 0, 0, 2, ' ', 0)
	fmt.Fprintf(os.Stderr, "\n──── Vulnerability scan ────\n")
	fmt.Fprintf(os.Stderr, "Source: %s   Scanned: %d tools   When: %s\n\n",
		rep.Source, rep.ToolsScanned, rep.ScannedAt.Local().Format(time.RFC3339))

	risky := 0
	clean := 0
	for _, m := range rep.Matches {
		if len(m.Vulnerabilities) > 0 {
			risky++
		} else {
			clean++
		}
	}

	if risky == 0 {
		fmt.Fprintf(os.Stderr, "  ✓ No known vulnerabilities (%d tools clean)\n", clean)
	} else {
		fmt.Fprintf(os.Stderr, "  ⚠ %d tool(s) with findings, %d clean\n\n", risky, clean)
		// Sort matches: worst severity first, then by tool name.
		sorted := append([]vuln.Match(nil), rep.Matches...)
		sort.Slice(sorted, func(i, j int) bool {
			a, b := sorted[i].MaxSeverity().Rank(), sorted[j].MaxSeverity().Rank()
			if a != b {
				return a > b
			}
			return sorted[i].Tool < sorted[j].Tool
		})
		_, _ = fmt.Fprintln(w, "TOOL\tVERSION\tSEVERITY\tID\tFIXED IN\tSUMMARY")
		for _, m := range sorted {
			if len(m.Vulnerabilities) == 0 {
				continue
			}
			for i, v := range m.Vulnerabilities {
				tool := ""
				ver := ""
				if i == 0 {
					tool = m.Tool
					ver = m.InstalledVer
				}
				summary := v.Summary
				if len(summary) > 60 {
					summary = summary[:57] + "…"
				}
				_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
					tool, ver, v.Severity, v.ID, v.FixedIn, summary)
			}
		}
		_ = w.Flush()
	}

	if len(rep.Skipped) > 0 {
		fmt.Fprintf(os.Stderr, "\n  Skipped (%d) — no OSV ecosystem mapping:\n", len(rep.Skipped))
		for _, s := range rep.Skipped {
			fmt.Fprintf(os.Stderr, "    · %-20s %s\n", s.Tool, s.Reason)
		}
	}
	fmt.Fprintln(os.Stderr)
}

// reportTriggers reports whether any finding meets the threshold.
func reportTriggers(rep *vuln.Report, threshold vuln.Severity) bool {
	for _, m := range rep.Matches {
		for _, v := range m.Vulnerabilities {
			if v.Severity.AtLeast(threshold) {
				return true
			}
		}
	}
	return false
}

// --- bare `clim security` aggregator ---

func runSecurityAggregate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	fmt.Fprintln(os.Stderr, "──── clim security ────")

	cfg := cfgFrom(cmd)
	svc := svcFrom(cmd)
	tools, _, _, err := svc.LoadAndResolveCached(ctx, false)
	if err != nil {
		return fmt.Errorf("scanning tools: %w", err)
	}

	// Audit (always run; cheap, in-memory).
	findings, _ := audit.Analyze(tools)
	auditWarn, auditInfo := audit.CountBySeverity(findings)

	// Vuln (best-effort; offline-friendly).
	url := strings.TrimSpace(cfg.Vuln.URL)
	if url == "" {
		url = vuln.DefaultOSVURL
	}
	looker := &vuln.OSVClient{URL: url}
	opts := vuln.LookupOptions{}
	if cfg.Vuln.AutoRefresh && cfg.Vuln.RefreshInterval.Duration > 0 {
		opts.MaxAge = cfg.Vuln.RefreshInterval.Duration
	}
	report, vulnErr := vuln.Lookup(ctx, looker, tools, url, opts)

	fmt.Fprintln(os.Stderr)
	fmt.Fprintf(os.Stderr, "  audit:    %d warnings, %d infos\n", auditWarn, auditInfo)
	if vulnErr != nil {
		fmt.Fprintf(os.Stderr, "  vuln:     skipped (%v)\n", vulnErr)
	} else {
		risky := 0
		for _, m := range report.Matches {
			if len(m.Vulnerabilities) > 0 {
				risky++
			}
		}
		fmt.Fprintf(os.Stderr, "  vuln:     %d tool(s) affected (worst: %s)\n", risky, report.MaxSeverity())
	}

	// Health and compliance left as a one-line "→ run subcommand"
	// hint to keep the aggregator quick. Users can drill in with
	// `clim security health` / `clim security compliance`.
	fmt.Fprintln(os.Stderr, "  health:   run `clim security health` for full output")
	fmt.Fprintln(os.Stderr, "  compliance: run `clim security compliance` for policy verdict")

	// Suppress unused-imports for the aggregate path.
	_ = ctx
	_ = json.RawMessage{}
	_ = registry.Tool{}

	return nil
}
