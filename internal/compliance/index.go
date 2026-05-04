package compliance

import (
	"strings"

	"github.com/nassiharel/clim/internal/registry"
)

// ToolStatus represents the compliance state of a single tool.
type ToolStatus int

const (
	StatusUnknown   ToolStatus = iota // no policy, or policy doesn't apply
	StatusCompliant                   // passes all rules
	StatusWarning                     // has warnings (e.g. license not in allowed list)
	StatusViolation                   // hard violation (blocked tool/license, disallowed source)
)

// String returns a short human-readable label.
func (s ToolStatus) String() string {
	switch s {
	case StatusCompliant:
		return "compliant"
	case StatusWarning:
		return "warning"
	case StatusViolation:
		return "violation"
	default:
		return "unknown"
	}
}

// toolEntry holds the per-tool compliance state.
type toolEntry struct {
	Status        ToolStatus
	Reasons       []string
	BlocksInstall bool // true when installing/upgrading this tool would itself violate policy
}

// Index provides fast O(1) compliance lookup per tool.
// Built from a Policy + tool list. Safe for concurrent reads.
type Index struct {
	policy   *Policy
	entries  map[string]toolEntry // key: lowercased tool name
	missing  []string             // required tools not installed
	policyOK bool                 // false if policy is nil
}

// BuildIndex constructs a tool→status index from a policy and tools.
// If policy is nil, all tools report StatusUnknown.
func BuildIndex(policy *Policy, tools []registry.Tool) *Index {
	idx := &Index{
		policy:   policy,
		entries:  make(map[string]toolEntry, len(tools)),
		policyOK: policy != nil,
	}
	if policy == nil {
		return idx
	}

	blockedToolSet := toSet(policy.BlockedTools)
	allowedSourceSet := toSet(policy.AllowedSources)
	allowedLicenseSet := toSet(policy.AllowedLicenses)
	blockedLicenseSet := toSet(policy.BlockedLicenses)

	// Map of required tool name → required version constraint (may be
	// empty). Used both for the "is this tool required?" check and for
	// the per-tool version-satisfaction check below.
	requiredVersions := make(map[string]string, len(policy.RequiredTools))
	for _, r := range policy.RequiredTools {
		requiredVersions[strings.ToLower(r.Name)] = r.Version
	}

	// Per-tool evaluation.
	for i := range tools {
		t := &tools[i]
		key := strings.ToLower(t.Name)
		entry := toolEntry{Status: StatusCompliant}

		// Blocked tool — hard violation, and blocks install.
		if blockedToolSet[key] {
			entry.Status = StatusViolation
			entry.BlocksInstall = true
			entry.Reasons = append(entry.Reasons, "blocked by policy")
		}

		// Source check (only meaningful for installed tools).
		if t.IsInstalled() {
			if primary := t.PrimaryInstance(); primary != nil {
				src := strings.ToLower(string(primary.Source))
				if len(allowedSourceSet) > 0 && !allowedSourceSet[src] {
					entry.Status = StatusViolation
					entry.BlocksInstall = true
					entry.Reasons = append(entry.Reasons,
						"installed via "+string(primary.Source)+" — only "+strings.Join(policy.AllowedSources, ", ")+" allowed")
				}
			}
		}

		// License checks.
		license := ""
		if t.GitHubInfo != nil {
			license = t.GitHubInfo.License
		}
		if license != "" {
			lkey := strings.ToLower(license)
			if blockedLicenseSet[lkey] {
				entry.Status = StatusViolation
				entry.BlocksInstall = true
				entry.Reasons = append(entry.Reasons, "license "+license+" is blocked")
			} else if len(allowedLicenseSet) > 0 && !allowedLicenseSet[lkey] {
				if entry.Status != StatusViolation {
					entry.Status = StatusWarning
				}
				entry.Reasons = append(entry.Reasons, "license "+license+" not in allowed list")
			}
		}

		// Version constraint check for required tools — keep the index
		// in lock-step with compliance.Check so the TUI badges and
		// install-blocking match the doctor view's verdict.
		if constraint, isRequired := requiredVersions[key]; isRequired && constraint != "" && t.IsInstalled() {
			primary := t.PrimaryInstance()
			if primary != nil && primary.Version != "" && !versionSatisfies(primary.Version, constraint) {
				entry.Status = StatusViolation
				// Version mismatch on an installed tool: upgrade IS the
				// remediation, so we don't block install/upgrade — only
				// the doctor view + badge surface the constraint.
				entry.Reasons = append(entry.Reasons,
					"version "+primary.Version+" does not satisfy "+constraint)
			}
		}

		idx.entries[key] = entry
	}

	// Required tools missing.
	for _, r := range policy.RequiredTools {
		key := strings.ToLower(r.Name)
		entry, exists := idx.entries[key]
		if !exists {
			idx.missing = append(idx.missing, r.Name)
			continue
		}
		// Find the actual tool to check installation.
		var installed bool
		for i := range tools {
			if strings.EqualFold(tools[i].Name, r.Name) && tools[i].IsInstalled() {
				installed = true
				break
			}
		}
		if !installed {
			idx.missing = append(idx.missing, r.Name)
			entry.Reasons = append(entry.Reasons, "required tool not installed")
			if entry.Status == StatusCompliant {
				entry.Status = StatusViolation
			}
			// IMPORTANT: do NOT set BlocksInstall here — installing
			// the tool is the *remediation* for this violation, not
			// a new violation. CanInstall must permit it.
			idx.entries[key] = entry
		}
	}

	return idx
}

// Status returns the compliance status for a tool.
//
//   - StatusUnknown when the index is nil or no policy is configured.
//   - The tracked status (Compliant / Warning / Violation) when the
//     tool was evaluated against the policy.
//   - StatusCompliant for tools the policy doesn't reference (no rules
//     apply, so they pass by default).
func (idx *Index) Status(toolName string) ToolStatus {
	if idx == nil || !idx.policyOK {
		return StatusUnknown
	}
	if entry, ok := idx.entries[strings.ToLower(toolName)]; ok {
		return entry.Status
	}
	return StatusCompliant
}

// Reasons returns human-readable explanations for the tool's status.
func (idx *Index) Reasons(toolName string) []string {
	if idx == nil || !idx.policyOK {
		return nil
	}
	if entry, ok := idx.entries[strings.ToLower(toolName)]; ok {
		return entry.Reasons
	}
	return nil
}

// CanInstall reports whether installing or upgrading a tool is
// permitted under the policy. Returns (true, "") when allowed,
// (false, reason) when blocked. Always returns true if the index is
// nil or has no policy.
//
// "Required-but-not-installed" violations do NOT block installs —
// installing the tool is the remediation, not a new violation. Only
// per-tool legitimacy violations (blocked tool, disallowed source,
// blocked license) flip BlocksInstall and refuse the action.
func (idx *Index) CanInstall(toolName string) (bool, string) {
	if idx == nil || !idx.policyOK {
		return true, ""
	}
	if entry, ok := idx.entries[strings.ToLower(toolName)]; ok {
		if entry.BlocksInstall {
			reason := "policy violation"
			if len(entry.Reasons) > 0 {
				reason = strings.Join(entry.Reasons, "; ")
			}
			return false, reason
		}
	}
	// Tool not in index — allow (could be unknown to policy).
	return true, ""
}

// HasPolicy returns true when a policy is configured.
func (idx *Index) HasPolicy() bool {
	return idx != nil && idx.policyOK
}

// PolicyName returns the policy name, or "" if none.
func (idx *Index) PolicyName() string {
	if idx == nil || idx.policy == nil {
		return ""
	}
	return idx.policy.Name
}

// Policy returns the policy this index was built from, or nil. Used by
// callers (e.g. the TUI) that want to rebuild the index after a tool
// state change without re-loading the policy from disk/network.
func (idx *Index) Policy() *Policy {
	if idx == nil {
		return nil
	}
	return idx.policy
}

// MissingRequired returns required tool names that aren't installed.
func (idx *Index) MissingRequired() []string {
	if idx == nil {
		return nil
	}
	return idx.missing
}

// Counts returns the number of tools per status.
func (idx *Index) Counts() (compliant, warnings, violations int) {
	if idx == nil {
		return 0, 0, 0
	}
	for _, e := range idx.entries {
		switch e.Status {
		case StatusCompliant:
			compliant++
		case StatusWarning:
			warnings++
		case StatusViolation:
			violations++
		}
	}
	return
}
