package registry

import (
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type toolsFile struct {
	Tools []toolDef `yaml:"tools"`
}

type toolDef struct {
	Name        string     `yaml:"name"`
	DisplayName string     `yaml:"display_name"`
	Category    string     `yaml:"category"`
	Tags        []string   `yaml:"tags,omitempty"`
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

// DefaultToolsFromBytes loads tools from raw catalog YAML bytes, merging with
// the user's local customizations (custom tools, package ID overrides).
// The catalogData is the authority for tool metadata; the user file preserves
// user-added custom tools and non-empty package ID overrides.
func DefaultToolsFromBytes(catalogData []byte) []Tool {
	catalogDefs := parseToolDefs(catalogData)
	if catalogDefs == nil {
		return nil
	}

	path, err := ToolsPath()
	if err != nil {
		return defsToTools(catalogDefs)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		// No user file yet — write one from catalog defaults.
		if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr == nil {
			_ = writeToolDefs(path, catalogDefs)
		}
		return defsToTools(catalogDefs)
	}

	userDefs := parseToolDefs(data)
	if userDefs == nil {
		// Invalid YAML — rewrite from catalog so the file is usable again.
		_ = writeToolDefs(path, catalogDefs)
		return defsToTools(catalogDefs)
	}

	merged, changed := mergeToolDefs(catalogDefs, userDefs)
	if changed {
		slog.Debug("marketplace merge updated user file", "path", path, "tools", len(merged))
		_ = writeToolDefs(path, merged)
	}

	return defsToTools(merged)
}

func writeToolDefs(path string, defs []toolDef) error {
	f := toolsFile{Tools: defs}
	data, err := yaml.Marshal(&f)
	if err != nil {
		return err
	}

	header := "# clim — Tool Marketplace\n# Edit this file to add, remove, or configure tools.\n\n"
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
			Tags:        d.Tags,
			BinaryNames: d.BinaryNames,
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

// mergeToolDefs merges catalog defaults with user-customized definitions.
// Catalog tools provide the base; user file overrides non-empty package IDs.
// User-added custom tools (not in catalog) are preserved.
// Returns the merged list and whether anything changed vs the user's original.
func mergeToolDefs(catalog, user []toolDef) ([]toolDef, bool) {
	// Index user defs by name for O(1) lookup.
	userMap := make(map[string]*toolDef, len(user))
	for i := range user {
		userMap[user[i].Name] = &user[i]
	}

	changed := false
	merged := make([]toolDef, 0, len(catalog)+len(user))

	// Walk catalog in order — this defines the canonical ordering.
	seen := make(map[string]struct{}, len(catalog))
	for _, e := range catalog {
		seen[e.Name] = struct{}{}

		u, exists := userMap[e.Name]
		if !exists {
			// New catalog tool — add it.
			merged = append(merged, e)
			changed = true
			continue
		}

		// Tool exists in both — merge fields.
		m := e // start from catalog (authority on display_name, category, binary_names, tags)
		m.Packages = mergePackages(e.Packages, u.Packages)

		if m.Packages != u.Packages {
			changed = true
		}
		if m.DisplayName != u.DisplayName || m.Category != u.Category ||
			!slicesEqual(m.BinaryNames, u.BinaryNames) || !slicesEqual(m.Tags, u.Tags) {
			changed = true
		}

		merged = append(merged, m)
	}

	// Append user-only custom tools (not in catalog).
	for _, u := range user {
		if _, exists := seen[u.Name]; !exists {
			merged = append(merged, u)
		}
	}

	return merged, changed
}

// mergePackages merges package IDs: user non-empty values win, catalog fills gaps.
func mergePackages(catalog, user packageDef) packageDef {
	return packageDef{
		Winget: pickNonEmpty(user.Winget, catalog.Winget),
		Choco:  pickNonEmpty(user.Choco, catalog.Choco),
		Brew:   pickNonEmpty(user.Brew, catalog.Brew),
		Apt:    pickNonEmpty(user.Apt, catalog.Apt),
		Snap:   pickNonEmpty(user.Snap, catalog.Snap),
		NPM:    pickNonEmpty(user.NPM, catalog.NPM),
	}
}

func pickNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

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
