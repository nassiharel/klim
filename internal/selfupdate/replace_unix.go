//go:build !windows

package selfupdate

import "os"

// cleanupOld deletes the old binary. On Unix this succeeds immediately
// because the OS allows deleting a file even when a process has it open
// (the inode stays until the last fd closes).
func cleanupOld(oldPath string) {
	_ = os.Remove(oldPath)
}
