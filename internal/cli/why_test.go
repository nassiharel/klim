package cli

import "testing"

// TestFormatWhyRef_PreservesConstraint guards against the regression
// where `klim why` showed version constraints only for required
// teamfile pins, dropping them for optional teamfiles and all project
// refs. CollectReferences preserves the constraint for every kind, so
// the renderer must too.
func TestFormatWhyRef_PreservesConstraint(t *testing.T) {
	cases := []struct {
		ref  whyReference
		want string
	}{
		{
			Reference{Kind: "teamfile", Path: "/p/.klim.yaml", Required: true, Constraint: ">=1.28"},
			".klim.yaml (required >=1.28) — /p/.klim.yaml",
		},
		{
			Reference{Kind: "teamfile", Path: "/p/.klim.yaml", Required: false, Constraint: "~3.12"},
			".klim.yaml (optional ~3.12) — /p/.klim.yaml",
		},
		{
			Reference{Kind: "project", Name: "myapp", Path: "/p/.klim.yaml", Required: false, Constraint: "~1.5"},
			`Project "myapp" (optional ~1.5) — /p/.klim.yaml`,
		},
		{
			Reference{Kind: "project", Name: "myapp", Path: "/p/.klim.yaml", Required: true, Constraint: ">=2.0"},
			`Project "myapp" (required >=2.0) — /p/.klim.yaml`,
		},
		{
			Reference{Kind: "teamfile", Path: "/p/.klim.yaml", Required: true},
			".klim.yaml (required) — /p/.klim.yaml",
		},
	}
	for _, c := range cases {
		got := FormatReference(c.ref)
		if got != c.want {
			t.Errorf("ref=%+v\n  got:  %s\n  want: %s", c.ref, got, c.want)
		}
	}
}
