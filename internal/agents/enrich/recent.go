package enrich

import (
	"errors"
	"io"
	"os"
)

// TailBufferBytes is the default cap used by [ReadTail] when reading
// the trailing portion of a (potentially huge) JSONL file. 64 KiB is
// enough to comfortably contain the last several hundred events on
// real-world sessions; anything older won't change the derived live
// state anyway since the staleness threshold is 60s.
const TailBufferBytes int64 = 64 * 1024

// ReadTail opens the file at `path`, seeks to at most `maxBytes` from
// the end, and returns the bytes read. When the file is shorter than
// maxBytes the whole file is returned. The function is best-effort:
// any open/seek/read error returns (nil, nil) so callers can treat a
// missing or unreadable transcript as "no enrichment available" rather
// than failing the surrounding scan.
//
// The returned bytes may start mid-line. Callers that want exact
// JSON-decoded events should pass `trimToLine: true` to drop the
// (likely truncated) first line — this matters for newline-delimited
// formats like JSONL.
func ReadTail(path string, maxBytes int64, trimToLine bool) []byte {
	if maxBytes <= 0 {
		maxBytes = TailBufferBytes
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil
	}
	size := info.Size()

	startAt := int64(0)
	if size > maxBytes {
		startAt = size - maxBytes
	}
	if startAt > 0 {
		if _, err := f.Seek(startAt, io.SeekStart); err != nil {
			return nil
		}
	}

	return readTailFromReader(f, size-startAt, startAt > 0 && trimToLine)
}

// readTailFromReader reads up to `want` bytes from r and applies the
// trim-to-line logic. Separated from ReadTail so the short-read
// (io.ErrUnexpectedEOF) path can be exercised by tests without
// racing against the filesystem.
//
// On short read — which happens when the file shrinks between
// Stat() and the ReadFull (log rotation, a concurrent writer
// truncating) — the returned slice is trimmed to the bytes actually
// read so callers never see trailing NUL bytes that would otherwise
// break downstream JSONL parsers.
//
// Any other read error returns nil (stay best-effort).
func readTailFromReader(r io.Reader, want int64, trimToLine bool) []byte {
	if want <= 0 {
		return nil
	}
	buf := make([]byte, want)
	n, err := io.ReadFull(r, buf)
	if err != nil {
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			return nil
		}
		buf = buf[:n]
	}
	if trimToLine {
		// Drop everything up to (and including) the first newline so
		// the consumer's first decode doesn't see a half-line.
		for i := 0; i < len(buf); i++ {
			if buf[i] == '\n' {
				return buf[i+1:]
			}
		}
		return nil
	}
	return buf
}
