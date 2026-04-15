package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/nassiharel/clim/internal/registry"
)

// ErrDevBuild is returned when attempting to self-update a development build.
var ErrDevBuild = errors.New("cannot self-update a development build; install a release binary or use 'go install'")

// Result describes what happened during an update attempt.
type Result struct {
	CurrentVersion string
	LatestVersion  string
	Updated        bool // true if a new binary was installed
}

// UpdateAvailable reports whether a newer version exists.
func (r *Result) UpdateAvailable() bool {
	return r.LatestVersion != "" &&
		registry.CompareVersions(r.CurrentVersion, r.LatestVersion) < 0
}

// ReleaseChecker abstracts fetching the latest release information.
type ReleaseChecker interface {
	FetchLatestRelease(ctx context.Context) (*Release, error)
}

// Options allows callers to override defaults (primarily for testing).
type Options struct {
	CheckOnly      bool           // if true, only check — don't download or install
	ExecPath       string         // overrides os.Executable()
	GOOS           string         // overrides runtime.GOOS
	GOARCH         string         // overrides runtime.GOARCH
	ReleaseChecker ReleaseChecker // overrides default GitHubClient
	HTTPClient     *http.Client   // used for asset downloads
}

func (o *Options) goos() string {
	if o != nil && o.GOOS != "" {
		return o.GOOS
	}
	return runtime.GOOS
}

func (o *Options) goarch() string {
	if o != nil && o.GOARCH != "" {
		return o.GOARCH
	}
	return runtime.GOARCH
}

func (o *Options) execPath() (string, error) {
	if o != nil && o.ExecPath != "" {
		return o.ExecPath, nil
	}
	return os.Executable()
}

func (o *Options) releaseChecker() ReleaseChecker {
	if o != nil && o.ReleaseChecker != nil {
		return o.ReleaseChecker
	}
	return &GitHubClient{}
}

func (o *Options) httpClient() *http.Client {
	if o != nil && o.HTTPClient != nil {
		return o.HTTPClient
	}
	return &http.Client{Timeout: 60 * time.Second}
}

// Update checks for a newer version of clim and, if found, downloads and
// installs it by replacing the running binary. It returns a Result describing
// what happened.
func Update(ctx context.Context, currentVersion string, opts *Options) (*Result, error) {
	if opts == nil {
		opts = &Options{}
	}
	result := &Result{CurrentVersion: currentVersion}

	// Guard: dev builds cannot self-update.
	if currentVersion == "dev" || currentVersion == "" {
		return nil, ErrDevBuild
	}

	// 1. Fetch latest release from GitHub.
	gh := opts.releaseChecker()
	rel, err := gh.FetchLatestRelease(ctx)
	if err != nil {
		return nil, fmt.Errorf("checking for updates: %w", err)
	}
	result.LatestVersion = rel.Version()

	// 2. Compare versions.
	if registry.CompareVersions(currentVersion, result.LatestVersion) >= 0 {
		return result, nil // already up to date
	}

	// 3. Check-only mode: report availability without downloading.
	if opts.CheckOnly {
		return result, nil
	}

	// 4. Find the correct asset for this platform.
	goos, goarch := opts.goos(), opts.goarch()
	assetURL, err := AssetURL(rel, goos, goarch)
	if err != nil {
		return nil, err
	}

	// 5. Download the archive.
	archiveData, assetName, err := downloadAsset(ctx, opts.httpClient(), assetURL)
	if err != nil {
		return nil, fmt.Errorf("downloading update: %w", err)
	}

	// 5b. Verify archive checksum against checksums.txt from the release.
	if err := verifyAssetChecksum(ctx, opts.httpClient(), rel, assetName, archiveData); err != nil {
		return nil, fmt.Errorf("checksum verification: %w", err)
	}

	// 6. Extract the binary.
	newBinary, err := ExtractBinary(bytes.NewReader(archiveData), assetName, goos)
	if err != nil {
		return nil, fmt.Errorf("extracting update: %w", err)
	}

	// 7. Replace the running binary.
	execPath, err := opts.execPath()
	if err != nil {
		return nil, fmt.Errorf("locating current binary: %w", err)
	}
	if err := ReplaceBinary(execPath, newBinary); err != nil {
		return nil, fmt.Errorf("installing update: %w", err)
	}

	result.Updated = true
	return result, nil
}

// maxDownloadSize limits the total download size to prevent memory exhaustion
// from a corrupted response or misconfigured server. Release archives are
// typically 10-20 MB; 200 MB is a generous upper bound.
const maxDownloadSize = 200 << 20 // 200 MB

// downloadAsset fetches an archive from the given URL and returns its raw bytes
// along with the filename (derived from the URL path).
// The download is capped at maxDownloadSize to prevent unbounded memory usage.
func downloadAsset(ctx context.Context, client *http.Client, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("creating download request: %w", err)
	}
	req.Header.Set("User-Agent", "clim/selfupdate")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("downloading asset: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download returned %s", resp.Status)
	}

	// Early reject if Content-Length is reported and exceeds the limit.
	if resp.ContentLength > maxDownloadSize {
		return nil, "", fmt.Errorf("download too large: %d bytes (max %d)", resp.ContentLength, maxDownloadSize)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadSize+1))
	if err != nil {
		return nil, "", fmt.Errorf("reading download body: %w", err)
	}
	if int64(len(body)) > maxDownloadSize {
		return nil, "", fmt.Errorf("download too large: exceeded %d bytes", maxDownloadSize)
	}

	name := filepath.Base(url)
	return body, name, nil
}

// verifyAssetChecksum downloads checksums.txt from the release and verifies
// that the archive data matches the expected SHA256 hash. This guards against
// corrupted downloads and basic supply-chain tampering.
func verifyAssetChecksum(ctx context.Context, client *http.Client, rel *Release, assetName string, archiveData []byte) error {
	// Find the checksums.txt asset in the release.
	var checksumURL string
	for _, a := range rel.Assets {
		if a.Name == "checksums.txt" {
			checksumURL = a.BrowserDownloadURL
			break
		}
	}
	if checksumURL == "" {
		// No checksums.txt in this release — skip verification.
		// This allows older releases that predate checksum generation to still work.
		return nil
	}

	checksumData, _, err := downloadAsset(ctx, client, checksumURL)
	if err != nil {
		return fmt.Errorf("downloading checksums.txt: %w", err)
	}

	// Compute actual hash.
	actualHash := sha256.Sum256(archiveData)
	actualHex := hex.EncodeToString(actualHash[:])

	// Find the expected hash for our asset in checksums.txt.
	// Format: "<hex>  <filename>" (two spaces, matching goreleaser output).
	for _, line := range strings.Split(string(checksumData), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[1] == assetName {
			expectedHex := strings.ToLower(parts[0])
			if actualHex != expectedHex {
				return fmt.Errorf("SHA256 mismatch for %s: expected %s, got %s", assetName, expectedHex, actualHex)
			}
			return nil
		}
	}

	return fmt.Errorf("no checksum entry found for %s in checksums.txt", assetName)
}
