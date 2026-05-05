package envid

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/fileutil"
)

// ReadFile loads a Profile from a YAML file on disk.
func ReadFile(path string) (*Profile, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- caller-supplied path
	if err != nil {
		return nil, fmt.Errorf("envid.ReadFile: %w", err)
	}
	p, err := unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("envid.ReadFile: parse %s: %w", path, err)
	}
	if p.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("%w: file=%d, supported=%d", ErrSchemaMismatch, p.SchemaVersion, SchemaVersion)
	}
	return p, nil
}

// WriteFile serializes p to disk via fileutil.AtomicWrite so concurrent
// readers never see a half-written file.
func WriteFile(path string, p *Profile) error {
	if p == nil {
		return errors.New("envid.WriteFile: nil profile")
	}
	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("envid.WriteFile: marshal: %w", err)
	}
	return fileutil.AtomicWrite(path, data, 0o644)
}
