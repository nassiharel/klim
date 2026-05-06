package custompacks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nassiharel/klim/internal/registry"
)

func TestAddLoadDelete(t *testing.T) {
	// Redirect storage to temp dir.
	tmp := t.TempDir()
	t.Setenv("AppData", tmp)         // Windows
	t.Setenv("XDG_CONFIG_HOME", tmp) // Linux
	t.Setenv("HOME", tmp)            // macOS fallback

	pack := registry.Pack{
		Name:        "test-pack",
		DisplayName: "Test Pack",
		Description: "A test pack.",
		ToolNames:   []string{"git", "gh"},
	}

	// Add.
	if err := Add(pack); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Load.
	packs, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(packs) != 1 {
		t.Fatalf("expected 1 pack, got %d", len(packs))
	}
	if packs[0].Name != "test-pack" {
		t.Errorf("name = %q, want test-pack", packs[0].Name)
	}
	if len(packs[0].ToolNames) != 2 {
		t.Errorf("tools = %v, want 2", packs[0].ToolNames)
	}

	// Add second.
	pack2 := registry.Pack{Name: "pack2", ToolNames: []string{"docker"}}
	if err := Add(pack2); err != nil {
		t.Fatalf("Add pack2: %v", err)
	}
	packs, _ = Load()
	if len(packs) != 2 {
		t.Fatalf("expected 2 packs, got %d", len(packs))
	}

	// Replace existing.
	pack.ToolNames = []string{"git", "gh", "fzf"}
	if err := Add(pack); err != nil {
		t.Fatalf("Add (replace): %v", err)
	}
	packs, _ = Load()
	if len(packs) != 2 {
		t.Fatalf("expected 2 packs after replace, got %d", len(packs))
	}
	for _, p := range packs {
		if p.Name == "test-pack" && len(p.ToolNames) != 3 {
			t.Errorf("replaced pack tools = %v, want 3", p.ToolNames)
		}
	}

	// Delete.
	if err := Delete("test-pack"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	packs, _ = Load()
	if len(packs) != 1 {
		t.Fatalf("expected 1 pack after delete, got %d", len(packs))
	}
	if packs[0].Name != "pack2" {
		t.Errorf("remaining pack = %q, want pack2", packs[0].Name)
	}
}

func TestExists(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AppData", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)

	exists, err := Exists("nope")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("expected false for non-existent pack")
	}

	_ = Add(registry.Pack{Name: "yes", ToolNames: []string{"git"}})
	exists, _ = Exists("yes")
	if !exists {
		t.Error("expected true for existing pack")
	}
}

func TestLoadEmptyFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AppData", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)

	// No file → empty slice.
	packs, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(packs) != 0 {
		t.Errorf("expected 0 packs, got %d", len(packs))
	}
}

func TestDisplayNameDefault(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AppData", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)

	_ = Add(registry.Pack{Name: "no-display", ToolNames: []string{"git"}})
	packs, _ := Load()
	if packs[0].DisplayName != "no-display" {
		t.Errorf("display_name = %q, want no-display", packs[0].DisplayName)
	}
}

func TestStoragePath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AppData", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)

	path, err := StoragePath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "custom-packs.yaml" {
		t.Errorf("filename = %q, want custom-packs.yaml", filepath.Base(path))
	}
}

func TestDeleteNonExistent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AppData", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)

	// Deleting a pack that doesn't exist should not error.
	if err := Delete("ghost"); err != nil {
		t.Fatalf("Delete non-existent: %v", err)
	}

	// Verify file was created (empty packs).
	path, _ := StoragePath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected file to be created after Delete")
	}
}
