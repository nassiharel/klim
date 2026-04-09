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
	tokenPrefix = "clim:v1:"

	// maxTokenSize limits the decoded payload to prevent abuse.
	maxTokenSize = 64 << 10 // 64 KB
)

// Encode converts a list of tool names into a compact, URL-safe share token.
// The format is: clim:v1:<base64url(gzip(comma-separated names))>
func Encode(names []string) (string, error) {
	if len(names) == 0 {
		return "", errors.New("no tools to encode")
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

	if !strings.HasPrefix(token, "clim:") {
		return nil, errors.New("not a valid clim share token")
	}

	if !strings.HasPrefix(token, tokenPrefix) {
		// Has "clim:" but wrong version.
		ver := strings.SplitN(token, ":", 3)
		if len(ver) >= 2 {
			return nil, fmt.Errorf("unsupported token version %q (this clim supports v1)", ver[1])
		}
		return nil, errors.New("malformed share token")
	}

	data := strings.TrimPrefix(token, tokenPrefix)
	if data == "" {
		return nil, errors.New("empty share token")
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

	payload, err := io.ReadAll(io.LimitReader(gz, maxTokenSize))
	if err != nil {
		return nil, fmt.Errorf("decompressing token: %w", err)
	}

	raw := strings.TrimSpace(string(payload))
	if raw == "" {
		return nil, errors.New("share token contains no tools")
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
		return nil, errors.New("share token contains no tools")
	}

	return cleaned, nil
}
