package envid

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadWriteFile_RoundTrip(t *testing.T) {
	p := fixtureProfile()
	p.Hash = ComputeHash(p)

	dir := t.TempDir()
	path := filepath.Join(dir, "envid.yaml")
	if err := WriteFile(path, p); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got.Hash != p.Hash {
		t.Errorf("Hash mismatch: got %q want %q", got.Hash, p.Hash)
	}
	if len(got.Tools) != len(p.Tools) {
		t.Errorf("Tools length mismatch: got %d want %d", len(got.Tools), len(p.Tools))
	}
}

func TestReadFile_RejectsOversize(t *testing.T) {
	// File-form should reject inputs larger than the same cap
	// Decode applies, so a malicious profile.yaml can't exhaust
	// memory.
	dir := t.TempDir()
	path := filepath.Join(dir, "big.yaml")
	huge := make([]byte, maxDecompressedLen+10)
	for i := range huge {
		huge[i] = 'x'
	}
	if err := os.WriteFile(path, huge, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := ReadFile(path); err == nil {
		t.Error("ReadFile should refuse oversize input")
	} else if !errors.Is(err, ErrPayloadTooLarge) {
		t.Errorf("err = %v; want ErrPayloadTooLarge", err)
	}
}

func TestWriteFile_RejectsOversize(t *testing.T) {
	// Symmetric with Encode/Decode/ReadFile size caps: never write
	// a file we couldn't read back.
	dir := t.TempDir()
	path := filepath.Join(dir, "big.yaml")
	huge := strings.Repeat("x", maxDecompressedLen)
	p := &Profile{SchemaVersion: SchemaVersion, Tools: []Tool{{Name: huge}}}
	if err := WriteFile(path, p); err == nil {
		t.Error("WriteFile should refuse oversize payload")
	} else if !errors.Is(err, ErrPayloadTooLarge) {
		t.Errorf("err = %v; want ErrPayloadTooLarge", err)
	}
}
