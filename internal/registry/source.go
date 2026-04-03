package registry

// SourceType identifies the API used to check for the latest version of a tool.
type SourceType string

const (
	SourceGitHub SourceType = "github"
	SourcePyPI   SourceType = "pypi"
	SourceNPM    SourceType = "npm"
	SourceCustom SourceType = "custom"
)

// VersionSource describes how to look up the latest version of a tool.
type VersionSource struct {
	Type       SourceType
	Repo       string // "owner/repo" for GitHub
	Package    string // package name for PyPI or npm
	URLPattern string // for Custom type (e.g., go.dev, nodejs.org)
}
