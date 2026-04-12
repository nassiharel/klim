package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// maxBinarySize caps the extracted binary size to prevent memory exhaustion
// from a malicious or corrupted archive.
const maxBinarySize = 200 << 20 // 200 MB

// binaryName returns "clim" or "clim.exe" depending on OS.
func binaryName(goos string) string {
	if goos == "windows" {
		return "clim.exe"
	}
	return "clim"
}

// ExtractBinary reads an archive from r and returns the raw bytes of the clim
// binary inside it. The archive format is determined by archiveName's suffix.
func ExtractBinary(r io.Reader, archiveName string, goos string) ([]byte, error) {
	switch {
	case strings.HasSuffix(archiveName, ".tar.gz"),
		strings.HasSuffix(archiveName, ".tgz"):
		return extractFromTarGz(r, goos)
	case strings.HasSuffix(archiveName, ".zip"):
		return extractFromZip(r, goos)
	default:
		return nil, fmt.Errorf("unsupported archive format: %s", archiveName)
	}
}

func extractFromTarGz(r io.Reader, goos string) ([]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("opening gzip reader: %w", err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	target := binaryName(goos)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading tar entry: %w", err)
		}
		// Match basename — archives may have a top-level directory.
		if filepath.Base(hdr.Name) == target && hdr.Typeflag == tar.TypeReg {
			if hdr.Size > maxBinarySize {
				return nil, fmt.Errorf("binary in archive is too large (%d bytes, max %d)", hdr.Size, maxBinarySize)
			}
			// Read up to maxBinarySize + 1 so we can detect truncation even
			// if the tar header size is untrustworthy.
			data, err := io.ReadAll(io.LimitReader(tr, maxBinarySize+1))
			if err != nil {
				return nil, fmt.Errorf("reading binary from archive: %w", err)
			}
			if int64(len(data)) > maxBinarySize {
				return nil, fmt.Errorf("binary in archive exceeds maximum size (%d bytes)", maxBinarySize)
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("binary %q not found in archive", target)
}

func extractFromZip(r io.Reader, goos string) ([]byte, error) {
	// zip requires io.ReaderAt, so buffer into memory.
	// This is acceptable — release archives are typically <20 MB.
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("buffering zip archive: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		return nil, fmt.Errorf("opening zip reader: %w", err)
	}

	target := binaryName(goos)
	for _, f := range zr.File {
		if filepath.Base(f.Name) == target {
			if f.UncompressedSize64 > maxBinarySize {
				return nil, fmt.Errorf("binary in zip is too large (%d bytes, max %d)", f.UncompressedSize64, maxBinarySize)
			}
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("opening zip entry: %w", err)
			}

			// Read up to maxBinarySize + 1 so we can detect truncation even
			// if the zip header size is untrustworthy.
			data, err := io.ReadAll(io.LimitReader(rc, maxBinarySize+1))
			_ = rc.Close()
			if err != nil {
				return nil, fmt.Errorf("reading binary from zip: %w", err)
			}
			if int64(len(data)) > maxBinarySize {
				return nil, fmt.Errorf("binary in zip exceeds maximum size (%d bytes)", maxBinarySize)
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("binary %q not found in zip archive", target)
}
