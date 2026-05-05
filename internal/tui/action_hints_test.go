package tui

import (
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
