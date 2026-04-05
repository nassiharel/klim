//go:build windows

package scanner

import (
	"os"
	"path/filepath"
	"strings"
)

// isSystemDir returns true if the directory is an OS system directory
// that should be skipped when scanning for developer tools.
//
// On Windows, this skips:
//   - Everything under %SystemRoot% (typically C:\Windows), including
//     System32, SysWOW64, and their subdirectories.
//   - Git for Windows' bundled Unix environment directories (usr\bin,
//     mingw64\bin, mingw32\bin) which contain 200+ coreutils like cat,
//     grep, awk, etc. that are not developer tools per se.
func isSystemDir(dir string) bool {
	absDir := cleanLower(dir)

	// Skip everything under %SystemRoot% (C:\Windows).
	systemRoot := os.Getenv("SystemRoot")
	if systemRoot == "" {
		systemRoot = `C:\Windows`
	}
	if isUnderDir(absDir, cleanLower(systemRoot)) {
		return true
	}

	// Skip Git for Windows' bundled Unix coreutils directories.
	// These are typically at:
	//   C:\Program Files\Git\usr\bin     (~100 coreutils: cat, grep, sed, awk, ...)
	//   C:\Program Files\Git\mingw64\bin (~80 MinGW tools: bzip2, curl, openssl, ...)
	//   C:\Program Files\Git\mingw32\bin (32-bit variant, rare)
	// The main Git executables (git.exe, git-bash.exe) live in
	// C:\Program Files\Git\cmd which is NOT matched here.
	if isGitBundledDir(absDir) {
		return true
	}

	return false
}

// isGitBundledDir checks if the directory is a Git-for-Windows bundled
// Unix environment directory (usr\bin, mingw64\bin, mingw32\bin).
func isGitBundledDir(lowerDir string) bool {
	gitBundledSuffixes := []string{
		`\usr\bin`,
		`\usr\lib`,
		`\mingw64\bin`,
		`\mingw64\libexec`,
		`\mingw32\bin`,
		`\mingw32\libexec`,
	}

	for _, suffix := range gitBundledSuffixes {
		if strings.HasSuffix(lowerDir, suffix) {
			// Verify it's actually under a "Git" directory.
			parent := lowerDir[:len(lowerDir)-len(suffix)]
			if strings.HasSuffix(parent, `\git`) || strings.Contains(parent, `\git\`) {
				return true
			}
		}
	}

	return false
}

// cleanLower returns the absolute, cleaned, lowercased version of a path.
func cleanLower(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return strings.ToLower(filepath.Clean(abs))
}

// isUnderDir checks if child is equal to or under parent (both pre-cleaned and lowered).
func isUnderDir(child, parent string) bool {
	if child == parent {
		return true
	}
	return strings.HasPrefix(child, parent+string(filepath.Separator))
}
