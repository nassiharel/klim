// Package compliance validates installed tools against a company policy file.
// Policies define allowed sources, licenses, blocked tools, and required tools.
package compliance

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/klim/internal/fileutil"
	"github.com/nassiharel/klim/internal/registry"
)

// SamplePolicyYAML is the starter policy template written by both the
// `klim security compliance init` CLI command and the TUI's compliance
// sub-tab. Keeping a single source prevents the two surfaces from
// drifting apart over time.
const SamplePolicyYAML = `# klim Compliance Policy
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

// WriteSamplePolicy writes the starter compliance policy at path. It
// refuses to overwrite an existing file so users never lose their
// customizations to a misclick. The parent directory is created
// on demand via fileutil.EnsureDir.
//
// Both the CLI (`klim security compliance init`) and the TUI's
// compliance sub-tab call this helper; if the template ever needs to
// change, edit SamplePolicyYAML and both surfaces stay in sync.
func WriteSamplePolicy(path string) error {
	if path == "" {
		return fmt.Errorf("compliance policy path must not be empty")
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists", path)
	}
	if err := fileutil.EnsureDir(path); err != nil {
		return fmt.Errorf("creating policy directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(SamplePolicyYAML), 0o644); err != nil {
		return fmt.Errorf("writing policy: %w", err)
	}
	return nil
}

// Policy defines a company's tool compliance rules.
type Policy struct {
	Name            string         `yaml:"name"`
	Description     string         `yaml:"description,omitempty"`
	AllowedSources  []string       `yaml:"allowed_sources,omitempty"`
	AllowedLicenses []string       `yaml:"allowed_licenses,omitempty"`
	BlockedTools    []string       `yaml:"blocked_tools,omitempty"`
	BlockedLicenses []string       `yaml:"blocked_licenses,omitempty"`
	RequiredTools   []RequiredTool `yaml:"required_tools,omitempty"`
	// MaxVulnSeverity, when set, makes Index.CanInstall return false
	// for any tool whose vuln scan found a CVE/GHSA at or above this
	// severity. Values: "low" / "medium" / "high" / "critical".
	// Empty (default) means vuln findings don't block installs (they
	// still surface in the security badge and `klim security vuln`).
	MaxVulnSeverity string `yaml:"max_vuln_severity,omitempty"`
}

// RequiredTool defines a tool that must be installed.
type RequiredTool struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version,omitempty"` // e.g. ">=2.50"
}

// Violation represents a single compliance issue.
type Violation struct {
	Severity string `json:"severity"` // "error", "warning"
	Tool     string `json:"tool"`
	Rule     string `json:"rule"`
	Message  string `json:"message"`
}

// Result holds the outcome of a compliance check.
type Result struct {
	PolicyName string      `json:"policy_name"`
	Violations []Violation `json:"violations"`
	Compliant  bool        `json:"compliant"`
}

// maxPolicySize limits the policy file to prevent memory exhaustion.
const maxPolicySize = 1 << 20 // 1 MB

// LoadPolicy reads and parses a policy file.
func LoadPolicy(path string) (*Policy, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("reading policy %s: %w", path, err)
	}
	if info.Size() > maxPolicySize {
		return nil, fmt.Errorf("policy file %s too large (%d bytes, max %d)", path, info.Size(), maxPolicySize)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading policy %s: %w", path, err)
	}
	p, err := parsePolicy(data)
	if err != nil {
		return nil, fmt.Errorf("policy %s: %w", path, err)
	}
	return p, nil
}

// parsePolicy parses YAML bytes into a Policy.
func parsePolicy(data []byte) (*Policy, error) {
	var p Policy
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing policy: %w", err)
	}
	if p.Name == "" {
		p.Name = "Unnamed Policy"
	}
	return &p, nil
}

// Check validates installed tools against a policy and returns violations.
// vulnSeverityByTool, when non-nil, supplies the worst known vuln
// severity per tool (typically read from the vuln cache); tools whose
// severity meets policy.MaxVulnSeverity get a "vulnerable" violation.
// Pass nil to skip the vuln-severity gate (useful from tests or when
// the cache hasn't been populated).
func Check(policy *Policy, tools []registry.Tool, vulnSeverityByTool map[string]string) Result {
	result := Result{
		PolicyName: policy.Name,
		Compliant:  true,
	}

	blockedSet := toSet(policy.BlockedTools)
	allowedSourceSet := toSet(policy.AllowedSources)
	allowedLicenseSet := toSet(policy.AllowedLicenses)
	blockedLicenseSet := toSet(policy.BlockedLicenses)

	threshold := strings.TrimSpace(policy.MaxVulnSeverity)
	thresholdRank := severityRank(threshold)
	if threshold != "" && thresholdRank == 0 {
		// Surface bad threshold values as a load-time issue rather
		// than silently ignoring them. Add an info violation so the
		// operator notices the policy isn't doing what they think.
		result.Violations = append(result.Violations, Violation{
			Tool:     "_policy_",
			Rule:     "invalid_max_vuln_severity",
			Message:  "max_vuln_severity=" + threshold + " is not one of low/medium/high/critical — vuln gate disabled",
			Severity: "warning",
		})
	}

	toolMap := make(map[string]*registry.Tool)
	for i := range tools {
		toolMap[strings.ToLower(tools[i].Name)] = &tools[i]
	}

	// Check each installed tool.
	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		primary := t.PrimaryInstance()
		if primary == nil {
			continue
		}

		// Blocked tools.
		if blockedSet[strings.ToLower(t.Name)] {
			result.addError(t.Name, "blocked_tool",
				t.DisplayName+" is blocked by company policy")
		}

		// Allowed sources.
		if len(allowedSourceSet) > 0 && !allowedSourceSet[strings.ToLower(string(primary.Source))] {
			result.addError(t.Name, "disallowed_source",
				fmt.Sprintf("Installed via %s — only %s allowed", primary.Source, strings.Join(policy.AllowedSources, ", ")))
		}

		// License checks.
		license := ""
		if t.GitHubInfo != nil {
			license = t.GitHubInfo.License
		}
		if license != "" {
			if len(allowedLicenseSet) > 0 && !allowedLicenseSet[strings.ToLower(license)] {
				result.addWarning(t.Name, "disallowed_license",
					fmt.Sprintf("License %q is not in the approved list", license))
			}
			if blockedLicenseSet[strings.ToLower(license)] {
				result.addError(t.Name, "blocked_license",
					fmt.Sprintf("License %q is blocked by company policy", license))
			}
		}

		// Vuln-severity gate. Only runs when the policy has a valid
		// threshold AND the caller supplied a severity map (typically
		// read from the local vuln cache). If the cache hasn't been
		// populated, this gate is silently skipped — that's a feature:
		// `klim install` shouldn't refuse to run because the user
		// hasn't run `klim security vuln` yet. Operators who want to
		// require a fresh scan can run it in CI.
		if thresholdRank > 0 && len(vulnSeverityByTool) > 0 {
			if sev, ok := vulnSeverityByTool[t.Name]; ok && severityRank(sev) >= thresholdRank {
				result.addError(t.Name, "vulnerable",
					fmt.Sprintf("Has known vulnerabilities at severity %s (policy threshold: %s)",
						strings.ToUpper(sev), strings.ToUpper(threshold)))
			}
		}
	}

	// Check required tools.
	for _, req := range policy.RequiredTools {
		t, ok := toolMap[strings.ToLower(req.Name)]
		if !ok || !t.IsInstalled() {
			result.addError(req.Name, "required_missing",
				req.Name+" is required but not installed")
			continue
		}
		if req.Version != "" {
			primary := t.PrimaryInstance()
			if primary != nil && primary.Version != "" {
				if !versionSatisfies(primary.Version, req.Version) {
					result.addError(req.Name, "required_version",
						fmt.Sprintf("Version %s does not satisfy %s", primary.Version, req.Version))
				}
			}
		}
	}

	return result
}

func (r *Result) addError(tool, rule, message string) {
	r.Violations = append(r.Violations, Violation{
		Severity: "error",
		Tool:     tool,
		Rule:     rule,
		Message:  message,
	})
	r.Compliant = false
}

func (r *Result) addWarning(tool, rule, message string) {
	r.Violations = append(r.Violations, Violation{
		Severity: "warning",
		Tool:     tool,
		Rule:     rule,
		Message:  message,
	})
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[strings.ToLower(strings.TrimSpace(item))] = true
	}
	return s
}

// versionSatisfies checks if an installed version meets a constraint.
// Supports >=X.Y.Z format. Otherwise performs exact match (ignoring leading 'v').
func versionSatisfies(installed, constraint string) bool {
	constraint = strings.TrimSpace(constraint)
	if strings.HasPrefix(constraint, ">=") {
		minVersion := strings.TrimPrefix(constraint, ">=")
		return registry.CompareVersions(installed, strings.TrimSpace(minVersion)) >= 0
	}
	// Exact match.
	return strings.TrimPrefix(installed, "v") == strings.TrimPrefix(constraint, "v")
}
