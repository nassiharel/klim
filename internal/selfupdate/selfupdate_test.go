package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// --- GitHub tests ---

func TestRelease_Version(t *testing.T) {
	tests := []struct {
		tag  string
		want string
	}{
		{"v2.1.92", "2.1.92"},
		{"v1.0.0", "1.0.0"},
		{"2.0.0", "2.0.0"},
		{"v0.1.0-beta", "0.1.0-beta"},
	}
	for _, tt := range tests {
		r := Release{TagName: tt.tag}
		if got := r.Version(); got != tt.want {
			t.Errorf("Release{TagName: %q}.Version() = %q, want %q", tt.tag, got, tt.want)
		}
	}
}

func TestAssetURL(t *testing.T) {
	rel := &Release{
		TagName: "v2.1.92",
		Assets: []Asset{
			{Name: "clim_2.1.92_darwin_arm64.tar.gz", BrowserDownloadURL: "https://example.com/darwin_arm64.tar.gz"},
			{Name: "clim_2.1.92_darwin_amd64.tar.gz", BrowserDownloadURL: "https://example.com/darwin_amd64.tar.gz"},
			{Name: "clim_2.1.92_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/linux_amd64.tar.gz"},
			{Name: "clim_2.1.92_linux_arm64.tar.gz", BrowserDownloadURL: "https://example.com/linux_arm64.tar.gz"},
			{Name: "clim_2.1.92_windows_amd64.zip", BrowserDownloadURL: "https://example.com/windows_amd64.zip"},
			{Name: "checksums.txt", BrowserDownloadURL: "https://example.com/checksums.txt"},
		},
	}

	tests := []struct {
		goos, goarch string
		wantURL      string
		wantErr      bool
	}{
		{"darwin", "arm64", "https://example.com/darwin_arm64.tar.gz", false},
		{"darwin", "amd64", "https://example.com/darwin_amd64.tar.gz", false},
		{"linux", "amd64", "https://example.com/linux_amd64.tar.gz", false},
		{"linux", "arm64", "https://example.com/linux_arm64.tar.gz", false},
		{"windows", "amd64", "https://example.com/windows_amd64.zip", false},
		{"freebsd", "amd64", "", true},
		{"windows", "arm64", "", true},
	}
	for _, tt := range tests {
		url, err := AssetURL(rel, tt.goos, tt.goarch)
		if tt.wantErr {
			if err == nil {
				t.Errorf("AssetURL(%s/%s) expected error, got %q", tt.goos, tt.goarch, url)
			}
			continue
		}
		if err != nil {
			t.Errorf("AssetURL(%s/%s) unexpected error: %v", tt.goos, tt.goarch, err)
			continue
		}
		if url != tt.wantURL {
			t.Errorf("AssetURL(%s/%s) = %q, want %q", tt.goos, tt.goarch, url, tt.wantURL)
		}
	}
}

func TestFetchLatestRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/test-owner/test-repo/releases/latest" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("unexpected Accept header: %s", r.Header.Get("Accept"))
		}
		_ = json.NewEncoder(w).Encode(Release{
			TagName: "v3.0.0",
			Assets:  []Asset{{Name: "clim_3.0.0_linux_amd64.tar.gz"}},
		})
	}))
	defer srv.Close()

	client := &GitHubClient{
		BaseURL: srv.URL,
		Owner:   "test-owner",
		Repo:    "test-repo",
	}

	rel, err := client.FetchLatestRelease(context.Background())
	if err != nil {
		t.Fatalf("FetchLatestRelease() error: %v", err)
	}
	if rel.Version() != "3.0.0" {
		t.Errorf("Version() = %q, want %q", rel.Version(), "3.0.0")
	}
	if len(rel.Assets) != 1 {
		t.Errorf("len(Assets) = %d, want 1", len(rel.Assets))
	}
}

func TestFetchLatestRelease_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	client := &GitHubClient{BaseURL: srv.URL}
	_, err := client.FetchLatestRelease(context.Background())
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention 403, got: %v", err)
	}
}

// --- Archive tests ---

func buildTestTarGz(t *testing.T, binaryName string, binaryContent []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	// Add a directory entry (mimics GoReleaser nesting).
	_ = tw.WriteHeader(&tar.Header{
		Name:     "clim_1.0.0_linux_amd64/",
		Typeflag: tar.TypeDir,
		Mode:     0o755,
	})

	// Add the binary.
	_ = tw.WriteHeader(&tar.Header{
		Name:     "clim_1.0.0_linux_amd64/" + binaryName,
		Size:     int64(len(binaryContent)),
		Mode:     0o755,
		Typeflag: tar.TypeReg,
	})
	_, _ = tw.Write(binaryContent)

	// Add a README (should be ignored).
	readmeContent := []byte("# clim")
	_ = tw.WriteHeader(&tar.Header{
		Name:     "clim_1.0.0_linux_amd64/README.md",
		Size:     int64(len(readmeContent)),
		Mode:     0o644,
		Typeflag: tar.TypeReg,
	})
	_, _ = tw.Write(readmeContent)

	if err := tw.Close(); err != nil {
		t.Fatalf("closing tar writer: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("closing gzip writer: %v", err)
	}
	return buf.Bytes()
}

func buildTestZip(t *testing.T, binaryName string, binaryContent []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	f, err := zw.Create("clim_1.0.0_windows_amd64/" + binaryName)
	if err != nil {
		t.Fatalf("creating zip entry: %v", err)
	}
	if _, err := f.Write(binaryContent); err != nil {
		t.Fatalf("writing zip entry: %v", err)
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("closing zip writer: %v", err)
	}
	return buf.Bytes()
}

func TestExtractBinary_TarGz(t *testing.T) {
	want := []byte("#!/bin/sh\necho hello\n")
	archive := buildTestTarGz(t, "clim", want)

	got, err := ExtractBinary(bytes.NewReader(archive), "clim_1.0.0_linux_amd64.tar.gz", "linux")
	if err != nil {
		t.Fatalf("ExtractBinary() error: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("extracted binary = %q, want %q", got, want)
	}
}

func TestExtractBinary_Zip(t *testing.T) {
	want := []byte("MZ fake exe content")
	archive := buildTestZip(t, "clim.exe", want)

	got, err := ExtractBinary(bytes.NewReader(archive), "clim_1.0.0_windows_amd64.zip", "windows")
	if err != nil {
		t.Fatalf("ExtractBinary() error: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("extracted binary = %q, want %q", got, want)
	}
}

func TestExtractBinary_NotFound(t *testing.T) {
	archive := buildTestTarGz(t, "other-tool", []byte("content"))

	_, err := ExtractBinary(bytes.NewReader(archive), "test.tar.gz", "linux")
	if err == nil {
		t.Fatal("expected error when binary not found")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

func TestExtractBinary_UnsupportedFormat(t *testing.T) {
	_, err := ExtractBinary(bytes.NewReader(nil), "archive.7z", "linux")
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("error should mention 'unsupported', got: %v", err)
	}
}

// --- Replace tests ---

func TestReplaceBinary(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "clim")

	// Write "old" binary.
	if err := os.WriteFile(binPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("writing old binary: %v", err)
	}

	// Replace with "new".
	if err := ReplaceBinary(binPath, []byte("new-binary")); err != nil {
		t.Fatalf("ReplaceBinary() error: %v", err)
	}

	got, err := os.ReadFile(binPath)
	if err != nil {
		t.Fatalf("reading replaced binary: %v", err)
	}
	if string(got) != "new-binary" {
		t.Errorf("binary content = %q, want %q", got, "new-binary")
	}

	// On Unix, .old should be cleaned up.
	if runtime.GOOS != "windows" {
		if _, err := os.Stat(binPath + ".old"); !os.IsNotExist(err) {
			t.Error(".old file should have been cleaned up on Unix")
		}
	}
}

func TestReplaceBinary_Symlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink test not reliable on Windows CI")
	}

	dir := t.TempDir()
	realPath := filepath.Join(dir, "clim-real")
	linkPath := filepath.Join(dir, "clim")

	if err := os.WriteFile(realPath, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realPath, linkPath); err != nil {
		t.Fatal(err)
	}

	if err := ReplaceBinary(linkPath, []byte("new")); err != nil {
		t.Fatalf("ReplaceBinary() through symlink error: %v", err)
	}

	// The real file should have the new content.
	got, _ := os.ReadFile(realPath)
	if string(got) != "new" {
		t.Errorf("real binary content = %q, want %q", got, "new")
	}
}

// --- End-to-end tests ---

func TestUpdate_DevBuildBlocked(t *testing.T) {
	_, err := Update(context.Background(), "dev", nil)
	if !errors.Is(err, ErrDevBuild) {
		t.Fatalf("expected ErrDevBuild, got %v", err)
	}
}

func TestUpdate_EmptyVersionBlocked(t *testing.T) {
	_, err := Update(context.Background(), "", nil)
	if !errors.Is(err, ErrDevBuild) {
		t.Fatalf("expected ErrDevBuild, got %v", err)
	}
}

func TestUpdate_AlreadyUpToDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Release{TagName: "v1.0.0"})
	}))
	defer srv.Close()

	result, err := Update(context.Background(), "1.0.0", &Options{
		ReleaseChecker: &GitHubClient{BaseURL: srv.URL},
	})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if result.Updated {
		t.Error("expected Updated=false for same version")
	}
	if result.LatestVersion != "1.0.0" {
		t.Errorf("LatestVersion = %q, want %q", result.LatestVersion, "1.0.0")
	}
}

func TestUpdate_CurrentNewerThanLatest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Release{TagName: "v1.0.0"})
	}))
	defer srv.Close()

	result, err := Update(context.Background(), "2.0.0", &Options{
		ReleaseChecker: &GitHubClient{BaseURL: srv.URL},
	})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if result.Updated {
		t.Error("expected Updated=false when current > latest")
	}
}

func TestUpdate_EndToEnd(t *testing.T) {
	newBinary := []byte("#!/bin/sh\necho v2.0.0\n")
	archive := buildTestTarGz(t, "clim", newBinary)

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases/latest"):
			_ = json.NewEncoder(w).Encode(Release{
				TagName: "v2.0.0",
				Assets: []Asset{
					{
						Name:               "clim_2.0.0_linux_amd64.tar.gz",
						BrowserDownloadURL: srv.URL + "/download/clim_2.0.0_linux_amd64.tar.gz",
					},
				},
			})
		case strings.HasPrefix(r.URL.Path, "/download/"):
			_, _ = w.Write(archive)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Create a fake "current" binary on disk.
	dir := t.TempDir()
	execPath := filepath.Join(dir, "clim")
	if err := os.WriteFile(execPath, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	result, err := Update(context.Background(), "1.0.0", &Options{
		ExecPath:       execPath,
		GOOS:           "linux",
		GOARCH:         "amd64",
		ReleaseChecker: &GitHubClient{BaseURL: srv.URL},
		HTTPClient:     srv.Client(),
	})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if !result.Updated {
		t.Error("expected Updated=true")
	}
	if result.LatestVersion != "2.0.0" {
		t.Errorf("LatestVersion = %q, want %q", result.LatestVersion, "2.0.0")
	}

	// Verify the binary was replaced.
	got, err := os.ReadFile(execPath)
	if err != nil {
		t.Fatalf("reading updated binary: %v", err)
	}
	if !bytes.Equal(got, newBinary) {
		t.Errorf("binary content = %q, want %q", got, newBinary)
	}
}

func TestUpdate_CheckOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Release{TagName: "v2.0.0"})
	}))
	defer srv.Close()

	result, err := Update(context.Background(), "1.0.0", &Options{
		CheckOnly:      true,
		ReleaseChecker: &GitHubClient{BaseURL: srv.URL},
	})
	if err != nil {
		t.Fatalf("Update() error: %v", err)
	}
	if result.Updated {
		t.Error("expected Updated=false in check-only mode")
	}
	if !result.UpdateAvailable() {
		t.Error("expected UpdateAvailable()=true when latest > current")
	}
	if result.LatestVersion != "2.0.0" {
		t.Errorf("LatestVersion = %q, want %q", result.LatestVersion, "2.0.0")
	}
}

func TestResult_UpdateAvailable(t *testing.T) {
	tests := []struct {
		name   string
		result Result
		want   bool
	}{
		{"newer available", Result{CurrentVersion: "1.0.0", LatestVersion: "2.0.0"}, true},
		{"same version", Result{CurrentVersion: "1.0.0", LatestVersion: "1.0.0"}, false},
		{"current is newer", Result{CurrentVersion: "2.0.0", LatestVersion: "1.0.0"}, false},
		{"empty latest", Result{CurrentVersion: "1.0.0", LatestVersion: ""}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.result.UpdateAvailable(); got != tt.want {
				t.Errorf("UpdateAvailable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReplaceBinary_CleansUpStaleFiles(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "clim")

	// Write "current" binary.
	if err := os.WriteFile(binPath, []byte("current"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Simulate stale files from a previous update.
	if err := os.WriteFile(binPath+".old", []byte("stale-old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binPath+".new", []byte("stale-new"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Replace should succeed despite stale files.
	if err := ReplaceBinary(binPath, []byte("updated")); err != nil {
		t.Fatalf("ReplaceBinary() error: %v", err)
	}

	got, _ := os.ReadFile(binPath)
	if string(got) != "updated" {
		t.Errorf("binary content = %q, want %q", got, "updated")
	}
}
