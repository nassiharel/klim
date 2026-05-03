package trail

import (
	"errors"
	"fmt"
	"os"
	"sort"
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
	release, err := acquireLock(r.lock, true)
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
			// Render each candidate using a long-enough prefix to
			// disambiguate. Short() is always 7 chars, but if two
			// distinct objects share that prefix it'd print the same
			// short hash twice and give the user no usable
			// disambiguation. Find the smallest prefix length L >= 7
			// such that all candidates have distinct prefixes; fall
			// back to the full 64-char hash if even that's needed.
			labels := disambiguatedHashes(seenObject)
			sort.Strings(labels)
			return nil, fmt.Errorf("trail: ref %q is ambiguous (matches %d distinct objects: %s)",
				refSpec, len(seenObject), strings.Join(labels, ", "))
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

// disambiguatedHashes renders each ObjectID with the shortest prefix
// that's unique across the set, with a minimum of 7 chars (the
// canonical Short() length). Falls back to full 64-char IDs only when
// even those collide — which is impossible for distinct object IDs by
// construction, but is the safe fallback if the input ever contained
// duplicates.
func disambiguatedHashes(set map[ObjectID]struct{}) []string {
	ids := make([]ObjectID, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	for n := 7; n <= 64; n++ {
		seen := make(map[string]struct{}, len(ids))
		out := make([]string, 0, len(ids))
		collision := false
		for _, id := range ids {
			pref := string(id)
			if len(pref) > n {
				pref = pref[:n]
			}
			if _, dup := seen[pref]; dup {
				collision = true
				break
			}
			seen[pref] = struct{}{}
			out = append(out, pref)
		}
		if !collision {
			return out
		}
	}
	// Defensive: full hashes still collide. Render them anyway.
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, string(id))
	}
	return out
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
