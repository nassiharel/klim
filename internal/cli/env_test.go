package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/nassiharel/clim/internal/envid"
)

// TestLoadProfile_TokenMalformedIsUsageError exercises the
// ExitUsage (2) mapping for every user-caused decode failure that
// loadProfile recognises. This is the contract the env-id docs
// promise: malformed tokens are usage errors, not runtime errors.
//
// Inputs that don't start with `clim:env:` are interpreted as file
// paths (handled separately by envid.ReadFile and mapped to
// ExitRuntime when the file doesn't exist), so they're excluded
// from this matrix — their failure mode is genuine I/O, not a
// malformed-token usage error.
func TestLoadProfile_TokenMalformedIsUsageError(t *testing.T) {
	cases := []struct {
		name  string
		token string
	}{
		{"unknown version", "clim:env:v99:abc"},
		{"empty body", "clim:env:v1:"},
		{"corrupt base64", "clim:env:v1:!!!notbase64!!!"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadProfile(tc.token)
			if err == nil {
				t.Fatalf("loadProfile(%q) returned nil; expected usage error", tc.token)
			}
			var ue *UsageError
			if !errors.As(err, &ue) {
				t.Errorf("loadProfile(%q) err = %v (%T), want *UsageError", tc.token, err, err)
			}
		})
	}
}

// TestLoadProfile_ValidTokenRoundTrip verifies the happy path: a
// freshly encoded token decodes back into a profile with matching
// content. This protects the most common use of the command (paste
// from chat → show).
func TestLoadProfile_ValidTokenRoundTrip(t *testing.T) {
	p := &envid.Profile{
		SchemaVersion: envid.SchemaVersion,
		Clim:          envid.ClimInfo{Version: "v0.0.0"},
		Tools:         []envid.Tool{{Name: "jq"}},
	}
	p.Hash = envid.ComputeHash(p)
	token, err := envid.Encode(p)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := loadProfile(token)
	if err != nil {
		t.Fatalf("loadProfile: %v", err)
	}
	if got.Clim.Version != "v0.0.0" || len(got.Tools) != 1 || got.Tools[0].Name != "jq" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

// TestIsUserCausedDecodeError covers the sentinel-list the
// loadProfile error wrapping relies on. If a new envid sentinel is
// added without being registered here, malformed tokens of that
// flavour would silently exit 1 instead of 2.
func TestIsUserCausedDecodeError(t *testing.T) {
	yes := []error{
		envid.ErrInvalidToken,
		envid.ErrEmptyToken,
		envid.ErrUnknownVersion,
		envid.ErrTokenTooLarge,
		envid.ErrPayloadTooLarge,
		envid.ErrSchemaMismatch,
		envid.ErrCorruptToken,
	}
	for _, e := range yes {
		if !isUserCausedDecodeError(e) {
			t.Errorf("isUserCausedDecodeError(%v) = false; expected true", e)
		}
	}
	if isUserCausedDecodeError(errors.New("internal pipe error")) {
		t.Error("plain errors.New should not be treated as user-caused")
	}
}

// TestEnvCmd_NoArgsValidator catches the regression where bare
// 'clim env' silently swallowed positional arguments instead of
// rejecting them. cobra.NoArgs is set in env.go; this test makes
// sure nobody removes it.
func TestEnvCmd_NoArgsValidator(t *testing.T) {
	if envCmd.Args == nil {
		t.Fatal("envCmd.Args is nil; expected cobra.NoArgs")
	}
	if err := envCmd.Args(envCmd, []string{"unexpected"}); err == nil {
		t.Error("envCmd.Args accepted positional args; cobra.NoArgs should reject")
	}
}

// TestRenderProfileDiff_RecomputesHash makes sure the diff helper
// doesn't trust the user-controlled .Hash field on the remote
// profile — a forged token claiming "I match your env" mustn't
// short-circuit the diff to "match".
func TestRenderProfileDiff_RecomputesHash(t *testing.T) {
	local := &envid.Profile{SchemaVersion: envid.SchemaVersion, Tools: []envid.Tool{{Name: "fzf"}}}
	local.Hash = envid.ComputeHash(local)

	// Remote claims the same hash but its real content is different.
	forged := &envid.Profile{
		SchemaVersion: envid.SchemaVersion,
		Tools:         []envid.Tool{{Name: "evil"}},
		Hash:          local.Hash, // user-supplied; we should ignore it
	}

	var buf strings.Builder
	renderProfileDiffTo(&buf, local, forged)
	out := buf.String()
	if strings.Contains(out, "Same hash — environments match.") {
		t.Errorf("forged matching hash incorrectly accepted as match:\n%s", out)
	}
	if !strings.Contains(out, "may have been edited") {
		t.Errorf("expected forged-hash warning, got:\n%s", out)
	}
}
