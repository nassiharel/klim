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

	// If one version has extra trailing segments, check they're zeros
	// (or build metadata that doesn't affect the release version).
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
