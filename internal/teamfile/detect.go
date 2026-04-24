package teamfile

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DetectedTool represents a tool detected from project files.
type DetectedTool struct {
	Name   string // tool name (matches marketplace catalog)
	Source string // which file triggered detection (e.g. "Dockerfile", ".github/workflows/ci.yml")
}

// DetectFromProject scans a project directory for configuration files and
// infers which CLI tools the project requires. Returns deduplicated tool names
// with the source file that triggered each detection.
func DetectFromProject(dir string) []DetectedTool {
	seen := make(map[string]string) // name → source
	var results []DetectedTool

	add := func(name, source string) {
		if _, ok := seen[name]; !ok {
			seen[name] = source
			results = append(results, DetectedTool{Name: name, Source: source})
		}
	}

	// --- File existence checks ---
	fileToolMap := map[string][]string{
		"Dockerfile":          {"docker"},
		"docker-compose.yml":  {"docker", "docker-compose"},
		"docker-compose.yaml": {"docker", "docker-compose"},
		"compose.yml":         {"docker", "docker-compose"},
		"compose.yaml":        {"docker", "docker-compose"},
		"Makefile":            {"make"},
		"Justfile":            {"just"},
		"Taskfile.yml":        {"go-task"},
		"Taskfile.yaml":       {"go-task"},
		"package.json":        {"node", "npm"},
		"bun.lockb":           {"bun"},
		"deno.json":           {"deno"},
		"deno.jsonc":          {"deno"},
		"go.mod":              {"go"},
		"go.sum":              {"go"},
		".golangci.yml":       {"golangci-lint"},
		".golangci.yaml":      {"golangci-lint"},
		".golangci.toml":      {"golangci-lint"},
		".golangci.json":      {"golangci-lint"},
		".goreleaser.yml":     {"go"},
		".goreleaser.yaml":    {"go"},
		"Cargo.toml":          {"cargo"},
		"Cargo.lock":          {"cargo"},
		"rustfmt.toml":        {"cargo"},
		"clippy.toml":         {"cargo"},
		"pyproject.toml":      {"python3"},
		"requirements.txt":    {"python3"},
		"Pipfile":             {"python3"},
		"setup.py":            {"python3"},
		"Gemfile":             {"ruby"},
		"pom.xml":             {"java", "mvn"},
		"build.gradle":        {"java", "gradle"},
		"build.gradle.kts":    {"java", "gradle"},
		"CMakeLists.txt":      {"cmake"},
		".nvmrc":              {"node", "nvm"},
		".node-version":       {"node"},
		".python-version":     {"python3"},
		".ruby-version":       {"ruby"},
		".tool-versions":      {"asdf"},
		".mise.toml":          {"mise"},
		"ansible.cfg":         {"ansible"},
		"playbook.yml":        {"ansible"},
		"flake.nix":           {"nix"},
		".pre-commit-config.yaml": {"git"},
		".eslintrc.json":      {"node"},
		".eslintrc.yml":       {"node"},
		".prettierrc":         {"node"},
		".prettierrc.json":    {"node"},
		"tsconfig.json":       {"node"},
		"yarn.lock":           {"node"},
		"pnpm-lock.yaml":      {"node"},
		"package-lock.json":   {"node", "npm"},
		".dockerignore":       {"docker"},
		".helmignore":         {"helm"},
		"skaffold.yaml":       {"docker", "kubectl"},
		"tilt_config.json":    {"docker"},
		"Tiltfile":            {"docker"},
		"Vagrantfile":         {"vagrant"},
		"Procfile":            {"node"},
		".terraform.lock.hcl": {"terraform"},
		"terragrunt.hcl":      {"terraform"},
		".trivyignore":        {"trivy"},
		"sonar-project.properties": {"sonar-scanner"},
		"buf.yaml":            {"buf"},
		"buf.gen.yaml":        {"buf"},
		".nvim.lua":           {"nvim"},
		".vscode/settings.json": {"code"},
		"renovate.json":       {"git"},
		"dependabot.yml":      {"git"},
	}

	for file, tools := range fileToolMap {
		if fileExists(filepath.Join(dir, file)) {
			for _, t := range tools {
				add(t, file)
			}
		}
	}

	// --- Directory existence checks ---
	dirToolMap := map[string][]string{
		".github":    {"gh"},
		".gitlab":    {"git"},
		"terraform":  {"terraform"},
		"helm":       {"helm", "kubectl"},
		"charts":     {"helm", "kubectl"},
		"k8s":        {"kubectl"},
		"kubernetes": {"kubectl"},
		"kustomize":  {"kubectl", "kustomize"},
		".azure":     {"az"},
		"bicep":      {"az"},
		"pulumi":     {"pulumi"},
		"ansible":    {"ansible"},
	}

	for d, tools := range dirToolMap {
		if dirExists(filepath.Join(dir, d)) {
			for _, t := range tools {
				add(t, d+"/")
			}
		}
	}

	// --- Glob pattern checks ---
	globPatterns := map[string][]string{
		"*.tf":       {"terraform"},
		"*.tf.json":  {"terraform"},
		"*.bicep":    {"az"},
		"Chart.yaml": {"helm"},
	}

	for pattern, tools := range globPatterns {
		matches, _ := filepath.Glob(filepath.Join(dir, pattern))
		if len(matches) == 0 {
			matches, _ = filepath.Glob(filepath.Join(dir, "**", pattern))
		}
		if len(matches) > 0 {
			for _, t := range tools {
				add(t, pattern)
			}
		}
	}

	// --- CI/CD file scanning ---
	ciFiles := []string{
		".github/workflows/*.yml",
		".github/workflows/*.yaml",
		".gitlab-ci.yml",
		"azure-pipelines.yml",
		"Jenkinsfile",
		".circleci/config.yml",
	}

	for _, pattern := range ciFiles {
		matches, _ := filepath.Glob(filepath.Join(dir, pattern))
		for _, f := range matches {
			scanFileForTools(f, filepath.Base(f), add)
		}
	}

	// --- Dockerfile scanning ---
	for _, df := range []string{"Dockerfile", "Dockerfile.*"} {
		matches, _ := filepath.Glob(filepath.Join(dir, df))
		for _, f := range matches {
			scanDockerfile(f, add)
		}
	}

	return results
}

// scanFileForTools reads a file and looks for tool name references.
func scanFileForTools(path, source string, add func(string, string)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	// Tool keywords to detect in CI/config files.
	keywords := map[string]string{
		"kubectl":        "kubectl",
		"helm":           "helm",
		"terraform":      "terraform",
		"docker":         "docker",
		"docker-compose": "docker-compose",
		"az ":            "az",
		"az login":       "az",
		"aws ":           "aws",
		"gcloud":         "gcloud",
		"node ":          "node",
		"npm ":           "npm",
		"yarn":           "node",
		"pnpm":           "node",
		"go build":       "go",
		"go test":        "go",
		"go run":         "go",
		"golangci-lint":  "golangci-lint",
		"staticcheck":    "go",
		"goreleaser":     "go",
		"cargo":          "cargo",
		"rustup":         "cargo",
		"python":         "python3",
		"pip ":           "python3",
		"java":           "java",
		"mvn":            "mvn",
		"gradle":         "gradle",
		"dotnet":         "dotnet",
		"gh ":            "gh",
		"git ":           "git",
		"jq ":            "jq",
		"yq ":            "yq",
		"curl":           "curl",
		"make ":          "make",
		"ansible":        "ansible",
		"packer":         "packer",
		"vault":          "vault",
		"consul":         "consul",
		"cosign":         "cosign",
		"trivy":          "trivy",
		"k9s":            "k9s",
		"kind":           "kind",
		"minikube":       "minikube",
		"k3d":            "k3d",
		"flux":           "flux",
		"argocd":         "argocd",
		"istioctl":       "istioctl",
		"cilium":         "cilium",
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.ToLower(scanner.Text())
		for keyword, tool := range keywords {
			if strings.Contains(line, keyword) {
				add(tool, source)
			}
		}
	}
}

// scanDockerfile looks for FROM images and RUN commands that imply tools.
var dockerFromRe = regexp.MustCompile(`(?i)^FROM\s+(\S+)`)

func scanDockerfile(path string, add func(string, string)) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	add("docker", "Dockerfile")

	imageTools := map[string]string{
		"node":    "node",
		"golang":  "go",
		"python":  "python3",
		"rust":    "cargo",
		"ruby":    "ruby",
		"openjdk": "java",
		"maven":   "mvn",
		"gradle":  "gradle",
		"dotnet":  "dotnet",
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if m := dockerFromRe.FindStringSubmatch(line); len(m) > 1 {
			image := strings.Split(m[1], ":")[0] // strip tag
			image = strings.Split(image, "/")[len(strings.Split(image, "/"))-1] // strip registry
			if tool, ok := imageTools[strings.ToLower(image)]; ok {
				add(tool, "Dockerfile (FROM)")
			}
		}
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
