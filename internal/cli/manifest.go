package cli

// manifest types shared between export and import commands.

// exportManifest is the top-level YAML structure for import/export.
type exportManifest struct {
	GeneratedBy string       `yaml:"generated_by"`
	OS          string       `yaml:"os"`
	Arch        string       `yaml:"arch"`
	Tools       []exportTool `yaml:"tools"`
}

// exportTool represents a single tool in the manifest.
type exportTool struct {
	Name        string         `yaml:"name"`
	DisplayName string         `yaml:"display_name"`
	Version     string         `yaml:"version,omitempty"`
	Source      string         `yaml:"source,omitempty"`
	Category    string         `yaml:"category"`
	Packages    exportPackages `yaml:"packages,omitempty"`
}

// exportPackages holds package manager identifiers for cross-platform installs.
type exportPackages struct {
	Winget string `yaml:"winget,omitempty"`
	Choco  string `yaml:"choco,omitempty"`
	Brew   string `yaml:"brew,omitempty"`
	Apt    string `yaml:"apt,omitempty"`
	Snap   string `yaml:"snap,omitempty"`
	NPM    string `yaml:"npm,omitempty"`
}
