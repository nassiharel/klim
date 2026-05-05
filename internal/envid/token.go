package envid

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// tokenPrefix is prepended to every Env ID token for recognition
	// and versioning. Mirrors internal/share's `clim:v1:` pattern.
	tokenPrefix = "clim:env:v1:" //nolint:gosec // not a credential, just a token format prefix

	// maxEncodedLen caps the base64 portion before any decoding work.
	// 256 KB is generous — typical 30-tool envs are under 1 KB.
	maxEncodedLen = 256 << 10

	// maxDecompressedLen caps the YAML payload after gunzip to
	// prevent zip-bomb-style abuse.
	maxDecompressedLen = 64 << 10
)

// Sentinel errors for token I/O.
var (
	ErrInvalidToken    = errors.New("not a valid clim env token")
	ErrTokenTooLarge   = errors.New("env token too large")
	ErrPayloadTooLarge = errors.New("env payload too large after decompression")
	ErrUnknownVersion  = errors.New("unknown env schema version")
	ErrEmptyToken      = errors.New("empty env token")
	ErrCorruptToken    = errors.New("env token is corrupt")
	ErrSchemaMismatch  = errors.New("env schema version does not match this clim build")
)

// Encode serializes p into a compact base64 token.
func Encode(p *Profile) (string, error) {
	if p == nil {
		return "", errors.New("envid.Encode: nil profile")
	}
	yml, err := yaml.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("envid.Encode: marshal: %w", err)
	}

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(yml); err != nil {
		return "", fmt.Errorf("envid.Encode: gzip write: %w", err)
	}
	if err := gz.Close(); err != nil {
		return "", fmt.Errorf("envid.Encode: gzip close: %w", err)
	}
	return tokenPrefix + base64.RawURLEncoding.EncodeToString(buf.Bytes()), nil
}

// Decode extracts a Profile from a compact token. Returns a typed
// error for each of: missing prefix, wrong version, oversize payload,
// gzip / yaml corruption, or schema-version mismatch.
func Decode(token string) (*Profile, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrEmptyToken
	}
	if !strings.HasPrefix(token, "clim:env:") {
		return nil, ErrInvalidToken
	}
	if !strings.HasPrefix(token, tokenPrefix) {
		parts := strings.SplitN(token, ":", 4)
		if len(parts) >= 3 {
			return nil, fmt.Errorf("%w: token version %q (this clim supports v1)", ErrUnknownVersion, parts[2])
		}
		return nil, ErrInvalidToken
	}
	encoded := strings.TrimPrefix(token, tokenPrefix)
	if encoded == "" {
		return nil, ErrEmptyToken
	}
	if len(encoded) > maxEncodedLen {
		return nil, fmt.Errorf("%w: %d bytes (max %d)", ErrTokenTooLarge, len(encoded), maxEncodedLen)
	}

	gzData, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: base64 decode: %w", ErrCorruptToken, err)
	}

	gz, err := gzip.NewReader(bytes.NewReader(gzData))
	if err != nil {
		return nil, fmt.Errorf("%w: gzip header: %w", ErrCorruptToken, err)
	}
	defer func() { _ = gz.Close() }()

	yml, err := io.ReadAll(io.LimitReader(gz, int64(maxDecompressedLen+1)))
	if err != nil {
		return nil, fmt.Errorf("%w: gunzip: %w", ErrCorruptToken, err)
	}
	if len(yml) > maxDecompressedLen {
		return nil, fmt.Errorf("%w: %d bytes (max %d)", ErrPayloadTooLarge, len(yml), maxDecompressedLen)
	}

	p, err := unmarshal(yml)
	if err != nil {
		return nil, fmt.Errorf("%w: unmarshal: %w", ErrCorruptToken, err)
	}
	if p.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("%w: token=%d, supported=%d", ErrSchemaMismatch, p.SchemaVersion, SchemaVersion)
	}
	return p, nil
}

func unmarshal(data []byte) (*Profile, error) {
	var p Profile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(false) // additive forward-compat: tolerate unknown fields
	if err := dec.Decode(&p); err != nil {
		return nil, err
	}
	return &p, nil
}
