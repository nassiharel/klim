package fileutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// TestAtomicWritePreservesDanglingSymlink covers the case
// filepath.EvalSymlinks rejects: the link's target doesn't exist yet
// (e.g. a fresh repo where someone has staged a symlink pointing at
// where the manifest will live). AtomicWrite must create the target
// file and leave the link intact, not replace the link with a regular
// file.
func TestAtomicWritePreservesDanglingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation not reliable on Windows CI")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt") // intentionally does NOT exist
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	if err := AtomicWrite(link, []byte("first"), 0o644); err != nil {
		t.Fatalf("AtomicWrite via dangling symlink: %v", err)
	}

	// Symlink must still be a symlink.
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Errorf("AtomicWrite replaced the dangling symlink with a regular file (mode=%v)", info.Mode())
	}
	// Target must now exist with the written bytes.
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("target not created at %s: %v", target, err)
	}
	if string(got) != "first" {
		t.Errorf("target contents = %q, want %q", got, "first")
	}
}

// TestAtomicWriteFollowsSymlinkChain covers the multi-hop case: a
// symlink to a symlink to a regular file. The write must end up at
// the final regular file, with both intermediate links intact.
func TestAtomicWriteFollowsSymlinkChain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation not reliable on Windows CI")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(real, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	mid := filepath.Join(dir, "mid.txt")
	if err := os.Symlink(real, mid); err != nil {
		t.Fatal(err)
	}
	top := filepath.Join(dir, "top.txt")
	if err := os.Symlink(mid, top); err != nil {
		t.Fatal(err)
	}

	if err := AtomicWrite(top, []byte("v2"), 0o644); err != nil {
		t.Fatalf("AtomicWrite via symlink chain: %v", err)
	}

	for _, link := range []string{top, mid} {
		info, err := os.Lstat(link)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("link %s was replaced with a regular file", link)
		}
	}
	got, _ := os.ReadFile(real)
	if string(got) != "v2" {
		t.Errorf("end-of-chain content = %q, want v2", got)
	}
}

// TestAtomicWriteRelativeSymlink covers a relative-target link like
// `.clim.yaml -> ../shared/template.yaml`. Readlink returns the
// raw relative target; we must resolve it against the symlink's
// parent dir rather than the process cwd.
func TestAtomicWriteRelativeSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation not reliable on Windows CI")
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(real, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(sub, "link.txt")
	// Relative link from sub/link.txt → ../real.txt
	if err := os.Symlink("../real.txt", link); err != nil {
		t.Fatal(err)
	}

	if err := AtomicWrite(link, []byte("v2"), 0o644); err != nil {
		t.Fatalf("AtomicWrite via relative symlink: %v", err)
	}

	got, _ := os.ReadFile(real)
	if string(got) != "v2" {
		t.Errorf("relative-symlink target = %q, want v2", got)
	}
	info, _ := os.Lstat(link)
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("relative symlink was replaced with a regular file")
	}
}

// TestAtomicWriteSymlinkCycleDetected verifies that a cycle (link A
// → link B → link A) is reported as an error rather than spinning
// forever or silently writing somewhere unexpected.
func TestAtomicWriteSymlinkCycleDetected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation not reliable on Windows CI")
	}
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	// a → b, b → a — both dangling at creation time, but we still
	// detect the cycle when resolveLinkTarget walks the chain.
	if err := os.Symlink(b, a); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(a, b); err != nil {
		t.Fatal(err)
	}

	err := AtomicWrite(a, []byte("doomed"), 0o644)
	if err == nil {
		t.Fatal("expected error for symlink cycle, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") && !strings.Contains(err.Error(), "depth") {
		t.Errorf("error should mention cycle/depth, got: %v", err)
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
