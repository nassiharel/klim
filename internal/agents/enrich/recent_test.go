package enrich

import (
	"bytes"
	"io"
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

// shortReader emits `data` then returns io.EOF on the next Read.
// io.ReadFull turns the trailing EOF into io.ErrUnexpectedEOF when
// the caller wanted more bytes than `data` contained — exactly the
// state the production ReadTail hits when a file shrinks between
// Stat() and ReadFull().
type shortReader struct {
	data []byte
	pos  int
}

func (r *shortReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// TestReadTailFromReader_ShortReadTrimmedNoNULPadding is the
// regression for the "short read returns NUL-padded buffer" PR
// comment. Pre-fix, when io.ReadFull returned io.ErrUnexpectedEOF
// the function returned the full-length buf (containing trailing
// zeros) — which broke any downstream parser that scans for
// newlines or decodes JSONL line-by-line.
//
// We exercise the short-read path directly via readTailFromReader
// so the test doesn't have to race against the filesystem. The
// production ReadTail wraps this helper, so a regression in the
// trim logic is caught here.
func TestReadTailFromReader_ShortReadTrimmedNoNULPadding(t *testing.T) {
	t.Parallel()
	const payload = "hello\nworld"
	r := &shortReader{data: []byte(payload)}
	// Ask for more bytes than the reader has → io.ReadFull returns
	// io.ErrUnexpectedEOF. The returned slice MUST be sized to the
	// bytes actually read (len(payload)), not the requested length.
	got := readTailFromReader(r, int64(len(payload)+50), false)
	if len(got) != len(payload) {
		t.Fatalf("got len=%d (%q), want len=%d (no NUL padding)",
			len(got), got, len(payload))
	}
	if bytes.IndexByte(got, 0) >= 0 {
		t.Errorf("got contains NUL bytes: %q", got)
	}
	if string(got) != payload {
		t.Errorf("got %q, want %q", got, payload)
	}
}

// TestReadTailFromReader_OtherErrorReturnsNil pins the best-effort
// contract: ReadFull errors other than io.ErrUnexpectedEOF (e.g.
// permission denied mid-stream) return nil so callers can treat the
// whole tail read as failed.
func TestReadTailFromReader_OtherErrorReturnsNil(t *testing.T) {
	t.Parallel()
	// errReader emits some bytes then fails with a non-EOF error;
	// readTailFromReader must return nil so callers don't silently
	// consume corrupted state.
	r := &errReader{data: []byte("partial"), failAt: 3}
	if got := readTailFromReader(r, 100, false); got != nil {
		t.Errorf("non-EOF error: got %d bytes, want nil", len(got))
	}
}

// errReader emits up to `failAt` bytes then returns a non-EOF error.
type errReader struct {
	data   []byte
	pos    int
	failAt int
}

func (r *errReader) Read(p []byte) (int, error) {
	if r.pos >= r.failAt {
		return 0, io.ErrClosedPipe
	}
	n := copy(p, r.data[r.pos:r.failAt])
	r.pos += n
	return n, nil
}
