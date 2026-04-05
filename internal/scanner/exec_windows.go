//go:build windows

package scanner

import (
	"os"
	"path/filepath"
	"strings"
)

// defaultPathExt is used when the PATHEXT environment variable is empty.
var defaultPathExt = []string{".COM", ".EXE", ".BAT", ".CMD", ".VBS", ".VBE", ".JS", ".JSE", ".WSF", ".WSH", ".MSC", ".PS1"}

// isExecutable returns true if the file's extension matches one of the
// entries in the PATHEXT environment variable.
func isExecutable(name string, _ os.FileInfo) bool {
	ext := strings.ToUpper(filepath.Ext(name))
	if ext == "" {
		return false
	}
	for _, pe := range pathExtensions() {
		if ext == pe {
			return true
		}
	}
	return false
}

// normalizeName strips the PATHEXT suffix from the filename so that
// "git.exe" becomes "git". This ensures include/exclude lists work
// uniformly across platforms.
func normalizeName(name string) string {
	ext := strings.ToUpper(filepath.Ext(name))
	if ext == "" {
		return name
	}
	for _, pe := range pathExtensions() {
		if ext == pe {
			return strings.TrimSuffix(name, filepath.Ext(name))
		}
	}
	return name
}

// pathExtensions parses the PATHEXT environment variable, falling back
// to a sensible default if it is empty.
func pathExtensions() []string {
	env := os.Getenv("PATHEXT")
	if env == "" {
		return defaultPathExt
	}

	parts := strings.Split(env, ";")
	exts := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			exts = append(exts, strings.ToUpper(p))
		}
	}
	return exts
}
