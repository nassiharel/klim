//go:build windows

package selfupdate

import "os"

// cleanupOld attempts to delete the old binary. On Windows, the running
// process locks the executable file, so this will typically fail.
// The .old file can be cleaned up on the next invocation.
func cleanupOld(oldPath string) {
	_ = os.Remove(oldPath)
}
