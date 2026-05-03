package cli

import "testing"

func TestFormatStarCount(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		// Under 1k unchanged.
		{0, "0"},
		{42, "42"},
		{999, "999"},
		// 1k–99.9k uses one decimal.
		{1000, "1.0k"},
		{1234, "1.2k"},
		{12345, "12.3k"},
		// 100k–999k uses integer k.
		{100000, "100k"},
		{500000, "500k"},
		{999999, "999k"},
		// 1M–9.9M uses one decimal.
		{1000000, "1.0M"},
		{1500000, "1.5M"},
		{9999999, "10.0M"},
		// 10M+ uses integer M (matches TUI's formatStars contract).
		{10000000, "10M"},
		{109000000, "109M"},
	}
	for _, c := range cases {
		got := formatStarCount(c.in)
		if got != c.want {
			t.Errorf("formatStarCount(%d) = %q, want %q", c.in, got, c.want)
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
