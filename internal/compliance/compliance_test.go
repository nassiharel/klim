package compliance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteSamplePolicy_CreatesValidYAML verifies that a freshly written
// policy is parseable through the same LoadPolicy path the runtime uses.
// Without this, drift in the template (a stray tab, accidental smart
// quote, etc.) would only surface when a user tries to load it.
func TestWriteSamplePolicy_CreatesValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.yaml")

	if err := WriteSamplePolicy(path); err != nil {
		t.Fatalf("WriteSamplePolicy: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat after write: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("policy file is empty")
	}

	policy, err := LoadPolicy(path)
	if err != nil {
		t.Fatalf("LoadPolicy on freshly written sample: %v", err)
	}
	if policy.Name == "" {
		t.Errorf("expected non-empty policy name in sample, got empty")
	}
	// Spot-check sentinel content from the template so a refactor that
	// silently empties out the YAML still fails this test.
	for _, want := range []string{"allowed_sources", "required_tools"} {
		if !containsField(policy, want) {
			t.Errorf("sample policy missing field %q", want)
		}
	}
}

// TestWriteSamplePolicy_RefusesOverwrite asserts the safety guard that
// keeps a misclick (CLI or TUI) from silently replacing a customized
// policy with the starter template.
func TestWriteSamplePolicy_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.yaml")

	if err := os.WriteFile(path, []byte("name: existing\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := WriteSamplePolicy(path)
	if err == nil {
		t.Fatal("expected refusal when target file exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' in error, got: %v", err)
	}

	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read after refusal: %v", readErr)
	}
	if string(got) != "name: existing\n" {
		t.Errorf("WriteSamplePolicy mutated existing file, got: %q", string(got))
	}
}

// TestWriteSamplePolicy_CreatesParentDir verifies the helper materialises
// the parent directory on demand. The CLI/TUI both rely on this to avoid
// a separate mkdir step.
func TestWriteSamplePolicy_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "compliance", "policy.yaml")

	if err := WriteSamplePolicy(path); err != nil {
		t.Fatalf("WriteSamplePolicy: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file at %s, stat err: %v", path, err)
	}
}

// TestWriteSamplePolicy_RejectsEmptyPath guards against the helper being
// called with a zero-value path — that would otherwise fall through to
// os.WriteFile("", …) which produces a confusing OS-level error.
func TestWriteSamplePolicy_RejectsEmptyPath(t *testing.T) {
	if err := WriteSamplePolicy(""); err == nil {
		t.Fatal("expected error for empty path")
	}
}

// containsField does a coarse "is this field populated?" check on the
// loaded policy. We avoid asserting exact tools/licenses so the
// template can evolve without forcing a test churn.
func containsField(p *Policy, field string) bool {
	switch field {
	case "allowed_sources":
		return len(p.AllowedSources) > 0
	case "allowed_licenses":
		return len(p.AllowedLicenses) > 0
	case "blocked_licenses":
		return len(p.BlockedLicenses) > 0
	case "required_tools":
		return len(p.RequiredTools) > 0
	}
	return false
}
