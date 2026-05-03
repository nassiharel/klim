package trail

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Resolve maps a refSpec to an Entry. Supported forms:
//
//	HEAD              the newest entry
//	HEAD~N            N entries back from HEAD (N >= 0)
//	@<index>          exact entry index (0-based)
//	<hash>            full 64-char object hash
//	<short>           7+ char object hash prefix (must be unambiguous)
//	<label>           an explicit Entry.Label match (must be unique)
//
// Resolve maps a refSpec to an Entry. Supported forms:
//
//	HEAD              the newest entry
//	HEAD~N            N entries back from HEAD (N >= 0)
//	@<index>          exact entry index (0-based)
//	<hash>            full 64-char object hash
//	<short>           7+ char object hash prefix (must be unambiguous)
//	<label>           an explicit Entry.Label match (must be unique)
//
// "latest" is accepted as an alias for HEAD for terminology comfort.
//
// Resolve acquires the trail lock to load log.yaml so a concurrent
// capture/prune can't observe the lock-free read mid-rename and report
// a transient empty trail. Internal callers that already hold the lock
// (e.g. Show, Diff) must use resolveSpec directly to avoid deadlock.
func Resolve(refSpec string) (*Entry, error) {
	refSpec = strings.TrimSpace(refSpec)
	if refSpec == "" {
		return nil, errors.New("trail: empty ref")
	}
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
	return resolveSpec(lf, refSpec)
}

// resolveSpec is Resolve's pure, lock-free core. The caller must hold
// the trail lock and have loaded lf already.
func resolveSpec(lf *logFile, refSpec string) (*Entry, error) {
	refSpec = strings.TrimSpace(refSpec)
	if refSpec == "" {
		return nil, errors.New("trail: empty ref")
	}
	if len(lf.Entries) == 0 {
		return nil, errors.New("trail: no entries (run `clim trail capture` to create one)")
	}

	// HEAD / HEAD~N / latest.
	if refSpec == "HEAD" || refSpec == "latest" {
		e := lf.Entries[len(lf.Entries)-1]
		return &e, nil
	}
	if rest, ok := strings.CutPrefix(refSpec, "HEAD~"); ok {
		n, err := strconv.Atoi(rest)
		if err != nil || n < 0 {
			return nil, fmt.Errorf("trail: invalid HEAD~ ref %q", refSpec)
		}
		if n >= len(lf.Entries) {
			return nil, fmt.Errorf("trail: HEAD~%d is out of range (%d entries total)", n, len(lf.Entries))
		}
		e := lf.Entries[len(lf.Entries)-1-n]
		return &e, nil
	}
	if rest, ok := strings.CutPrefix(refSpec, "@"); ok {
		idx, err := strconv.Atoi(rest)
		if err != nil || idx < 0 {
			return nil, fmt.Errorf("trail: invalid @<index> ref %q", refSpec)
		}
		for i := range lf.Entries {
			if lf.Entries[i].Index == idx {
				return &lf.Entries[i], nil
			}
		}
		return nil, fmt.Errorf("trail: no entry with index %d", idx)
	}

	// Hash (full or prefix). All-hex chars and at least 7 long.
	if isHexPrefix(refSpec) && len(refSpec) >= 7 {
		// Dedupe matches by Object — multiple entries can point at the
		// same snapshot (that's the whole dedupe story), so we should
		// only consider the hash ambiguous when the prefix matches
		// genuinely different objects. When all matches share one
		// object, return the newest entry that points at it.
		seenObject := make(map[ObjectID]struct{})
		var matches []*Entry
		for i := range lf.Entries {
			if strings.HasPrefix(string(lf.Entries[i].Object), strings.ToLower(refSpec)) {
				if _, dup := seenObject[lf.Entries[i].Object]; !dup {
					seenObject[lf.Entries[i].Object] = struct{}{}
				}
				matches = append(matches, &lf.Entries[i])
			}
		}
		switch {
		case len(seenObject) == 0:
			// fall through to label resolution
		case len(seenObject) == 1:
			// One distinct object — return the newest entry that points at it.
			for i := len(lf.Entries) - 1; i >= 0; i-- {
				if strings.HasPrefix(string(lf.Entries[i].Object), strings.ToLower(refSpec)) {
					return &lf.Entries[i], nil
				}
			}
			// Unreachable given matches != nil.
			return matches[len(matches)-1], nil
		default:
			short := make([]string, 0, len(seenObject))
			for id := range seenObject {
				short = append(short, id.Short())
			}
			return nil, fmt.Errorf("trail: ref %q is ambiguous (matches %d distinct objects: %s)",
				refSpec, len(seenObject), strings.Join(short, ", "))
		}
	}

	// Label match. Must be unique.
	var labelMatches []*Entry
	for i := range lf.Entries {
		if lf.Entries[i].Label != "" && lf.Entries[i].Label == refSpec {
			labelMatches = append(labelMatches, &lf.Entries[i])
		}
	}
	switch len(labelMatches) {
	case 0:
		return nil, fmt.Errorf("trail: ref %q not found", refSpec)
	case 1:
		return labelMatches[0], nil
	default:
		return nil, fmt.Errorf("trail: label %q is ambiguous (matches %d entries)", refSpec, len(labelMatches))
	}
}

// isHexPrefix reports whether s is composed entirely of [0-9a-fA-F].
func isHexPrefix(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// readHEAD reads the integer pointer in HEAD, or returns -1 when missing.
// (Currently informational; the canonical newest entry is the slice tail.)
//
//nolint:unused // Used by debug tooling and integration tests only.
func readHEAD(r fsRoots) (int, error) {
	data, err := os.ReadFile(r.head)
	if errors.Is(err, os.ErrNotExist) {
		return -1, nil
	}
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(data))
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("parsing HEAD: %w", err)
	}
	return n, nil
}
