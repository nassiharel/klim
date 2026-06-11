package enrich

import (
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// GroupNoisyParents are CWD path segments that are not useful as group
// names on their own ("repos", "github", etc. tell you nothing about
// what the session was about). When the last segment of a cwd is in
// this set we fall through to the next grouping heuristic.
//
// Sourced from ghcpCliDashboard/src/grouping.py.
var GroupNoisyParents = map[string]bool{
	"users":      true,
	"user":       true,
	"home":       true,
	"repos":      true,
	"repo":       true,
	"github":     true,
	"github.com": true,
	"src":        true,
	"dev":        true,
	"code":       true,
	"projects":   true,
	"work":       true,
	"workspace":  true,
}

// GroupKeywordBuckets maps title-substring keywords to group names. The
// resolver returns the first bucket whose key occurs (case-insensitive)
// in the session title — useful when a session was started outside any
// repo (e.g. in $HOME).
var GroupKeywordBuckets = []struct {
	Keyword string
	Group   string
}{
	{"pr review", "PR Reviews"},
	{"code review", "PR Reviews"},
	{"bug", "Bug Fixes"},
	{"fix", "Bug Fixes"},
	{"docs", "Docs"},
	{"documentation", "Docs"},
	{"refactor", "Refactors"},
	{"test", "Tests"},
}

// Resolve picks the smart group label for a session.
//
// The four-step fallback chain mirrors ghcpCliDashboard:
//
//  1. user-supplied mapping: if any key in `mappings` is a substring
//     of cwd, return its value.
//  2. repository name, if non-empty.
//  3. last segment of cwd, when not in GroupNoisyParents.
//  4. keyword bucket on title, else "Other".
//
// Special case: when cwd equals (or is) the user's home directory,
// return "🏠 Home" so personal sessions sort under a clear header.
//
// All arguments are tolerated empty — the function never panics and
// always returns a non-empty group name (the literal "Other" is the
// last-resort fallback).
func Resolve(cwd, repo, title, home string, mappings map[string]string) string {
	// 1. mappings: substring match (so "/dev/klim" matches the key
	//    "klim"). Iterate keys in a deterministic order so two
	//    overlapping patterns always resolve to the same group across
	//    runs / refreshes. Longer keys are tried first — a more
	//    specific pattern beats a more generic one (e.g. "klim-fork"
	//    wins over "klim" when the cwd contains both).
	if cwd != "" && len(mappings) > 0 {
		keys := make([]string, 0, len(mappings))
		for k := range mappings {
			if k != "" {
				keys = append(keys, k)
			}
		}
		sort.Slice(keys, func(i, j int) bool {
			if len(keys[i]) != len(keys[j]) {
				return len(keys[i]) > len(keys[j])
			}
			return keys[i] < keys[j]
		})
		for _, needle := range keys {
			if strings.Contains(cwd, needle) {
				return mappings[needle]
			}
		}
	}

	// Home special case (computed after mappings so a user can
	// override it explicitly).
	if cwd != "" && home != "" && samePath(cwd, home) {
		return "🏠 Home"
	}

	// 2. repository name.
	if r := strings.TrimSpace(repo); r != "" {
		return r
	}

	// 3. last segment of cwd, unless it's noisy.
	if cwd != "" {
		last := strings.ToLower(filepath.Base(filepath.Clean(cwd)))
		if last != "" && last != "." && last != "/" && !GroupNoisyParents[last] {
			return filepath.Base(filepath.Clean(cwd))
		}
	}

	// 4. keyword bucket on the title.
	if title != "" {
		lower := strings.ToLower(title)
		for _, b := range GroupKeywordBuckets {
			if strings.Contains(lower, b.Keyword) {
				return b.Group
			}
		}
	}

	return "Other"
}

// samePath compares two paths after cleaning. It does NOT resolve
// symlinks.
//
// On Windows the comparison is case-insensitive — NTFS / Windows
// shell APIs treat paths as case-insensitive in practice, so a cwd
// like `C:\Users\Nas` (from the install registry, which preserves
// the registered casing) must match `%USERPROFILE%` of `C:\Users\nas`
// for the "🏠 Home" grouping special-case to fire reliably.
// POSIX semantics stay case-sensitive — Linux / macOS HFS can have
// distinct entries at `/Users/Nas` and `/Users/nas`.
func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
