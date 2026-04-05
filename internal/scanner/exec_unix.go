//go:build !windows

package scanner

import "os"

// isExecutable returns true if the file has any executable permission bit set.
func isExecutable(_ string, info os.FileInfo) bool {
	return info.Mode()&0111 != 0
}

// normalizeName returns the basename as-is on Unix systems.
func normalizeName(name string) string {
	return name
}
