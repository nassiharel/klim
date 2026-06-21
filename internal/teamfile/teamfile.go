// Package teamfile handles .klim.yaml team manifest files — parsing,
// discovery (walking parent dirs), and checking installed tools against
// version constraints.
package teamfile

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/klim/internal/registry"
)

// FileName is the filename for the .klim.yaml team manifest file.
const FileName = ".klim.yaml"

// TeamFile is the top-level .klim.yaml schema.
type TeamFile struct {
	Name     string         `yaml:"name,omitempty"`
	Tools    []RequiredTool `yaml:"tools"`
	Optional []RequiredTool `yaml:"optional,omitempty"`
}

// RequiredTool defines a single tool requirement.
type RequiredTool struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version,omitempty"` // e.g. ">=1.28"
}

// CheckStatus indicates the result of checking one tool.
type CheckStatus int

// CheckStatus constants describe the result of checking one required tool.
const (
	// StatusOK indicates the tool is installed with a satisfactory version.
	StatusOK       CheckStatus = iota // installed, version satisfied
	StatusMissing                     // in catalog but not installed
	StatusOutdated                    // installed but version too old
	StatusUnknown                     // not in catalog at all
)

// CheckResult holds the result of checking one required tool.
type CheckResult struct {
	Tool     RequiredTool
	Status   CheckStatus
	Version  string // installed version ("" if missing)
	Message  string // human-readable status
	Optional bool   // true if this is an optional tool
}

// Find walks up from startDir looking for .klim.yaml. Returns the full path
// or empty string if not found. Stops at filesystem root.
func Find(startDir string) string {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, FileName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break // reached root
		}
		dir = parent
	}
	return ""
}

// Parse reads and parses a .klim.yaml file.
func Parse(path string) (*TeamFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var tf TeamFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&tf); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(tf.Tools) == 0 && len(tf.Optional) == 0 {
		return nil, fmt.Errorf("%s has no tools defined", path)
	}
	// Validate tool names (required).
	seen := make(map[string]bool)
	for i, t := range tf.Tools {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			return nil, fmt.Errorf("required tool at index %d has no name", i)
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate tool %q", name)
		}
		seen[name] = true
	}
	// Validate tool names (optional).
	for i, t := range tf.Optional {
		name := strings.TrimSpace(t.Name)
		if name == "" {
			return nil, fmt.Errorf("optional tool at index %d has no name", i)
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate tool %q (in both required and optional)", name)
		}
		seen[name] = true
	}
	return &tf, nil
}

// Check validates installed tools against team file requirements.
// Returns results for both required and optional tools (Optional field set accordingly).
func Check(tf *TeamFile, tools []registry.Tool) []CheckResult {
	toolMap := make(map[string]*registry.Tool, len(tools))
	for i := range tools {
		toolMap[tools[i].Name] = &tools[i]
	}

	results := make([]CheckResult, 0, len(tf.Tools)+len(tf.Optional))

	// Check required tools.
	for _, req := range tf.Tools {
		r := checkOneTool(req, toolMap)
		results = append(results, r)
	}

	// Check optional tools.
	for _, req := range tf.Optional {
		r := checkOneTool(req, toolMap)
		r.Optional = true
		results = append(results, r)
	}

	return results
}

// checkOneTool checks a single required tool against the tool map.
func checkOneTool(req RequiredTool, toolMap map[string]*registry.Tool) CheckResult {
	rt, exists := toolMap[req.Name]

	if !exists {
		return CheckResult{
			Tool:    req,
			Status:  StatusUnknown,
			Message: "UNKNOWN TOOL (not in marketplace)",
		}
	}

	if !rt.IsInstalled() {
		return CheckResult{
			Tool:    req,
			Status:  StatusMissing,
			Message: "NOT INSTALLED",
		}
	}

	ver := rt.InstalledVersion()
	if req.Version == "" {
		return CheckResult{
			Tool:    req,
			Status:  StatusOK,
			Version: ver,
			Message: "OK",
		}
	}

	// Validate constraint syntax.
	if !ValidConstraint(req.Version) {
		return CheckResult{
			Tool:    req,
			Status:  StatusOutdated,
			Version: ver,
			Message: fmt.Sprintf("invalid constraint: %q", req.Version),
		}
	}

	// Parse and check version constraint.
	op, constraint := ParseConstraint(req.Version)
	satisfied := checkConstraint(op, ver, constraint)

	if satisfied {
		return CheckResult{
			Tool:    req,
			Status:  StatusOK,
			Version: ver,
			Message: fmt.Sprintf("OK (%s %s)", op, constraint),
		}
	}
	return CheckResult{
		Tool:    req,
		Status:  StatusOutdated,
		Version: ver,
		Message: fmt.Sprintf("%s %s required", op, constraint),
	}
}

// ParseConstraint splits a version constraint string into operator and version.
// ">=1.28" → (">=", "1.28"), "1.28" → (">=", "1.28"), "" → ("", "")
func ParseConstraint(s string) (op, ver string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", ""
	}
	for _, prefix := range []string{">=", "<=", "!=", "==", ">", "<", "="} {
		if strings.HasPrefix(s, prefix) {
			return prefix, strings.TrimSpace(s[len(prefix):])
		}
	}
	// No operator — treat as minimum version.
	return ">=", s
}

// ValidConstraint reports whether a version constraint string is valid
// (has a non-empty numeric version after the operator). Allows optional
// leading "v" prefix (e.g. ">=v1.28") since registry.CompareVersions
// strips it.
func ValidConstraint(s string) bool {
	op, ver := ParseConstraint(s)
	if op == "" && ver == "" {
		return true // no constraint = valid (means "any version")
	}
	if ver == "" {
		return false // operator with no version
	}
	// Strip optional "v" prefix.
	v := strings.TrimPrefix(ver, "v")
	return len(v) > 0 && v[0] >= '0' && v[0] <= '9'
}

// checkConstraint evaluates a version constraint.
func checkConstraint(op, installed, constraint string) bool {
	if installed == "" || constraint == "" {
		return installed != ""
	}
	cmp := registry.CompareVersions(installed, constraint)
	switch op {
	case ">=":
		return cmp >= 0
	case ">":
		return cmp > 0
	case "<=":
		return cmp <= 0
	case "<":
		return cmp < 0
	case "=", "==":
		return cmp == 0
	case "!=":
		return cmp != 0
	}
	return cmp >= 0 // default: treat as >=
}

// Summary counts results by status.
func Summary(results []CheckResult) (ok, missing, outdated, unknown int) {
	for _, r := range results {
		switch r.Status {
		case StatusOK:
			ok++
		case StatusMissing:
			missing++
		case StatusOutdated:
			outdated++
		case StatusUnknown:
			unknown++
		}
	}
	return
}

// AllSatisfied returns true if all required tools pass checks.
// Optional tool failures do not affect the result.
func AllSatisfied(results []CheckResult) bool {
	for _, r := range results {
		if r.Optional {
			continue
		}
		if r.Status != StatusOK {
			return false
		}
	}
	return true
}

// Generate creates a .klim.yaml from installed tools.
func Generate(tools []registry.Tool, includeVersion bool) *TeamFile {
	tf := &TeamFile{}
	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		req := RequiredTool{Name: t.Name}
		if includeVersion {
			ver := t.InstalledVersion()
			if ver != "" {
				req.Version = ">=" + ver
			}
		}
		tf.Tools = append(tf.Tools, req)
	}
	return tf
}

// Write writes the manifest to path.
//
// Uses os.WriteFile (truncate-in-place on existing files) rather than
// a temp-file + rename atomic write because preserving the original
// file's inode and metadata matters more than crash-atomicity for
// this workflow:
//
//   - Hardlinks pointing at the manifest stay live (atomic rename
//     would replace the inode and break them).
//   - POSIX ACLs and xattrs (e.g. setfacl -m on a shared template)
//     ride along — atomic rename would drop them.
//   - Windows ACLs (icacls grants on a hardened manifest) ride along
//     for the same reason.
//   - Existing file mode is preserved by os.WriteFile on overwrite,
//     so a manually `chmod 600` manifest stays 0600 across re-inits.
//   - Symlinks are followed (writes go through to the target,
//     preserving the link itself).
//
// The trade-off is no atomicity on crash. That's acceptable here:
// `klim project init` is interactive and easy to re-run; there are no
// concurrent readers of the manifest at write time.
func Write(tf *TeamFile, path string) error {
	data, err := yaml.Marshal(tf)
	if err != nil {
		return fmt.Errorf("marshalling: %w", err)
	}
	header := "# .klim.yaml — Team tool requirements\n# Generated by klim project init\n# See: https://github.com/nassiharel/klim\n\n"
	return os.WriteFile(path, []byte(header+string(data)), 0o644)
}

// AddToolToFile adds a tool to the required or optional list in an existing .klim.yaml.
func AddToolToFile(path, toolName string, optional bool) error {
	tf, err := Parse(path)
	if err != nil {
		return err
	}

	// Check not already in either list.
	for _, t := range tf.Tools {
		if t.Name == toolName {
			return fmt.Errorf("%s already in required tools", toolName)
		}
	}
	for _, t := range tf.Optional {
		if t.Name == toolName {
			return fmt.Errorf("%s already in optional tools", toolName)
		}
	}

	req := RequiredTool{Name: toolName}
	if optional {
		tf.Optional = append(tf.Optional, req)
	} else {
		tf.Tools = append(tf.Tools, req)
	}

	return Write(tf, path)
}
