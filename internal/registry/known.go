package registry

// SourceType identifies the upstream source for latest version checks.
type SourceType string

const (
	SourceGitHub SourceType = "github"
	SourcePyPI   SourceType = "pypi"
	SourceNPM    SourceType = "npm"
)

// LatestSource describes where to check for the latest version of a tool.
type LatestSource struct {
	Type    SourceType
	Repo    string // "owner/repo" for GitHub
	Package string // package name for PyPI/NPM
}

// KnownTool holds curated metadata for a recognized developer tool.
type KnownTool struct {
	DisplayName  string
	LatestSource LatestSource
}

// KnownTools maps binary names (lowercase) to their curated metadata.
// Tools not in this map still get version detection (PE/Go buildinfo)
// but won't have latest-version or display-name info.
var KnownTools = map[string]KnownTool{
	// Version control
	"git":     {DisplayName: "Git", LatestSource: LatestSource{Type: SourceGitHub, Repo: "git-for-windows/git"}},
	"git-lfs": {DisplayName: "Git LFS", LatestSource: LatestSource{Type: SourceGitHub, Repo: "git-lfs/git-lfs"}},
	"gh":      {DisplayName: "GitHub CLI", LatestSource: LatestSource{Type: SourceGitHub, Repo: "cli/cli"}},
	"tig":     {DisplayName: "Tig", LatestSource: LatestSource{Type: SourceGitHub, Repo: "jonas/tig"}},

	// Cloud & infrastructure
	"az":        {DisplayName: "Azure CLI", LatestSource: LatestSource{Type: SourcePyPI, Package: "azure-cli"}},
	"azd":       {DisplayName: "Azure Dev CLI", LatestSource: LatestSource{Type: SourceGitHub, Repo: "Azure/azure-dev"}},
	"terraform": {DisplayName: "Terraform", LatestSource: LatestSource{Type: SourceGitHub, Repo: "hashicorp/terraform"}},
	"kubectl":   {DisplayName: "kubectl", LatestSource: LatestSource{Type: SourceGitHub, Repo: "kubernetes/kubernetes"}},
	"helm":      {DisplayName: "Helm", LatestSource: LatestSource{Type: SourceGitHub, Repo: "helm/helm"}},
	"k9s":       {DisplayName: "K9s", LatestSource: LatestSource{Type: SourceGitHub, Repo: "derailed/k9s"}},
	"kubelogin": {DisplayName: "Kubelogin", LatestSource: LatestSource{Type: SourceGitHub, Repo: "Azure/kubelogin"}},

	// Containers
	"docker":         {DisplayName: "Docker", LatestSource: LatestSource{Type: SourceGitHub, Repo: "moby/moby"}},
	"docker-compose": {DisplayName: "Docker Compose", LatestSource: LatestSource{Type: SourceGitHub, Repo: "docker/compose"}},

	// Languages & runtimes
	"go":      {DisplayName: "Go", LatestSource: LatestSource{Type: SourceGitHub, Repo: "golang/go"}},
	"node":    {DisplayName: "Node.js", LatestSource: LatestSource{Type: SourceGitHub, Repo: "nodejs/node"}},
	"python":  {DisplayName: "Python", LatestSource: LatestSource{Type: SourceGitHub, Repo: "python/cpython"}},
	"python3": {DisplayName: "Python", LatestSource: LatestSource{Type: SourceGitHub, Repo: "python/cpython"}},
	"dotnet":  {DisplayName: ".NET", LatestSource: LatestSource{Type: SourceGitHub, Repo: "dotnet/runtime"}},
	"rustc":   {DisplayName: "Rust", LatestSource: LatestSource{Type: SourceGitHub, Repo: "rust-lang/rust"}},

	// Package managers
	"npm":   {DisplayName: "npm", LatestSource: LatestSource{Type: SourceNPM, Package: "npm"}},
	"npx":   {DisplayName: "npx"},
	"uv":    {DisplayName: "uv", LatestSource: LatestSource{Type: SourceGitHub, Repo: "astral-sh/uv"}},
	"uvx":   {DisplayName: "uvx"},
	"choco": {DisplayName: "Chocolatey"},
	"cargo": {DisplayName: "Cargo"},

	// Editors & tools
	"code":          {DisplayName: "VS Code", LatestSource: LatestSource{Type: SourceGitHub, Repo: "microsoft/vscode"}},
	"code-insiders": {DisplayName: "VS Code Insiders"},
	"claude":        {DisplayName: "Claude Code"},

	// CLI utilities
	"jq":   {DisplayName: "jq", LatestSource: LatestSource{Type: SourceGitHub, Repo: "jqlang/jq"}},
	"fzf":  {DisplayName: "fzf", LatestSource: LatestSource{Type: SourceGitHub, Repo: "junegunn/fzf"}},
	"bat":  {DisplayName: "bat", LatestSource: LatestSource{Type: SourceGitHub, Repo: "sharkdp/bat"}},
	"fd":   {DisplayName: "fd", LatestSource: LatestSource{Type: SourceGitHub, Repo: "sharkdp/fd"}},
	"ffmpeg": {DisplayName: "FFmpeg"},

	// Shells
	"pwsh": {DisplayName: "PowerShell", LatestSource: LatestSource{Type: SourceGitHub, Repo: "PowerShell/PowerShell"}},
}

// LookupKnown returns the KnownTool metadata for a binary name, if it exists.
func LookupKnown(name string) (KnownTool, bool) {
	kt, ok := KnownTools[name]
	return kt, ok
}
