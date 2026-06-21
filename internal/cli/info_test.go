package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/registry"
)

// Star-count formatting tests live in internal/githubfmt; the CLI uses
// that package directly so the contract is exercised there.

func TestNotFoundError_IsUsageError(t *testing.T) {
	// A typo on the tool name is malformed user input, so it must
	// surface as a UsageError so Run() maps it to exit code 2.
	// Otherwise scripts can't tell `klim tool info kubctl` (typo) apart
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
	got := FormatReference(Reference{
		Kind: "teamfile", Path: "/home/me/.klim.yaml",
		Required: false, Constraint: ">=1.28",
	})
	want := ".klim.yaml (optional >=1.28) — /home/me/.klim.yaml"
	if got != want {
		t.Errorf("optional teamfile with constraint:\n  got:  %s\n  want: %s", got, want)
	}

	// Required teamfile pin: same constraint format.
	got = FormatReference(Reference{
		Kind: "teamfile", Path: "/home/me/.klim.yaml",
		Required: true, Constraint: ">=1.28",
	})
	want = ".klim.yaml (required >=1.28) — /home/me/.klim.yaml"
	if got != want {
		t.Errorf("required teamfile with constraint:\n  got:  %s\n  want: %s", got, want)
	}

	// Project optional with constraint — both role and constraint must appear.
	got = FormatReference(Reference{
		Kind: "project", Name: "myapp", Path: "/projects/myapp/.klim.yaml",
		Required: false, Constraint: "~1.5",
	})
	want = `Project "myapp" (optional ~1.5) — /projects/myapp/.klim.yaml`
	if got != want {
		t.Errorf("project optional with constraint:\n  got:  %s\n  want: %s", got, want)
	}

	// Empty constraint: role appears alone, no trailing space.
	got = FormatReference(Reference{
		Kind: "teamfile", Path: "/home/me/.klim.yaml", Required: true,
	})
	want = ".klim.yaml (required) — /home/me/.klim.yaml"
	if got != want {
		t.Errorf("teamfile required no constraint:\n  got:  %s\n  want: %s", got, want)
	}
}

// TestBuildInfoReport_JSONContract locks the documented JSON shape of
// `klim tool info <tool> --output json`. Specifically:
//   - empty arrays must serialize as [] (not null) for tags/instances/
//     packages/references/related_tools/warnings
//   - GitHub block is populated when GitHubInfo is present
//   - non-empty packages list is preserved in canonical order (winget,
//     choco, scoop, brew, apt, snap, npm) — drift here would change
//     `klim tool info --output json | jq` consumers' assumptions
func TestBuildInfoReport_JSONContract(t *testing.T) {
	chdirTemp(t)
	redirectConfig(t)

	tool := registry.Tool{
		Name:        "kubectl",
		DisplayName: "kubectl",
		Category:    "Containers",
		Tags:        []string{"k8s"},
		GitHubSlug:  "kubernetes/kubectl",
		GitHubInfo: &registry.GitHubInfo{
			Stars:       3300,
			Forks:       997,
			Description: "Kubernetes CLI",
			Homepage:    "https://kubernetes.io",
			License:     "Apache-2.0",
			Topics:      []string{"k8s", "cli"},
			PushedAt:    "2026-04-15T12:00:00Z",
		},
		Packages: registry.PackageIDs{
			Winget: "Kubernetes.kubectl",
			Brew:   "kubernetes-cli",
		},
	}
	cmd := withRefscanCtx(t, nil)
	report := buildInfoReport(cmd, &tool, []registry.Tool{tool})

	// Marshal+unmarshal so we test the actual JSON wire shape, not just
	// the in-memory struct.
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Empty-array fields MUST be present as [] (not null/missing).
	for _, key := range []string{"tags", "instances", "packages", "references", "related_tools", "warnings"} {
		v, ok := got[key]
		if !ok {
			t.Errorf("missing required field %q in JSON", key)
			continue
		}
		// Either a non-nil slice (with elements) or [] — never null.
		if v == nil {
			t.Errorf("field %q is null; expected [] or array", key)
		}
		if _, isSlice := v.([]any); !isSlice {
			t.Errorf("field %q is %T; expected []any", key, v)
		}
	}

	// GitHub block present + populated.
	gh, ok := got["github"].(map[string]any)
	if !ok {
		t.Fatalf("github block missing or wrong type: %v", got["github"])
	}
	if gh["slug"] != "kubernetes/kubectl" {
		t.Errorf("github.slug = %v", gh["slug"])
	}
	if gh["url"] != "https://github.com/kubernetes/kubectl" {
		t.Errorf("github.url = %v", gh["url"])
	}
	if gh["license"] != "Apache-2.0" {
		t.Errorf("github.license = %v", gh["license"])
	}

	// Packages list: winget first, brew second (canonical display order).
	pkgs, _ := got["packages"].([]any)
	if len(pkgs) != 2 {
		t.Fatalf("packages len = %d, want 2: %+v", len(pkgs), pkgs)
	}
	first := pkgs[0].(map[string]any)
	if first["source"] != "winget" || first["id"] != "Kubernetes.kubectl" {
		t.Errorf("packages[0] = %v", first)
	}
	second := pkgs[1].(map[string]any)
	if second["source"] != "brew" || second["id"] != "kubernetes-cli" {
		t.Errorf("packages[1] = %v", second)
	}
}

// TestBuildInfoReport_GitHubTopicsAlwaysArray locks the contract
// that the GitHub block's topics field is always serialized as an
// array (never omitted), so consumers can iterate without nil-checks
// — the same guarantee the documented JSON shape advertises for
// every collection field.
func TestBuildInfoReport_GitHubTopicsAlwaysArray(t *testing.T) {
	chdirTemp(t)
	redirectConfig(t)

	// Tool with GitHub metadata but no topics.
	tool := registry.Tool{
		Name:       "kubectl",
		GitHubSlug: "kubernetes/kubectl",
		GitHubInfo: &registry.GitHubInfo{
			Stars:  1000,
			Topics: nil,
		},
	}
	cmd := withRefscanCtx(t, nil)
	report := buildInfoReport(cmd, &tool, []registry.Tool{tool})
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(raw), `"topics":[]`) {
		t.Errorf("expected `\"topics\":[]` in JSON, got: %s", raw)
	}
}
