package catalog

import (
	"testing"
)

func TestDiff_NewTools(t *testing.T) {
	local := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
`)
	remote := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
  - name: fzf
    display_name: fzf
    category: CLI
    binary_names: [fzf]
`)

	result := Diff(local, remote)

	if !result.HasChanges() {
		t.Fatal("expected HasChanges=true")
	}
	if len(result.NewTools) != 1 || result.NewTools[0] != "fzf" {
		t.Errorf("NewTools = %v, want [fzf]", result.NewTools)
	}
	if len(result.ChangedTools) != 0 {
		t.Errorf("ChangedTools = %v, want empty", result.ChangedTools)
	}
}

func TestDiff_ChangedFields(t *testing.T) {
	local := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
`)
	remote := []byte(`tools:
  - name: git
    display_name: Git SCM
    category: Version Control
    binary_names: [git]
`)

	result := Diff(local, remote)

	if !result.HasChanges() {
		t.Fatal("expected HasChanges=true")
	}
	changes, ok := result.ChangedTools["git"]
	if !ok {
		t.Fatal("expected git in ChangedTools")
	}
	if len(changes) != 2 {
		t.Errorf("expected 2 changed fields, got %d: %v", len(changes), changes)
	}
}

func TestDiff_NoChanges(t *testing.T) {
	data := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
    packages:
      brew: git
`)

	result := Diff(data, data)

	if result.HasChanges() {
		t.Errorf("expected no changes, got NewTools=%v ChangedTools=%v",
			result.NewTools, result.ChangedTools)
	}
}

func TestDiff_EmptyLocal(t *testing.T) {
	remote := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
`)

	result := Diff(nil, remote)

	if len(result.NewTools) != 1 {
		t.Errorf("expected 1 new tool, got %d", len(result.NewTools))
	}
}

func TestDiff_EmptyRemote(t *testing.T) {
	local := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
`)

	result := Diff(local, nil)

	if result.HasChanges() {
		t.Error("expected no changes when remote is empty (removals are ignored)")
	}
}

func TestDiff_PackagesChanged(t *testing.T) {
	local := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
    packages:
      brew: git
`)
	remote := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
    packages:
      brew: git
      winget: Git.Git
`)

	result := Diff(local, remote)

	if !result.HasChanges() {
		t.Fatal("expected changes when packages differ")
	}
	changes := result.ChangedTools["git"]
	found := false
	for _, c := range changes {
		if c == "packages" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'packages' in changed fields, got %v", changes)
	}
}

func TestDiff_TagsChanged(t *testing.T) {
	local := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
    tags: [vcs]
`)
	remote := []byte(`tools:
  - name: git
    display_name: Git
    category: VCS
    binary_names: [git]
    tags: [vcs, scm, version-control]
`)

	result := Diff(local, remote)

	if !result.HasChanges() {
		t.Fatal("expected changes when tags differ")
	}
}

func TestIsValidCatalog(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"valid", []byte("tools:\n  - name: git\n"), true},
		{"empty tools", []byte("tools: []\n"), false},
		{"invalid yaml", []byte("{{invalid"), false},
		{"empty", []byte(""), false},
		{"nil", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidCatalog(tt.data)
			if got != tt.want {
				t.Errorf("isValidCatalog() = %v, want %v", got, tt.want)
			}
		})
	}
}
