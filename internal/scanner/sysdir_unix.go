//go:build !windows

package scanner

import (
	"path/filepath"
	"strings"
)

// systemPrefixes are OS-level directories that contain system administration
// tools rather than developer tools. Skipped by default.
var systemPrefixes = []string{
	"/sbin",
	"/usr/sbin",
	"/usr/libexec",
}

// isSystemDir returns true if the directory is an OS system directory
// that should be skipped when scanning for developer tools.
//
// On Unix, this skips /sbin, /usr/sbin, and /usr/libexec.
// Notably, /usr/bin is NOT skipped since it contains many dev tools.
func isSystemDir(dir string) bool {
	cleaned := filepath.Clean(dir)

	for _, prefix := range systemPrefixes {
		if cleaned == prefix || strings.HasPrefix(cleaned, prefix+"/") {
			return true
		}
	}

	return false
}
