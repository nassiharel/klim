package trail

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nassiharel/clim/internal/registry"
)

// useTempDir redirects trail storage into t.TempDir() for the lifetime of t.
func useTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	restore := overrideRoot(dir)
	t.Cleanup(restore)
	return dir
}

// fakeTool builds a registry.Tool with one PATH instance.
func fakeTool(name, version string, source registry.InstallSource, path string) registry.Tool {
	return registry.Tool{
		Name: name,
		Instances: []registry.Instance{
			{Path: path, Version: version, Source: source},
		},
	}
}

// orderedTools returns a deterministic toolset for assertions.
func orderedTools() []registry.Tool {
	return []registry.Tool{
		fakeTool("git", "2.53.0", registry.SourceWinget, "/usr/bin/git"),
		fakeTool("gh", "2.74.2", registry.SourceWinget, "/usr/bin/gh"),
		fakeTool("kubectl", "1.31.0", registry.SourceBrew, "/usr/local/bin/kubectl"),
	}
}

// TestCapture_DedupesIdenticalEnvironments verifies that two captures of the
// same canonical environment produce the same ObjectID and one stored
// object on disk, while still appending two distinct log entries.
func TestCapture_DedupesIdenticalEnvironments(t *testing.T) {
	dir := useTempDir(t)
	tools := orderedTools()

	e1, err := Capture(OpCapture, "first", tools)
	if err != nil {
		t.Fatalf("capture 1: %v", err)
	}
	e2, err := Capture(OpCapture, "second", tools)
	if err != nil {
		t.Fatalf("capture 2: %v", err)
	}

	if e1.Object != e2.Object {
		t.Fatalf("expected identical ObjectID, got %s vs %s", e1.Object.Short(), e2.Object.Short())
	}
	if e1.Index == e2.Index {
		t.Fatalf("expected distinct entry indexes, got %d twice", e1.Index)
	}

	objectFile := filepath.Join(dir, "objects", string(e1.Object[:2]), string(e1.Object[2:])+".yaml")
	if !fileExists(objectFile) {
		t.Fatalf("expected object file at %s", objectFile)
	}
}

// TestCapture_OrderIndependent confirms that re-ordering the input tools
// does not change the resulting ObjectID — proving the canonical-form
// hashing.
func TestCapture_OrderIndependent(t *testing.T) {
	useTempDir(t)
	a := orderedTools()
	b := []registry.Tool{a[2], a[0], a[1]} // reorder

	idA, _, err := hashSnapshot(canonicalSnapshot("linux", "amd64", a))
	if err != nil {
		t.Fatal(err)
	}
	idB, _, err := hashSnapshot(canonicalSnapshot("linux", "amd64", b))
	if err != nil {
		t.Fatal(err)
	}
	if idA != idB {
		t.Fatalf("hashes differ on reordered input: %s vs %s", idA.Short(), idB.Short())
	}
}

// TestLog_NewestFirst confirms that Log returns entries in descending Time
// order regardless of insertion order. (Insertion order is also descending
// here, but Log makes no implicit assumption about that.)
func TestLog_NewestFirst(t *testing.T) {
	useTempDir(t)
	tools := orderedTools()

	for i := 0; i < 3; i++ {
		if _, err := Capture(OpCapture, "", tools); err != nil {
			t.Fatalf("capture %d: %v", i, err)
		}
		// Captures have UTC time at second precision; sleep enough that newest-last is meaningful.
		time.Sleep(2 * time.Millisecond)
	}

	entries, err := Log(LogOptions{})
	if err != nil {
		t.Fatalf("Log: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}
	for i := 1; i < len(entries); i++ {
		if entries[i-1].Index < entries[i].Index {
			t.Fatalf("entries[%d].Index=%d should be > entries[%d].Index=%d (newest-first)",
				i-1, entries[i-1].Index, i, entries[i].Index)
		}
	}
}

func TestResolve_HEADAndAncestors(t *testing.T) {
	useTempDir(t)
	tools := orderedTools()
	for i := 0; i < 3; i++ {
		if _, err := Capture(OpCapture, "", tools); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct {
		spec    string
		wantIdx int
	}{
		{"HEAD", 2},
		{"latest", 2},
		{"HEAD~0", 2},
		{"HEAD~1", 1},
		{"HEAD~2", 0},
		{"@0", 0},
		{"@1", 1},
	}
	for _, tc := range cases {
		t.Run(tc.spec, func(t *testing.T) {
			e, err := Resolve(tc.spec)
			if err != nil {
				t.Fatalf("Resolve(%q): %v", tc.spec, err)
			}
			if e.Index != tc.wantIdx {
				t.Fatalf("Resolve(%q) -> index %d, want %d", tc.spec, e.Index, tc.wantIdx)
			}
		})
	}
}

func TestResolve_HashPrefix(t *testing.T) {
	useTempDir(t)
	tools := orderedTools()
	e, err := Capture(OpCapture, "", tools)
	if err != nil {
		t.Fatal(err)
	}
	prefix := string(e.Object[:7])
	got, err := Resolve(prefix)
	if err != nil {
		t.Fatalf("Resolve(%q): %v", prefix, err)
	}
	if got.Object != e.Object {
		t.Fatalf("hash prefix resolved to wrong entry: got %s, want %s", got.Object.Short(), e.Object.Short())
	}
	// Full hash should also work.
	got, err = Resolve(string(e.Object))
	if err != nil {
		t.Fatalf("Resolve(full hash): %v", err)
	}
	if got.Object != e.Object {
		t.Fatalf("full hash resolved to wrong entry")
	}
}

func TestResolve_Label(t *testing.T) {
	useTempDir(t)
	tools := orderedTools()
	if _, err := Capture(OpCapture, "before-upgrade", tools); err != nil {
		t.Fatal(err)
	}
	if _, err := Capture(OpCapture, "", tools); err != nil {
		t.Fatal(err)
	}
	e, err := Resolve("before-upgrade")
	if err != nil {
		t.Fatalf("Resolve(label): %v", err)
	}
	if e.Label != "before-upgrade" {
		t.Fatalf("got label %q", e.Label)
	}
}

func TestResolve_Errors(t *testing.T) {
	useTempDir(t)
	tools := orderedTools()
	// no entries yet
	if _, err := Resolve("HEAD"); err == nil {
		t.Fatalf("expected error on empty trail")
	}
	if _, err := Capture(OpCapture, "", tools); err != nil {
		t.Fatal(err)
	}
	// out of range
	if _, err := Resolve("HEAD~5"); err == nil {
		t.Fatalf("expected out-of-range error")
	}
	// bad form
	if _, err := Resolve(""); err == nil {
		t.Fatalf("expected empty-ref error")
	}
	if _, err := Resolve("HEAD~bogus"); err == nil {
		t.Fatalf("expected bad-suffix error")
	}
	// unknown label
	if _, err := Resolve("does-not-exist"); err == nil {
		t.Fatalf("expected unknown-ref error")
	}
}

func TestDiff_AllChangeKinds(t *testing.T) {
	useTempDir(t)
	a := []registry.Tool{
		fakeTool("git", "2.53.0", registry.SourceWinget, "/git"),    // unchanged
		fakeTool("gh", "2.74.0", registry.SourceWinget, "/gh"),      // version-changed
		fakeTool("docker", "27.3.1", registry.SourceWinget, "/dkr"), // source-changed
		fakeTool("removed", "1.0", registry.SourceBrew, "/removed"), // removed
	}
	b := []registry.Tool{
		fakeTool("git", "2.53.0", registry.SourceWinget, "/git"),
		fakeTool("gh", "2.74.2", registry.SourceWinget, "/gh"),
		fakeTool("docker", "27.3.1", registry.SourceBrew, "/dkr"),
		fakeTool("added", "0.1", registry.SourceBrew, "/added"),
	}
	if _, err := Capture(OpCapture, "a", a); err != nil {
		t.Fatal(err)
	}
	if _, err := Capture(OpCapture, "b", b); err != nil {
		t.Fatal(err)
	}

	d, err := Diff("a", "b")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(d.Added) != 1 || d.Added[0].Name != "added" {
		t.Errorf("Added = %+v", d.Added)
	}
	if len(d.Removed) != 1 || d.Removed[0].Name != "removed" {
		t.Errorf("Removed = %+v", d.Removed)
	}
	if len(d.VersionChanged) != 1 || d.VersionChanged[0].Name != "gh" {
		t.Errorf("VersionChanged = %+v", d.VersionChanged)
	}
	if len(d.SourceChanged) != 1 || d.SourceChanged[0].Name != "docker" {
		t.Errorf("SourceChanged = %+v", d.SourceChanged)
	}
	if d.VersionChanged[0].From != "2.74.0" || d.VersionChanged[0].To != "2.74.2" {
		t.Errorf("version change body wrong: %+v", d.VersionChanged[0])
	}
	if d.SourceChanged[0].From != "winget" || d.SourceChanged[0].To != "brew" {
		t.Errorf("source change body wrong: %+v", d.SourceChanged[0])
	}
}

func TestPrune_KeepN(t *testing.T) {
	useTempDir(t)

	mkUnique := func(suffix string) []registry.Tool {
		// Different version each time so each capture creates a unique object.
		return []registry.Tool{fakeTool("toolA", "1.0."+suffix, registry.SourceWinget, "/a")}
	}
	for i := 0; i < 5; i++ {
		if _, err := Capture(OpCapture, "", mkUnique(string(rune('0'+i)))); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Prune(PruneOptions{Keep: 2})
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if res.EntriesKept != 2 || res.EntriesRemoved != 3 {
		t.Fatalf("entries: kept=%d removed=%d (want 2/3)", res.EntriesKept, res.EntriesRemoved)
	}
	if res.ObjectsKept != 2 || res.ObjectsRemoved != 3 {
		t.Fatalf("objects: kept=%d removed=%d (want 2/3)", res.ObjectsKept, res.ObjectsRemoved)
	}

	entries, err := Log(LogOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("post-prune entries: %d", len(entries))
	}
}

func TestPrune_KeepReferencedObjectsAcrossDuplicates(t *testing.T) {
	useTempDir(t)
	tools := orderedTools()
	for i := 0; i < 4; i++ {
		if _, err := Capture(OpCapture, "", tools); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Prune(PruneOptions{Keep: 2})
	if err != nil {
		t.Fatalf("Prune: %v", err)
	}
	if res.EntriesRemoved != 2 {
		t.Fatalf("expected 2 entries removed, got %d", res.EntriesRemoved)
	}
	// All 4 entries shared 1 object — that object is still referenced.
	if res.ObjectsRemoved != 0 {
		t.Fatalf("expected 0 objects removed (single-object trail), got %d", res.ObjectsRemoved)
	}
	if res.ObjectsKept != 1 {
		t.Fatalf("expected 1 kept object, got %d", res.ObjectsKept)
	}
}

func TestStrictDecode_RejectsUnknownFields(t *testing.T) {
	useTempDir(t)
	bad := []byte(`schema_version: 1
os: linux
arch: amd64
tools: []
unknown_field: oops
`)
	_, err := decodeSnapshot(bad, "deadbeef")
	if err == nil || !strings.Contains(err.Error(), "unknown_field") {
		t.Fatalf("expected unknown-field error, got %v", err)
	}
}

func TestStrictDecode_RejectsUnknownSchemaVersion(t *testing.T) {
	useTempDir(t)
	bad := []byte(`schema_version: 99
os: linux
arch: amd64
tools: []
`)
	_, err := decodeSnapshot(bad, "deadbeef")
	if err == nil || !strings.Contains(err.Error(), "schema version") {
		t.Fatalf("expected schema-version error, got %v", err)
	}
}

// fileExists is a tiny test helper.
func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
