package generate

import (
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/registry"
	"github.com/nassiharel/klim/internal/teamfile"
)

func TestPmName(t *testing.T) {
	cases := map[string]string{
		"alpine":  "apk",
		"fedora":  "dnf",
		"macos":   "brew",
		"windows": "winget",
		"ubuntu":  "apt-get",
		"debian":  "apt-get",
		"":        "apt-get",
	}
	for in, want := range cases {
		if got := pmName(in); got != want {
			t.Errorf("pmName(%q): want %s, got %s", in, want, got)
		}
	}
}

func TestDefaultBaseImage(t *testing.T) {
	cases := map[string]string{
		"alpine":  "alpine:3.20",
		"fedora":  "fedora:41",
		"debian":  "debian:bookworm",
		"ubuntu":  "ubuntu:24.04",
		"unknown": "ubuntu:24.04",
		"":        "ubuntu:24.04",
	}
	for in, want := range cases {
		if got := defaultBaseImage(in); got != want {
			t.Errorf("defaultBaseImage(%q): want %s, got %s", in, want, got)
		}
	}
}

func TestBulkInstallCmd(t *testing.T) {
	if got := bulkInstallCmd("alpine", []string{"git", "curl"}); got != "apk add --no-cache git curl" {
		t.Errorf("alpine: got %q", got)
	}
	if got := bulkInstallCmd("fedora", []string{"git"}); got != "dnf install -y git" {
		t.Errorf("fedora: got %q", got)
	}
	if got := bulkInstallCmd("ubuntu", []string{"git", "curl"}); !strings.Contains(got, "apt-get install -y git curl") {
		t.Errorf("ubuntu: got %q", got)
	}
	if got := bulkInstallCmd("ubuntu", nil); got != "" {
		t.Errorf("empty pkgs should yield empty string, got %q", got)
	}
}

func TestResolveInstalls_MatchesCatalogAndPreservesUnknowns(t *testing.T) {
	tf := &teamfile.TeamFile{
		Tools: []teamfile.RequiredTool{
			{Name: "git", Version: ">=2.40"},
		},
		Optional: []teamfile.RequiredTool{
			{Name: "kubectl", Version: ""},
			{Name: "made-up-tool", Version: ""},
		},
	}
	catalog := []registry.Tool{
		{
			Name: "git",
			Packages: registry.PackageIDs{
				Brew:   "git",
				Apt:    "git",
				Winget: "Git.Git",
			},
		},
		{
			Name: "kubectl",
			Packages: registry.PackageIDs{
				Brew:   "kubectl",
				Apt:    "kubectl",
				Winget: "Kubernetes.kubectl",
			},
		},
	}
	installs := ResolveInstalls(tf, catalog)
	if len(installs) != 3 {
		t.Fatalf("want 3 installs (1 required + 2 optional), got %d", len(installs))
	}
	if installs[0].Name != "git" || installs[0].Brew != "git" || installs[0].Version != ">=2.40" {
		t.Errorf("git resolution wrong: %+v", installs[0])
	}
	if installs[1].Name != "kubectl" || installs[1].Brew != "kubectl" {
		t.Errorf("kubectl resolution wrong: %+v", installs[1])
	}
	// Unknown tool should still appear, with no package IDs filled in.
	if installs[2].Name != "made-up-tool" || installs[2].Brew != "" {
		t.Errorf("unknown tool should retain Name and have empty PMs: %+v", installs[2])
	}
}

func TestClassifyTools_PerOS(t *testing.T) {
	installs := []ToolInstall{
		{Name: "git", Apt: "git", Brew: "git", Winget: "Git.Git", Choco: "git", Scoop: "git"},
		{Name: "node", Brew: "node", NPM: "node"},
		{Name: "tool-snap-only", Snap: "tool-snap-only"},
		{Name: "tool-npm-only", NPM: "tool-npm-only"},
	}

	t.Run("ubuntu", func(t *testing.T) {
		sys, brew, npm, _, _, _, snap := classifyTools(installs, "ubuntu")
		// apt wins for git (priority order in classifyTools default branch).
		if !contains(sys, "git") {
			t.Errorf("ubuntu: git should land in sysPkgs (apt), got sys=%v", sys)
		}
		if !contains(brew, "node") {
			t.Errorf("ubuntu: node should land in brewPkgs (no apt for node here), got brew=%v", brew)
		}
		if !contains(snap, "tool-snap-only") {
			t.Errorf("ubuntu: snap-only tool should land in snap, got snap=%v", snap)
		}
		if !contains(npm, "tool-npm-only") {
			t.Errorf("ubuntu: npm-only tool should land in npm, got npm=%v", npm)
		}
	})

	t.Run("macos", func(t *testing.T) {
		_, brew, npm, _, _, _, _ := classifyTools(installs, "macos")
		if !contains(brew, "git") {
			t.Errorf("macos: git should land in brewPkgs, got %v", brew)
		}
		if !contains(npm, "tool-npm-only") {
			t.Errorf("macos: npm-only should fall through to npm, got %v", npm)
		}
	})

	t.Run("windows", func(t *testing.T) {
		_, _, _, winget, _, _, _ := classifyTools(installs, "windows")
		if !contains(winget, "Git.Git") {
			t.Errorf("windows: git should use winget id, got %v", winget)
		}
	})
}

func TestGitHubAction_OutputShape(t *testing.T) {
	installs := []ToolInstall{
		{Name: "git", Apt: "git", Brew: "git", Winget: "Git.Git"},
	}
	cases := []struct {
		os         string
		wantRunner string
		wantSetup  string
	}{
		{"ubuntu", "ubuntu-latest", "apt-get install -y git"},
		{"macos", "macos-latest", "brew install git"},
		{"windows", "windows-latest", "winget install --id Git.Git"},
		{"", "ubuntu-latest", "apt-get install -y git"}, // default
	}
	for _, c := range cases {
		got := GitHubAction(installs, Options{OS: c.os, ProjectName: "demo"})
		if !strings.Contains(got, "runs-on: "+c.wantRunner) {
			t.Errorf("os=%s: missing runner %q in:\n%s", c.os, c.wantRunner, got)
		}
		if !strings.Contains(got, c.wantSetup) {
			t.Errorf("os=%s: missing install line %q in:\n%s", c.os, c.wantSetup, got)
		}
		if !strings.Contains(got, "Project: demo") {
			t.Errorf("os=%s: ProjectName not surfaced: %s", c.os, got)
		}
	}
}

func TestDockerfile_PerOS(t *testing.T) {
	installs := []ToolInstall{
		{Name: "git", Apt: "git", Brew: "git"},
		{Name: "yarn", NPM: "yarn"},
	}
	cases := []struct {
		os, wantBase, wantInstall string
	}{
		{"alpine", "alpine:3.20", "apk add --no-cache"},
		{"fedora", "fedora:41", "dnf install -y"},
		{"ubuntu", "ubuntu:24.04", "apt-get update"},
	}
	for _, c := range cases {
		got := Dockerfile(installs, Options{OS: c.os})
		if !strings.Contains(got, "FROM "+c.wantBase) {
			t.Errorf("os=%s: missing FROM %s in:\n%s", c.os, c.wantBase, got)
		}
		if !strings.Contains(got, c.wantInstall) {
			t.Errorf("os=%s: missing %s in:\n%s", c.os, c.wantInstall, got)
		}
		if !strings.Contains(got, "npm install -g yarn") {
			t.Errorf("os=%s: npm tool missing: %s", c.os, got)
		}
	}

	// Custom BaseImage override.
	got := Dockerfile(installs, Options{OS: "ubuntu", BaseImage: "ubuntu:22.04"})
	if !strings.Contains(got, "FROM ubuntu:22.04") {
		t.Errorf("BaseImage override not honoured: %s", got)
	}
}

func TestDevContainer_FeaturesAndPostCreate(t *testing.T) {
	installs := []ToolInstall{
		{Name: "kubectl"},                 // → known feature
		{Name: "terraform"},               // → known feature
		{Name: "ripgrep", Apt: "ripgrep"}, // → postCreate (no feature)
		{Name: "yarn", NPM: "yarn"},       // → postCreate via npm
	}
	got := DevContainer(installs, Options{ProjectName: "myproj"})
	if !strings.Contains(got, `"name": "myproj"`) {
		t.Errorf("project name missing: %s", got)
	}
	if !strings.Contains(got, "kubectl-helm-minikube") {
		t.Errorf("kubectl feature should be referenced: %s", got)
	}
	if !strings.Contains(got, "terraform:1") {
		t.Errorf("terraform feature should be referenced: %s", got)
	}
	if !strings.Contains(got, "sudo apt-get install -y ripgrep") {
		t.Errorf("ripgrep postCreate via apt missing: %s", got)
	}
	if !strings.Contains(got, "npm install -g yarn") {
		t.Errorf("yarn postCreate via npm missing: %s", got)
	}
}

func TestDevContainer_DefaultProjectName(t *testing.T) {
	got := DevContainer(nil, Options{})
	if !strings.Contains(got, `"name": "klim-project"`) {
		t.Errorf("default project name missing: %s", got)
	}
}

func TestDevcontainerFeature(t *testing.T) {
	if devcontainerFeature(ToolInstall{Name: "go"}) == "" {
		t.Errorf("expected go feature")
	}
	if devcontainerFeature(ToolInstall{Name: "totally-fake"}) != "" {
		t.Errorf("expected empty for unknown tool")
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
