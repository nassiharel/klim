package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
)

// ReplaceBinary replaces the binary at execPath with newBinary.
// It uses a rename-swap strategy that is safe on all platforms:
//
//  1. Write new binary to a temp file in the same dir (unpredictable name)
//  2. Rename current execPath → execPath.old
//  3. Rename temp → execPath
//  4. Delete execPath.old  (best-effort; may fail on Windows)
//
// If step 3 fails, it attempts to roll back by renaming .old back.
func ReplaceBinary(execPath string, newBinary []byte) error {
	// Resolve symlinks so we replace the actual file, not the link.
	realPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("resolving symlinks for %s: %w", execPath, err)
	}

	dir := filepath.Dir(realPath)
	oldPath := realPath + ".old"

	// Clean up stale .old from a previous update (e.g. Windows where
	// .old couldn't be deleted because the binary was still running).
	_ = os.Remove(oldPath)

	// 1. Write new binary to a temp file in the same directory (same
	//    filesystem ⇒ rename is atomic). os.CreateTemp avoids predictable
	//    names and TOCTOU races.
	tmp, err := os.CreateTemp(dir, ".klim-update-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(newBinary); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("writing new binary: %w", err)
	}
	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("setting binary permissions: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	// 2. Rename current → .old
	if err := os.Rename(realPath, oldPath); err != nil {
		_ = os.Remove(tmpPath) // clean up
		return fmt.Errorf("backing up current binary: %w", err)
	}

	// 3. Rename temp → target
	if err := os.Rename(tmpPath, realPath); err != nil {
		// Roll back: restore the original binary.
		if rbErr := os.Rename(oldPath, realPath); rbErr != nil {
			return fmt.Errorf("moving new binary failed (%w) AND rollback failed (%w) — restore manually from %s", err, rbErr, oldPath)
		}
		return fmt.Errorf("moving new binary into place: %w", err)
	}

	// 4. Clean up old binary (best-effort, platform-specific).
	cleanupOld(oldPath)

	return nil
}
