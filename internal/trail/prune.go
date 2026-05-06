package trail

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PruneOptions configures Prune behavior. Both filters apply (AND): an
// entry is kept only if it satisfies both Keep and OlderThan.
type PruneOptions struct {
	// Keep is the maximum number of newest entries to retain. 0 disables.
	Keep int

	// OlderThan drops entries whose Time is strictly before now - OlderThan.
	// Zero disables the time filter.
	OlderThan time.Duration
}

// PruneResult summarises a Prune call.
type PruneResult struct {
	EntriesKept    int `yaml:"entries_kept"    json:"entries_kept"`
	EntriesRemoved int `yaml:"entries_removed" json:"entries_removed"`
	ObjectsKept    int `yaml:"objects_kept"    json:"objects_kept"`
	ObjectsRemoved int `yaml:"objects_removed" json:"objects_removed"`
}

// Prune applies opts to the trail, removing log entries that fall outside
// the retention window and garbage-collecting any objects that no entry
// references afterwards.
//
// At least one of Keep / OlderThan must be set, otherwise Prune is a no-op.
func Prune(opts PruneOptions) (PruneResult, error) {
	var res PruneResult
	if opts.Keep <= 0 && opts.OlderThan <= 0 {
		return res, nil
	}
	r, err := resolveRoots()
	if err != nil {
		return res, err
	}
	release, err := acquireLock(r.lock, false)
	if err != nil {
		return res, err
	}
	defer release()

	lf, err := loadLog(r)
	if err != nil {
		return res, err
	}

	cutoff := time.Time{}
	if opts.OlderThan > 0 {
		cutoff = time.Now().Add(-opts.OlderThan)
	}

	// First pass (chronological): decide which entries pass the time
	// filter. We do this before applying --keep so the two filters
	// combine with AND (drop if too old, then keep newest N of the
	// survivors) regardless of clock skew. A reverse-pass that
	// applied both filters at once would let an old entry that
	// happened to satisfy --since still consume a --keep slot when
	// timestamps weren't monotonic with index — which can happen
	// after a clock jump or when a synced trail mixes captures from
	// different machines.
	timeFiltered := make([]Entry, 0, len(lf.Entries))
	for _, e := range lf.Entries {
		if !cutoff.IsZero() && e.Time.Before(cutoff) {
			continue
		}
		timeFiltered = append(timeFiltered, e)
	}

	// Second pass: keep the newest --keep entries from the
	// time-filtered set. Walk newest-first by index (the entries are
	// stored in monotonic-index order) so "newest" is well-defined
	// even when timestamps disagree with indices.
	kept := timeFiltered
	if opts.Keep > 0 && len(timeFiltered) > opts.Keep {
		kept = timeFiltered[len(timeFiltered)-opts.Keep:]
	}

	res.EntriesKept = len(kept)
	res.EntriesRemoved = len(lf.Entries) - len(kept)

	// VALIDATE BEFORE COMMITTING. Every kept entry must have a
	// usable (present + content-correct) object on disk. Verifying
	// up front means a corrupted/missing object can't trick prune
	// into rewriting log.yaml with references show/diff can never
	// load. readObject re-hashes the stored bytes against the id,
	// so this catches both ENOENT and silent on-disk corruption.
	for _, e := range kept {
		if _, err := readObject(r, e.Object); err != nil {
			return res, fmt.Errorf("trail: kept entry @%d (%s) is unusable, refusing to prune: %w",
				e.Index, e.Object.Short(), err)
		}
	}

	// Remember whether the log was successfully committed but HEAD
	// failed to update, so we can still GC orphaned objects below.
	// Treating *HeadWriteError as a hard failure would skip gcObjects
	// even though the entry deletions are durable, leaving orphans on
	// disk until the next prune.
	var headErr error
	if res.EntriesRemoved > 0 {
		lf.Entries = kept
		if err := saveLog(r, lf); err != nil {
			var hwe *HeadWriteError
			if errors.As(err, &hwe) {
				headErr = err
			} else {
				return res, err
			}
		}
	}

	keptObjects, removedObjects, err := gcObjects(r, kept)
	if err != nil {
		return res, err
	}
	res.ObjectsKept = keptObjects
	res.ObjectsRemoved = removedObjects
	// Surface the HEAD write error after GC has done its work, so
	// callers see both the partial success (entries + objects pruned)
	// and the warning that HEAD is now stale.
	if headErr != nil {
		return res, headErr
	}
	return res, nil
}

// gcObjects walks objects/ and removes any file whose hash isn't
// referenced by an entry in keep. The pre-saveLog validation in Prune
// has already verified that every kept entry's object exists and
// hashes correctly, so this function only handles the cleanup half:
// orphans, garbage names, and non-yaml files.
func gcObjects(r fsRoots, keep []Entry) (int, int, error) {
	wanted := make(map[ObjectID]struct{}, len(keep))
	for _, e := range keep {
		wanted[e.Object] = struct{}{}
	}
	keptCount := 0
	removedCount := 0
	if _, err := os.Stat(r.objects); os.IsNotExist(err) {
		return 0, 0, nil
	}
	err := filepath.WalkDir(r.objects, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		// Anything under objects/ that doesn't look like one of our
		// content-addressed files is garbage — klim owns this dir
		// (see writeObject) and the only valid layout is
		// <aa>/<bb...>.yaml where the full filename is a 64-char
		// hex Object ID.
		if !strings.HasSuffix(path, ".yaml") {
			if rmErr := os.Remove(path); rmErr != nil { //nolint:gosec
				return fmt.Errorf("removing unrecognized file %s: %w", path, rmErr)
			}
			removedCount++
			return nil
		}
		rel, err := filepath.Rel(r.objects, path)
		if err != nil {
			return err
		}
		id := pathToID(rel)
		if !id.IsValid() {
			if rmErr := os.Remove(path); rmErr != nil { //nolint:gosec
				return fmt.Errorf("removing malformed object file %s: %w", path, rmErr)
			}
			removedCount++
			return nil
		}
		if _, want := wanted[id]; want {
			keptCount++
			return nil
		}
		if err := os.Remove(path); err != nil { //nolint:gosec // G122: trail.objects is owned by klim, no symlinks expected
			return fmt.Errorf("removing orphan object %s: %w", id.Short(), err)
		}
		removedCount++
		return nil
	})
	if err != nil {
		return keptCount, removedCount, err
	}
	// Best-effort: prune empty fanout dirs.
	pruneEmptyFanout(r.objects)
	return keptCount, removedCount, nil
}

// pathToID extracts the canonical 64-char object id from a relative
// path of the exact form `<aa>/<bb...>.yaml` (where aa is exactly 2
// hex chars and bb... is the remaining 62). Any other shape — root
// level files, single-segment paths, three-or-more segments, or
// segments whose lengths don't add up to 64 — returns the empty
// ObjectID so the caller treats the file as garbage and removes it.
// readObject only ever resolves the canonical path via objectPath;
// this strict reverse mapping makes prune match.
func pathToID(rel string) ObjectID {
	rel = strings.TrimSuffix(rel, ".yaml")
	rel = filepath.ToSlash(rel)
	parts := strings.Split(rel, "/")
	if len(parts) != 2 {
		return ""
	}
	if len(parts[0]) != 2 || len(parts[1]) != 62 {
		return ""
	}
	// objectPath / readObject only ever look under the lowercase
	// fanout, so we must reject mixed-case file names rather than
	// silently lowercasing them. A hand-renamed `objects/AB/...`
	// would otherwise be treated as a kept object that show/diff
	// can never actually open.
	id := ObjectID(parts[0] + parts[1])
	if !id.IsValid() {
		return ""
	}
	return id
}

func pruneEmptyFanout(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(root, e.Name())
		children, err := os.ReadDir(sub)
		if err == nil && len(children) == 0 {
			_ = os.Remove(sub)
		}
	}
}
