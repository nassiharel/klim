package teamfile

import (
	"bufio"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// DetectedTool represents a tool detected from project files.
type DetectedTool struct {
	Name   string // tool name (must match marketplace catalog)
	Source string // which file triggered detection
}

// fileRule maps a file path (relative to project root) to tool names.
type fileRule struct {
	file  string
	tools []string
}

// dirRule maps a directory name to tool names.
type dirRule struct {
	dir   string
	tools []string
}

// keyword maps a substring to a tool name for CI/config file scanning.
type keyword struct {
	pattern string
	tool    string
}

// DetectFromProject scans a project directory for configuration files and
// infers which CLI tools the project requires. Returns deduplicated tool names
// sorted by name, with the source file that triggered each detection.
//
// Tool names MUST match the marketplace catalog (e.g. "go" not "golang",
// "node" not "nodejs", "docker-compose" not "docker compose").
func DetectFromProject(dir string) []DetectedTool {
	seen := make(map[string]string) // name → source
	var results []DetectedTool

	add := func(name, source string) {
		if _, ok := seen[name]; !ok {
			seen[name] = source
			results = append(results, DetectedTool{Name: name, Source: source})
		}
	}

	// --- File existence checks (ordered for determinism) ---
	fileRules := []fileRule{
		// Docker
		{"Dockerfile", []string{"docker"}},
		{".dockerignore", []string{"docker"}},
		{"docker-compose.yml", []string{"docker", "docker-compose"}},
		{"docker-compose.yaml", []string{"docker", "docker-compose"}},
		{"compose.yml", []string{"docker", "docker-compose"}},
		{"compose.yaml", []string{"docker", "docker-compose"}},
		{"skaffold.yaml", []string{"docker", "kubectl"}},
		// Build systems
		{"Makefile", nil},         // make not in catalog
		{"Justfile", []string{"just"}},
		// Go
		{"go.mod", []string{"go"}},
		{"go.sum", []string{"go"}},
		{".golangci.yml", []string{"golangci-lint"}},
		{".golangci.yaml", []string{"golangci-lint"}},
		{".golangci.toml", []string{"golangci-lint"}},
		{".golangci.json", []string{"golangci-lint"}},
		{".goreleaser.yml", []string{"go"}},
		{".goreleaser.yaml", []string{"go"}},
		// Node / JS
		{"package.json", []string{"node", "npm"}},
		{"package-lock.json", []string{"node", "npm"}},
		{"yarn.lock", []string{"node"}},
		{"pnpm-lock.yaml", []string{"node"}},
		{"bun.lockb", []string{"bun"}},
		{"tsconfig.json", []string{"node"}},
		{".eslintrc.json", []string{"node"}},
		{".eslintrc.yml", []string{"node"}},
		{".prettierrc", []string{"node"}},
		{".prettierrc.json", []string{"node"}},
		{".nvmrc", []string{"node"}},
		{".node-version", []string{"node"}},
		// Deno
		{"deno.json", []string{"deno"}},
		{"deno.jsonc", []string{"deno"}},
		// Rust
		{"Cargo.toml", []string{"cargo"}},
		{"Cargo.lock", []string{"cargo"}},
		// Python
		{"pyproject.toml", []string{"python3"}},
		{"requirements.txt", []string{"python3"}},
		{"Pipfile", []string{"python3"}},
		{"setup.py", []string{"python3"}},
		{".python-version", []string{"python3"}},
		// Ruby
		{"Gemfile", nil},           // ruby not in catalog
		{".ruby-version", nil},
		// Java / JVM
		{"pom.xml", []string{"java"}},
		{"build.gradle", []string{"java"}},
		{"build.gradle.kts", []string{"java"}},
		// .NET
		// C/C++
		{"CMakeLists.txt", []string{"cmake"}},
		// Terraform / IaC
		{".terraform.lock.hcl", []string{"terraform"}},
		{"terragrunt.hcl", nil},    // terragrunt not in catalog
		// Ansible
		{"ansible.cfg", []string{"ansible"}},
		{"playbook.yml", []string{"ansible"}},
		// Version managers
		{".tool-versions", []string{"asdf"}},
		{".mise.toml", []string{"mise"}},
		// Git / SCM
		{".pre-commit-config.yaml", []string{"git"}},
		{".github/dependabot.yml", []string{"git"}},
		{".github/dependabot.yaml", []string{"git"}},
		// Misc
		{".vscode/settings.json", []string{"code"}},
	}

	for _, rule := range fileRules {
		if fileExists(filepath.Join(dir, rule.file)) {
			for _, t := range rule.tools {
				add(t, rule.file)
			}
		}
	}

	// --- Directory existence checks (ordered) ---
	dirRules := []dirRule{
		{".github", []string{"gh"}},
		{"terraform", []string{"terraform"}},
		{"helm", []string{"helm", "kubectl"}},
		{"charts", []string{"helm", "kubectl"}},
		{"k8s", []string{"kubectl"}},
		{"kubernetes", []string{"kubectl"}},
		{"kustomize", []string{"kubectl", "kustomize"}},
		{"ansible", []string{"ansible"}},
	}

	for _, rule := range dirRules {
		if dirExists(filepath.Join(dir, rule.dir)) {
			for _, t := range rule.tools {
				add(t, rule.dir+"/")
			}
		}
	}

	// --- Recursive file search for specific patterns ---
	walkPatterns := map[string][]string{
		".tf": {"terraform"},
	}
	// Walk only top-level and one level deep to avoid scanning node_modules etc.
	for _, depth := range []string{"", "*"} {
		for ext, tools := range walkPatterns {
			pattern := filepath.Join(dir, depth, "*"+ext)
			matches, _ := filepath.Glob(pattern)
			if len(matches) > 0 {
				for _, t := range tools {
					add(t, "*"+ext)
				}
			}
		}
	}

	// --- CI/CD file scanning ---
	ciPatterns := []string{
		".github/workflows/*.yml",
		".github/workflows/*.yaml",
	}
	ciFiles := []string{
		".gitlab-ci.yml",
		"azure-pipelines.yml",
		".circleci/config.yml",
	}

	for _, pattern := range ciPatterns {
		matches, _ := filepath.Glob(filepath.Join(dir, pattern))
		for _, f := range matches {
			scanFileForTools(f, filepath.Base(f), add)
		}
	}
	for _, f := range ciFiles {
		fullPath := filepath.Join(dir, f)
		if fileExists(fullPath) {
			scanFileForTools(fullPath, f, add)
		}
	}

	// --- Dockerfile scanning ---
	for _, df := range []string{"Dockerfile"} {
		fullPath := filepath.Join(dir, df)
		if fileExists(fullPath) {
			scanDockerfile(fullPath, add)
		}
	}
	// Also check Dockerfile.* variants.
	matches, _ := filepath.Glob(filepath.Join(dir, "Dockerfile.*"))
	for _, f := range matches {
		scanDockerfile(f, add)
	}

	// Sort results by name for deterministic output.
	sort.Slice(results, func(i, j int) bool {
		return strings.ToLower(results[i].Name) < strings.ToLower(results[j].Name)
	})

	return results
}

// ciKeywords maps substrings found in CI config files to catalog tool names.
// Order doesn't matter here — results are deduped by name.
var ciKeywords = []keyword{
	{"kubectl", "kubectl"},
	{"helm", "helm"},
	{"terraform", "terraform"},
	{"docker", "docker"},
	{"docker-compose", "docker-compose"},
	{"az login", "az"},
	{"az ", "az"},
	{"aws ", "aws"},
	{"gcloud", "gcloud"},
	{"node ", "node"},
	{"npm ", "npm"},
	{"go build", "go"},
	{"go test", "go"},
	{"go run", "go"},
	{"golangci-lint", "golangci-lint"},
	{"cargo", "cargo"},
	{"python", "python3"},
	{"pip ", "python3"},
	{"java", "java"},
	{"dotnet", "dotnet"},
	{"gh ", "gh"},
	{"git ", "git"},
	{"jq ", "jq"},
	{"yq ", "yq"},
	{"ansible", "ansible"},
	{"packer", "packer"},
	{"cosign", "cosign"},
	{"kind", "kind"},
	{"minikube", "minikube"},
	{"k3d", "k3d"},
	{"flux", "flux"},
	{"argocd", "argocd"},
	{"istioctl", "istioctl"},
	{"cilium", "cilium"},
}

// scanFileForTools reads a file and looks for tool name references.
func scanFileForTools(path, source string, add func(string, string)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.ToLower(scanner.Text())
		for _, kw := range ciKeywords {
			if strings.Contains(line, kw.pattern) {
				add(kw.tool, source)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		slog.Debug("scanner error reading CI file", "path", path, "error", err)
	}
}

// scanDockerfile looks for FROM images that imply tools.
var dockerFromRe = regexp.MustCompile(`(?i)^FROM\s+(\S+)`)

func scanDockerfile(path string, add func(string, string)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	add("docker", "Dockerfile")

	// Map Docker image base names to catalog tool names.
	imageTools := map[string]string{
		"node":    "node",
		"golang":  "go",
		"python":  "python3",
		"rust":    "cargo",
		"dotnet":  "dotnet",
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if m := dockerFromRe.FindStringSubmatch(line); len(m) > 1 {
			image := strings.Split(m[1], ":")[0]                                   // strip tag
			image = strings.Split(image, "/")[len(strings.Split(image, "/"))-1]    // strip registry
			if tool, ok := imageTools[strings.ToLower(image)]; ok {
				add(tool, "Dockerfile (FROM)")
			}
		}
	}
	if err := scanner.Err(); err != nil {
		slog.Debug("scanner error reading Dockerfile", "path", path, "error", err)
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
