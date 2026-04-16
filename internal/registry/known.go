package registry

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type toolsFile struct {
	Tools []ToolDef `yaml:"tools"`
	Packs []packDef `yaml:"packs,omitempty"`
}

// ToolDef is the YAML structure for a single tool definition.
// Exported so the catalog and marketplace packages can reuse it.
type ToolDef struct {
	Name        string     `yaml:"name"`
	DisplayName string     `yaml:"display_name"`
	Category    string     `yaml:"category"`
	Tags        []string   `yaml:"tags,omitempty"`
	BinaryNames []string   `yaml:"binary_names"`
	Packages    PackageDef `yaml:"packages"`
}

type packDef struct {
	Name        string   `yaml:"name"`
	DisplayName string   `yaml:"display_name"`
	Description string   `yaml:"description,omitempty"`
	Tools       []string `yaml:"tools"`
}

// PackageDef is the YAML structure for package manager identifiers.
// Exported so the catalog and marketplace packages can reuse it.
type PackageDef struct {
	Winget string `yaml:"winget,omitempty"`
	Choco  string `yaml:"choco,omitempty"`
	Brew   string `yaml:"brew,omitempty"`
	Apt    string `yaml:"apt,omitempty"`
	Snap   string `yaml:"snap,omitempty"`
	NPM    string `yaml:"npm,omitempty"`
}

// ToolsFromBytes parses catalog YAML bytes into a slice of Tools.
func ToolsFromBytes(data []byte) []Tool {
	defs := parseToolDefs(data)
	if defs == nil {
		return nil
	}
	return defsToTools(defs)
}

func parseToolDefs(data []byte) []ToolDef {
	var f toolsFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil
	}
	return f.Tools
}

// ParsePacksFromBytes parses the packs section from catalog YAML bytes.
// Returns an error if the YAML is unparsable. Returns an empty (non-nil)
// slice if the YAML is valid but contains no packs.
func ParsePacksFromBytes(data []byte) ([]Pack, error) {
	var f toolsFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing packs: %w", err)
	}
	packs := make([]Pack, 0, len(f.Packs))
	for _, pd := range f.Packs {
		p := Pack{
			Name:        pd.Name,
			DisplayName: pd.DisplayName,
			Description: pd.Description,
			ToolNames:   pd.Tools,
		}
		if p.DisplayName == "" {
			p.DisplayName = p.Name
		}
		packs = append(packs, p)
	}
	return packs, nil
}

func defsToTools(defs []ToolDef) []Tool {
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
