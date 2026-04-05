package registry

// DefaultTools returns the curated list of developer tools that clim tracks.
// Each tool includes binary names, display name, and package manager identifiers.
func DefaultTools() []Tool {
	return []Tool{
		// --- Version Control ---
		{
			Name: "git", DisplayName: "Git",
			BinaryNames: []string{"git"},
			Packages: PackageIDs{
				Winget: "Git.Git", Choco: "git", Brew: "git", Apt: "git",
			},
		},
		{
			Name: "gh", DisplayName: "GitHub CLI",
			BinaryNames: []string{"gh"},
			Packages: PackageIDs{
				Winget: "GitHub.cli", Choco: "gh", Brew: "gh", Apt: "gh",
			},
		},

		// --- Cloud CLIs ---
		{
			Name: "az", DisplayName: "Azure CLI",
			BinaryNames: []string{"az"},
			Packages: PackageIDs{
				Winget: "Microsoft.AzureCLI", Brew: "azure-cli", Apt: "azure-cli",
			},
		},
		{
			Name: "azd", DisplayName: "Azure Dev CLI",
			BinaryNames: []string{"azd"},
			Packages: PackageIDs{
				Winget: "Microsoft.Azd", Brew: "azure/azd/azd",
			},
		},
		{
			Name: "aws", DisplayName: "AWS CLI",
			BinaryNames: []string{"aws"},
			Packages: PackageIDs{
				Winget: "Amazon.AWSCLI", Brew: "awscli",
			},
		},
		{
			Name: "gcloud", DisplayName: "Google Cloud CLI",
			BinaryNames: []string{"gcloud"},
			Packages: PackageIDs{
				Brew: "google-cloud-sdk", Snap: "google-cloud-cli",
			},
		},

		// --- Containers & Orchestration ---
		{
			Name: "docker", DisplayName: "Docker",
			BinaryNames: []string{"docker"},
			Packages: PackageIDs{
				Winget: "Docker.DockerDesktop", Brew: "docker", Apt: "docker.io",
			},
		},
		{
			Name: "docker-compose", DisplayName: "Docker Compose",
			BinaryNames: []string{"docker-compose"},
			Packages: PackageIDs{
				Brew: "docker-compose",
			},
		},
		{
			Name: "kubectl", DisplayName: "kubectl",
			BinaryNames: []string{"kubectl"},
			Packages: PackageIDs{
				Winget: "Kubernetes.kubectl", Choco: "kubernetes-cli",
				Brew: "kubernetes-cli", Snap: "kubectl",
			},
		},
		{
			Name: "helm", DisplayName: "Helm",
			BinaryNames: []string{"helm"},
			Packages: PackageIDs{
				Winget: "Helm.Helm", Choco: "kubernetes-helm",
				Brew: "helm", Snap: "helm",
			},
		},
		{
			Name: "k9s", DisplayName: "K9s",
			BinaryNames: []string{"k9s"},
			Packages: PackageIDs{
				Choco: "k9s", Brew: "derailed/k9s/k9s",
			},
		},
		{
			Name: "terraform", DisplayName: "Terraform",
			BinaryNames: []string{"terraform"},
			Packages: PackageIDs{
				Winget: "Hashicorp.Terraform", Choco: "terraform",
				Brew: "terraform", Snap: "terraform",
			},
		},
		{
			Name: "kubelogin", DisplayName: "Kubelogin",
			BinaryNames: []string{"kubelogin"},
			Packages: PackageIDs{
				Brew: "Azure/kubelogin/kubelogin",
			},
		},

		// --- Languages & Runtimes ---
		{
			Name: "go", DisplayName: "Go",
			BinaryNames: []string{"go"},
			Packages: PackageIDs{
				Winget: "GoLang.Go", Brew: "go", Apt: "golang-go", Snap: "go",
			},
		},
		{
			Name: "node", DisplayName: "Node.js",
			BinaryNames: []string{"node"},
			Packages: PackageIDs{
				Winget: "OpenJS.NodeJS", Brew: "node", Apt: "nodejs",
			},
		},
		{
			Name: "python", DisplayName: "Python",
			BinaryNames: []string{"python3", "python"},
			Packages: PackageIDs{
				Winget: "Python.Python.3.12", Brew: "python@3",
				Apt: "python3",
			},
		},
		{
			Name: "dotnet", DisplayName: ".NET",
			BinaryNames: []string{"dotnet"},
			Packages: PackageIDs{
				Winget: "Microsoft.DotNet.SDK.8", Brew: "dotnet",
				Apt: "dotnet-sdk-8.0", Snap: "dotnet-sdk",
			},
		},
		{
			Name: "rustc", DisplayName: "Rust",
			BinaryNames: []string{"rustc"},
			Packages: PackageIDs{
				Brew: "rust", Apt: "rustc",
			},
		},
		{
			Name: "cargo", DisplayName: "Cargo",
			BinaryNames: []string{"cargo"},
			Packages: PackageIDs{
				Brew: "rust", Apt: "cargo",
			},
		},
		{
			Name: "java", DisplayName: "Java",
			BinaryNames: []string{"java"},
			Packages: PackageIDs{
				Winget: "Microsoft.OpenJDK.21", Brew: "openjdk",
				Apt: "default-jdk",
			},
		},
		{
			Name: "ruby", DisplayName: "Ruby",
			BinaryNames: []string{"ruby"},
			Packages: PackageIDs{
				Winget: "RubyInstallerTeam.Ruby", Brew: "ruby", Apt: "ruby",
			},
		},

		// --- Package Managers ---
		{
			Name: "npm", DisplayName: "npm",
			BinaryNames: []string{"npm"},
			Packages: PackageIDs{NPM: "npm"},
		},
		{
			Name: "uv", DisplayName: "uv",
			BinaryNames: []string{"uv"},
			Packages: PackageIDs{
				Winget: "astral-sh.uv", Brew: "uv",
			},
		},

		// --- Editors ---
		{
			Name: "code", DisplayName: "VS Code",
			BinaryNames: []string{"code"},
			Packages: PackageIDs{
				Winget: "Microsoft.VisualStudioCode", Brew: "visual-studio-code",
			},
		},

		// --- CLI Utilities ---
		{
			Name: "jq", DisplayName: "jq",
			BinaryNames: []string{"jq"},
			Packages: PackageIDs{
				Winget: "jqlang.jq", Choco: "jq", Brew: "jq", Apt: "jq",
			},
		},
		{
			Name: "fzf", DisplayName: "fzf",
			BinaryNames: []string{"fzf"},
			Packages: PackageIDs{
				Winget: "junegunn.fzf", Choco: "fzf", Brew: "fzf", Apt: "fzf",
			},
		},
		{
			Name: "bat", DisplayName: "bat",
			BinaryNames: []string{"bat"},
			Packages: PackageIDs{
				Winget: "sharkdp.bat", Brew: "bat", Apt: "bat",
			},
		},
		{
			Name: "fd", DisplayName: "fd",
			BinaryNames: []string{"fd"},
			Packages: PackageIDs{
				Winget: "sharkdp.fd", Brew: "fd", Apt: "fd-find",
			},
		},

		// --- Shells ---
		{
			Name: "pwsh", DisplayName: "PowerShell",
			BinaryNames: []string{"pwsh"},
			Packages: PackageIDs{
				Winget: "Microsoft.PowerShell", Brew: "powershell",
				Apt: "powershell", Snap: "powershell",
			},
		},

		// --- Other ---
		{
			Name: "claude", DisplayName: "Claude Code",
			BinaryNames: []string{"claude"},
			Packages: PackageIDs{NPM: "@anthropic-ai/claude-code"},
		},
	}
}
