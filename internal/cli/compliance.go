package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/compliance"
	"github.com/nassiharel/clim/internal/progress"
)

var complianceCmd = &cobra.Command{
	Use:   "compliance",
	Short: "Validate tools against company compliance policy",
	Long: `Check installed tools against a compliance policy file (.clim-policy.yaml).

The policy defines which tools are allowed, blocked, required, and which
install sources and licenses are permitted.

Configure the policy path in config.yaml:
  compliance:
    policy: /path/to/.clim-policy.yaml

Or pass it directly:
  clim compliance check --policy .clim-policy.yaml`,
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
	Short: "Generate a sample .clim-policy.yaml",
	RunE:  runComplianceInit,
}

func init() {
	complianceCheckCmd.Flags().StringVar(&compliancePolicyFlag, "policy", "", "Path to policy file (overrides config)")
	complianceCheckCmd.Flags().BoolVar(&complianceJSONFlag, "json", false, "Output results as JSON")
	complianceCheckCmd.Flags().BoolVar(&complianceRefreshFlag, "refresh", false, "Force fresh scan")
	complianceShowCmd.Flags().StringVar(&compliancePolicyFlag, "policy", "", "Path to policy file (overrides config)")
	complianceInitCmd.Flags().StringVar(&compliancePolicyFlag, "policy", "", "Output path (default: .clim-policy.yaml)")

	complianceCmd.AddCommand(complianceCheckCmd)
	complianceCmd.AddCommand(complianceShowCmd)
	complianceCmd.AddCommand(complianceInitCmd)
	// Registered in root.go with command group.
}

func resolvePolicyPath() (string, error) {
	if compliancePolicyFlag != "" {
		return compliancePolicyFlag, nil
	}
	if cfg.Compliance.Policy != "" {
		return cfg.Compliance.Policy, nil
	}
	// Try default locations.
	for _, candidate := range []string{".clim-policy.yaml", ".clim-policy.yml"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no policy file found\n\nConfigure in config.yaml:\n  compliance:\n    policy: /path/to/.clim-policy.yaml\n\nOr pass directly:\n  clim compliance check --policy .clim-policy.yaml\n\nOr generate one:\n  clim compliance init")
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
	fmt.Fprintln(w, "  STATUS\tTOOL\tRULE\tMESSAGE")
	fmt.Fprintln(w, "  ------\t----\t----\t-------")
	for _, v := range result.Violations {
		icon := "⚠"
		if v.Severity == "error" {
			icon = "✗"
		}
		fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", icon, v.Tool, v.Rule, v.Message)
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
	path := ".clim-policy.yaml"
	if compliancePolicyFlag != "" {
		path = compliancePolicyFlag
	}

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}

	sample := `# clim Compliance Policy
# Place this file in your repo root or configure in config.yaml:
#   compliance:
#     policy: /path/to/.clim-policy.yaml

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
