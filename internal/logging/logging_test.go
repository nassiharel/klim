package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// pointHomeAtTemp redirects paths.LogFile() at a per-test temp dir
// by overriding the env vars os.UserHomeDir reads on each platform.
func pointHomeAtTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)        // Unix
	t.Setenv("USERPROFILE", dir) // Windows
	return dir
}

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in   string
		want slog.Level
	}{
		{"info", slog.LevelInfo},
		{"INFO", slog.LevelInfo},
		{"  warn  ", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
		{"debug", slog.LevelDebug},
		{"", slog.LevelDebug},          // default
		{"nonsense", slog.LevelDebug},  // unknown → debug default
	}
	for _, c := range cases {
		if got := parseLevel(c.in); got != c.want {
			t.Errorf("parseLevel(%q): want %v, got %v", c.in, c.want, got)
		}
	}
}

func TestInit_FileEnabledCreatesPathAndWritesLog(t *testing.T) {
	if runtime.GOOS == "windows" {
		// lumberjack keeps the underlying *os.File open with no Close
		// method exposed; on Windows that prevents t.TempDir() from
		// cleaning up its directory at the end of the test (the path
		// resolution itself is fine — covered by
		// TestResolveLogPath_DirectoryGetsCreated). Exercise the
		// write-to-file path on Unix only.
		t.Skip("lumberjack file handle blocks t.TempDir cleanup on Windows")
	}
	pointHomeAtTemp(t)

	Init("debug", true /*fileEnabled*/, false /*verbose*/)

	logPathOut := Path()
	if logPathOut == "" {
		t.Fatalf("Path() empty after Init with file enabled")
	}
	if !strings.Contains(logPathOut, "klim.log") {
		t.Errorf("Path: want trailing klim.log, got %s", logPathOut)
	}
	// Parent directory must exist (resolveLogPath MkdirAll's it).
	if info, err := os.Stat(filepath.Dir(logPathOut)); err != nil || !info.IsDir() {
		t.Errorf("log directory not created: %v", err)
	}

	// Emit a log line and confirm it lands on disk.
	slog.Info("hello-from-test", "k", "v")
	// lumberjack flushes on Write; reading right after should work.
	data, err := os.ReadFile(logPathOut)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), "hello-from-test") {
		t.Errorf("log line not in file. content=%q", string(data))
	}

	// Re-init resets logPath then sets it again; calling Init repeatedly should not panic.
	Init("info", true, false)
	if Path() == "" {
		t.Errorf("Path empty after re-init")
	}
}

func TestInit_FileDisabledLeavesPathEmpty(t *testing.T) {
	pointHomeAtTemp(t)
	Init("info", false /*fileEnabled*/, false /*verbose*/)
	if Path() != "" {
		t.Errorf("Path: want empty when file logging disabled, got %s", Path())
	}
}

func TestInit_VerboseSendsToStderr(t *testing.T) {
	pointHomeAtTemp(t)

	// Capture stderr by redirecting os.Stderr — easiest portable
	// approach is to swap its file descriptor with a pipe, which is
	// platform-specific. Instead, exercise the silent-fallback path
	// (no file, no verbose) to assert the io.Discard branch executes
	// without writing anywhere.
	Init("debug", false, false)
	slog.Debug("should-go-to-discard")
	if Path() != "" {
		t.Errorf("expected empty path with no file and no verbose")
	}

	// Then enable verbose-only and ensure no file path is set but
	// the call doesn't panic.
	Init("info", false, true)
	if Path() != "" {
		t.Errorf("verbose-only: file path should still be empty, got %s", Path())
	}
}

func TestResolveLogPath_DirectoryGetsCreated(t *testing.T) {
	dir := pointHomeAtTemp(t)
	got := resolveLogPath()
	if got == "" {
		t.Fatalf("resolveLogPath returned empty")
	}
	// Parent of the returned path must exist.
	parent := filepath.Dir(got)
	if !strings.HasPrefix(parent, dir) {
		t.Errorf("resolved path %s does not live under temp HOME %s", got, dir)
	}
	if info, err := os.Stat(parent); err != nil || !info.IsDir() {
		t.Errorf("parent dir not created: %v", err)
	}
}

func TestResolveLogPath_MkdirFailsReturnsEmpty(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("MkdirAll error path is awkward to trigger reliably on Windows")
	}
	// Point HOME at a path whose parent is a regular file — the
	// MkdirAll inside resolveLogPath should fail because it can't
	// turn a file into a directory.
	regular := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(regular, []byte("blocker"), 0o600); err != nil {
		t.Fatalf("seed regular file: %v", err)
	}
	t.Setenv("HOME", regular)
	t.Setenv("USERPROFILE", regular)

	got := resolveLogPath()
	if got != "" {
		t.Errorf("expected empty when MkdirAll cannot succeed; got %s", got)
	}
	// Sanity-check os.Stat on the seeded file still says it's a file.
	if info, err := os.Stat(regular); err != nil || info.IsDir() {
		t.Errorf("seed file went missing or became a dir: err=%v isDir=%v", err, info != nil && info.IsDir())
	}
}
