package registry

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed tools.yaml
var defaultToolsYAML []byte

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
	Winget string `yaml:"winget"`
	Choco  string `yaml:"choco"`
	Brew   string `yaml:"brew"`
	Apt    string `yaml:"apt"`
	Snap   string `yaml:"snap"`
	NPM    string `yaml:"npm"`
}

// ToolsPath returns the path to the tools.yaml file.
// Creates the file from embedded defaults on first run.
func ToolsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "clim", "tools.yaml")

	// If the file doesn't exist, create it from embedded defaults.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", fmt.Errorf("creating config dir: %w", err)
		}
		if err := os.WriteFile(path, defaultToolsYAML, 0o644); err != nil {
			return "", fmt.Errorf("writing default tools.yaml: %w", err)
		}
	}

	return path, nil
}

// DefaultTools loads tools from ~/.config/clim/tools.yaml.
// On first run, the file is created from embedded defaults.
func DefaultTools() []Tool {
	path, err := ToolsPath()
	if err != nil {
		// Fallback: parse embedded defaults directly.
		return defsToTools(parseToolDefs(defaultToolsYAML))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return defsToTools(parseToolDefs(defaultToolsYAML))
	}

	return defsToTools(parseToolDefs(data))
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

	header := "# clim — Tool Definitions\n# Edit this file to add, remove, or configure tools.\n# Set enabled: false to hide a tool from clim.\n\n"
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
