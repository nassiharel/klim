package registry

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDefsToTools(t *testing.T) {
	defs := []ToolDef{
		{
			Name:        "git",
			DisplayName: "Git",
			Category:    "VCS",
			Tags:        []string{"vcs"},
			BinaryNames: []string{"git"},
			Packages:    PackageDef{Brew: "git", Winget: "Git.Git"},
		},
		{
			Name: "mytool", // no display name or binary names — should use defaults
		},
	}

	tools := defsToTools(defs)

	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	// First tool: all fields populated.
	if tools[0].Name != "git" {
		t.Errorf("tool 0 Name = %q, want git", tools[0].Name)
	}
	if tools[0].DisplayName != "Git" {
		t.Errorf("tool 0 DisplayName = %q, want Git", tools[0].DisplayName)
	}
	if tools[0].Packages.Brew != "git" {
		t.Errorf("tool 0 Packages.Brew = %q, want git", tools[0].Packages.Brew)
	}

	// Second tool: defaults applied.
	if tools[1].DisplayName != "mytool" {
		t.Errorf("tool 1 DisplayName = %q, want mytool (default from name)", tools[1].DisplayName)
	}
	if len(tools[1].BinaryNames) != 1 || tools[1].BinaryNames[0] != "mytool" {
		t.Errorf("tool 1 BinaryNames = %v, want [mytool]", tools[1].BinaryNames)
	}
}

func TestParseToolDefs(t *testing.T) {
	tests := []struct {
		name string
		data string
		want int // expected tool count, -1 for nil
	}{
		{"valid", "tools:\n  - name: git\n  - name: fzf\n", 2},
		{"single tool", "tools:\n  - name: git\n", 1},
		{"empty tools", "tools: []\n", 0},
		{"invalid yaml", "{{not yaml", -1},
		{"empty", "", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseToolDefs([]byte(tt.data))
			if tt.want == -1 {
				if got != nil {
					t.Errorf("parseToolDefs() = %v, want nil", got)
				}
			} else {
				if len(got) != tt.want {
					t.Errorf("parseToolDefs() returned %d tools, want %d", len(got), tt.want)
				}
			}
		})
	}
}

func TestParsePacksFromBytes(t *testing.T) {
	t.Run("valid packs", func(t *testing.T) {
		data := []byte(`tools:
  - name: git
    display_name: Git
    binary_names: [git]
packs:
  - name: my-pack
    display_name: My Pack
    description: A test pack.
    tools: [git, gh, fzf]
  - name: mini
    tools: [git]
`)
		packs, err := ParsePacksFromBytes(data)
		if err != nil {
			t.Fatal(err)
		}
		if len(packs) != 2 {
			t.Fatalf("expected 2 packs, got %d", len(packs))
		}
		if packs[0].Name != "my-pack" {
			t.Errorf("pack 0 name = %q, want my-pack", packs[0].Name)
		}
		if packs[0].DisplayName != "My Pack" {
			t.Errorf("pack 0 display_name = %q, want My Pack", packs[0].DisplayName)
		}
		if packs[0].Description != "A test pack." {
			t.Errorf("pack 0 description = %q, want A test pack.", packs[0].Description)
		}
		if len(packs[0].ToolNames) != 3 {
			t.Errorf("pack 0 tools = %v, want 3 tools", packs[0].ToolNames)
		}
		// Mini pack: display_name defaults to name.
		if packs[1].DisplayName != "mini" {
			t.Errorf("pack 1 display_name = %q, want mini (defaulted from name)", packs[1].DisplayName)
		}
	})

	t.Run("no packs section", func(t *testing.T) {
		data := []byte(`tools:
  - name: git
    binary_names: [git]
`)
		packs, err := ParsePacksFromBytes(data)
		if err != nil {
			t.Fatal(err)
		}
		if len(packs) != 0 {
			t.Errorf("expected 0 packs, got %d", len(packs))
		}
	})

	t.Run("empty packs", func(t *testing.T) {
		data := []byte(`tools: []
packs: []
`)
		packs, err := ParsePacksFromBytes(data)
		if err != nil {
			t.Fatal(err)
		}
		if len(packs) != 0 {
			t.Errorf("expected 0 packs, got %d", len(packs))
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		_, err := ParsePacksFromBytes([]byte("{{invalid"))
		if err == nil {
			t.Error("expected error for invalid YAML")
		}
	})

	t.Run("nil input", func(t *testing.T) {
		packs, err := ParsePacksFromBytes(nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(packs) != 0 {
			t.Errorf("expected 0 packs for nil input, got %d", len(packs))
		}
	})
}

// findRepoRoot walks up from the current directory to find go.mod.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (go.mod)")
		}
		dir = parent
	}
}

// TestMarketplaceYAML_Integrity validates the marketplace by assembling
// individual tool and pack files from marketplace/tools/ and marketplace/packs/,
// then checking for duplicate names, empty fields, and dangling pack references.
func TestMarketplaceYAML_Integrity(t *testing.T) {
	root := findRepoRoot(t)

	// Assemble tools from individual files.
	toolFiles, err := filepath.Glob(filepath.Join(root, "marketplace", "tools", "*.yaml"))
	if err != nil {
		t.Fatalf("globbing tool files: %v", err)
	}
	if len(toolFiles) == 0 {
		t.Fatal("no tool files found in marketplace/tools/")
	}

	toolNames := make(map[string]struct{}, len(toolFiles))
	for _, f := range toolFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		var td ToolDef
		if err := yaml.Unmarshal(data, &td); err != nil {
			t.Fatalf("parsing %s: %v", f, err)
		}

		if td.Name == "" {
			t.Errorf("%s: tool with empty name", filepath.Base(f))
		}
		if _, exists := toolNames[td.Name]; exists {
			t.Errorf("duplicate tool name: %q", td.Name)
		}
		toolNames[td.Name] = struct{}{}

		if len(td.BinaryNames) == 0 {
			t.Errorf("tool %q has no binary_names", td.Name)
		}
	}

	// Assemble packs from individual files.
	packFiles, err := filepath.Glob(filepath.Join(root, "marketplace", "packs", "*.yaml"))
	if err != nil {
		t.Fatalf("globbing pack files: %v", err)
	}

	packNames := make(map[string]struct{}, len(packFiles))
	for _, f := range packFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		var pd struct {
			Name  string   `yaml:"name"`
			Tools []string `yaml:"tools"`
		}
		if err := yaml.Unmarshal(data, &pd); err != nil {
			t.Fatalf("parsing %s: %v", f, err)
		}

		if pd.Name == "" {
			t.Errorf("%s: pack with empty name", filepath.Base(f))
		}
		if _, exists := packNames[pd.Name]; exists {
			t.Errorf("duplicate pack name: %q", pd.Name)
		}
		packNames[pd.Name] = struct{}{}

		if len(pd.Tools) == 0 {
			t.Errorf("pack %q has no tools", pd.Name)
		}

		for _, toolName := range pd.Tools {
			if _, ok := toolNames[toolName]; !ok {
				t.Errorf("pack %q references undefined tool %q", pd.Name, toolName)
			}
		}

		seen := make(map[string]struct{}, len(pd.Tools))
		for _, toolName := range pd.Tools {
			if _, exists := seen[toolName]; exists {
				t.Errorf("pack %q has duplicate tool reference %q", pd.Name, toolName)
			}
			seen[toolName] = struct{}{}
		}
	}

	t.Logf("marketplace: %d tools, %d packs — all valid", len(toolFiles), len(packFiles))
}
