package trail

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/fileutil"
	"github.com/nassiharel/clim/internal/paths"
	"github.com/nassiharel/clim/internal/registry"
)

// fsRoots collects the per-call resolved trail filesystem paths.
// It exists primarily so tests can override TrailDir via overrideRoot.
type fsRoots struct {
	dir, head, log, lock, objects string
}

// rootOverride lets tests redirect trail storage into t.TempDir().
// Production code never sets it.
var rootOverride string

// overrideRoot sets trail's storage root for tests. Returns a restore
// function. NOT safe under concurrent use; tests that hit this should
// avoid t.Parallel() across the override.
func overrideRoot(dir string) func() {
	prev := rootOverride
	rootOverride = dir
	return func() { rootOverride = prev }
}

func resolveRoots() (fsRoots, error) {
	if rootOverride != "" {
		base := rootOverride
		return fsRoots{
			dir:     base,
			head:    filepath.Join(base, "HEAD"),
			log:     filepath.Join(base, "log.yaml"),
			lock:    filepath.Join(base, "log.lock"),
			objects: filepath.Join(base, "objects"),
		}, nil
	}
	dir, err := paths.TrailDir()
	if err != nil {
		return fsRoots{}, err
	}
	head, _ := paths.TrailHEAD()
	log, _ := paths.TrailLog()
	lock, _ := paths.TrailLock()
	obj, _ := paths.TrailObjects()
	return fsRoots{dir: dir, head: head, log: log, lock: lock, objects: obj}, nil
}

// objectPath maps an ObjectID to its <aa>/<bb...>.yaml path within objects/.
func (r fsRoots) objectPath(id ObjectID) string {
	if len(id) < 2 {
		return filepath.Join(r.objects, string(id)+".yaml")
	}
	return filepath.Join(r.objects, string(id[:2]), string(id[2:])+".yaml")
}

// writeObject writes the snapshot body content-addressed by id, idempotently.
//
// If the object already exists on disk, we re-hash the stored bytes and
// compare against id. A mismatch means a previous capture wrote a good
// object that has since been corrupted or hand-edited; trusting the
// filename alone would let a later capture of the same environment
// silently point at the corrupted data, breaking `show <ref>` after a
// successful `capture`. Failing here is the safe choice — the user can
// delete the offending file and recapture.
func writeObject(r fsRoots, id ObjectID, body []byte) error {
	path := r.objectPath(id)
	if existing, err := os.ReadFile(path); err == nil {
		sum := sha256.Sum256(existing)
		got := ObjectID(hex.EncodeToString(sum[:]))
		if got == id {
			return nil
		}
		return fmt.Errorf("trail object %s on disk has been corrupted: stored bytes hash to %s (delete %s and recapture)",
			id.Short(), got.Short(), path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspecting trail object %s: %w", id.Short(), err)
	}
	if err := fileutil.EnsureDir(path); err != nil {
		return fmt.Errorf("creating object dir: %w", err)
	}
	return fileutil.AtomicWrite(path, body, 0o644)
}

// readObject loads the snapshot at id from disk and verifies its integrity.
// The stored bytes are re-hashed and compared against id; a mismatch means
// the object file was corrupted or hand-edited and is rejected with a
// pointer to the offending file. This is what makes the store actually
// content-addressed (vs. just content-hashed-on-write).
func readObject(r fsRoots, id ObjectID) (*Snapshot, error) {
	path := r.objectPath(id)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading trail object %s: %w", id.Short(), err)
	}
	sum := sha256.Sum256(data)
	got := ObjectID(hex.EncodeToString(sum[:]))
	if got != id {
		return nil, fmt.Errorf("trail object %s failed integrity check: stored bytes hash to %s (corrupted or hand-edited %s)",
			id.Short(), got.Short(), path)
	}
	return decodeSnapshot(data, id)
}

// decodeSnapshot strictly decodes the YAML bytes into a Snapshot.
// Unknown fields and unknown schema versions are rejected.
func decodeSnapshot(data []byte, id ObjectID) (*Snapshot, error) {
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	var s Snapshot
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("parsing trail object %s: %w", id.Short(), err)
	}
	if s.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("trail object %s has unsupported schema version %d (this clim supports %d) — upgrade clim",
			id.Short(), s.SchemaVersion, SchemaVersion)
	}
	return &s, nil
}

// logFile is the on-disk shape of log.yaml.
type logFile struct {
	SchemaVersion int     `yaml:"schema_version"`
	Entries       []Entry `yaml:"entries"`
}

// loadLog reads log.yaml strictly. A missing file returns an empty log
// (treated as "fresh trail, no history yet"). A partially-corrupt or
// version-mismatched file fails closed — clim owns this format and we
// would rather refuse to run than silently mis-interpret history.
func loadLog(r fsRoots) (*logFile, error) {
	data, err := os.ReadFile(r.log)
	if errors.Is(err, os.ErrNotExist) {
		return &logFile{SchemaVersion: SchemaVersion}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading trail log: %w", err)
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	var lf logFile
	if err := dec.Decode(&lf); err != nil {
		return nil, fmt.Errorf("parsing trail log: %w", err)
	}
	// Strict: a non-empty log MUST carry an explicit schema_version.
	// We never wrote a versionless log (saveLog always sets it), so any
	// log we're reading without one was hand-edited or corrupted.
	if lf.SchemaVersion == 0 {
		return nil, fmt.Errorf("trail log is missing schema_version (corrupted or hand-edited?) — delete %s to start over", r.log)
	}
	if lf.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("trail log has unsupported schema version %d (this clim supports %d) — upgrade clim",
			lf.SchemaVersion, SchemaVersion)
	}
	return &lf, nil
}

// saveLog writes log.yaml + HEAD atomically. Caller must hold the trail lock.
func saveLog(r fsRoots, lf *logFile) error {
	if lf.SchemaVersion == 0 {
		lf.SchemaVersion = SchemaVersion
	}
	body, err := yaml.Marshal(lf)
	if err != nil {
		return fmt.Errorf("marshalling trail log: %w", err)
	}
	if err := fileutil.AtomicWrite(r.log, body, 0o644); err != nil {
		return fmt.Errorf("writing trail log: %w", err)
	}
	head := -1
	if len(lf.Entries) > 0 {
		head = lf.Entries[len(lf.Entries)-1].Index
	}
	if err := fileutil.AtomicWrite(r.head, []byte(fmt.Sprintf("%d\n", head)), 0o644); err != nil {
		return fmt.Errorf("writing trail HEAD: %w", err)
	}
	return nil
}

// reservedLabelReason returns a non-empty reason if label collides with a
// built-in ref syntax accepted by Resolve (HEAD, latest, HEAD~N, @N, or a
// hex prefix that would be interpreted as an object hash). Capture
// rejects such labels because Resolve always interprets the reserved
// form first, so the label could never be looked up. Empty return ⇒
// label is safe to use.
func reservedLabelReason(label string) string {
	if label == "HEAD" || label == "latest" {
		return "matches HEAD/latest"
	}
	if rest, ok := strings.CutPrefix(label, "HEAD~"); ok {
		if n, err := strconv.Atoi(rest); err == nil && n >= 0 {
			return "matches HEAD~N"
		}
	}
	if rest, ok := strings.CutPrefix(label, "@"); ok {
		if n, err := strconv.Atoi(rest); err == nil && n >= 0 {
			return "matches @<index>"
		}
	}
	if len(label) >= 7 && isHexPrefix(label) {
		return "looks like an object hash prefix"
	}
	return ""
}

// validOps is the closed set of operation kinds permitted in Entry.op.
// Capture rejects anything else so a typo or arbitrary string can't
// become permanent history data — the on-disk format keeps a small,
// well-defined operation vocabulary.
var validOps = map[string]struct{}{
	OpCapture: {},
	OpImport:  {},
	OpInstall: {},
	OpUpgrade: {},
	OpRemove:  {},
}

// Capture writes a new Entry pointing at the canonical Snapshot of tools.
// Two captures of an identical environment store one object and two entries.
//
// op identifies how the change was triggered (capture / import / install /
// upgrade / remove) and must be one of the constants in this package;
// arbitrary strings are rejected as a UsageError-equivalent.
//
// label is an optional user-provided tag — capture fails if the label
// is already used by another entry, so labels stay unique and
// `Resolve(<label>)` always points at one entry.
func Capture(op, label string, tools []registry.Tool) (*Entry, error) {
	if op == "" {
		op = OpCapture
	}
	if _, ok := validOps[op]; !ok {
		known := []string{OpCapture, OpImport, OpInstall, OpUpgrade, OpRemove}
		return nil, fmt.Errorf("trail: invalid op %q (valid: %s)", op, strings.Join(known, ", "))
	}
	label = strings.TrimSpace(label)
	if label != "" {
		if reason := reservedLabelReason(label); reason != "" {
			return nil, fmt.Errorf("trail: label %q conflicts with reserved ref syntax (%s); pick a different name", label, reason)
		}
	}
	r, err := resolveRoots()
	if err != nil {
		return nil, err
	}
	if err := fileutil.EnsureDir(filepath.Join(r.dir, "placeholder")); err != nil {
		return nil, fmt.Errorf("creating trail dir: %w", err)
	}

	snap := canonicalSnapshot(runtime.GOOS, runtime.GOARCH, tools)
	id, body, err := hashSnapshot(snap)
	if err != nil {
		return nil, err
	}

	// Acquire the lock BEFORE any disk mutation, so a concurrent prune
	// can't observe a freshly-written object as orphaned and GC it
	// before this entry is appended. Also lets us validate the label
	// against existing entries before writing the object — otherwise
	// a duplicate-label rejection would leave an orphan object on disk.
	release, err := acquireLock(r.lock)
	if err != nil {
		return nil, err
	}
	defer release()

	lf, err := loadLog(r)
	if err != nil {
		return nil, err
	}
	if label != "" {
		for i := range lf.Entries {
			if lf.Entries[i].Label == label {
				return nil, fmt.Errorf("trail: label %q is already used by entry @%d (use `clim trail capture` without --label, or pick a different name)",
					label, lf.Entries[i].Index)
			}
		}
	}

	// Write the object only after every pre-condition has been validated,
	// so we never leave an orphan if the entry append is rejected.
	if err := writeObject(r, id, body); err != nil {
		return nil, fmt.Errorf("writing trail object: %w", err)
	}

	nextIdx := 0
	if len(lf.Entries) > 0 {
		nextIdx = lf.Entries[len(lf.Entries)-1].Index + 1
	}
	entry := Entry{
		Index:     nextIdx,
		Object:    id,
		Time:      time.Now().UTC(),
		Operation: op,
		Label:     label,
		Summary:   summarize(r, lf, &snap),
	}
	lf.Entries = append(lf.Entries, entry)
	if err := saveLog(r, lf); err != nil {
		return nil, err
	}
	return &entry, nil
}

// summarize builds a one-line description of how this entry differs from
// the previous one. Empty for the first entry. Uses pre-computed roots so
// tests' overrideRoot is honored.
func summarize(r fsRoots, lf *logFile, current *Snapshot) string {
	if len(lf.Entries) == 0 {
		return fmt.Sprintf("%d tool(s)", len(current.Tools))
	}
	prevID := lf.Entries[len(lf.Entries)-1].Object
	prev, err := readObject(r, prevID)
	if err != nil {
		return ""
	}
	d := diffSnapshots(prev, current)
	parts := make([]string, 0, 4)
	if len(d.Added) > 0 {
		parts = append(parts, fmt.Sprintf("+%d", len(d.Added)))
	}
	if len(d.Removed) > 0 {
		parts = append(parts, fmt.Sprintf("-%d", len(d.Removed)))
	}
	if len(d.VersionChanged) > 0 {
		parts = append(parts, fmt.Sprintf("~%d", len(d.VersionChanged)))
	}
	if len(d.SourceChanged) > 0 {
		parts = append(parts, fmt.Sprintf("⇄%d", len(d.SourceChanged)))
	}
	if len(parts) == 0 {
		return "no changes"
	}
	return strings.Join(parts, " ")
}

// LogOptions filters Log results.
type LogOptions struct {
	Since time.Time // entries strictly after this time
	Limit int       // 0 = no limit
}

// Log returns entries newest-first, optionally filtered.
func Log(opts LogOptions) ([]Entry, error) {
	r, err := resolveRoots()
	if err != nil {
		return nil, err
	}
	lf, err := loadLog(r)
	if err != nil {
		return nil, err
	}
	out := make([]Entry, 0, len(lf.Entries))
	for i := len(lf.Entries) - 1; i >= 0; i-- {
		e := lf.Entries[i]
		if !opts.Since.IsZero() && !e.Time.After(opts.Since) {
			continue
		}
		out = append(out, e)
		if opts.Limit > 0 && len(out) >= opts.Limit {
			break
		}
	}
	return out, nil
}

// Show returns the snapshot referenced by refSpec along with the matched entry.
//
// The ref resolution and object read happen under a single trail lock so a
// concurrent `prune` cannot remove the just-resolved entry's object file
// between Resolve and readObject — otherwise `clim trail show <ref>` could
// fail with a missing-object error even though the ref was valid when the
// command started.
func Show(refSpec string) (*Snapshot, *Entry, error) {
	r, err := resolveRoots()
	if err != nil {
		return nil, nil, err
	}
	release, err := acquireLock(r.lock)
	if err != nil {
		return nil, nil, err
	}
	defer release()

	entry, err := Resolve(refSpec)
	if err != nil {
		return nil, nil, err
	}
	snap, err := readObject(r, entry.Object)
	if err != nil {
		return nil, nil, err
	}
	return snap, entry, nil
}
