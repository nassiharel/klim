package envid

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// fixtureProfile builds a small but realistic Profile for round-trip
// testing. Keep field coverage broad — adding a field to Profile
// without updating this fixture would silently miss test coverage.
func fixtureProfile() *Profile {
	return &Profile{
		SchemaVersion: SchemaVersion,
		Clim:          ClimInfo{Version: "v1.2.3", Commit: "abc1234"},
		GeneratedAt:   time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC),
		OS:            OSInfo{GOOS: "linux", Arch: "amd64", Distro: "Ubuntu 22.04"},
		PackageManagers: map[string]bool{
			"brew": true, "apt": true, "npm": false,
		},
		Tools: []Tool{
			{Name: "fzf", Version: "0.42.0", Source: "brew", Category: "CLI"},
			{Name: "jq", Version: "1.7", Source: "apt", Category: "CLI"},
		},
		Favorites: []string{"fzf", "jq"},
		Packs: []Pack{
			{Name: "my-cli", DisplayName: "My CLI", Tools: []string{"fzf", "jq"}},
		},
		Security: Security{
			AuditWarnings: 3,
			AuditInfos:    1,
			Verdicts:      VerdictsCounts{Clean: 5, Watch: 2, Risk: 0, Unknown: 0},
		},
	}
}

func TestEncodeDecode_RoundTrip(t *testing.T) {
	p := fixtureProfile()
	p.Hash = ComputeHash(p)

	token, err := Encode(p)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !strings.HasPrefix(token, "klim:env:v1:") {
		t.Errorf("token missing prefix: %s", token[:min(40, len(token))])
	}

	got, err := Decode(token)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Clim.Version != p.Clim.Version {
		t.Errorf("Clim.Version = %q, want %q", got.Clim.Version, p.Clim.Version)
	}
	if len(got.Tools) != 2 || got.Tools[0].Name != "fzf" {
		t.Errorf("Tools round-trip mismatch: %+v", got.Tools)
	}
	if !equalStringSlices(got.Favorites, p.Favorites) {
		t.Errorf("Favorites = %v, want %v", got.Favorites, p.Favorites)
	}
	if got.Hash != p.Hash {
		t.Errorf("Hash drift across round-trip: got %q, want %q", got.Hash, p.Hash)
	}
}

func TestComputeHash_StableAcrossTime(t *testing.T) {
	a := fixtureProfile()
	b := fixtureProfile()
	b.GeneratedAt = a.GeneratedAt.Add(48 * time.Hour)
	if ComputeHash(a) != ComputeHash(b) {
		t.Errorf("hash should ignore GeneratedAt; got %q vs %q", ComputeHash(a), ComputeHash(b))
	}
}

func TestComputeHash_ChangesOnContent(t *testing.T) {
	a := fixtureProfile()
	b := fixtureProfile()
	b.Tools = append(b.Tools, Tool{Name: "ripgrep", Source: "brew"})
	if ComputeHash(a) == ComputeHash(b) {
		t.Error("hash should change when Tools differ")
	}
}

func TestDecode_InvalidPrefix(t *testing.T) {
	_, err := Decode("not-a-token")
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("err = %v, want ErrInvalidToken", err)
	}
}

func TestDecode_UnknownVersion(t *testing.T) {
	_, err := Decode("klim:env:v99:abcdef")
	if !errors.Is(err, ErrUnknownVersion) {
		t.Errorf("err = %v, want ErrUnknownVersion", err)
	}
}

func TestDecode_EmptyToken(t *testing.T) {
	_, err := Decode("klim:env:v1:")
	if !errors.Is(err, ErrEmptyToken) {
		t.Errorf("err = %v, want ErrEmptyToken", err)
	}
}

func TestDecode_CorruptBase64(t *testing.T) {
	_, err := Decode("klim:env:v1:!!!notbase64!!!")
	if !errors.Is(err, ErrCorruptToken) {
		t.Errorf("err = %v, want ErrCorruptToken", err)
	}
}

func TestDecode_TamperedGzip(t *testing.T) {
	// Valid base64 of garbage bytes that aren't a gzip stream.
	_, err := Decode("klim:env:v1:AAECAwQFBgc")
	if !errors.Is(err, ErrCorruptToken) {
		t.Errorf("err = %v, want ErrCorruptToken", err)
	}
}

func TestDecode_TooLarge(t *testing.T) {
	huge := "klim:env:v1:" + strings.Repeat("A", maxEncodedLen+1)
	_, err := Decode(huge)
	if !errors.Is(err, ErrTokenTooLarge) {
		t.Errorf("err = %v, want ErrTokenTooLarge", err)
	}
}

func TestDecode_SchemaMismatch(t *testing.T) {
	p := fixtureProfile()
	p.SchemaVersion = 999
	token, err := Encode(p)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	_, err = Decode(token)
	if !errors.Is(err, ErrSchemaMismatch) {
		t.Errorf("err = %v, want ErrSchemaMismatch", err)
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestComputeHash_DoesNotMutateInput(t *testing.T) {
	// Hashing should be a pure function. canonicalize sorts and
	// rewrites slices on its receiver, so without the deepClone
	// guard ComputeHash would re-order the caller's Tools list.
	p := &Profile{
		SchemaVersion:   SchemaVersion,
		PackageManagers: map[string]bool{"brew": true},
		// Intentionally out-of-order to make any in-place sort visible.
		Tools:     []Tool{{Name: "zoxide"}, {Name: "fzf"}, {Name: "bat"}},
		Favorites: []string{"zoxide", "fzf", "bat", "fzf"}, // duplicate too
		Packs: []Pack{
			{Name: "b-pack", Tools: []string{"y", "x"}},
			{Name: "a-pack", Tools: []string{"y", "x"}},
		},
	}
	beforeTools := append([]Tool(nil), p.Tools...)
	beforeFavs := append([]string(nil), p.Favorites...)
	beforePacks := append([]Pack(nil), p.Packs...)
	beforePackTools := make([][]string, len(p.Packs))
	for i, pk := range p.Packs {
		beforePackTools[i] = append([]string(nil), pk.Tools...)
	}

	_ = ComputeHash(p)

	if !equalTools(p.Tools, beforeTools) {
		t.Errorf("ComputeHash mutated Tools: got %+v, want %+v", p.Tools, beforeTools)
	}
	if !equalStringSlices(p.Favorites, beforeFavs) {
		t.Errorf("ComputeHash mutated Favorites: got %v, want %v", p.Favorites, beforeFavs)
	}
	for i := range beforePacks {
		if p.Packs[i].Name != beforePacks[i].Name {
			t.Errorf("ComputeHash mutated Packs order: got %q at %d, want %q", p.Packs[i].Name, i, beforePacks[i].Name)
		}
		if !equalStringSlices(p.Packs[i].Tools, beforePackTools[i]) {
			t.Errorf("ComputeHash mutated Pack[%d].Tools: got %v, want %v", i, p.Packs[i].Tools, beforePackTools[i])
		}
	}
}

func TestComputeHash_DedupesDuplicateTools(t *testing.T) {
	a := &Profile{
		SchemaVersion: SchemaVersion,
		Tools:         []Tool{{Name: "fzf"}},
	}
	b := &Profile{
		SchemaVersion: SchemaVersion,
		Tools:         []Tool{{Name: "fzf"}, {Name: "fzf"}},
	}
	if ComputeHash(a) != ComputeHash(b) {
		t.Errorf("hash should be stable across duplicate Tool entries; %q != %q", ComputeHash(a), ComputeHash(b))
	}
}

func equalTools(a, b []Tool) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestEncode_RejectsOversizePayload(t *testing.T) {
	// Synthesize a profile whose marshalled YAML exceeds the
	// decompressed cap — Encode must refuse rather than emit a
	// token Decode would always reject.
	p := &Profile{SchemaVersion: SchemaVersion}
	huge := strings.Repeat("x", maxDecompressedLen)
	p.Tools = []Tool{{Name: huge}}
	if _, err := Encode(p); err == nil {
		t.Error("Encode should refuse oversize payload")
	} else if !errors.Is(err, ErrPayloadTooLarge) {
		t.Errorf("err = %v; want ErrPayloadTooLarge", err)
	}
}

func TestDecode_CanonicalizesProfile(t *testing.T) {
	// Decode must return a canonical profile so downstream apply
	// flows can't be tricked by a hand-tweaked token containing
	// duplicate or whitespace-only tool names.
	p := &Profile{
		SchemaVersion: SchemaVersion,
		Tools: []Tool{
			{Name: "fzf"},
			{Name: " fzf "}, // whitespace-padded duplicate
			{Name: ""},      // empty
			{Name: "bat"},
		},
		Favorites: []string{"jq", "jq", " "},
		Packs: []Pack{
			{Name: "p", Tools: []string{"a", "a"}},
			{Name: " ", Tools: []string{"x"}}, // empty after trim — drops
		},
	}
	tok, err := Encode(p)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	got, err := Decode(tok)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(got.Tools) != 2 {
		t.Errorf("expected 2 tools after canonicalize, got %d: %+v", len(got.Tools), got.Tools)
	}
	if len(got.Favorites) != 1 || got.Favorites[0] != "jq" {
		t.Errorf("expected favorites=[jq], got %v", got.Favorites)
	}
	if len(got.Packs) != 1 {
		t.Errorf("expected 1 pack after canonicalize, got %d: %+v", len(got.Packs), got.Packs)
	}
	if len(got.Packs[0].Tools) != 1 {
		t.Errorf("expected pack tools deduped to 1, got %v", got.Packs[0].Tools)
	}
}
