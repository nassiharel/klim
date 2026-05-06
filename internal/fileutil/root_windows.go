//go:build windows

package fileutil

// runningAsRoot reports whether the current process has root/EUID-0
// privileges. On Windows there is no euid model, so we conservatively
// return false — tests that gate on this run normally on Windows.
// (The Administrator-equivalent check would need golang.org/x/sys
// which we don't import here; the only callers care about the POSIX
// "root bypasses chmod gates" case anyway.)
func runningAsRoot() bool {
	return false
}
