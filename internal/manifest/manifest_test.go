package manifest

import (
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/klim/internal/registry"
)

func TestFromRegistryTool_PopulatesEverything(t *testing.T) {
	tool := registry.Tool{
		Name:        "git",
		DisplayName: "Git",
		Category:    "VCS",
		Packages: registry.PackageIDs{
			Winget: "Git.Git",
			Choco:  "git",
			Scoop:  "git",
			Brew:   "git",
			Apt:    "git",
			Snap:   "",
			NPM:    "",
		},
		Instances: []registry.Instance{
			{Path: "/usr/bin/git", Version: "2.43.0", Source: registry.SourceBrew},
		},
	}
	got := FromRegistryTool(tool)
	if got.Name != "git" {
		t.Errorf("Name: want git, got %s", got.Name)
	}
	if got.DisplayName != "Git" {
		t.Errorf("DisplayName: want Git, got %s", got.DisplayName)
	}
	if got.Category != "VCS" {
		t.Errorf("Category: want VCS, got %s", got.Category)
	}
	if got.Version != "2.43.0" {
		t.Errorf("Version (from PrimaryInstance): want 2.43.0, got %s", got.Version)
	}
	if got.Source != string(registry.SourceBrew) {
		t.Errorf("Source: want brew, got %s", got.Source)
	}
	if got.Packages.Winget != "Git.Git" {
		t.Errorf("Packages.Winget: want Git.Git, got %s", got.Packages.Winget)
	}
}

func TestFromRegistryTool_NotInstalledLeavesVersionAndSourceEmpty(t *testing.T) {
	tool := registry.Tool{
		Name:     "kubectl",
		Category: "Containers",
		// No Instances → PrimaryInstance returns nil.
	}
	got := FromRegistryTool(tool)
	if got.Version != "" {
		t.Errorf("Version should be empty when not installed, got %q", got.Version)
	}
	if got.Source != "" {
		t.Errorf("Source should be empty when not installed, got %q", got.Source)
	}
}

func TestManifest_RoundTrip(t *testing.T) {
	original := Manifest{
		GeneratedBy: "klim export",
		OS:          "windows",
		Arch:        "amd64",
		Tools: []Tool{
			{
				Name:        "git",
				DisplayName: "Git",
				Version:     "2.43.0",
				Source:      "winget",
				Category:    "VCS",
				Packages: Packages{
					Winget: "Git.Git",
					Brew:   "git",
					Apt:    "git",
				},
			},
			{
				Name:        "fzf",
				DisplayName: "fzf",
				Category:    "CLI",
				Packages:    Packages{Winget: "junegunn.fzf"},
			},
		},
	}

	data, err := yaml.Marshal(&original)
	if err != nil {
		t.Fatal(err)
	}

	var restored Manifest
	if err := yaml.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}

	if restored.GeneratedBy != original.GeneratedBy {
		t.Errorf("GeneratedBy = %q, want %q", restored.GeneratedBy, original.GeneratedBy)
	}
	if restored.OS != original.OS {
		t.Errorf("OS = %q, want %q", restored.OS, original.OS)
	}
	if restored.Arch != original.Arch {
		t.Errorf("Arch = %q, want %q", restored.Arch, original.Arch)
	}
	if len(restored.Tools) != len(original.Tools) {
		t.Fatalf("Tools count = %d, want %d", len(restored.Tools), len(original.Tools))
	}
	for i := range original.Tools {
		if restored.Tools[i].Name != original.Tools[i].Name {
			t.Errorf("Tool[%d].Name = %q, want %q", i, restored.Tools[i].Name, original.Tools[i].Name)
		}
		if restored.Tools[i].Packages.Winget != original.Tools[i].Packages.Winget {
			t.Errorf("Tool[%d].Packages.Winget = %q, want %q", i, restored.Tools[i].Packages.Winget, original.Tools[i].Packages.Winget)
		}
	}
}

func TestManifest_OmitEmpty(t *testing.T) {
	m := Manifest{
		Tools: []Tool{
			{Name: "git", Category: "VCS"},
		},
	}

	data, err := yaml.Marshal(&m)
	if err != nil {
		t.Fatal(err)
	}

	s := string(data)
	// version, source, and packages fields should be omitted when empty.
	if contains(s, "version:") {
		t.Error("expected version to be omitted")
	}
	if contains(s, "source:") {
		t.Error("expected source to be omitted")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && stringContains(s, substr)
}

func stringContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
