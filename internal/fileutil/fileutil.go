// Package fileutil provides shared file I/O primitives for clim:
// atomic writes, YAML serialization, and directory helpers.
package fileutil

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// AtomicWrite writes data to path atomically via a temp file + rename.
//
// Two robustness behaviours layered on top of the basic temp+rename:
//
//   - **Symlink preservation**: when path is a symlink, the temp file
//     is created next to the symlink's target and the rename targets
//     the resolved path. This keeps the symlink intact (matches what
//     os.WriteFile already does on POSIX). Without this, every
//     overwrite would silently replace the symlink with a regular
//     file.
//
//   - **Crash-safe overwrite on Windows**: os.Rename in Go 1.5+ uses
//     MoveFileExW(..., MOVEFILE_REPLACE_EXISTING) and is atomic on
//     NTFS in the common case. On filesystems / scenarios where it
//     fails (e.g. cross-volume, locked files), we fall back to a
//     rename-rename-remove sequence: rename existing dest →
//     dest.bak-XXX, rename tmp → dest, remove the backup. At every
//     point in that sequence either dest or dest.bak-XXX has the
//     correct contents — the previous "remove and retry" path could
//     leave the dest gone entirely if the second rename failed.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	// Resolve symlinks so we write to the target file, not to the
	// link itself. EvalSymlinks fails when path doesn't exist (the
	// common first-write case) — fall back to the original path.
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	}

	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, filepath.Base(target)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	// Happy path: most filesystems do an atomic replace here.
	if err := os.Rename(tmpPath, target); err == nil {
		return nil
	}

	// Fallback: backup-rename-cleanup so we never have a window where
	// `target` is missing entirely.
	backup := target + ".clim-bak"
	// Best-effort: clear any leftover backup from a previous crash.
	_ = os.Remove(backup)

	if _, statErr := os.Stat(target); statErr == nil {
		if err := os.Rename(target, backup); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("backing up %s before atomic write: %w", target, err)
		}
	}
	if err := os.Rename(tmpPath, target); err != nil {
		// Roll back: restore the backup if we created one.
		if _, backupErr := os.Stat(backup); backupErr == nil {
			_ = os.Rename(backup, target)
		}
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename %s → %s: %w", tmpPath, target, err)
	}
	// Tmp is now in place; remove the backup. A failure here is
	// non-fatal — the new file is already at target.
	_ = os.Remove(backup)
	return nil
}

// EnsureDir creates all parent directories for path if they don't exist.
func EnsureDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

// ReadYAML reads a YAML file at path and unmarshals into dest.
// Returns found=false with nil error if the file does not exist.
func ReadYAML(path string, dest any) (found bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("reading %s: %w", filepath.Base(path), err)
	}
	if err := yaml.Unmarshal(data, dest); err != nil {
		return false, fmt.Errorf("parsing %s: %w", filepath.Base(path), err)
	}
	return true, nil
}

// WriteYAML marshals src to YAML, prepends header, ensures parent dirs exist,
// and writes atomically.
func WriteYAML(path string, src any, header string) error {
	data, err := yaml.Marshal(src)
	if err != nil {
		return fmt.Errorf("marshalling %s: %w", filepath.Base(path), err)
	}

	if err := EnsureDir(path); err != nil {
		return err
	}

	content := []byte(header + string(data))
	return AtomicWrite(path, content, 0o644)
}
