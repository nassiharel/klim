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

// TestHash_PathIndependent verifies that two snapshots with the same
// (Name, Version, Source) but different Path values hash to the same
// ObjectID. Path is per-machine and excluded from the canonical hash.
func TestHash_PathIndependent(t *testing.T) {
	useTempDir(t)
	a := []registry.Tool{fakeTool("git", "2.53.0", registry.SourceWinget, "/usr/bin/git")}
	b := []registry.Tool{fakeTool("git", "2.53.0", registry.SourceWinget, "C:\\Program Files\\Git\\cmd\\git.exe")}
	idA, _, err := hashSnapshot(canonicalSnapshot("linux", "amd64", a))
	if err != nil {
		t.Fatal(err)
	}
	idB, _, err := hashSnapshot(canonicalSnapshot("linux", "amd64", b))
	if err != nil {
		t.Fatal(err)
	}
	if idA != idB {
		t.Fatalf("hashes differ on Path-only diff: %s vs %s (Path should not be part of hash)", idA.Short(), idB.Short())
	}
}

// TestResolve_HashAfterDedupe regression-tests the bug where two entries
// pointing at the same object made every hash-prefix lookup ambiguous.
// After dedupe, `Resolve("<hash>")` must still return the (newest) entry
// for that object.
func TestResolve_HashAfterDedupe(t *testing.T) {
	useTempDir(t)
	tools := orderedTools()
	e1, err := Capture(OpCapture, "first", tools)
	if err != nil {
		t.Fatal(err)
	}
	e2, err := Capture(OpCapture, "second", tools)
	if err != nil {
		t.Fatal(err)
	}
	if e1.Object != e2.Object {
		t.Fatalf("setup: expected dedupe; got distinct objects %s and %s", e1.Object.Short(), e2.Object.Short())
	}
	got, err := Resolve(string(e1.Object[:7]))
	if err != nil {
		t.Fatalf("Resolve(prefix): %v", err)
	}
	// Newest entry wins on tie.
	if got.Index != e2.Index {
		t.Fatalf("Resolve hash after dedupe: got entry @%d, want @%d (newest)", got.Index, e2.Index)
	}
}

// TestCapture_LabelUniqueness verifies that re-using a label fails fast
// rather than creating an ambiguous label that breaks Resolve.
func TestCapture_LabelUniqueness(t *testing.T) {
	useTempDir(t)
	tools := orderedTools()
	if _, err := Capture(OpCapture, "before-upgrade", tools); err != nil {
		t.Fatalf("first capture: %v", err)
	}
	_, err := Capture(OpCapture, "before-upgrade", tools)
	if err == nil {
		t.Fatal("expected duplicate-label error, got nil")
	}
	if !strings.Contains(err.Error(), "label") || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("unexpected error: %v", err)
	}
	// First capture's label must still resolve unambiguously.
	e, err := Resolve("before-upgrade")
	if err != nil {
		t.Fatalf("Resolve(label) after rejected duplicate: %v", err)
	}
	if e.Label != "before-upgrade" {
		t.Fatalf("got label %q", e.Label)
	}
}

// TestLoadLog_RejectsVersionlessLog verifies that a hand-edited or
// corrupted log without schema_version is rejected rather than silently
// treated as the current version.
func TestLoadLog_RejectsVersionlessLog(t *testing.T) {
	dir := useTempDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "log.yaml")
	// Note: missing schema_version on purpose.
	body := []byte("entries: []\n")
	if err := os.WriteFile(logPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Log(LogOptions{}); err == nil || !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("expected missing-schema_version error, got %v", err)
	}
}

// TestCapture_StoredBodyHasNoPaths verifies the on-disk Snapshot body
// drops Tool.Path entirely. Storing paths in dedupe-shared bodies would
// freeze the first capture's paths and mislead later trail show calls.
func TestCapture_StoredBodyHasNoPaths(t *testing.T) {
	dir := useTempDir(t)
	tools := []registry.Tool{
		fakeTool("git", "2.53.0", registry.SourceWinget, "C:\\Program Files\\Git\\cmd\\git.exe"),
	}
	e, err := Capture(OpCapture, "", tools)
	if err != nil {
		t.Fatal(err)
	}
	objPath := filepath.Join(dir, "objects", string(e.Object[:2]), string(e.Object[2:])+".yaml")
	body, err := os.ReadFile(objPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "path:") {
		t.Fatalf("stored snapshot body contains a path: field — paths should be dropped:\n%s", body)
	}
	if strings.Contains(string(body), "git.exe") || strings.Contains(string(body), "/git") {
		t.Fatalf("stored snapshot body contains a binary path:\n%s", body)
	}
}

// TestCapture_RejectsInvalidOp verifies the closed-set validation on op.
func TestCapture_RejectsInvalidOp(t *testing.T) {
	useTempDir(t)
	tools := orderedTools()
	_, err := Capture("not-a-real-op", "", tools)
	if err == nil || !strings.Contains(err.Error(), "invalid op") {
		t.Fatalf("expected invalid-op error, got %v", err)
	}
	// Empty op should still default to OpCapture.
	if _, err := Capture("", "", tools); err != nil {
		t.Fatalf("empty op should default to capture, got %v", err)
	}
}

// TestCapture_RejectedLabelLeavesNoOrphan verifies that when capture
// fails the duplicate-label check, no orphan object is left behind.
// The original bug: writeObject ran before label validation, so the
// rejected capture wrote an object that needed a later prune to clean up.
func TestCapture_RejectedLabelLeavesNoOrphan(t *testing.T) {
	dir := useTempDir(t)

	// First capture establishes the env + label.
	if _, err := Capture(OpCapture, "shared", orderedTools()); err != nil {
		t.Fatal(err)
	}

	// Second capture with the SAME label but DIFFERENT env. If the bug
	// regresses, this would write a new object before failing on label.
	otherTools := []registry.Tool{
		fakeTool("only-tool", "9.9.9", registry.SourceBrew, "/anywhere"),
	}
	_, err := Capture(OpCapture, "shared", otherTools)
	if err == nil {
		t.Fatal("expected duplicate-label error, got nil")
	}

	// Count objects on disk; should be exactly 1 (the first capture's).
	objects := 0
	_ = filepath.WalkDir(filepath.Join(dir, "objects"), func(p string, d os.DirEntry, _ error) error {
		if d != nil && !d.IsDir() && strings.HasSuffix(p, ".yaml") {
			objects++
		}
		return nil
	})
	if objects != 1 {
		t.Fatalf("expected 1 object after rejected dup-label capture, found %d (orphan from rejected capture?)", objects)
	}
}

// TestReadObject_DetectsCorruption verifies the content-addressed
// integrity check rejects a hand-edited object body whose hash no
// longer matches its filename.
func TestReadObject_DetectsCorruption(t *testing.T) {
	dir := useTempDir(t)
	e, err := Capture(OpCapture, "", orderedTools())
	if err != nil {
		t.Fatal(err)
	}
	objPath := filepath.Join(dir, "objects", string(e.Object[:2]), string(e.Object[2:])+".yaml")
	original, err := os.ReadFile(objPath)
	if err != nil {
		t.Fatal(err)
	}
	// Append a comment line — strict YAML decode will still parse this,
	// but the hash will no longer match the filename.
	tampered := append([]byte(nil), original...)
	tampered = append(tampered, []byte("# tampered\n")...)
	if err := os.WriteFile(objPath, tampered, 0o644); err != nil { //nolint:gosec // G703: test writes to a path it constructed from t.TempDir(); no taint
		t.Fatal(err)
	}
	_, err = Resolve("HEAD")
	if err != nil {
		t.Fatalf("Resolve still works (only reads log.yaml): %v", err)
	}
	if _, _, err := Show("HEAD"); err == nil {
		t.Fatal("expected integrity-check error on tampered object body, got nil")
	} else if !strings.Contains(err.Error(), "integrity check") {
		t.Fatalf("expected 'integrity check' in error, got: %v", err)
	}
}

// TestWriteObject_DetectsCorruptedReuse exercises the writeObject
// integrity check: if a previous capture wrote a healthy object that
// was later corrupted on disk, a subsequent capture of the same env
// must not silently accept the bad bytes.
func TestWriteObject_DetectsCorruptedReuse(t *testing.T) {
	dir := useTempDir(t)
	e, err := Capture(OpCapture, "", orderedTools())
	if err != nil {
		t.Fatal(err)
	}
	objPath := filepath.Join(dir, "objects", string(e.Object[:2]), string(e.Object[2:])+".yaml")
	if err := os.WriteFile(objPath, []byte("garbage\n"), 0o644); err != nil { //nolint:gosec
		t.Fatal(err)
	}
	_, err = Capture(OpCapture, "second", orderedTools())
	if err == nil {
		t.Fatal("expected error capturing on top of corrupted object, got nil")
	}
	if !strings.Contains(err.Error(), "corrupted") {
		t.Fatalf("expected 'corrupted' in error, got: %v", err)
	}
}

// TestCapture_RejectsReservedLabel verifies that labels colliding with
// Resolve's built-in ref syntax are rejected up front, so users can't
// create entries that are unresolvable by name.
func TestCapture_RejectsReservedLabel(t *testing.T) {
	useTempDir(t)
	cases := []string{"HEAD", "latest", "HEAD~0", "HEAD~5", "@0", "@42", "abcdef0", "DEADBEEF"}
	for _, label := range cases {
		_, err := Capture(OpCapture, label, orderedTools())
		if err == nil {
			t.Errorf("Capture(%q) accepted reserved label", label)
			continue
		}
		if !strings.Contains(err.Error(), "reserved ref syntax") {
			t.Errorf("Capture(%q): expected 'reserved ref syntax' in error, got: %v", label, err)
		}
	}
}

// TestDiff_SourceAndVersionChange verifies that when a tool migrates
// between sources AND bumps its version in the same step, the diff
// preserves the version delta on the SourceChange record.
func TestDiff_SourceAndVersionChange(t *testing.T) {
	useTempDir(t)
	a := []registry.Tool{fakeTool("docker", "1.2", registry.SourceWinget, "/d")}
	b := []registry.Tool{fakeTool("docker", "1.3", registry.SourceBrew, "/d")}
	if _, err := Capture(OpCapture, "a", a); err != nil {
		t.Fatal(err)
	}
	if _, err := Capture(OpCapture, "b", b); err != nil {
		t.Fatal(err)
	}
	d, err := Diff("a", "b")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.SourceChanged) != 1 {
		t.Fatalf("SourceChanged = %+v", d.SourceChanged)
	}
	sc := d.SourceChanged[0]
	if sc.FromVersion != "1.2" || sc.ToVersion != "1.3" {
		t.Errorf("expected version delta 1.2 → 1.3, got %q → %q", sc.FromVersion, sc.ToVersion)
	}
	if len(d.VersionChanged) != 0 {
		t.Errorf("did not expect a separate VersionChanged record: %+v", d.VersionChanged)
	}
}

// TestCapture_RejectsControlCharLabels guards against tabs / newlines /
// other control runes in labels. Such characters would corrupt the
// `clim trail log` tabwriter output (tabs split columns, newlines inject
// extra rows) or break terminal rendering of `clim trail show`.
func TestCapture_RejectsControlCharLabels(t *testing.T) {
	useTempDir(t)
	cases := []string{
		"with\ttab",
		"with\nnewline",
		"with\rcarriage",
		"bell\x07char",
	}
	for _, label := range cases {
		_, err := Capture(OpCapture, label, orderedTools())
		if err == nil {
			t.Errorf("Capture(%q) accepted control-character label", label)
			continue
		}
		if !strings.Contains(err.Error(), "invalid label") {
			t.Errorf("Capture(%q): expected 'invalid label' in error, got: %v", label, err)
		}
	}
}

// TestDecodeSnapshot_VersionlessIsCorruption ensures a snapshot file
// with a missing schema_version surfaces as "corrupted or hand-edited"
// rather than the generic "upgrade clim" message — we have never
// written versionless snapshots, so the latter would mislead users.
func TestDecodeSnapshot_VersionlessIsCorruption(t *testing.T) {
	body := []byte("tools: []\nos: linux\narch: amd64\n")
	_, err := decodeSnapshot(body, ObjectID(strings.Repeat("0", 64)))
	if err == nil {
		t.Fatal("expected error decoding versionless snapshot")
	}
	if !strings.Contains(err.Error(), "missing schema_version") {
		t.Errorf("expected 'missing schema_version' message, got: %v", err)
	}
	if strings.Contains(err.Error(), "upgrade clim") {
		t.Errorf("error should not say 'upgrade clim'; got: %v", err)
	}
}

// TestDecodeSnapshot_FutureVersionStillSaysUpgrade verifies the genuine
// forward-compat path is preserved: schema_version > current means a
// newer clim wrote this file.
func TestDecodeSnapshot_FutureVersionStillSaysUpgrade(t *testing.T) {
	body := []byte("schema_version: 9999\ntools: []\nos: linux\narch: amd64\n")
	_, err := decodeSnapshot(body, ObjectID(strings.Repeat("0", 64)))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "upgrade clim") {
		t.Errorf("expected 'upgrade clim' for future version, got: %v", err)
	}
}

// TestLoadLog_RejectsInvalidObjectID guards against a hand-edited
// log.yaml whose entries point at an arbitrary path fragment.
// Entry.Object is later passed to objectPath, so accepting non-hex
// values would let a corrupted log read files outside objects/.
func TestLoadLog_RejectsInvalidObjectID(t *testing.T) {
	dir := useTempDir(t)
	if _, err := Capture(OpCapture, "", orderedTools()); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "log.yaml")
	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	// Replace the object id with a path-traversal-looking string.
	tampered := strings.Replace(string(body), "object: ", "object: ../etc/passwd #", 1)
	if err := os.WriteFile(logPath, []byte(tampered), 0o644); err != nil { //nolint:gosec
		t.Fatal(err)
	}
	if _, err := Log(LogOptions{}); err == nil {
		t.Fatal("expected error loading log with invalid object id")
	} else if !strings.Contains(err.Error(), "invalid object id") {
		t.Fatalf("got: %v", err)
	}
}

// TestLoadLog_RejectsTrailingYAMLDocs ensures the strict-decoding
// guarantee covers extra YAML documents glued onto log.yaml.
func TestLoadLog_RejectsTrailingYAMLDocs(t *testing.T) {
	dir := useTempDir(t)
	if _, err := Capture(OpCapture, "", orderedTools()); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "log.yaml")
	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	tampered := append(body, []byte("---\nschema_version: 1\nentries: []\n")...)
	if err := os.WriteFile(logPath, tampered, 0o644); err != nil { //nolint:gosec
		t.Fatal(err)
	}
	if _, err := Log(LogOptions{}); err == nil {
		t.Fatal("expected error loading log with trailing YAML doc")
	} else if !strings.Contains(err.Error(), "trailing YAML content") {
		t.Fatalf("got: %v", err)
	}
}

// TestCapture_FailsClosedOnCorruptPredecessor ensures that a corrupted
// previous snapshot blocks new captures rather than silently appending
// an entry with an empty Summary. Otherwise users could keep extending
// broken history indefinitely.
func TestCapture_FailsClosedOnCorruptPredecessor(t *testing.T) {
	dir := useTempDir(t)
	first, err := Capture(OpCapture, "", orderedTools())
	if err != nil {
		t.Fatal(err)
	}
	objPath := filepath.Join(dir, "objects", string(first.Object[:2]), string(first.Object[2:])+".yaml")
	if err := os.WriteFile(objPath, []byte("garbage\n"), 0o644); err != nil { //nolint:gosec
		t.Fatal(err)
	}
	tools := append(orderedTools(), fakeTool("new", "1.0", registry.SourceBrew, "/n"))
	_, err = Capture(OpCapture, "", tools)
	if err == nil {
		t.Fatal("expected capture to fail on corrupt predecessor, got nil")
	}
	if !strings.Contains(err.Error(), "previous entry") {
		t.Fatalf("expected 'previous entry' in error, got: %v", err)
	}
}

// TestDecodeSnapshot_RejectsTrailingDocs ensures the strict-decoding
// guarantee covers object files too: a hand-edited file with a
// trailing YAML document is rejected rather than silently accepted.
func TestDecodeSnapshot_RejectsTrailingDocs(t *testing.T) {
	body := []byte("schema_version: 1\ntools: []\nos: linux\narch: amd64\n---\nschema_version: 1\ntools: []\nos: linux\narch: amd64\n")
	_, err := decodeSnapshot(body, ObjectID(strings.Repeat("0", 64)))
	if err == nil {
		t.Fatal("expected error decoding trailing-doc snapshot")
	}
	if !strings.Contains(err.Error(), "trailing YAML content") {
		t.Errorf("expected 'trailing YAML content' in error, got: %v", err)
	}
}

// TestLoadLog_MissingButRemnantsExist guards against silently
// re-initializing an empty trail when log.yaml has been lost while
// other state (HEAD or objects/) is still on disk. That would hide
// the prior history behind a fresh @0 entry instead of surfacing the
// corruption.
func TestLoadLog_MissingButRemnantsExist(t *testing.T) {
	dir := useTempDir(t)
	if _, err := Capture(OpCapture, "", orderedTools()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "log.yaml")); err != nil {
		t.Fatal(err)
	}
	// HEAD + objects/ still exist; loading must fail closed.
	if _, err := Log(LogOptions{}); err == nil {
		t.Fatal("expected error when log.yaml is missing but remnants remain")
	} else if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("got: %v", err)
	}
}

// TestLoadLog_RejectsDuplicateIndex verifies that hand-edited logs
// with non-unique entry indices fail closed — otherwise Resolve("@N")
// would silently return the first match instead of erroring out.
func TestLoadLog_RejectsDuplicateIndex(t *testing.T) {
	dir := useTempDir(t)
	if _, err := Capture(OpCapture, "first", orderedTools()); err != nil {
		t.Fatal(err)
	}
	if _, err := Capture(OpCapture, "second", append(orderedTools(), fakeTool("x", "1.0", registry.SourceBrew, "/x"))); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "log.yaml")
	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	// Replace the index of the second entry with the first one's index.
	tampered := strings.Replace(string(body), "index: 1", "index: 0", 1)
	if err := os.WriteFile(logPath, []byte(tampered), 0o644); err != nil { //nolint:gosec
		t.Fatal(err)
	}
	if _, err := Log(LogOptions{}); err == nil {
		t.Fatal("expected error on duplicate index")
	} else if !strings.Contains(err.Error(), "duplicate") && !strings.Contains(err.Error(), "increasing") {
		t.Fatalf("got: %v", err)
	}
}

// TestCapture_OrphanCleanupOnSaveFailure exercises the rollback path
// when saveLog fails after writeObject succeeded. We can't easily
// induce a real saveLog failure under the temp dir, so this test
// asserts the surface contract by checking that a corrupted
// predecessor (which causes summarize to error BEFORE writeObject
// runs in the new ordering) leaves no orphaned object on disk.
func TestCapture_NoOrphanWhenSummarizeFails(t *testing.T) {
	dir := useTempDir(t)
	first, err := Capture(OpCapture, "", orderedTools())
	if err != nil {
		t.Fatal(err)
	}
	objPath := filepath.Join(dir, "objects", string(first.Object[:2]), string(first.Object[2:])+".yaml")
	if err := os.WriteFile(objPath, []byte("garbage\n"), 0o644); err != nil { //nolint:gosec
		t.Fatal(err)
	}
	// Try to capture a *different* environment so a brand-new object
	// would have been written under the old ordering; under the new
	// ordering, summarize fails first and nothing is written.
	tools := append(orderedTools(), fakeTool("brand-new", "9.9", registry.SourceBrew, "/n"))
	_, err = Capture(OpCapture, "", tools)
	if err == nil {
		t.Fatal("expected capture failure on corrupt predecessor")
	}
	// Walk objects/ and confirm no second object was written. The
	// only file present should be the corrupted one we put there.
	objsDir := filepath.Join(dir, "objects")
	count := 0
	_ = filepath.Walk(objsDir, func(path string, info os.FileInfo, werr error) error {
		if werr == nil && !info.IsDir() {
			count++
		}
		return nil
	})
	if count != 1 {
		t.Errorf("expected 1 object on disk (the corrupted predecessor), found %d", count)
	}
}
