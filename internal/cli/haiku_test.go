package cli

import (
	"errors"
	"testing"
)

// TestHaikuCmd_RequiresOneArg verifies Cobra's Args validator rejects
// both empty and multi-arg invocations with a UsageError (exit 2).
// We assert errors.As(*UsageError) so a regression to a plain error
// (which would silently change the CLI exit code from 2 to 1) is
// caught.
func TestHaikuCmd_RequiresOneArg(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"no args", []string{}},
		{"too many args", []string{"a", "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := haikuCmd.Args(haikuCmd, tc.args)
			if err == nil {
				t.Fatalf("haikuCmd.Args(%v) returned nil; want UsageError", tc.args)
			}
			var ue *UsageError
			if !errors.As(err, &ue) {
				t.Errorf("haikuCmd.Args(%v) err = %v (%T); want *UsageError", tc.args, err, err)
			}
		})
	}
}

// TestHaikuCmd_HasOutputFlag confirms --output is wired so JSON/YAML
// callers don't silently fall back to text.
func TestHaikuCmd_HasOutputFlag(t *testing.T) {
	if haikuCmd.Flags().Lookup("output") == nil {
		t.Error("haikuCmd is missing --output flag")
	}
	if haikuCmd.Flags().Lookup("seed") == nil {
		t.Error("haikuCmd is missing --seed flag")
	}
}

// TestHaikuReport_SeedFieldType protects the canonical structured
// shape: tool name (string), resolved seed (int64 + decimal string
// for JS-safe round-trip), three lines.
func TestHaikuReport_SeedFieldType(t *testing.T) {
	r := haikuReport{Tool: "t", Seed: 42, SeedString: "42", Lines: [3]string{"a", "b", "c"}}
	if r.Seed != 42 || r.SeedString != "42" {
		t.Errorf("Seed fields not preserved: %+v", r)
	}
	if len(r.Lines) != 3 {
		t.Errorf("Lines length = %d; want 3", len(r.Lines))
	}
}

// TestHaikuCmd_LongHelp_NoMisleadingNetworkClaim guards against
// re-introducing the standalone "No network. No agent. Pure delight."
// promise (PR-78 review). The current help is allowed to mention
// network in context (e.g. explaining when a cache refresh happens).
func TestHaikuCmd_LongHelp_NoMisleadingNetworkClaim(t *testing.T) {
	if got := haikuCmd.Long; got == "" {
		t.Fatal("haikuCmd.Long is empty")
	} else if containsCaseInsensitive(got, "No network. No agent.") {
		t.Errorf("haikuCmd.Long still contains the misleading 'No network. No agent.' promise:\n%s", got)
	}
}

// Tiny case-insensitive helper kept local — strings.Contains with
// ToLower elsewhere in cli/ would be fine too, but isolating it
// makes the test's intent obvious.
func containsCaseInsensitive(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	ls, lsub := lowerCopy(s), lowerCopy(substr)
	for i := 0; i+len(lsub) <= len(ls); i++ {
		if ls[i:i+len(lsub)] == lsub {
			return true
		}
	}
	return false
}

func lowerCopy(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// (no global errors stub — add an `errors` import only when a test
// actually needs *UsageError or similar wrapping.)
