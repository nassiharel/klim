package snapshot

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/registry"
)

func pointHomeAtTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)        // Unix
	t.Setenv("USERPROFILE", dir) // Windows
	return dir
}

func installedTool(name, version string) registry.Tool {
	return registry.Tool{
		Name: name,
		Instances: []registry.Instance{
			{Path: "/bin/" + name, Version: version, Source: registry.SourceBrew},
		},
	}
}

func TestSanitizeName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"abc", "abc"},
		{"  Mixed Case  ", "mixed-case"},
		{"hello/world", "hello-world"},
		{"my snap_with-keep", "my-snap_with-keep"},
		{"unicode✓test", "unicode-test"},
		{"", ""},
	}
	for _, c := range cases {
		if got := sanitizeName(c.in); got != c.want {
			t.Errorf("sanitizeName(%q): want %q, got %q", c.in, c.want, got)
		}
	}
}

func TestSave_WritesFileAndReturnsPath(t *testing.T) {
	pointHomeAtTemp(t)
	tools := []registry.Tool{installedTool("git", "2.50")}

	path, err := Save(tools, "")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !strings.HasSuffix(path, ".yaml") {
		t.Errorf("path should end in .yaml, got %s", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("snapshot file not on disk: %v", err)
	}

	// With label, path should include the sanitised label.
	pathLabeled, err := Save(tools, "Pre-Refactor")
	if err != nil {
		t.Fatalf("Save labelled: %v", err)
	}
	if !strings.Contains(filepath.Base(pathLabeled), "pre-refactor") {
		t.Errorf("labelled path should contain sanitised label, got %s", filepath.Base(pathLabeled))
	}
}

func TestSaveAndLoad_RoundTripPreservesContents(t *testing.T) {
	pointHomeAtTemp(t)
	tools := []registry.Tool{installedTool("git", "2.50"), installedTool("node", "22.0")}

	path, err := Save(tools, "round-trip")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Load by full filename.
	got, err := Load(filepath.Base(path))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Name != "round-trip" {
		t.Errorf("name: want round-trip, got %q", got.Name)
	}
	if len(got.Tools) != 2 {
		t.Errorf("tools: want 2, got %d", len(got.Tools))
	}
	if got.OS != runtime.GOOS {
		t.Errorf("OS: want %s, got %s", runtime.GOOS, got.OS)
	}
}

func TestList_ReturnsNewestFirst(t *testing.T) {
	pointHomeAtTemp(t)
	// Save three snapshots with distinct labels.
	tools := []registry.Tool{installedTool("git", "2.50")}
	for _, l := range []string{"first", "second", "third"} {
		if _, err := Save(tools, l); err != nil {
			t.Fatalf("Save %s: %v", l, err)
		}
	}
	entries, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(entries))
	}
	// Sorted by ModTime descending; the file written most recently
	// should appear first. Filenames embed a timestamp resolution of
	// seconds, but ModTime is finer. Verify order monotonically
	// non-increasing.
	for i := 1; i < len(entries); i++ {
		if entries[i-1].CreatedAt.Before(entries[i].CreatedAt) {
			t.Errorf("entries[%d].CreatedAt (%v) earlier than entries[%d] (%v)",
				i-1, entries[i-1].CreatedAt, i, entries[i].CreatedAt)
		}
	}
}

func TestList_NoDirectoryReturnsNilWithoutError(t *testing.T) {
	pointHomeAtTemp(t)
	// No Save calls → snapshots dir doesn't exist yet.
	entries, err := List()
	if err != nil {
		t.Errorf("List on missing dir: want no error, got %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("want zero entries, got %d", len(entries))
	}
}

func TestDelete_RemovesFile(t *testing.T) {
	pointHomeAtTemp(t)
	tools := []registry.Tool{installedTool("git", "2.50")}
	path, err := Save(tools, "deleteme")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := Delete(filepath.Base(path)); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still exists after Delete: %v", err)
	}
}

func TestResolveSnapshotPath_RejectsTraversalAndSpecialChars(t *testing.T) {
	pointHomeAtTemp(t)
	bad := []string{"..", "a/b", "a\\b", "a..b", "snap\x00null"}
	for _, s := range bad {
		if _, err := resolveSnapshotPath(s); err == nil {
			t.Errorf("resolveSnapshotPath(%q): want error, got nil", s)
		}
	}
}

func TestResolveSnapshotPath_PrefixSuffixSubstringMatch(t *testing.T) {
	pointHomeAtTemp(t)
	tools := []registry.Tool{installedTool("git", "2.50")}
	// Save a single snapshot; the full filename embeds today's
	// timestamp + a sanitised label suffix, so we know the base
	// ends with "-pre-refactor-baseline".
	path, err := Save(tools, "pre-refactor-baseline")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	base := strings.TrimSuffix(filepath.Base(path), ".yaml")
	want := base + ".yaml"

	cases := []struct {
		name, query string
	}{
		// Substring match — the query sits in the middle of the label.
		{"substring", "refactor"},
		// Suffix match — the query is the trailing segment of the label.
		{"suffix", "baseline"},
		// Prefix match — the query is the leading timestamp characters.
		// resolveSnapshotPath checks HasPrefix against the full base,
		// so the year prefix unambiguously hits a single file here.
		{"prefix", base[:4]},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := resolveSnapshotPath(c.query)
			if err != nil {
				t.Fatalf("resolveSnapshotPath(%q): %v", c.query, err)
			}
			if filepath.Base(got) != want {
				t.Errorf("%s match for %q: want %s, got %s", c.name, c.query, want, filepath.Base(got))
			}
		})
	}
}

func TestResolveSnapshotPath_AmbiguousErrors(t *testing.T) {
	pointHomeAtTemp(t)
	tools := []registry.Tool{installedTool("git", "2.50")}
	if _, err := Save(tools, "alpha-snap"); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(tools, "alpha-other"); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveSnapshotPath("alpha"); err == nil {
		t.Errorf("expected ambiguous error, got nil")
	} else if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("expected error to mention ambiguity, got %v", err)
	}
}

func TestResolveSnapshotPath_NotFoundError(t *testing.T) {
	pointHomeAtTemp(t)
	if _, err := resolveSnapshotPath("nope"); err == nil {
		t.Errorf("expected not found error, got nil")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got %v", err)
	}
}

func TestSaveProfile_RejectsEmptyName(t *testing.T) {
	pointHomeAtTemp(t)
	tools := []registry.Tool{installedTool("git", "2.50")}
	if _, err := SaveProfile(tools, "   "); err == nil {
		t.Errorf("expected error for empty profile name, got nil")
	}
	// All-whitespace input sanitizes to empty; sanitizeName trims
	// spaces but does not reject all-dash output. The helper checks
	// safe == "" so inputs that are entirely whitespace fail; inputs
	// that are entirely punctuation (e.g. "!!!") would sanitize to
	// "---" and pass the empty-check.
}

func TestProfile_SaveLoadListDelete(t *testing.T) {
	pointHomeAtTemp(t)
	tools := []registry.Tool{installedTool("git", "2.50"), installedTool("docker", "27.0")}

	path, err := SaveProfile(tools, "My Profile")
	if err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	if !strings.HasSuffix(path, "my-profile.yaml") {
		t.Errorf("profile path should be sanitized and end in .yaml, got %s", path)
	}

	got, err := LoadProfile("My Profile")
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if got.Name != "My Profile" {
		t.Errorf("loaded profile name: want %q, got %q", "My Profile", got.Name)
	}
	if len(got.Tools) != 2 {
		t.Errorf("want 2 tools, got %d", len(got.Tools))
	}

	entries, err := ListProfiles()
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("ListProfiles: want 1, got %d", len(entries))
	}

	if err := DeleteProfile("My Profile"); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("profile file still on disk after delete: %v", err)
	}
}

func TestProfile_LoadAndDeleteRejectEmptyName(t *testing.T) {
	pointHomeAtTemp(t)
	if _, err := LoadProfile("   "); err == nil {
		t.Errorf("LoadProfile empty: want error")
	}
	if err := DeleteProfile("   "); err == nil {
		t.Errorf("DeleteProfile empty: want error")
	}
}

func TestReadSnapshot_RejectsOversizedFile(t *testing.T) {
	pointHomeAtTemp(t)
	dir, err := snapshotsDir()
	if err != nil {
		t.Fatalf("snapshotsDir: %v", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write a >10MB file.
	huge := filepath.Join(dir, "huge.yaml")
	f, err := os.Create(huge)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	chunk := make([]byte, 1<<20) // 1MB of zeroes
	for i := 0; i < 11; i++ {
		if _, err := f.Write(chunk); err != nil {
			t.Fatal(err)
		}
	}
	_ = f.Close()

	if _, err := readSnapshot(huge); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Errorf("expected 'too large' error, got %v", err)
	}
}

func TestReadSnapshot_MissingFileErrors(t *testing.T) {
	pointHomeAtTemp(t)
	if _, err := readSnapshot(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Errorf("expected error for missing file, got nil")
	}
}
