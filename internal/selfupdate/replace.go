package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
)

// ReplaceBinary replaces the binary at execPath with newBinary.
// It uses a rename-swap strategy that is safe on all platforms:
//
//  1. Write new binary to execPath.new  (same dir = same filesystem → rename is atomic)
//  2. Rename current execPath → execPath.old
//  3. Rename execPath.new → execPath
//  4. Delete execPath.old  (best-effort; may fail on Windows)
//
// If step 3 fails, it attempts to roll back by renaming .old back.
func ReplaceBinary(execPath string, newBinary []byte) error {
	// Resolve symlinks so we replace the actual file, not the link.
	realPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("resolving symlinks for %s: %w", execPath, err)
	}

	oldPath := realPath + ".old"
	newTmpPath := realPath + ".new"

	// Clean up stale files from a previous update (e.g. Windows where
	// .old couldn't be deleted because the binary was still running).
	_ = os.Remove(oldPath)
	_ = os.Remove(newTmpPath)

	// 1. Write new binary to a temp file in the same directory.
	if err := os.WriteFile(newTmpPath, newBinary, 0o755); err != nil {
		return fmt.Errorf("writing new binary: %w", err)
	}

	// 2. Rename current → .old
	if err := os.Rename(realPath, oldPath); err != nil {
		_ = os.Remove(newTmpPath) // clean up
		return fmt.Errorf("backing up current binary: %w", err)
	}

	// 3. Rename .new → target
	if err := os.Rename(newTmpPath, realPath); err != nil {
		// Try to roll back.
		_ = os.Rename(oldPath, realPath)
		return fmt.Errorf("moving new binary into place: %w", err)
	}

	// 4. Clean up old binary (best-effort, platform-specific).
	cleanupOld(oldPath)

	return nil
}
