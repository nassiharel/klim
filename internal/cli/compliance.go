package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/compliance"
	"github.com/nassiharel/clim/internal/fileutil"
	"github.com/nassiharel/clim/internal/paths"
	"github.com/nassiharel/clim/internal/progress"
)

var complianceCmd = &cobra.Command{
	Use:   "compliance",
	Short: "Validate tools against company compliance policy",
	Long: `Check installed tools against a compliance policy file.

The policy defines which tools are allowed, blocked, required, and which
install sources and licenses are permitted.

The policy file is stored globally at ~/.config/clim/compliance/policy.yaml.
Configure a custom path in config.yaml:
  compliance:
    policy: /custom/path/to/policy.yaml

Or pass it directly:
  clim compliance check --policy /path/to/policy.yaml`,
}

var compliancePolicyFlag string
var complianceJSONFlag bool
var complianceRefreshFlag bool

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

func init() {
	complianceCheckCmd.Flags().StringVar(&compliancePolicyFlag, "policy", "", "Path to policy file (overrides config)")
	complianceCheckCmd.Flags().BoolVar(&complianceJSONFlag, "json", false, "Output results as JSON")
	complianceCheckCmd.Flags().BoolVar(&complianceRefreshFlag, "refresh", false, "Force fresh scan")
	complianceShowCmd.Flags().StringVar(&compliancePolicyFlag, "policy", "", "Path to policy file (overrides config)")
	complianceInitCmd.Flags().StringVar(&compliancePolicyFlag, "policy", "", "Output path (default: ~/.config/clim/compliance/policy.yaml)")

	complianceCmd.AddCommand(complianceCheckCmd)
	complianceCmd.AddCommand(complianceShowCmd)
	complianceCmd.AddCommand(complianceInitCmd)
	// Registered in root.go with command group.
}

func resolvePolicyPath() (string, error) {
	if compliancePolicyFlag != "" {
		return compliancePolicyFlag, nil
	}
	path := findPolicyPath()
	if path != "" {
		return path, nil
	}
	return "", errors.New("no policy file found\n\nGenerate one:\n  clim compliance init\n\nOr configure a custom path in config.yaml:\n  compliance:\n    policy: /path/to/policy.yaml\n\nOr pass directly:\n  clim compliance check --policy /path/to/policy.yaml")
}

// findPolicyPath returns the policy file path from config or default
// global location, or empty string if none exists. Shared across commands.
func findPolicyPath() string {
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
	policyPath, err := resolvePolicyPath()
	if err != nil {
		return err
	}

	policy, err := compliance.LoadPolicy(policyPath)
	if err != nil {
		return err
	}

	sp := progress.New("Scanning tools...")
	tools, _, _, err := svc.LoadAndResolveCached(cmd.Context(), complianceRefreshFlag)
	if err != nil {
		sp.Fail(err.Error())
		return err
	}
	sp.Done("Done")

	result := compliance.Check(policy, tools)

	if complianceJSONFlag {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
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
	policyPath, err := resolvePolicyPath()
	if err != nil {
		return err
	}

	policy, err := compliance.LoadPolicy(policyPath)
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
# Stored at: ~/.config/clim/compliance/policy.yaml
# Override in config.yaml:
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
	fmt.Fprintln(os.Stderr, "  clim compliance check")
	return nil
}

func join(items []string) string {
	return strings.Join(items, ", ")
}
