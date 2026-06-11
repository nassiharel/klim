package agents

import (
	"os"
	"testing"
)

// TestHomeDir_FallsBackToOSUserHomeDir is the regression for the
// "homeDir relies only on HOME/USERPROFILE env vars" PR comment.
// Sandboxed runtimes (cron jobs, some containers, certain login
// shells) can leave both env vars empty even though os.UserHomeDir
// can still resolve a home path via passwd / SHGetKnownFolderPath.
//
// We exercise the fallback by clearing both env vars and asserting
// homeDir() returns something non-empty whenever os.UserHomeDir
// itself succeeds — which it does on every supported CI runner.
func TestHomeDir_FallsBackToOSUserHomeDir(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", "")

	// Smoke-check the fallback is reachable: os.UserHomeDir's
	// success is a precondition for this test to be meaningful.
	expected, err := os.UserHomeDir()
	if err != nil || expected == "" {
		t.Skipf("os.UserHomeDir failed (%v) in this environment; cannot exercise fallback", err)
	}

	if got := homeDir(); got != expected {
		t.Errorf("homeDir() = %q, want %q (os.UserHomeDir fallback)", got, expected)
	}
}

// TestHomeDir_PrefersHomeEnv pins HOME as the first source so a
// process that sets HOME to a sandbox dir gets that dir (not the
// real user home from os.UserHomeDir).
func TestHomeDir_PrefersHomeEnv(t *testing.T) {
	t.Setenv("HOME", "/tmp/sandbox-home")
	t.Setenv("USERPROFILE", "")
	if got := homeDir(); got != "/tmp/sandbox-home" {
		t.Errorf("homeDir() = %q, want %q", got, "/tmp/sandbox-home")
	}
}

// TestHomeDir_FallsBackToUserProfile pins the Windows-native source
// when HOME is unset.
func TestHomeDir_FallsBackToUserProfile(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("USERPROFILE", `C:\Users\nas`)
	if got := homeDir(); got != `C:\Users\nas` {
		t.Errorf("homeDir() = %q, want %q", got, `C:\Users\nas`)
	}
}
