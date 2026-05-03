package cli

import (
	"errors"
	"strings"
	"testing"
)

// Star-count formatting tests live in internal/githubfmt; the CLI uses
// that package directly so the contract is exercised there.

func TestNotFoundError_IsUsageError(t *testing.T) {
	// A typo on the tool name is malformed user input, so it must
	// surface as a UsageError so Run() maps it to exit code 2.
	// Otherwise scripts can't tell `clim info kubctl` (typo) apart
	// from a genuine runtime failure (exit 1).
	for _, suggestion := range []string{"", "kubectl"} {
		err := notFoundError("kubctl", suggestion)
		var ue *UsageError
		if !errors.As(err, &ue) {
			t.Errorf("suggestion=%q: expected *UsageError, got %T (%v)", suggestion, err, err)
			continue
		}
		if !strings.Contains(err.Error(), "kubctl") {
			t.Errorf("error should reference offending name; got %q", err.Error())
		}
		if suggestion != "" && !strings.Contains(err.Error(), suggestion) {
			t.Errorf("error should include suggestion %q; got %q", suggestion, err.Error())
		}
	}
}

func TestFormatInfoRef_PreservesConstraint(t *testing.T) {
	// Optional teamfile pin must show its version constraint.
	got := formatInfoRef(infoReference{
		Kind: "teamfile", Path: "/home/me/.clim.yaml",
		Required: false, Constraint: ">=1.28",
	})
	want := ".clim.yaml (optional >=1.28) — /home/me/.clim.yaml"
	if got != want {
		t.Errorf("optional teamfile with constraint:\n  got:  %s\n  want: %s", got, want)
	}

	// Required teamfile pin: same constraint format.
	got = formatInfoRef(infoReference{
		Kind: "teamfile", Path: "/home/me/.clim.yaml",
		Required: true, Constraint: ">=1.28",
	})
	want = ".clim.yaml (required >=1.28) — /home/me/.clim.yaml"
	if got != want {
		t.Errorf("required teamfile with constraint:\n  got:  %s\n  want: %s", got, want)
	}

	// Project optional with constraint — both role and constraint must appear.
	got = formatInfoRef(infoReference{
		Kind: "project", Name: "myapp", Path: "/projects/myapp/.clim.yaml",
		Required: false, Constraint: "~1.5",
	})
	want = `Project "myapp" (optional ~1.5) — /projects/myapp/.clim.yaml`
	if got != want {
		t.Errorf("project optional with constraint:\n  got:  %s\n  want: %s", got, want)
	}

	// Empty constraint: role appears alone, no trailing space.
	got = formatInfoRef(infoReference{
		Kind: "teamfile", Path: "/home/me/.clim.yaml", Required: true,
	})
	want = ".clim.yaml (required) — /home/me/.clim.yaml"
	if got != want {
		t.Errorf("teamfile required no constraint:\n  got:  %s\n  want: %s", got, want)
	}
}
