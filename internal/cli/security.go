package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/audit"
	"github.com/nassiharel/klim/internal/config"
	"github.com/nassiharel/klim/internal/vuln"
)

// ResolveVulnSourceKey returns the OSV endpoint string used as the
// cache key for vulnerability scan results. Surfaces that read the
// cache passively (`klim info`, web `/security`) must use the same
// key as `klim security vuln`, otherwise they look at a different
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
//	klim security             — quick aggregate summary (audit + vuln)
//	klim security audit       — audit installed tools (archived/stale/license)
//	klim security vuln        — CVE/GHSA lookup against installed tools
//	klim security compliance  — validate against policy
//
// Environment health (PATH conflicts, multi-installs, etc.) lives under
// the top-level `klim health` command, not here.
var securityCmd = &cobra.Command{
	Use:   "security",
	Short: "Security checks: audit, vulnerabilities, compliance",
	Long: `klim security is the umbrella for every security-related
check klim performs. Run with no arguments to print a quick summary
across the cheap subset (audit + vuln); for full output of any one
check, run the matching subcommand directly.

Subcommands:
  audit       Audit installed tools (archived upstream, stale, license)
  vuln        Look up known CVEs/GHSAs against installed versions
  compliance  Validate installed tools against a compliance policy

For environment health (PATH issues, multi-installs, missing PMs)
use the top-level 'klim health' command.

Exit codes (bare 'klim security'):
  0  No audit warnings, no vulns
  1  Vuln lookup hard-failed (network, OSV down, etc.)
  3  Audit warnings or vulnerability findings present`,
	GroupID: "health",
	RunE:    runSecurityAggregate,
}

func init() {
	// `klim security health` is gone — Health is now a top-level
	// command (see health.go / root.go). Audit, vuln, and compliance
	// stay under security where they belong.
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
tool. Results are cached locally per-source so switching vuln.url to
a different mirror doesn't reuse another source's data.

Coverage today is npm only — OSV.dev rejects ecosystem="Homebrew" and
ecosystem="GitHub" with HTTP 400, so brew-only tools and tools known
only by GitHub slug land in the "skipped" list with a clear reason.
Adding PyPI/Go/crates is unblocked by future catalog metadata work.

Exit codes:
  0  No findings, OR --fail-on not set / threshold not met
  1  Vuln lookup hard-failed (network, OSV down, etc.)
  2  Usage error (bad --fail-on / --output value)
  3  Findings present at or above --fail-on threshold`,
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
		// Findings at or above threshold → ExitPartialFailure (3):
		// the scan succeeded as a whole, but some items failed the
		// gate. This matches the documented exit code contract and
		// makes CI gating predictable across `klim security vuln`
		// and `klim security`.
		risky := 0
		clean := 0
		for _, m := range report.Matches {
			if len(m.Vulnerabilities) > 0 {
				risky++
			} else {
				clean++
			}
		}
		return &PartialFailureError{
			Op:        "vulnerability scan",
			Succeeded: clean,
			Failed:    risky,
		}
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

// --- bare `klim security` aggregator ---

// runSecurityAggregate runs the cheap subset of security checks
// (audit + vuln) and prints a one-paragraph summary. Returns a
// non-nil error when there's something the user should act on:
// audit warnings, vulnerabilities at any severity, or a hard
// failure of the vuln lookup. The umbrella exit code lets CI gate
// on `klim security` directly.
func runSecurityAggregate(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()
	fmt.Fprintln(os.Stderr, "──── klim security ────")

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
	risky := 0
	if vulnErr != nil {
		fmt.Fprintf(os.Stderr, "  vuln:     lookup failed (%v)\n", vulnErr)
	} else {
		for _, m := range report.Matches {
			if len(m.Vulnerabilities) > 0 {
				risky++
			}
		}
		fmt.Fprintf(os.Stderr, "  vuln:     %d tool(s) affected (worst: %s)\n", risky, report.MaxSeverity())
	}

	// Health and compliance left as a one-line "→ run subcommand"
	// hint to keep the aggregator quick. Users can drill in with
	// `klim security health` / `klim security compliance`.
	fmt.Fprintln(os.Stderr, "  health:   run `klim security health` for full output")
	fmt.Fprintln(os.Stderr, "  compliance: run `klim security compliance` for policy verdict")

	// Aggregate exit code (PartialFailure = 3, the documented contract):
	//   - vuln lookup hard failure → ExitRuntime (1) so CI fails closed
	//     (a transient OSV outage shouldn't claim "no vulns").
	//   - any vulns OR audit warnings → PartialFailure (3). Audit
	//     *infos* alone are observational and don't gate.
	if vulnErr != nil {
		return fmt.Errorf("vuln lookup failed: %w", vulnErr)
	}
	if risky > 0 || auditWarn > 0 {
		return &PartialFailureError{
			Op:        "security",
			Succeeded: 0, // we don't track per-check pass count here
			Failed:    risky + auditWarn,
		}
	}
	return nil
}
