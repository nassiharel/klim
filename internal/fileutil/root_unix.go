//go:build !windows

package fileutil

import "os"

// runningAsRoot reports whether the current process has root/EUID-0
// privileges. Used by tests to skip permission-bit-dependent cases
// (root bypasses chmod gates). Always false on Windows — see the
// _windows.go variant.
func runningAsRoot() bool {
	return os.Geteuid() == 0
}
