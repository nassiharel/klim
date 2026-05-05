package tui

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestActionFailureHint_WingetNotInstalled(t *testing.T) {
	got := actionFailureHint([]string{"winget", "uninstall", "--id", "jqlang.jq"}, wingetExitNotInstalled)
	if !strings.Contains(got, "isn't installed") {
		t.Errorf("expected 'isn't installed' guidance, got %q", got)
	}
	if !strings.Contains(got, "winget list") {
		t.Errorf("expected 'winget list' suggestion, got %q", got)
	}
}

func TestActionFailureHint_WingetNoPackage(t *testing.T) {
	got := actionFailureHint([]string{"winget", "install", "--id", "Bogus.Tool"}, wingetExitNoPackageAvailable)
	if !strings.Contains(got, "no package matching") {
		t.Errorf("expected 'no package matching' guidance, got %q", got)
	}
}

func TestActionFailureHint_NotWinget(t *testing.T) {
	// Non-winget commands should not trigger winget-specific hints.
	if got := actionFailureHint([]string{"brew", "uninstall", "jq"}, 1); got != "" {
		t.Errorf("brew failure should produce no hint, got %q", got)
	}
}

func TestActionFailureHint_UnknownExitCode(t *testing.T) {
	if got := actionFailureHint([]string{"winget", "uninstall", "--id", "x"}, 1); got != "" {
		t.Errorf("unknown exit code should produce no hint, got %q", got)
	}
}

func TestActionFailureHint_EmptyArgs(t *testing.T) {
	if got := actionFailureHint(nil, wingetExitNotInstalled); got != "" {
		t.Errorf("empty args should produce no hint, got %q", got)
	}
}

func TestErrMsgWithHint_NoErr(t *testing.T) {
	if got := errMsgWithHint([]string{"winget"}, nil); got != "" {
		t.Errorf("nil err should produce empty string, got %q", got)
	}
}

func TestErrMsgWithHint_NonExitError(t *testing.T) {
	// A plain error (not *exec.ExitError) gets its message but no hint.
	got := errMsgWithHint([]string{"winget"}, errors.New("boom"))
	if got != "boom" {
		t.Errorf("got %q, want plain message", got)
	}
}

func TestHintFromError_NotExecExitError(t *testing.T) {
	if got := hintFromError([]string{"winget"}, errors.New("boom")); got != "" {
		t.Errorf("non-exit error should produce no hint, got %q", got)
	}
}

func TestHintFromError_RealExecExitError(t *testing.T) {
	// Sanity-check the *exec.ExitError unwrap path actually works
	// through real Go plumbing. Pack/backup TUI flows pass us the
	// error directly off tea.ExecProcess; if errors.As stopped
	// recognising *exec.ExitError, the friendly winget hint would
	// silently disappear and pure-unit tests using errors.New
	// wouldn't catch it.
	//
	// Reproducing winget's 0x8A150014 from a shell isn't possible
	// (POSIX caps exit codes at 0-255), so we temporarily stub
	// wingetExitNotInstalled to a small code (42), spawn a helper
	// that exits with that, and assert hintFromError returns the
	// real NO_APPLICATIONS_FOUND hint string. That asserts both
	// the unwrap AND the code-to-hint mapping.
	if runtime.GOOS == "windows" {
		t.Skip("helper-process pattern needs a POSIX shell; the unwrap logic is platform-neutral")
	}
	defer func(orig int) { wingetExitNotInstalled = orig }(wingetExitNotInstalled)
	wingetExitNotInstalled = 42

	cmd := exec.Command(os.Args[0], "-test.run=TestHintHelperProcess")
	cmd.Env = append(os.Environ(), "GO_HELPER_PROCESS=1")
	err := cmd.Run()
	if err == nil {
		t.Fatal("helper process should have exited non-zero")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError, got %T (%v)", err, err)
	}

	// With wingetExitNotInstalled stubbed to 42, the unwrap path
	// should produce the real NO_APPLICATIONS_FOUND hint.
	got := hintFromError([]string{"winget", "uninstall"}, err)
	if !strings.Contains(got, "isn't installed") {
		t.Errorf("expected NO_APPLICATIONS_FOUND hint, got %q", got)
	}

	// And the same error against a non-winget command must NOT
	// trigger the hint — proves args[0] gating still applies.
	if hint := hintFromError([]string{"brew", "uninstall"}, err); hint != "" {
		t.Errorf("non-winget cmd should produce no hint even with stubbed code, got %q", hint)
	}
}

// TestHintHelperProcess is the child entry-point for
// TestHintFromError_RealExecExitError. The test driver re-execs the
// test binary with -test.run=TestHintHelperProcess and a sentinel
// env var; we then exit with a known non-zero code. Standard Go
// helper-process pattern — the surrounding test verifies the
// *exec.ExitError unwrap rather than depending on a specific code.
func TestHintHelperProcess(t *testing.T) {
	if os.Getenv("GO_HELPER_PROCESS") != "1" {
		return
	}
	os.Exit(42)
}
