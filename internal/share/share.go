// Package share provides compact token encoding for sharing tool lists
// via chat messages. Tokens carry only tool names — the receiver's
// embedded catalog supplies all package manager IDs.
package share

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	// tokenPrefix is prepended to every share token for recognition and versioning.
	tokenPrefix = "klim:v1:"

	// maxTokenSize limits the decompressed payload to prevent abuse.
	maxTokenSize = 64 << 10 // 64 KB

	// maxEncodedLen caps the base64-encoded portion of the token to prevent
	// a large allocation before decoding even begins. Legitimate tokens are
	// small (gzip-compressed tool names); this is intentionally generous.
	maxEncodedLen = 256 << 10 // 256 KB
)

// Sentinel errors for token encoding/decoding.
var (
	ErrEmptyToolList  = errors.New("no tools to encode")
	ErrInvalidToken   = errors.New("not a valid klim share token")
	ErrMalformedToken = errors.New("malformed share token")
	ErrEmptyToken     = errors.New("empty share token")
	ErrNoToolsInToken = errors.New("share token contains no tools")
)

// Encode converts a list of tool names into a compact, URL-safe share token.
// The format is: klim:v1:<base64url(gzip(comma-separated names))>
func Encode(names []string) (string, error) {
	if len(names) == 0 {
		return "", ErrEmptyToolList
	}

	payload := strings.Join(names, ",")

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte(payload)); err != nil {
		return "", fmt.Errorf("compressing payload: %w", err)
	}
	if err := gz.Close(); err != nil {
		return "", fmt.Errorf("finalizing gzip: %w", err)
	}

	encoded := base64.RawURLEncoding.EncodeToString(buf.Bytes())
	return tokenPrefix + encoded, nil
}

// Decode extracts tool names from a share token.
// Returns an error if the token is malformed, uses an unknown version,
// or contains no tool names.
func Decode(token string) ([]string, error) {
	token = strings.TrimSpace(token)

	if !strings.HasPrefix(token, "klim:") {
		return nil, ErrInvalidToken
	}

	if !strings.HasPrefix(token, tokenPrefix) {
		// Has "klim:" but wrong version.
		ver := strings.SplitN(token, ":", 3)
		if len(ver) >= 2 {
			return nil, fmt.Errorf("unsupported token version %q (this klim supports v1)", ver[1])
		}
		return nil, ErrMalformedToken
	}

	data := strings.TrimPrefix(token, tokenPrefix)
	if data == "" {
		return nil, ErrEmptyToken
	}

	if len(data) > maxEncodedLen {
		return nil, fmt.Errorf("share token too large (%d bytes, max %d)", len(data), maxEncodedLen)
	}

	compressed, err := base64.RawURLEncoding.DecodeString(data)
	if err != nil {
		return nil, fmt.Errorf("invalid token encoding: %w", err)
	}

	gz, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, fmt.Errorf("invalid token data: %w", err)
	}
	defer func() { _ = gz.Close() }()

	// Read one extra byte beyond the limit so we can distinguish a
	// payload that fits from one that exceeds maxTokenSize.
	payload, err := io.ReadAll(io.LimitReader(gz, maxTokenSize+1))
	if err != nil {
		return nil, fmt.Errorf("decompressing token: %w", err)
	}
	if int64(len(payload)) > maxTokenSize {
		return nil, fmt.Errorf("decompressed token payload too large (max %d bytes)", maxTokenSize)
	}

	raw := strings.TrimSpace(string(payload))
	if raw == "" {
		return nil, ErrNoToolsInToken
	}

	names := strings.Split(raw, ",")

	// Filter out empty entries from trailing commas or double commas.
	cleaned := names[:0]
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n != "" {
			cleaned = append(cleaned, n)
		}
	}

	if len(cleaned) == 0 {
		return nil, ErrNoToolsInToken
	}

	return cleaned, nil
}
