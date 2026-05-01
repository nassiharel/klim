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
// On Windows, os.Rename fails if the destination exists, so we remove
// the destination and retry once.
func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
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

	if err := os.Rename(tmpPath, path); err != nil {
		// Windows: Rename fails if destination exists. Remove and retry.
		removeErr := os.Remove(path)
		if removeErr != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("rename %s → %s failed, remove dest also failed: %w", tmpPath, path, removeErr)
		}
		if retryErr := os.Rename(tmpPath, path); retryErr != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("rename retry %s → %s: %w", tmpPath, path, retryErr)
		}
	}
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
