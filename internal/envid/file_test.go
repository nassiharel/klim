package envid

import (
	"path/filepath"
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
