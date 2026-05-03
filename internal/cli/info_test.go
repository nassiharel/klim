package cli

import "testing"

// Star-count formatting tests live in internal/githubfmt; the CLI uses
// that package directly so the contract is exercised there.

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
