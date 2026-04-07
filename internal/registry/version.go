package registry

import (
	"strconv"
	"strings"
)

// VersionsMatch checks if two version strings refer to the same release,
// accounting for:
//   - Trailing ".0" segments: "7.6.0" = "7.6.0.500" (build number ignored)
//   - PE version padding: "1.23.1400" ≈ "1.23.14" (patch×100 encoding)
//   - Extra build metadata: "2.53.0.2" ≈ "2.53.0" (git windows build tag)
func VersionsMatch(installed, latest string) bool {
	iParts := parseSegments(installed)
	lParts := parseSegments(latest)

	if len(iParts) == 0 || len(lParts) == 0 {
		return installed == latest
	}

	// Compare the minimum number of segments both versions share.
	minLen := len(iParts)
	if len(lParts) < minLen {
		minLen = len(lParts)
	}

	for i := 0; i < minLen; i++ {
		if iParts[i] != lParts[i] {
			// Check PE padding: 1400 might encode 14 (×100).
			if isPaddedMatch(iParts[i], lParts[i]) {
				continue
			}
			return false
		}
	}

	// If one version has extra trailing segments, they must be zeros
	// (build numbers / metadata that don't affect the release version).
	longer := iParts
	if len(lParts) > len(iParts) {
		longer = lParts
	}
	for i := minLen; i < len(longer); i++ {
		if longer[i] != 0 {
			return false
		}
	}
	return true
}

// parseSegments splits a version into integer segments.
// "1.23.14" → [1, 23, 14], "2.53.0.2" → [2, 53, 0, 2]
func parseSegments(v string) []int {
	parts := strings.Split(v, ".")
	segments := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			break // stop at non-numeric segments (e.g. "windows" in "2.53.0.windows.1")
		}
		segments = append(segments, n)
	}
	return segments
}

// isPaddedMatch checks if one number is a zero-padded encoding of the other.
// PE versions sometimes encode patch "14" as "1400" (×100).
func isPaddedMatch(a, b int) bool {
	if a == b {
		return true
	}
	if a > b && b > 0 && a%b == 0 {
		factor := a / b
		return factor == 10 || factor == 100 || factor == 1000
	}
	if b > a && a > 0 && b%a == 0 {
		factor := b / a
		return factor == 10 || factor == 100 || factor == 1000
	}
	return false
}

// CompareVersions compares two version strings numerically, segment by segment.
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
// Parsing stops at the first non-numeric segment (e.g. "2.53.0.windows.1"
// is compared as [2, 53, 0]), so versions differing only in non-numeric
// suffixes are treated as equal.
func CompareVersions(a, b string) int {
	aParts := parseSegments(a)
	bParts := parseSegments(b)

	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}

	for i := 0; i < maxLen; i++ {
		var av, bv int
		if i < len(aParts) {
			av = aParts[i]
		}
		if i < len(bParts) {
			bv = bParts[i]
		}
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}
	return 0
}
