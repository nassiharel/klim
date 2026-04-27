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

// DetectResult holds detection output including stats.
type DetectResult struct {
	Tools      []DetectedTool
	Suggestions []DetectedTool // tools suggested based on ecosystem (not from files)
	FilesScanned int
	DirsScanned  int
}

// DetectFromProject scans a project directory for configuration files and
// infers which CLI tools the project requires. Returns deduplicated tool names
// sorted by name, with the source file that triggered each detection.
//
// Tool names MUST match the marketplace catalog (e.g. "go" not "golang",
// "node" not "nodejs", "docker-compose" not "docker compose").
func DetectFromProject(dir string) DetectResult {
	// Guard: refuse to scan filesystem roots. Also cap total files scanned.
	absDir, _ := filepath.Abs(dir)
	if isFilesystemRoot(absDir) {
		return DetectResult{}
	}

	seen := make(map[string]string) // name → source
	var results []DetectedTool
	filesScanned := 0
	dirsScanned := 0
	const maxFiles = 10000 // safety cap

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
		{"pip.conf", []string{"python3"}},
		{"pip.ini", []string{"python3"}},
		{"pytest.ini", []string{"python3"}},
		{"setup.cfg", []string{"python3"}},
		{"uv.lock", []string{"uv", "python3"}},
		{"uv.toml", []string{"uv"}},
		// Ruby
		{"Gemfile", nil},           // ruby not in catalog
		{".ruby-version", nil},
		// Java / JVM
		{"pom.xml", []string{"java"}},
		{"build.gradle", []string{"java"}},
		{"build.gradle.kts", []string{"java"}},
		// .NET
		{"global.json", []string{"dotnet"}},
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

	// --- Recursive file search (max depth 3) ---
	// Catches Dockerfiles, pyproject.toml, Chart.yaml, *.bicep, *.tf, etc.
	// in subdirectories like services/*/Dockerfile, helm-charts/*/Chart.yaml.
	skipDirs := map[string]bool{
		"node_modules": true, ".git": true, ".venv": true, "venv": true,
		"__pycache__": true, ".terraform": true, "vendor": true, "dist": true,
		"build": true, "bin": true, "obj": true, ".next": true, "target": true,
	}

	type walkRule struct {
		filename string   // exact filename match (case-insensitive)
		ext      string   // extension match (e.g. ".tf", ".bicep")
		tools    []string // catalog tool names
	}
	walkRules := []walkRule{
		{filename: "Dockerfile", tools: []string{"docker"}},
		{filename: "pyproject.toml", tools: []string{"python3"}},
		{filename: "requirements.txt", tools: []string{"python3"}},
		{filename: "setup.py", tools: []string{"python3"}},
		{filename: "Chart.yaml", tools: []string{"helm", "kubectl"}},
		{filename: "values.yaml", tools: []string{"helm"}},
		{filename: "Cargo.toml", tools: []string{"cargo"}},
		{filename: "go.mod", tools: []string{"go"}},
		{filename: "package.json", tools: []string{"node", "npm"}},
		{filename: "global.json", tools: []string{"dotnet"}},
		{filename: "pytest.ini", tools: []string{"python3"}},
		{filename: "setup.cfg", tools: []string{"python3"}},
		{ext: ".tf", tools: []string{"terraform"}},
		{ext: ".bicep", tools: []string{"az"}},
		{ext: ".csproj", tools: []string{"dotnet"}},
		{ext: ".sln", tools: []string{"dotnet"}},
		{ext: ".fsproj", tools: []string{"dotnet"}},
	}

	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		// Safety cap — abort scan if project is too large.
		if filesScanned >= maxFiles {
			return filepath.SkipAll
		}
		// Compute depth relative to root.
		rel, _ := filepath.Rel(dir, path)
		depth := strings.Count(rel, string(filepath.Separator))
		if depth > 3 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip junk directories.
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			dirsScanned++
			return nil
		}
		filesScanned++
		name := d.Name()
		lowerName := strings.ToLower(name)
		for _, rule := range walkRules {
			if rule.filename != "" && strings.EqualFold(name, rule.filename) {
				for _, t := range rule.tools {
					add(t, rel)
				}
			}
			if rule.ext != "" && strings.HasSuffix(lowerName, rule.ext) {
				for _, t := range rule.tools {
					add(t, rel)
				}
			}
		}
		// Scan Dockerfile.* variants.
		if strings.HasPrefix(lowerName, "dockerfile") {
			add("docker", rel)
		}
		return nil
	})

	// --- CI/CD file scanning ---
	// CI workflow patterns — use filepath.Join for cross-platform separators.
	ghWorkflowDir := filepath.Join(dir, ".github", "workflows")
	ciGlobs := []string{
		filepath.Join(ghWorkflowDir, "*.yml"),
		filepath.Join(ghWorkflowDir, "*.yaml"),
	}
	ciFiles := []string{
		".gitlab-ci.yml",
		"azure-pipelines.yml",
		"azure-pipelines.yaml",
		".circleci/config.yml",
	}

	// Also glob for azure-pipelines*.yml variants.
	azPipeGlobs := []string{
		"azure-pipelines*.yml",
		"azure-pipelines*.yaml",
		"azurepipelines*.yml",
		"azurepipelines*.yaml",
	}

	for _, pattern := range ciGlobs {
		matches, _ := filepath.Glob(pattern)
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
	for _, pattern := range azPipeGlobs {
		matches, _ := filepath.Glob(filepath.Join(dir, pattern))
		for _, f := range matches {
			scanFileForTools(f, filepath.Base(f), add)
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

	// Ecosystem-based suggestions: if we detected certain languages/platforms,
	// suggest commonly paired tools that weren't explicitly detected.
	suggestions := suggestEcosystemTools(seen)

	return DetectResult{
		Tools:        results,
		Suggestions:  suggestions,
		FilesScanned: filesScanned,
		DirsScanned:  dirsScanned,
	}
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

	source := filepath.Base(path)
	add("docker", source)

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
				add(tool, source+" (FROM)")
			}
		}
	}
	if err := scanner.Err(); err != nil {
		slog.Debug("scanner error reading Dockerfile", "path", path, "error", err)
	}
}

// suggestEcosystemTools returns additional tool suggestions based on
// which ecosystems were detected. These are common companion tools that
// weren't explicitly found in project files.
func suggestEcosystemTools(detected map[string]string) []DetectedTool {
	type suggestion struct {
		ifDetected string // if this tool was detected...
		suggest    string // ...suggest this companion
		reason     string
	}

	rules := []suggestion{
		// Python ecosystem
		{"python3", "uv", "fast Python package manager"},
		{"python3", "docker", "containerize Python services"},
		{"python3", "jq", "process JSON output from APIs"},

		// Go ecosystem
		{"go", "golangci-lint", "lint Go code"},
		{"go", "docker", "containerize Go binaries"},

		// Node ecosystem
		{"node", "docker", "containerize Node services"},

		// Kubernetes ecosystem
		{"kubectl", "k9s", "interactive Kubernetes dashboard"},
		{"kubectl", "helm", "manage Kubernetes packages"},
		{"helm", "kubectl", "deploy Helm charts"},

		// Docker ecosystem
		{"docker", "docker-compose", "multi-container orchestration"},

		// Azure ecosystem
		{"az", "kubectl", "manage AKS clusters"},
		{"az", "terraform", "infrastructure as code"},
		{"az", "helm", "deploy to AKS"},

		// Terraform ecosystem
		{"terraform", "az", "provision Azure resources"},

		// Git ecosystem
		{"git", "gh", "GitHub CLI for PRs and issues"},

		// .NET ecosystem
		{"dotnet", "docker", "containerize .NET services"},
		{"dotnet", "az", "deploy to Azure"},
	}

	var suggestions []DetectedTool
	seen := make(map[string]bool)
	for _, r := range rules {
		if _, ok := detected[r.ifDetected]; !ok {
			continue // prerequisite not detected
		}
		if _, ok := detected[r.suggest]; ok {
			continue // already detected
		}
		if seen[r.suggest] {
			continue // already suggested
		}
		seen[r.suggest] = true
		suggestions = append(suggestions, DetectedTool{
			Name:   r.suggest,
			Source: "suggested: " + r.reason,
		})
	}
	return suggestions
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// isFilesystemRoot checks if a path is a filesystem root (/, C:\, etc.).
func isFilesystemRoot(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return abs == filepath.Dir(abs)
}
