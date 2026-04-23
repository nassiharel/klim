package favorites

import (
	"os"
	"path/filepath"
	"testing"
)

func setup(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)           // Windows
	t.Setenv("XDG_CONFIG_HOME", dir)    // Linux
	t.Setenv("HOME", dir)              // macOS fallback
}

func TestLoadEmpty(t *testing.T) {
	setup(t)
	names, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("expected empty, got %v", names)
	}
}

func TestAddAndLoad(t *testing.T) {
	setup(t)
	if err := Add("git"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := Add("fzf"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Duplicate add — should be no-op.
	if err := Add("git"); err != nil {
		t.Fatalf("Add dup: %v", err)
	}

	names, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("expected 2, got %d: %v", len(names), names)
	}
	// Save sorts alphabetically.
	if names[0] != "fzf" || names[1] != "git" {
		t.Fatalf("unexpected order: %v", names)
	}
}

func TestRemove(t *testing.T) {
	setup(t)
	_ = Add("git")
	_ = Add("fzf")
	_ = Add("bat")

	if err := Remove("fzf"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	names, _ := Load()
	if len(names) != 2 {
		t.Fatalf("expected 2 after remove, got %d", len(names))
	}

	// Remove absent — no error.
	if err := Remove("nonexistent"); err != nil {
		t.Fatalf("Remove absent: %v", err)
	}
}

func TestContains(t *testing.T) {
	setup(t)
	_ = Add("git")

	ok, err := Contains("git")
	if err != nil || !ok {
		t.Fatalf("expected true, got %v %v", ok, err)
	}
	ok, err = Contains("nope")
	if err != nil || ok {
		t.Fatalf("expected false, got %v %v", ok, err)
	}
}

func TestToggle(t *testing.T) {
	setup(t)

	added, err := Toggle("git")
	if err != nil || !added {
		t.Fatalf("Toggle add: added=%v err=%v", added, err)
	}

	added, err = Toggle("git")
	if err != nil || added {
		t.Fatalf("Toggle remove: added=%v err=%v", added, err)
	}

	names, _ := Load()
	if len(names) != 0 {
		t.Fatalf("expected empty after toggle off, got %v", names)
	}
}

func TestSet(t *testing.T) {
	setup(t)
	_ = Add("git")
	_ = Add("fzf")

	s, err := Set()
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if !s["git"] || !s["fzf"] || s["nope"] {
		t.Fatalf("unexpected set: %v", s)
	}
}

func TestStoragePath(t *testing.T) {
	setup(t)
	p, err := StoragePath()
	if err != nil {
		t.Fatalf("StoragePath: %v", err)
	}
	if filepath.Base(p) != "favorites.yaml" {
		t.Fatalf("unexpected path: %s", p)
	}
}

func TestSaveCreatesDir(t *testing.T) {
	setup(t)
	if err := Save([]string{"git"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	p, _ := StoragePath()
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("file not created: %v", err)
	}
}
