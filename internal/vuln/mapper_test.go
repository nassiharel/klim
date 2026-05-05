package vuln

import (
	"testing"

	"github.com/nassiharel/clim/internal/registry"
)

func TestMap_NotInstalled(t *testing.T) {
	tool := registry.Tool{Name: "jq"}
	coords, reason := Map(tool)
	if coords != nil {
		t.Errorf("uninstalled tool should produce no coords, got %v", coords)
	}
	if reason != "not installed" {
		t.Errorf("reason = %q, want 'not installed'", reason)
	}
}

func TestMap_NoVersion(t *testing.T) {
	tool := registry.Tool{
		Name: "jq",
		Instances: []registry.Instance{
			{Path: "/usr/bin/jq", Version: "", Source: registry.SourceApt},
		},
	}
	_, reason := Map(tool)
	if reason != "no version detected" {
		t.Errorf("reason = %q", reason)
	}
}

func TestMap_NPMOnly(t *testing.T) {
	tool := registry.Tool{
		Name: "yarn",
		Packages: registry.PackageIDs{
			NPM: "yarn",
		},
		Instances: []registry.Instance{
			{Path: "/usr/local/bin/yarn", Version: "1.22.0", Source: registry.SourceNPM},
		},
	}
	coords, reason := Map(tool)
	if reason != "" {
		t.Fatalf("unexpected skip: %q", reason)
	}
	if len(coords) != 1 {
		t.Fatalf("coords = %d, want 1", len(coords))
	}
	if coords[0].Ecosystem != EcosystemNPM || coords[0].Package != "yarn" || coords[0].Version != "1.22.0" {
		t.Errorf("coord = %+v", coords[0])
	}
}

func TestMap_BrewOnlyIsSkipped(t *testing.T) {
	// Homebrew is not a valid OSV ecosystem, so a brew-only tool
	// is skipped (npm is the only currently-supported ecosystem).
	tool := registry.Tool{
		Name:       "jq",
		GitHubSlug: "stedolan/jq",
		Packages: registry.PackageIDs{
			Brew: "jq",
		},
		Instances: []registry.Instance{
			{Path: "/usr/local/bin/jq", Version: "1.7", Source: registry.SourceBrew},
		},
	}
	coords, reason := Map(tool)
	if coords != nil {
		t.Errorf("brew-only tool should not produce coords, got %v", coords)
	}
	if reason == "" {
		t.Error("expected a skip reason for brew-only tool")
	}
}

func TestMap_NoEcosystemMapping(t *testing.T) {
	// Installed via apt, no NPM/Brew id, no GitHub slug — nothing to query.
	tool := registry.Tool{
		Name: "exotic",
		Packages: registry.PackageIDs{
			Apt: "exotic",
		},
		Instances: []registry.Instance{
			{Path: "/usr/bin/exotic", Version: "1.0", Source: registry.SourceApt},
		},
	}
	coords, reason := Map(tool)
	if coords != nil {
		t.Errorf("expected no coords, got %v", coords)
	}
	if reason == "" {
		t.Error("expected a skip reason")
	}
}
