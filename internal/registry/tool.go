package registry

// Tool describes a CLI tool that clim can detect, check, and upgrade.
type Tool struct {
	// Name is the short identifier used in commands (e.g., "az", "gh").
	Name string

	// DisplayName is the human-readable name (e.g., "Azure CLI").
	DisplayName string

	// BinaryNames are the executable names to search for, tried in order.
	// e.g., ["python3", "python"] — tries python3 first.
	BinaryNames []string

	// VersionArgs are the arguments passed to the binary to get its version.
	// e.g., ["--version"] or ["version", "--output", "json"].
	VersionArgs []string

	// VersionRegex extracts the version string from the command output.
	// Must have exactly one capture group for the version.
	VersionRegex string

	// LatestSource describes how to check for the latest available version.
	LatestSource VersionSource

	// InstallCmds maps runtime.GOOS to the command for installing/upgrading.
	// e.g., "darwin": ["brew", "install", "azure-cli"]
	InstallCmds map[string][]string

	// Homepage is the URL for the tool's documentation.
	Homepage string
}

// DefaultTools returns the built-in registry of supported CLI tools.
func DefaultTools() []Tool {
	return []Tool{
		{
			Name:         "az",
			DisplayName:  "Azure CLI",
			BinaryNames:  []string{"az"},
			VersionArgs:  []string{"version"},
			VersionRegex: `"azure-cli":\s*"(\d+\.\d+\.\d+)"`,
			LatestSource: VersionSource{
				Type:    SourcePyPI,
				Package: "azure-cli",
			},
			InstallCmds: map[string][]string{
				"darwin":  {"brew", "install", "azure-cli"},
				"linux":   {"curl", "-sL", "https://aka.ms/InstallAzureCLIDeb", "|", "bash"},
				"windows": {"winget", "install", "--id", "Microsoft.AzureCLI", "--accept-package-agreements", "--accept-source-agreements"},
			},
			Homepage: "https://learn.microsoft.com/cli/azure",
		},
		{
			Name:         "azd",
			DisplayName:  "Azure Dev CLI",
			BinaryNames:  []string{"azd"},
			VersionArgs:  []string{"version"},
			VersionRegex: `(\d+\.\d+\.\d+)`,
			LatestSource: VersionSource{
				Type: SourceGitHub,
				Repo: "Azure/azure-dev",
			},
			InstallCmds: map[string][]string{
				"darwin":  {"brew", "install", "azure/azd/azd"},
				"linux":   {"curl", "-fsSL", "https://aka.ms/install-azd.sh", "|", "bash"},
				"windows": {"winget", "install", "--id", "Microsoft.Azd", "--accept-package-agreements", "--accept-source-agreements"},
			},
			Homepage: "https://learn.microsoft.com/azure/developer/azure-developer-cli",
		},
		{
			Name:         "gh",
			DisplayName:  "GitHub CLI",
			BinaryNames:  []string{"gh"},
			VersionArgs:  []string{"--version"},
			VersionRegex: `gh version (\d+\.\d+\.\d+)`,
			LatestSource: VersionSource{
				Type: SourceGitHub,
				Repo: "cli/cli",
			},
			InstallCmds: map[string][]string{
				"darwin":  {"brew", "install", "gh"},
				"linux":   {"apt", "install", "-y", "gh"},
				"windows": {"winget", "install", "--id", "GitHub.cli", "--accept-package-agreements", "--accept-source-agreements"},
			},
			Homepage: "https://cli.github.com",
		},
		{
			Name:         "copilot",
			DisplayName:  "GitHub Copilot CLI",
			BinaryNames:  []string{"github-copilot-cli"},
			VersionArgs:  []string{"--version"},
			VersionRegex: `(\d+\.\d+\.\d+)`,
			LatestSource: VersionSource{
				Type:    SourceNPM,
				Package: "@githubnext/github-copilot-cli",
			},
			InstallCmds: map[string][]string{
				"darwin":  {"npm", "install", "-g", "@githubnext/github-copilot-cli"},
				"linux":   {"npm", "install", "-g", "@githubnext/github-copilot-cli"},
				"windows": {"npm", "install", "-g", "@githubnext/github-copilot-cli"},
			},
			Homepage: "https://githubnext.com/projects/copilot-cli",
		},
		{
			Name:         "kubectl",
			DisplayName:  "kubectl",
			BinaryNames:  []string{"kubectl"},
			VersionArgs:  []string{"version", "--client", "-o", "json"},
			VersionRegex: `"gitVersion":\s*"v(\d+\.\d+\.\d+)"`,
			LatestSource: VersionSource{
				Type: SourceGitHub,
				Repo: "kubernetes/kubernetes",
			},
			InstallCmds: map[string][]string{
				"darwin":  {"brew", "install", "kubernetes-cli"},
				"linux":   {"snap", "install", "kubectl", "--classic"},
				"windows": {"winget", "install", "--id", "Kubernetes.kubectl", "--accept-package-agreements", "--accept-source-agreements"},
			},
			Homepage: "https://kubernetes.io/docs/reference/kubectl",
		},
		{
			Name:         "docker",
			DisplayName:  "Docker",
			BinaryNames:  []string{"docker"},
			VersionArgs:  []string{"--version"},
			VersionRegex: `Docker version (\d+\.\d+\.\d+)`,
			LatestSource: VersionSource{
				Type: SourceGitHub,
				Repo: "moby/moby",
			},
			InstallCmds: map[string][]string{
				"darwin":  {"brew", "install", "--cask", "docker"},
				"linux":   {"apt", "install", "-y", "docker.io"},
				"windows": {"winget", "install", "--id", "Docker.DockerDesktop", "--accept-package-agreements", "--accept-source-agreements"},
			},
			Homepage: "https://docs.docker.com",
		},
		{
			Name:         "terraform",
			DisplayName:  "Terraform",
			BinaryNames:  []string{"terraform"},
			VersionArgs:  []string{"version", "-json"},
			VersionRegex: `"terraform_version":\s*"(\d+\.\d+\.\d+)"`,
			LatestSource: VersionSource{
				Type: SourceGitHub,
				Repo: "hashicorp/terraform",
			},
			InstallCmds: map[string][]string{
				"darwin":  {"brew", "install", "terraform"},
				"linux":   {"snap", "install", "terraform", "--classic"},
				"windows": {"winget", "install", "--id", "Hashicorp.Terraform", "--accept-package-agreements", "--accept-source-agreements"},
			},
			Homepage: "https://www.terraform.io",
		},
		{
			Name:         "helm",
			DisplayName:  "Helm",
			BinaryNames:  []string{"helm"},
			VersionArgs:  []string{"version", "--short"},
			VersionRegex: `v?(\d+\.\d+\.\d+)`,
			LatestSource: VersionSource{
				Type: SourceGitHub,
				Repo: "helm/helm",
			},
			InstallCmds: map[string][]string{
				"darwin":  {"brew", "install", "helm"},
				"linux":   {"snap", "install", "helm", "--classic"},
				"windows": {"winget", "install", "--id", "Helm.Helm", "--accept-package-agreements", "--accept-source-agreements"},
			},
			Homepage: "https://helm.sh",
		},
		{
			Name:         "go",
			DisplayName:  "Go",
			BinaryNames:  []string{"go"},
			VersionArgs:  []string{"version"},
			VersionRegex: `go(\d+\.\d+\.?\d*)`,
			LatestSource: VersionSource{
				Type:       SourceCustom,
				URLPattern: "https://go.dev/dl/?mode=json",
			},
			InstallCmds: map[string][]string{
				"darwin":  {"brew", "install", "go"},
				"linux":   {"snap", "install", "go", "--classic"},
				"windows": {"winget", "install", "--id", "GoLang.Go", "--accept-package-agreements", "--accept-source-agreements"},
			},
			Homepage: "https://go.dev",
		},
		{
			Name:         "node",
			DisplayName:  "Node.js",
			BinaryNames:  []string{"node"},
			VersionArgs:  []string{"--version"},
			VersionRegex: `v?(\d+\.\d+\.\d+)`,
			LatestSource: VersionSource{
				Type:       SourceCustom,
				URLPattern: "https://nodejs.org/dist/index.json",
			},
			InstallCmds: map[string][]string{
				"darwin":  {"brew", "install", "node"},
				"linux":   {"apt", "install", "-y", "nodejs"},
				"windows": {"winget", "install", "--id", "OpenJS.NodeJS", "--accept-package-agreements", "--accept-source-agreements"},
			},
			Homepage: "https://nodejs.org",
		},
		{
			Name:         "python",
			DisplayName:  "Python",
			BinaryNames:  []string{"python3", "python"},
			VersionArgs:  []string{"--version"},
			VersionRegex: `Python (\d+\.\d+\.\d+)`,
			LatestSource: VersionSource{
				Type:       SourceCustom,
				URLPattern: "https://endoflife.date/api/python.json",
			},
			InstallCmds: map[string][]string{
				"darwin":  {"brew", "install", "python@3"},
				"linux":   {"apt", "install", "-y", "python3"},
				"windows": {"winget", "install", "--id", "Python.Python.3.12", "--accept-package-agreements", "--accept-source-agreements"},
			},
			Homepage: "https://python.org",
		},
		{
			Name:         "git",
			DisplayName:  "Git",
			BinaryNames:  []string{"git"},
			VersionArgs:  []string{"--version"},
			VersionRegex: `git version (\d+\.\d+\.\d+)`,
			LatestSource: VersionSource{
				Type: SourceGitHub,
				Repo: "git-for-windows/git",
			},
			InstallCmds: map[string][]string{
				"darwin":  {"brew", "install", "git"},
				"linux":   {"apt", "install", "-y", "git"},
				"windows": {"winget", "install", "--id", "Git.Git", "--accept-package-agreements", "--accept-source-agreements"},
			},
			Homepage: "https://git-scm.com",
		},
	}
}
