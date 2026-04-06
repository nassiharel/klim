// Package manifest defines the shared YAML schema for tool export/import manifests.
// Used by both the CLI commands and the TUI to ensure a single source of truth.
package manifest

// Manifest is the top-level YAML structure for import/export.
type Manifest struct {
	GeneratedBy string `yaml:"generated_by"`
	OS          string `yaml:"os"`
	Arch        string `yaml:"arch"`
	Tools       []Tool `yaml:"tools"`
}

// Tool represents a single tool entry in a manifest.
type Tool struct {
	Name        string   `yaml:"name"`
	DisplayName string   `yaml:"display_name"`
	Version     string   `yaml:"version,omitempty"`
	Source      string   `yaml:"source,omitempty"`
	Category    string   `yaml:"category"`
	Packages    Packages `yaml:"packages,omitempty"`
}

// Packages holds package manager identifiers for cross-platform installs.
type Packages struct {
	Winget string `yaml:"winget,omitempty"`
	Choco  string `yaml:"choco,omitempty"`
	Brew   string `yaml:"brew,omitempty"`
	Apt    string `yaml:"apt,omitempty"`
	Snap   string `yaml:"snap,omitempty"`
	NPM    string `yaml:"npm,omitempty"`
}
