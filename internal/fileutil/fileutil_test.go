package fileutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// mustSymlink creates a symlink, skipping the test if the platform
// refuses (typically Windows without admin or developer-mode). On any
// other failure it fails the test. By trying first instead of
// blanket-skipping on Windows, CI runners that *do* have developer
// mode enabled (the GitHub Actions windows-latest image since 2023
// when running as a regular user with developer mode) actually
// exercise the symlink test path.
func mustSymlink(t *testing.T, oldname, newname string) {
	t.Helper()
	err := os.Symlink(oldname, newname)
	if err == nil {
		return
	}
	if errors.Is(err, fs.ErrPermission) {
		t.Skipf("symlink creation requires elevated privileges on this platform: %v", err)
	}
	// Windows-specific message when developer mode isn't enabled and
	// the user isn't admin: ERROR_PRIVILEGE_NOT_HELD ("A required
	// privilege is not held by the client.").
	if runtime.GOOS == "windows" {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "privilege") || strings.Contains(msg, "not held") {
			t.Skipf("Windows symlink creation requires admin or developer-mode: %v", err)
		}
	}
	t.Fatalf("os.Symlink(%q, %q): %v", oldname, newname, err)
}

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
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(target, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	mustSymlink(t, target, link)

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
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt") // intentionally does NOT exist
	link := filepath.Join(dir, "link.txt")
	mustSymlink(t, target, link)

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
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(real, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	mid := filepath.Join(dir, "mid.txt")
	mustSymlink(t, real, mid)
	top := filepath.Join(dir, "top.txt")
	mustSymlink(t, mid, top)

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
	mustSymlink(t, "../real.txt", link)

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
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	// a → b, b → a — both dangling at creation time, but we still
	// detect the cycle when resolveLinkTarget walks the chain.
	mustSymlink(t, b, a)
	mustSymlink(t, a, b)

	err := AtomicWrite(a, []byte("doomed"), 0o644)
	if err == nil {
		t.Fatal("expected error for symlink cycle, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") && !strings.Contains(err.Error(), "depth") {
		t.Errorf("error should mention cycle/depth, got: %v", err)
	}
}

// TestAtomicWritePreservesExistingMode guards against permission
// regression on overwrite. A user who manually `chmod 600`s a sensitive
// .clim.yaml must not have it broadened to 0644 by a subsequent
// `clim init --force`.
func TestAtomicWritePreservesExistingMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows file mode bits don't map to POSIX perms in a way
		// that makes 0o600 vs 0o644 meaningful here.
		t.Skip("POSIX file modes don't apply on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "private.txt")
	if err := os.WriteFile(path, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Caller passes 0o644 (the typical "create" perm) — but the
	// existing 0o600 must win.
	if err := AtomicWrite(path, []byte("v2"), 0o644); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("perm = %o, want 0600 (overwrite must preserve existing mode)", got)
	}
}

// TestAtomicWriteAppliesPermOnCreate complements the above: when the
// target doesn't exist yet, the supplied perm is the source of truth.
func TestAtomicWriteAppliesPermOnCreate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes don't apply on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "fresh.txt")

	if err := AtomicWrite(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("perm = %o, want 0600 on first-time write", got)
	}
}

// TestAtomicWriteDoesNotFallbackOnNonPermissionError verifies that
// any temp-file failure (here: missing parent directory → ENOENT)
// propagates to the caller. AtomicWrite no longer has a read-only-dir
// fallback at all, so every CreateTemp failure must surface.
func TestAtomicWriteDoesNotFallbackOnNonPermissionError(t *testing.T) {
	dir := t.TempDir()
	// Path under a non-existent subdirectory.
	path := filepath.Join(dir, "ghost-dir", "target.txt")

	err := AtomicWrite(path, []byte("v1"), 0o644)
	if err == nil {
		t.Fatal("expected error when parent dir doesn't exist")
	}
	// And the file mustn't have been created in some unexpected place.
	if _, statErr := os.Stat(path); statErr == nil {
		t.Errorf("target unexpectedly exists at %s after ENOENT", path)
	}
}

// TestAtomicWritePropagatesLstatErrors guards that a non-ENOENT
// failure during symlink-chain walking surfaces — we don't want to
// silently end up writing to a wrongly-resolved target. We exercise
// this by pointing AtomicWrite at a symlink whose intermediate hop
// has restrictive permissions on its parent dir, so Lstat returns
// EACCES rather than ENOENT.
func TestAtomicWritePropagatesLstatErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-specific permission semantics")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission bits")
	}
	dir := t.TempDir()
	// Build a directory we'll later make non-traversable, then a
	// symlink whose target is inside that dir.
	gated := filepath.Join(dir, "gated")
	if err := os.MkdirAll(gated, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(gated, "target.txt")
	if err := os.WriteFile(target, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	mustSymlink(t, target, link)

	// Remove search permission on `gated` so Lstat on the resolved
	// target returns EACCES.
	if err := os.Chmod(gated, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(gated, 0o755) })

	err := AtomicWrite(link, []byte("v2"), 0o644)
	if err == nil {
		t.Fatal("expected EACCES from Lstat to propagate")
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("expected fs.ErrPermission, got: %v", err)
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
