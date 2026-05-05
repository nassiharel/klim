// Package fileutil provides shared file I/O primitives for clim:
// atomic writes, YAML serialization, and directory helpers.
package fileutil

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// AtomicWrite writes data to path atomically via a temp file + rename.
// AtomicWrite writes data to path atomically via a temp file + rename.
//
// Atomicity. Since Go 1.5, os.Rename uses MoveFileExW(MOVEFILE_REPLACE_EXISTING)
// on Windows, so a single rename replaces an existing target without the
// previous remove-and-retry dance. POSIX rename(2) is always atomic for
// overwrite. **For an overwrite, either the old contents or the new
// contents are visible at the resolved target at every point — there
// is no window where the target is missing.** First-time writes are
// not "atomic" in this sense (the target obviously starts absent and
// becomes present at rename time); the guarantee applies only when
// path was already populated.
//
// All temp-file failures propagate to the caller. AtomicWrite never
// silently falls back to a non-atomic write — every current caller
// (catalog cache, scan cache, compliance cache, snapshot writer,
// trail log) relies on the atomicity guarantee for shared state with
// concurrent readers, and a quiet fallback would expose partially-
// written payloads exactly in the failure modes where atomicity
// matters.
//
// Permission preservation. When the target already exists, AtomicWrite
// reuses its current mode bits and ignores the supplied perm — matches
// os.WriteFile's overwrite behavior and prevents a rewrite from
// silently broadening a manually-restricted file. The perm argument
// is used only when the target is being created.
//
// Symlink preservation. When path is a symlink (including a *dangling*
// symlink whose target doesn't exist yet, provided the target's
// parent directory exists), the temp file is created next to the
// resolved target and the rename writes to that target. The symlink
// itself is left intact — callers that keep a cache file as a link
// to a shared mount don't lose that setup on overwrite. Symlink
// chains are followed up to maxSymlinkDepth hops; if a cycle is
// detected, resolveLinkTarget returns an error *before* any temp
// file is created and AtomicWrite never attempts the rename.
//
// `clim init` deliberately does NOT use AtomicWrite — see
// internal/teamfile.Write for the rationale (inode/ACL/xattr
// preservation outweighs crash-atomicity for a single-writer,
// interactive command).
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	target, err := resolveLinkTarget(path)
	if err != nil {
		return err
	}

	// Preserve the existing file's mode on overwrite.
	if existingPerm, exists := existingFilePerm(target); exists {
		perm = existingPerm
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

	if err := os.Rename(tmpPath, target); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomic rename %s → %s: %w", tmpPath, target, err)
	}
	return nil
}

// existingFilePerm returns the mode bits of an existing file at path,
// or (0, false) when the path doesn't exist or can't be stat'd.
// Used by AtomicWrite to preserve manually-restricted permissions on
// overwrite. Lstat is intentionally not used — by the time we call
// this, path has already been resolved through any symlinks.
func existingFilePerm(path string) (os.FileMode, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	return info.Mode().Perm(), true
}

// maxSymlinkDepth caps the number of links resolveLinkTarget will
// follow before giving up. POSIX SYMLOOP_MAX is typically 8, Linux
// follows up to 40; we sit comfortably between.
const maxSymlinkDepth = 32

// resolveLinkTarget walks symbolic links starting at path and returns
// the final non-symlink target — even when the target doesn't exist
// (dangling link). Used by AtomicWrite so writing through a symlink
// updates the eventual target file rather than replacing the link.
//
// Only "does not exist" errors from Lstat are treated as the final
// destination (the dangling-link case + the fresh-write case);
// every other Lstat error — permission denied, ENAMETOOLONG, EIO,
// path malformed — is propagated so the caller fails fast on the
// real cause instead of writing to a wrongly-resolved target.
//
// Errors on excessive link depth (cycle detected).
func resolveLinkTarget(path string) (string, error) {
	current := path
	for i := 0; i < maxSymlinkDepth; i++ {
		info, err := os.Lstat(current)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				// Dangling link → current is the final target the
				// caller should write to. The fresh-write case
				// (path doesn't exist yet) lands here too.
				return current, nil
			}
			// Real Lstat failure. Surface it so callers don't
			// silently end up writing to whatever the partial walk
			// left in `current`.
			return "", fmt.Errorf("lstat %s while resolving symlink chain from %s: %w", current, path, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			// Real file or directory — done walking.
			return current, nil
		}
		next, err := os.Readlink(current)
		if err != nil {
			return "", fmt.Errorf("reading symlink %s: %w", current, err)
		}
		// Relative targets resolve against the symlink's parent dir.
		if !filepath.IsAbs(next) {
			next = filepath.Join(filepath.Dir(current), next)
		}
		current = next
	}
	return "", fmt.Errorf("symlink depth exceeded at %s (>%d levels — possible cycle)", path, maxSymlinkDepth)
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
