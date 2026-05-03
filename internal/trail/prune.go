package trail

import (
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
	release, err := acquireLock(r.lock)
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
	if res.EntriesRemoved > 0 {
		lf.Entries = kept
		if err := saveLog(r, lf); err != nil {
			return res, err
		}
	}

	keptObjects, removedObjects, err := gcObjects(r, kept)
	if err != nil {
		return res, err
	}
	res.ObjectsKept = keptObjects
	res.ObjectsRemoved = removedObjects
	return res, nil
}

// gcObjects walks objects/ and removes any file whose hash isn't referenced
// by an entry in keep.
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
		// content-addressed files is garbage — clim owns this dir
		// (see writeObject) and the only valid layout is
		// <aa>/<bb...>.yaml where the full filename is a 64-char
		// hex Object ID. Garbage gets removed so trail prune
		// actually GCs the store as advertised, instead of leaving
		// stray files behind forever.
		if !strings.HasSuffix(path, ".yaml") {
			if rmErr := os.Remove(path); rmErr != nil { //nolint:gosec
				return fmt.Errorf("removing unrecognized file %s: %w", path, rmErr)
			}
			removedCount++
			return nil
		}
		// Reconstruct id from <objects>/<aa>/<rest>.yaml.
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
		if err := os.Remove(path); err != nil { //nolint:gosec // G122: trail.objects is owned by clim, no symlinks expected
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
	return ObjectID(strings.ToLower(parts[0] + parts[1]))
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
