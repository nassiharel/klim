// Package compliance validates installed tools against a company policy file.
// Policies define allowed sources, licenses, blocked tools, and required tools.
package compliance

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/registry"
)

// Policy defines a company's tool compliance rules.
type Policy struct {
	Name            string         `yaml:"name"`
	Description     string         `yaml:"description,omitempty"`
	AllowedSources  []string       `yaml:"allowed_sources,omitempty"`
	AllowedLicenses []string       `yaml:"allowed_licenses,omitempty"`
	BlockedTools    []string       `yaml:"blocked_tools,omitempty"`
	BlockedLicenses []string       `yaml:"blocked_licenses,omitempty"`
	RequiredTools   []RequiredTool `yaml:"required_tools,omitempty"`
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
	return parsePolicy(data)
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
func Check(policy *Policy, tools []registry.Tool) Result {
	result := Result{
		PolicyName: policy.Name,
		Compliant:  true,
	}

	blockedSet := toSet(policy.BlockedTools)
	allowedSourceSet := toSet(policy.AllowedSources)
	allowedLicenseSet := toSet(policy.AllowedLicenses)
	blockedLicenseSet := toSet(policy.BlockedLicenses)

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
