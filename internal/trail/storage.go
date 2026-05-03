package trail

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode"

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
// See writeObjectReportingCreated; this preserves the original signature
// for callers that don't need the boolean.
func writeObject(r fsRoots, id ObjectID, body []byte) error {
	_, err := writeObjectReportingCreated(r, id, body)
	return err
}

// writeObjectReportingCreated is writeObject's full-information variant:
// it returns true only when this call wrote a fresh file (vs. observed
// an existing one with matching hash). Capture uses the boolean to
// decide whether a saveLog rollback should also delete the object —
// touching an idempotent reuse would clobber state another in-flight
// capture might depend on.
//
// If the object already exists on disk, we re-hash the stored bytes and
// compare against id. A mismatch means a previous capture wrote a good
// object that has since been corrupted or hand-edited; trusting the
// filename alone would let a later capture of the same environment
// silently point at the corrupted data, breaking `show <ref>` after a
// successful `capture`. Failing here is the safe choice — the user can
// delete the offending file and recapture.
func writeObjectReportingCreated(r fsRoots, id ObjectID, body []byte) (bool, error) {
	path := r.objectPath(id)
	if existing, err := os.ReadFile(path); err == nil {
		sum := sha256.Sum256(existing)
		got := ObjectID(hex.EncodeToString(sum[:]))
		if got == id {
			return false, nil
		}
		return false, fmt.Errorf("trail object %s on disk has been corrupted: stored bytes hash to %s (delete %s and recapture)",
			id.Short(), got.Short(), path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspecting trail object %s: %w", id.Short(), err)
	}
	if err := fileutil.EnsureDir(path); err != nil {
		return false, fmt.Errorf("creating object dir: %w", err)
	}
	if err := fileutil.AtomicWrite(path, body, 0o644); err != nil {
		return false, err
	}
	return true, nil
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
// Unknown fields, unknown schema versions, and trailing YAML documents
// are rejected — clim writes exactly one document per object file, so
// any extra content was added by a hand-edit or corruption.
func decodeSnapshot(data []byte, id ObjectID) (*Snapshot, error) {
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	var s Snapshot
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("parsing trail object %s: %w", id.Short(), err)
	}
	// Same trailing-content guard as loadLog: a snapshot file with
	// extra YAML documents is rejected so the strict-decoding contract
	// holds for object files as well as the log.
	var trailing Snapshot
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("trail object %s has trailing YAML content (corrupted or hand-edited?)", id.Short())
	}
	// SchemaVersion == 0 means the field was missing; we have never
	// written versionless objects, so this is corruption / hand-edit
	// rather than a forward-compat issue. Distinguish the message so
	// users aren't told to "upgrade clim" when nothing newer can fix it.
	if s.SchemaVersion == 0 {
		return nil, fmt.Errorf("trail object %s is missing schema_version (corrupted or hand-edited?)", id.Short())
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
// only when the rest of the trail is also empty — if HEAD or any
// objects/ entries already exist, a missing log indicates partial
// corruption (lost rename, manual deletion) and we fail closed rather
// than silently start a fresh history at @0 that hides the prior
// snapshots. A partially-corrupt or version-mismatched file always
// fails closed.
func loadLog(r fsRoots) (*logFile, error) {
	data, err := os.ReadFile(r.log)
	if errors.Is(err, os.ErrNotExist) {
		if hasTrailRemnants(r) {
			return nil, fmt.Errorf("trail log is missing but other trail state still exists (HEAD or objects/) — refusing to silently start over; delete %s to recover from a known-empty state", r.dir)
		}
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
	// Strict: log.yaml must contain exactly one YAML document. A
	// hand-edited file with a trailing `---` separator and additional
	// content would otherwise be silently accepted, breaking the
	// "strict decoding" guarantee this feature advertises.
	var trailing logFile
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("trail log has trailing YAML content (corrupted or hand-edited?) — delete %s to start over", r.log)
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
	// Validate every entry's Object id, Index, and Label field. The
	// log is the canonical history record, and Capture has always
	// rejected reserved/control-character labels — accepting them at
	// load time would let a hand-edited file create entries that are
	// permanently unresolvable by name (or break trail-log/show
	// rendering). Entry.Object is later passed to objectPath which
	// uses it as a filesystem path fragment; entry indices form the
	// @<index> ref form which must stay unique and monotonic.
	seen := make(map[int]struct{}, len(lf.Entries))
	prevIdx := -1
	for i := range lf.Entries {
		e := &lf.Entries[i]
		if !e.Object.IsValid() {
			return nil, fmt.Errorf("trail log entry @%d has invalid object id %q (corrupted or hand-edited?)",
				e.Index, e.Object)
		}
		if e.Index < 0 {
			return nil, fmt.Errorf("trail log entry has negative index %d (corrupted or hand-edited?)", e.Index)
		}
		if _, dup := seen[e.Index]; dup {
			return nil, fmt.Errorf("trail log has duplicate entry index @%d (corrupted or hand-edited?)", e.Index)
		}
		if e.Index <= prevIdx {
			return nil, fmt.Errorf("trail log entries are not strictly increasing (got @%d after @%d, corrupted or hand-edited?)", e.Index, prevIdx)
		}
		// Same structural label rules Capture would have rejected up
		// front. Trim+ValidateLabel mirrors Capture; whitespace-only
		// labels become "" (no rejection), but leading/trailing space
		// indicates a hand-edit and is treated as corruption.
		if e.Label != strings.TrimSpace(e.Label) {
			return nil, fmt.Errorf("trail log entry @%d has label %q with leading/trailing whitespace (corrupted or hand-edited?)",
				e.Index, e.Label)
		}
		if err := ValidateLabel(e.Label); err != nil {
			return nil, fmt.Errorf("trail log entry @%d: %w", e.Index, err)
		}
		seen[e.Index] = struct{}{}
		prevIdx = e.Index
	}
	return &lf, nil
}

// hasTrailRemnants reports whether the trail dir contains state other
// than log.yaml. Used to distinguish "fresh install" from "log got
// lost". HEAD or any non-empty objects/ subtree counts as evidence the
// store was once populated. An unreadable objects/ directory (perms,
// transient IO) is treated as "remnants exist" — failing closed is
// safer than silently letting a fresh history start over a possibly-
// populated but unreadable store.
func hasTrailRemnants(r fsRoots) bool {
	if _, err := os.Stat(r.head); err == nil {
		return true
	}
	entries, err := os.ReadDir(r.objects)
	if err != nil {
		// Distinguish "objects/ doesn't exist yet" (real fresh install)
		// from any other error (likely real remnants we can't see).
		if errors.Is(err, os.ErrNotExist) {
			return false
		}
		return true
	}
	return len(entries) > 0
}

// saveLog writes log.yaml (and best-effort HEAD) under the trail lock.
//
// log.yaml is the durable record — once it's been atomically renamed,
// the trail is committed. HEAD is informational: readers always derive
// the newest entry from the slice tail of log.yaml, so a stale or
// missing HEAD doesn't break correctness. Therefore:
//
//   - If marshal or log.yaml write fails → return a regular error.
//     Callers may roll back the just-written object: nothing is
//     committed yet.
//   - If log.yaml write succeeds but HEAD write fails → return a
//     *headWriteError. The log is durable, so callers MUST NOT roll
//     back the object — the entry already exists in the canonical
//     record and a future capture (or any read) will refresh HEAD.
//
// Callers that don't care about the distinction can treat the error
// as a generic failure; errors.As(&headWriteError) recovers the
// classification.
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
		return &headWriteError{err: fmt.Errorf("writing trail HEAD: %w", err)}
	}
	return nil
}

// headWriteError signals that log.yaml was committed successfully but
// the HEAD pointer file failed to update. The trail is durable; the
// caller must not roll back the new object.
type headWriteError struct{ err error }

func (e *headWriteError) Error() string { return e.err.Error() }
func (e *headWriteError) Unwrap() error { return e.err }

// ValidateLabel performs the structural label checks Capture would
// otherwise apply only after loading the trail. Exposed so CLI runners
// can fail fast on bad labels (control characters, reserved ref syntax)
// before running a slow PATH scan and surface the failure as a usage
// error instead of a post-scan runtime error.
//
// Duplicate-label collisions are NOT checked here because they require
// reading log.yaml under the trail lock; that check stays in Capture.
func ValidateLabel(label string) error {
	label = strings.TrimSpace(label)
	if label == "" {
		return nil
	}
	if reason := invalidLabelReason(label); reason != "" {
		return fmt.Errorf("trail: invalid label %q: %s", label, reason)
	}
	if reason := reservedLabelReason(label); reason != "" {
		return fmt.Errorf("trail: label %q conflicts with reserved ref syntax (%s); pick a different name", label, reason)
	}
	return nil
}

// validOps is the closed set of operation kinds permitted in Entry.op.
// ValidateOp consults this so a typo or arbitrary string can't become
// permanent history data — the on-disk format keeps a small,
// well-defined operation vocabulary.
var validOps = map[string]struct{}{
	OpCapture: {},
	OpImport:  {},
	OpInstall: {},
	OpUpgrade: {},
	OpRemove:  {},
}

// ValidateOp returns an error if op is not one of the recognised Op*
// constants. Empty op is treated as OpCapture (the default), so no
// error.
func ValidateOp(op string) error {
	if op == "" {
		return nil
	}
	if _, ok := validOps[op]; !ok {
		known := []string{OpCapture, OpImport, OpInstall, OpUpgrade, OpRemove}
		return fmt.Errorf("trail: invalid op %q (valid: %s)", op, strings.Join(known, ", "))
	}
	return nil
}

// invalidLabelReason rejects labels that would corrupt the human-readable
// `clim trail log` table or `clim trail show` output: tabs and newlines
// would split columns / inject extra lines, and other control runes can
// produce malformed terminal output. Returning a non-empty string ⇒
// reject; empty ⇒ label is structurally OK (further checks may apply).
func invalidLabelReason(label string) string {
	for _, r := range label {
		switch {
		case r == '\t':
			return "labels must not contain tabs"
		case r == '\n' || r == '\r':
			return "labels must not contain line breaks"
		case unicode.IsControl(r):
			return fmt.Sprintf("labels must not contain control character %U", r)
		}
	}
	return ""
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

// LabelInUseError is returned by Capture when the requested label is
// already taken by another entry. CLI runners detect this with
// errors.As and surface it as a UsageError so the exit code matches
// docs/cli-conventions.md (malformed user input ⇒ exit 2).
type LabelInUseError struct {
	Label string
	Index int
}

func (e *LabelInUseError) Error() string {
	return fmt.Sprintf("trail: label %q is already used by entry @%d (use `clim trail capture` without --label, or pick a different name)",
		e.Label, e.Index)
}

// Capture writes a new Entry pointing at the canonical Snapshot of tools.
// Two captures of an identical environment store one object and two entries.
//
// op identifies how the change was triggered (capture / import / install /
// upgrade / remove) and must be one of the constants in this package;
// arbitrary strings are rejected as a UsageError-equivalent.
//
// label is an optional user-provided tag — capture fails if the label
// is already used by another entry (see LabelInUseError), so labels
// stay unique and `Resolve(<label>)` always points at one entry.
func Capture(op, label string, tools []registry.Tool) (*Entry, error) {
	// Op + label structural validation: shared with the CLI's pre-scan
	// validation so a non-CLI caller can't slip a bad value through
	// either. Reusing ValidateOp/ValidateLabel keeps a single source of
	// truth for the closed sets — the previous duplicate inline checks
	// here drifted from CLI-side wording the moment one was edited.
	if err := ValidateOp(op); err != nil {
		return nil, err
	}
	if op == "" {
		op = OpCapture
	}
	if err := ValidateLabel(label); err != nil {
		return nil, err
	}
	label = strings.TrimSpace(label)
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
	// and predecessor against existing entries before writing the
	// object — otherwise a rejected capture would leave an orphan
	// object on disk.
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
				return nil, &LabelInUseError{Label: label, Index: lf.Entries[i].Index}
			}
		}
	}

	// Compute the summary BEFORE writing the object: summarize reads
	// the previous snapshot, and a corrupted predecessor must abort
	// without leaving the new object orphaned on disk. Same reasoning
	// for any later failure — we'll only writeObject after we know
	// the entry will actually be appended.
	summary, err := summarize(r, lf, &snap)
	if err != nil {
		return nil, fmt.Errorf("trail: previous entry's snapshot is unreadable, cannot extend trail: %w (resolve the corruption or run `clim trail prune` to recover)", err)
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
		Summary:   summary,
	}
	lf.Entries = append(lf.Entries, entry)

	// Write the object, then commit the log. If saveLog fails after a
	// fresh object was written, roll back: delete the new file so we
	// don't leak an orphan that prune would later GC anyway. We do
	// NOT roll back if writeObject reused an existing matching object
	// (idempotent path) — distinguishing those without an extra stat
	// would be racy, so writeObject returns whether it created a new
	// file.
	createdObject, err := writeObjectReportingCreated(r, id, body)
	if err != nil {
		return nil, fmt.Errorf("writing trail object: %w", err)
	}
	if err := saveLog(r, lf); err != nil {
		var hwe *headWriteError
		if errors.As(err, &hwe) {
			// log.yaml is durable — the entry is committed. Don't
			// roll back the object; the only loss is the HEAD pointer
			// which is informational and will be refreshed on the
			// next capture or save.
			return &entry, err
		}
		if createdObject {
			_ = os.Remove(r.objectPath(id))
		}
		return nil, err
	}
	return &entry, nil
}

// summarize builds a one-line description of how this entry differs from
// the previous one. For the first entry (no predecessor) it returns
// "<n> tool(s)" so users can tell the trail head from a glance instead
// of seeing an empty Summary column. Returns an error if the previous
// snapshot can't be read — silently emitting an empty summary would
// paper over corruption and let users keep extending broken history.
func summarize(r fsRoots, lf *logFile, current *Snapshot) (string, error) {
	if len(lf.Entries) == 0 {
		return fmt.Sprintf("%d tool(s)", len(current.Tools)), nil
	}
	prevID := lf.Entries[len(lf.Entries)-1].Object
	prev, err := readObject(r, prevID)
	if err != nil {
		return "", err
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
		return "no changes", nil
	}
	return strings.Join(parts, " "), nil
}

// LogOptions filters Log results.
type LogOptions struct {
	Since time.Time // entries strictly after this time
	Limit int       // 0 = no limit
}

// Log returns entries newest-first, optionally filtered.
//
// Acquires the trail lock to read log.yaml so a concurrent capture/prune
// can't catch us mid-rename and report a transient empty history.
func Log(opts LogOptions) ([]Entry, error) {
	r, err := resolveRoots()
	if err != nil {
		return nil, err
	}
	release, err := acquireLock(r.lock)
	if err != nil {
		return nil, err
	}
	defer release()
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

	lf, err := loadLog(r)
	if err != nil {
		return nil, nil, err
	}
	entry, err := resolveSpec(lf, refSpec)
	if err != nil {
		return nil, nil, err
	}
	snap, err := readObject(r, entry.Object)
	if err != nil {
		return nil, nil, err
	}
	return snap, entry, nil
}
