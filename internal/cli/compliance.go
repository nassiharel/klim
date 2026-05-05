package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/compliance"
	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/fileutil"
	"github.com/nassiharel/clim/internal/paths"
	"github.com/nassiharel/clim/internal/progress"
	"github.com/nassiharel/clim/internal/vuln"
)

// loadVulnSeveritiesForCompliance reads the vuln cache (passive — no
// network) and returns a tool→severity map suitable for
// compliance.Check. Returns nil if no cache exists; the compliance
// gate then silently skips the vuln check (the operator can populate
// the cache via `clim security vuln`).
func loadVulnSeveritiesForCompliance() map[string]string {
	if rep, ok := vuln.ReadCache(ResolveVulnSourceKey()); ok {
		return rep.SeverityByTool()
	}
	return nil
}

var complianceCmd = &cobra.Command{
	Use:   "compliance",
	Short: "Validate tools against company compliance policy",
	Long: `Check installed tools against a compliance policy file.

The policy defines which tools are allowed, blocked, required, and which
install sources and licenses are permitted.

Policy resolution order (highest to lowest):
  1. --policy flag           (local file, per-invocation)
  2. --url flag              (remote URL, per-invocation; check & refresh)
  3. compliance.url          (remote URL in config.yaml, with cache + auto-refresh)
  4. compliance.policy       (local file in config.yaml)
  5. default global location (~/.config/clim/compliance/policy.yaml)

Subcommands:
  check    Validate installed tools against the policy.
  show     Show the resolved policy.
  init     Generate a sample .clim-policy.yaml.
  refresh  Force-refetch the policy from compliance.url (or --url) and update the cache.

Generate a default policy with: clim security compliance init`,
}

var compliancePolicyFlag string
var complianceURLFlag string
var complianceRefreshFlag bool
var complianceForceRefreshFlag bool
var complianceOutput func() (OutputFormat, error)

var complianceCheckCmd = &cobra.Command{
	Use:   "check",
	Short: "Check installed tools against compliance policy",
	RunE:  runComplianceCheck,
}

var complianceShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show the current compliance policy",
	RunE:  runComplianceShow,
}

var complianceInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate a sample compliance policy file",
	RunE:  runComplianceInit,
}

var complianceRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Force-refresh the compliance policy from its remote URL",
	Long: `Fetch the latest policy from the URL configured in compliance.url
and update the local cache. Fails if no URL is configured.`,
	RunE: runComplianceRefresh,
}

func init() {
	complianceCheckCmd.Flags().StringVar(&compliancePolicyFlag, "policy", "", "Path to policy file (overrides config)")
	complianceCheckCmd.Flags().StringVar(&complianceURLFlag, "url", "", "Remote policy URL (overrides config)")
	complianceOutput = addOutputFlag(complianceCheckCmd, OutputText, OutputJSON)
	complianceCheckCmd.Flags().BoolVar(&complianceRefreshFlag, "refresh", false, "Force fresh tool scan")
	complianceCheckCmd.Flags().BoolVar(&complianceForceRefreshFlag, "force-refresh-policy", false, "Force re-fetch policy from URL")
	complianceShowCmd.Flags().StringVar(&compliancePolicyFlag, "policy", "", "Path to policy file (overrides config)")
	complianceInitCmd.Flags().StringVar(&compliancePolicyFlag, "policy", "", "Output path (default: clim config directory)")
	// `refresh` reads complianceURLFlag too — register --url here so a
	// user can pass an ad-hoc remote without editing config.yaml first.
	complianceRefreshCmd.Flags().StringVar(&complianceURLFlag, "url", "", "Remote policy URL (overrides config)")

	complianceCmd.AddCommand(complianceCheckCmd)
	complianceCmd.AddCommand(complianceShowCmd)
	complianceCmd.AddCommand(complianceInitCmd)
	complianceCmd.AddCommand(complianceRefreshCmd)
	// Registered in root.go with command group.
}

// loadPolicyForCmd resolves and loads the policy. It uses the fetcher when a URL
// is configured, otherwise falls back to a local file. Returns (policy, source, err).
func loadPolicyForCmd(cmd *cobra.Command, forceRefresh bool) (*compliance.Policy, string, error) {
	cfg := cfgFrom(cmd)
	url := complianceURLFlag
	if url == "" {
		url = cfg.Compliance.URL
	}

	// Explicit --policy flag always wins (local file).
	if compliancePolicyFlag != "" {
		p, err := compliance.LoadPolicy(compliancePolicyFlag)
		return p, compliancePolicyFlag, err
	}

	// URL configured — fetch + cache.
	if url != "" {
		fetcher := &compliance.HTTPFetcher{URL: url}
		if forceRefresh {
			p, path, err := compliance.Refresh(cmd.Context(), fetcher)
			return p, path, err
		}
		opts := compliance.LoadOptions{}
		if cfg.Compliance.AutoRefresh && cfg.Compliance.RefreshInterval.Duration > 0 {
			opts.MaxAge = cfg.Compliance.RefreshInterval.Duration
		}
		p, path, err := compliance.LoadOrFetch(cmd.Context(), fetcher, opts)
		return p, path, err
	}

	// Fall back to local file.
	path := findPolicyPath(cfg)
	if path == "" {
		return nil, "", errors.New("no policy file or URL configured\n\nGenerate one:\n  clim security compliance init\n\nOr configure a URL in config.yaml:\n  compliance:\n    url: https://example.com/policy.yaml")
	}
	p, err := compliance.LoadPolicy(path)
	return p, path, err
}

func runComplianceRefresh(cmd *cobra.Command, args []string) error {
	cfg := cfgFrom(cmd)
	url := complianceURLFlag
	if url == "" {
		url = cfg.Compliance.URL
	}
	if url == "" {
		return errors.New("compliance.url not configured (set in config.yaml or pass --url)")
	}

	sp := progress.New("Fetching policy...")
	fetcher := &compliance.HTTPFetcher{URL: url}
	policy, path, err := compliance.Refresh(cmd.Context(), fetcher)
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	sp.Done(fmt.Sprintf("Policy refreshed (%s)", policy.Name))
	fmt.Fprintf(os.Stderr, "  Cache: %s\n", path)
	return nil
}

// findPolicyPath returns the policy file path from config or default
// global location, or empty string if none exists. Shared across commands.
func findPolicyPath(cfg *config.Config) string {
	if cfg.Compliance.Policy != "" {
		return cfg.Compliance.Policy
	}
	// Check default global location.
	if p, err := paths.CompliancePolicy(); err == nil {
		if _, statErr := os.Stat(p); statErr == nil {
			return p
		}
	}
	return ""
}

func runComplianceCheck(cmd *cobra.Command, args []string) error {
	out, err := complianceOutput()
	if err != nil {
		return err
	}

	policy, policyPath, err := loadPolicyForCmd(cmd, complianceForceRefreshFlag)
	if err != nil {
		return err
	}

	sp := progress.New("Scanning tools...")
	tools, _, _, err := svcFrom(cmd).LoadAndResolveCached(cmd.Context(), complianceRefreshFlag)
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	sp.Done("Done")

	result := compliance.Check(policy, tools, loadVulnSeveritiesForCompliance())

	if out == OutputJSON {
		if err := printJSON(result); err != nil {
			return err
		}
		if !result.Compliant {
			var errorCount int
			for _, v := range result.Violations {
				if v.Severity == "error" {
					errorCount++
				}
			}
			return fmt.Errorf("compliance check failed: %d error(s)", errorCount)
		}
		return nil
	}

	// Human output.
	fmt.Fprintf(os.Stderr, "\nPolicy: %s\n", result.PolicyName)
	fmt.Fprintf(os.Stderr, "Source: %s\n\n", policyPath)

	if len(result.Violations) == 0 {
		fmt.Fprintln(os.Stderr, "  ✓ All tools comply with policy!")
		return nil
	}

	w := tabwriter.NewWriter(os.Stderr, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "  STATUS\tTOOL\tRULE\tMESSAGE")
	_, _ = fmt.Fprintln(w, "  ------\t----\t----\t-------")
	for _, v := range result.Violations {
		icon := "⚠"
		if v.Severity == "error" {
			icon = "✗"
		}
		_, _ = fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", icon, v.Tool, v.Rule, v.Message)
	}
	_ = w.Flush()

	var errors, warnings int
	for _, v := range result.Violations {
		if v.Severity == "error" {
			errors++
		} else {
			warnings++
		}
	}
	fmt.Fprintf(os.Stderr, "\nResult: %d error(s), %d warning(s)\n", errors, warnings)

	if !result.Compliant {
		return fmt.Errorf("compliance check failed: %d violation(s)", errors)
	}
	return nil
}

func runComplianceShow(cmd *cobra.Command, args []string) error {
	policy, policyPath, err := loadPolicyForCmd(cmd, false)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "Policy: %s\n", policy.Name)
	if policy.Description != "" {
		fmt.Fprintf(os.Stderr, "  %s\n", policy.Description)
	}
	fmt.Fprintf(os.Stderr, "Source: %s\n\n", policyPath)

	if len(policy.AllowedSources) > 0 {
		fmt.Fprintf(os.Stderr, "Allowed sources: %s\n", join(policy.AllowedSources))
	}
	if len(policy.AllowedLicenses) > 0 {
		fmt.Fprintf(os.Stderr, "Allowed licenses: %s\n", join(policy.AllowedLicenses))
	}
	if len(policy.BlockedLicenses) > 0 {
		fmt.Fprintf(os.Stderr, "Blocked licenses: %s\n", join(policy.BlockedLicenses))
	}
	if len(policy.BlockedTools) > 0 {
		fmt.Fprintf(os.Stderr, "Blocked tools: %s\n", join(policy.BlockedTools))
	}
	if len(policy.RequiredTools) > 0 {
		fmt.Fprintf(os.Stderr, "Required tools:\n")
		for _, r := range policy.RequiredTools {
			ver := ""
			if r.Version != "" {
				ver = " " + r.Version
			}
			fmt.Fprintf(os.Stderr, "  - %s%s\n", r.Name, ver)
		}
	}
	return nil
}

func runComplianceInit(cmd *cobra.Command, args []string) error {
	path := compliancePolicyFlag
	if path == "" {
		p, err := paths.CompliancePolicy()
		if err != nil {
			return fmt.Errorf("resolving policy path: %w", err)
		}
		path = p
	}

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}

	sample := `# clim Compliance Policy
# Override location in config.yaml:
#   compliance:
#     policy: /custom/path/to/policy.yaml

name: "My Company Policy"
description: "Tool compliance rules for the engineering team"

# Only allow tools installed via these package managers
allowed_sources:
  - winget
  - brew
  - apt
  - scoop
  - npm

# Only allow these licenses (leave empty to allow all)
allowed_licenses:
  - MIT
  - Apache-2.0
  - BSD-2-Clause
  - BSD-3-Clause
  - ISC
  - MPL-2.0

# Block these licenses
blocked_licenses:
  - AGPL-3.0
  - GPL-3.0

# Explicitly blocked tools
blocked_tools: []

# Tools that must be installed
required_tools:
  - name: git
  - name: gh
    version: ">=2.40"
`

	if err := fileutil.EnsureDir(path); err != nil {
		return fmt.Errorf("creating policy directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(sample), 0644); err != nil {
		return fmt.Errorf("writing policy: %w", err)
	}
	fmt.Fprintf(os.Stderr, "✓ Policy file created: %s\n", path)
	fmt.Fprintln(os.Stderr, "  Edit it to match your company's requirements, then run:")
	fmt.Fprintln(os.Stderr, "  clim security compliance check")
	return nil
}

func join(items []string) string {
	return strings.Join(items, ", ")
}
