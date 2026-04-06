package registry

import (
	"fmt"
	"os"
	"path/filepath"

	clim "github.com/nassiharel/clim"
	"gopkg.in/yaml.v3"
)

var defaultToolsYAML = clim.MarketplaceYAML

type toolsFile struct {
	Tools []toolDef `yaml:"tools"`
}

type toolDef struct {
	Name        string     `yaml:"name"`
	DisplayName string     `yaml:"display_name"`
	Category    string     `yaml:"category"`
	Enabled     bool       `yaml:"enabled"`
	BinaryNames []string   `yaml:"binary_names"`
	Packages    packageDef `yaml:"packages"`
}

type packageDef struct {
	Winget string `yaml:"winget,omitempty"`
	Choco  string `yaml:"choco,omitempty"`
	Brew   string `yaml:"brew,omitempty"`
	Apt    string `yaml:"apt,omitempty"`
	Snap   string `yaml:"snap,omitempty"`
	NPM    string `yaml:"npm,omitempty"`
}

// ToolsPath returns the path to the user's marketplace.yaml file.
func ToolsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "clim", "marketplace.yaml")
	return path, nil
}

// DefaultTools loads tools by merging the embedded catalog with the user's config.
// New embedded tools are added, user customizations (enabled/disabled, package overrides)
// are preserved, and user-added custom tools are kept. If anything changed, the
// user's file is rewritten with the merged result.
func DefaultTools() []Tool {
	embeddedDefs := parseToolDefs(defaultToolsYAML)

	path, err := ToolsPath()
	if err != nil {
		return defsToTools(embeddedDefs)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		// First run or unreadable file — write embedded defaults.
		if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr == nil {
			_ = writeToolDefs(path, embeddedDefs)
		}
		return defsToTools(embeddedDefs)
	}

	userDefs := parseToolDefs(data)
	if userDefs == nil {
		return defsToTools(embeddedDefs)
	}

	merged, changed := mergeToolDefs(embeddedDefs, userDefs)
	if changed {
		_ = writeToolDefs(path, merged)
	}

	return defsToTools(merged)
}

// SetToolEnabled sets the enabled flag for a tool by name in the YAML file.
func SetToolEnabled(name string, enabled bool) error {
	path, err := ToolsPath()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	defs := parseToolDefs(data)
	found := false
	for i := range defs {
		if defs[i].Name == name {
			defs[i].Enabled = enabled
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("tool %q not found", name)
	}

	return writeToolDefs(path, defs)
}

func writeToolDefs(path string, defs []toolDef) error {
	f := toolsFile{Tools: defs}
	data, err := yaml.Marshal(&f)
	if err != nil {
		return err
	}

	header := "# clim — Tool Marketplace\n# Edit this file to add, remove, or configure tools.\n# Set enabled: false to hide a tool from clim.\n\n"
	return os.WriteFile(path, []byte(header+string(data)), 0o644)
}

func parseToolDefs(data []byte) []toolDef {
	var f toolsFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil
	}
	return f.Tools
}

func defsToTools(defs []toolDef) []Tool {
	tools := make([]Tool, 0, len(defs))
	for _, d := range defs {
		t := Tool{
			Name:        d.Name,
			DisplayName: d.DisplayName,
			Category:    d.Category,
			BinaryNames: d.BinaryNames,
			Disabled:    !d.Enabled,
			Packages: PackageIDs{
				Winget: d.Packages.Winget,
				Choco:  d.Packages.Choco,
				Brew:   d.Packages.Brew,
				Apt:    d.Packages.Apt,
				Snap:   d.Packages.Snap,
				NPM:    d.Packages.NPM,
			},
		}
		if t.DisplayName == "" {
			t.DisplayName = t.Name
		}
		if len(t.BinaryNames) == 0 {
			t.BinaryNames = []string{t.Name}
		}
		tools = append(tools, t)
	}
	return tools
}

// mergeToolDefs merges embedded defaults with user-customized definitions.
// Embedded tools provide the base catalog; user file overrides enabled state
// and non-empty package IDs. User-added custom tools (not in embedded) are preserved.
// Returns the merged list and whether anything changed vs the user's original.
func mergeToolDefs(embedded, user []toolDef) ([]toolDef, bool) {
	// Index user defs by name for O(1) lookup.
	userMap := make(map[string]*toolDef, len(user))
	for i := range user {
		userMap[user[i].Name] = &user[i]
	}

	changed := false
	merged := make([]toolDef, 0, len(embedded)+len(user))

	// Walk embedded in order — this defines the canonical ordering.
	seen := make(map[string]struct{}, len(embedded))
	for _, e := range embedded {
		seen[e.Name] = struct{}{}

		u, exists := userMap[e.Name]
		if !exists {
			// New embedded tool — add it.
			merged = append(merged, e)
			changed = true
			continue
		}

		// Tool exists in both — merge fields.
		m := e // start from embedded (authority on display_name, category, binary_names)
		m.Enabled = u.Enabled
		m.Packages = mergePackages(e.Packages, u.Packages)

		if m.Packages != u.Packages {
			changed = true // embedded filled in a package ID gap
		}
		if m.DisplayName != u.DisplayName || m.Category != u.Category || !slicesEqual(m.BinaryNames, u.BinaryNames) {
			changed = true // embedded metadata updated
		}

		merged = append(merged, m)
	}

	// Append user-only custom tools (not in embedded catalog).
	for _, u := range user {
		if _, exists := seen[u.Name]; !exists {
			merged = append(merged, u)
		}
	}

	return merged, changed
}

// mergePackages merges package IDs: user non-empty values win, embedded fills gaps.
func mergePackages(embedded, user packageDef) packageDef {
	return packageDef{
		Winget: pickNonEmpty(user.Winget, embedded.Winget),
		Choco:  pickNonEmpty(user.Choco, embedded.Choco),
		Brew:   pickNonEmpty(user.Brew, embedded.Brew),
		Apt:    pickNonEmpty(user.Apt, embedded.Apt),
		Snap:   pickNonEmpty(user.Snap, embedded.Snap),
		NPM:    pickNonEmpty(user.NPM, embedded.NPM),
	}
}

// pickNonEmpty returns the first non-empty string, or "".
func pickNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// slicesEqual reports whether two string slices have identical contents.
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
