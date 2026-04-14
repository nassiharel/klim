package detector

import (
	"testing"

	"github.com/nassiharel/clim/internal/registry"
)

func TestDetectGoBuildInfo_NonExistent(t *testing.T) {
	got := detectGoBuildInfo("/nonexistent/path/to/binary")
	if got != "" {
		t.Errorf("detectGoBuildInfo(nonexistent) = %q, want empty", got)
	}
}

func TestFallbackDetect_NonExistent(t *testing.T) {
	got := FallbackDetect("/nonexistent/path/to/binary")
	if got != "" {
		t.Errorf("FallbackDetect(nonexistent) = %q, want empty", got)
	}
}

func TestEnrichOne_SkipsExistingVersions(t *testing.T) {
	tool := &registry.Tool{
		Instances: []registry.Instance{
			{Path: "/nonexistent/bin/tool", Version: "1.2.3"},
		},
	}
	EnrichOne(tool)
	// Version should be unchanged — EnrichOne only fills empty versions.
	if tool.Instances[0].Version != "1.2.3" {
		t.Errorf("Version changed to %q, expected 1.2.3", tool.Instances[0].Version)
	}
}

func TestEnrichOne_NoInstances(t *testing.T) {
	tool := &registry.Tool{}
	EnrichOne(tool) // should not panic
	if len(tool.Instances) != 0 {
		t.Error("expected no instances")
	}
}

func TestEnrichFallback(t *testing.T) {
	tools := []registry.Tool{
		{Instances: []registry.Instance{{Path: "/nonexistent", Version: "1.0"}}},
		{Instances: []registry.Instance{{Path: "/also/nonexistent", Version: ""}}},
	}
	EnrichFallback(tools)
	// First tool: version unchanged (already set).
	if tools[0].Instances[0].Version != "1.0" {
		t.Errorf("tool 0 version = %q, want 1.0", tools[0].Instances[0].Version)
	}
	// Second tool: version still empty (binary doesn't exist, fallback returns "").
	if tools[1].Instances[0].Version != "" {
		t.Errorf("tool 1 version = %q, want empty", tools[1].Instances[0].Version)
	}
}
