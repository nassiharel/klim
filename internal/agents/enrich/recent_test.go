package enrich

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadTail(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Build a fixture with 200 numbered lines so we can verify the
	// tail-trim drops the first (partial) line.
	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteString("line ")
		// Pad to a fixed width so the file is well over 64 KiB worth
		// of bytes only if we generate enough — instead force a small
		// maxBytes in the test.
		b.WriteByte(byte('0' + i%10))
		b.WriteByte('\n')
	}
	path := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	t.Run("missing file returns nil", func(t *testing.T) {
		t.Parallel()
		if got := ReadTail(filepath.Join(dir, "nope.jsonl"), 1024, true); got != nil {
			t.Errorf("missing file: got %d bytes, want nil", len(got))
		}
	})

	t.Run("whole-file read when smaller than cap", func(t *testing.T) {
		t.Parallel()
		got := ReadTail(path, 1<<20, false)
		if len(got) != len(b.String()) {
			t.Errorf("whole-file read: got %d bytes, want %d", len(got), len(b.String()))
		}
	})

	t.Run("tail trim drops partial first line", func(t *testing.T) {
		t.Parallel()
		got := ReadTail(path, 50, true)
		if len(got) == 0 {
			t.Fatalf("tail trim produced no bytes")
		}
		// Should start at the beginning of a complete line.
		if !strings.HasPrefix(string(got), "line ") {
			t.Errorf("tail trim should start at a line boundary, got %q", got)
		}
	})
}
