package detector

import (
	"testing"

	"github.com/nassiharel/clim/internal/registry"
)

func TestVersionRegex(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"fzf", "0.71.0 (62899fd7)", "0.71.0"},
		{"simple semver", "1.2.3", "1.2.3"},
		{"two segments", "8.1", "8.1"},
		{"four segments", "1.2.3.4", "1.2.3.4"},
		{"python", "Python 3.12.1", "3.12.1"},
		{"docker", "Docker version 27.5.1, build 9f9e405", "27.5.1"},
		{"git", "git version 2.53.0", "2.53.0"},
		{"v-prefix", "v22.12.0", "22.12.0"},
		{"v-prefix with label", "Terraform v1.10.5", "1.10.5"},
		{"kubectl", "Client Version: v1.33.3", "1.33.3"},
		{"go version", "go version go1.23.4 windows/amd64", "1.23.4"},
		{"no version", "some random text", ""},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ""
			if m := versionRe.FindStringSubmatch(tt.input); len(m) >= 2 {
				got = m[1]
			}
			if got != tt.want {
				t.Errorf("versionRe on %q = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

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
