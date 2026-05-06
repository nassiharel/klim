package envid

import (
	"errors"
	"fmt"
	"io"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/klim/internal/fileutil"
)

// ReadFile loads a Profile from a YAML file on disk. The file is
// capped at maxDecompressedLen bytes (same limit Decode applies to
// the compressed-token form) so a maliciously-large profile.yaml
// can't be used to exhaust memory. Streamed via io.LimitReader so
// even a multi-GB file allocates at most maxDecompressedLen+1.
func ReadFile(path string) (*Profile, error) {
	f, err := os.Open(path) // #nosec G304 -- caller-supplied path
	if err != nil {
		return nil, fmt.Errorf("envid.ReadFile: %w", err)
	}
	defer func() { _ = f.Close() }()

	data, err := io.ReadAll(io.LimitReader(f, int64(maxDecompressedLen+1)))
	if err != nil {
		return nil, fmt.Errorf("envid.ReadFile: %w", err)
	}
	if len(data) > maxDecompressedLen {
		return nil, fmt.Errorf("%w: file %s exceeds %d bytes", ErrPayloadTooLarge, path, maxDecompressedLen)
	}

	p, err := unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("envid.ReadFile: parse %s: %w", path, err)
	}
	if p.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("%w: file=%d, supported=%d", ErrSchemaMismatch, p.SchemaVersion, SchemaVersion)
	}
	// Canonicalize for the same reason Decode does — the file
	// form is documented as safe to edit by hand, so dedup+sort
	// before downstream consumers see it.
	canonicalize(p)
	return p, nil
}

// WriteFile serializes p to disk via fileutil.AtomicWrite so concurrent
// readers never see a half-written file. The marshalled payload is
// capped at maxDecompressedLen (same limit ReadFile and token Decode
// enforce) so anything we write is guaranteed to round-trip back —
// no silently-creating-a-too-large-to-read artifact.
func WriteFile(path string, p *Profile) error {
	if p == nil {
		return errors.New("envid.WriteFile: nil profile")
	}
	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("envid.WriteFile: marshal: %w", err)
	}
	if len(data) > maxDecompressedLen {
		return fmt.Errorf("%w: marshalled %d bytes (max %d)", ErrPayloadTooLarge, len(data), maxDecompressedLen)
	}
	return fileutil.AtomicWrite(path, data, 0o644)
}
