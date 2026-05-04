package fileutil

import (
	"os"
	"path/filepath"
	"runtime"
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

// TestAtomicWritePreservesSymlink asserts that writing through a
// symlink updates the target file rather than replacing the symlink
// with a regular file. Repos that keep .clim.yaml as a symlink to a
// shared template would otherwise lose their setup on the first
// `clim init --force`.
func TestAtomicWritePreservesSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Symlink creation on Windows requires admin or developer-mode
		// privileges; skip rather than gate the rest of CI on that.
		t.Skip("symlink creation not reliable on Windows CI")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(target, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if err := AtomicWrite(link, []byte("v2"), 0o644); err != nil {
		t.Fatalf("AtomicWrite via symlink: %v", err)
	}

	// The symlink must still be a symlink (Lstat doesn't follow it).
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("AtomicWrite replaced the symlink with a regular file (mode=%v)", info.Mode())
	}
	// And the target must hold the new bytes.
	got, _ := os.ReadFile(target)
	if string(got) != "v2" {
		t.Errorf("target not updated through symlink: got %q, want %q", got, "v2")
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
