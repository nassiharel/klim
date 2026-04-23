package fileutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	if err := AtomicWrite(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("got %q, want %q", data, "hello")
	}
}

func TestAtomicWriteOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	_ = AtomicWrite(path, []byte("first"), 0o644)
	if err := AtomicWrite(path, []byte("second"), 0o644); err != nil {
		t.Fatalf("AtomicWrite overwrite: %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "second" {
		t.Fatalf("got %q, want %q", data, "second")
	}
}

func TestEnsureDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "file.txt")

	if err := EnsureDir(path); err != nil {
		t.Fatalf("EnsureDir: %v", err)
	}
	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("not a directory")
	}
}

type testYAML struct {
	Name  string   `yaml:"name"`
	Items []string `yaml:"items"`
}

func TestReadYAMLMissing(t *testing.T) {
	found, err := ReadYAML("/nonexistent/file.yaml", &testYAML{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatal("expected found=false for missing file")
	}
}

func TestReadYAMLPresent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	_ = os.WriteFile(path, []byte("name: foo\nitems:\n  - a\n  - b\n"), 0o644)

	var dest testYAML
	found, err := ReadYAML(path, &dest)
	if err != nil {
		t.Fatalf("ReadYAML: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if dest.Name != "foo" || len(dest.Items) != 2 {
		t.Fatalf("unexpected: %+v", dest)
	}
}

func TestReadYAMLBadContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	_ = os.WriteFile(path, []byte("{{invalid yaml"), 0o644)

	_, err := ReadYAML(path, &testYAML{})
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestWriteYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "out.yaml")

	src := testYAML{Name: "bar", Items: []string{"x"}}
	if err := WriteYAML(path, &src, "# header\n"); err != nil {
		t.Fatalf("WriteYAML: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data[:9]) != "# header\n" {
		t.Fatalf("header missing: %q", data[:20])
	}

	// Round-trip.
	var dest testYAML
	found, err := ReadYAML(path, &dest)
	if err != nil || !found {
		t.Fatalf("round-trip failed: found=%v err=%v", found, err)
	}
	if dest.Name != "bar" {
		t.Fatalf("got name=%q", dest.Name)
	}
}
