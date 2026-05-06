package detector

import (
	"debug/buildinfo"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/nassiharel/klim/internal/registry"
)

// FallbackDetect reads version information from a binary file without executing it.
// Used as a fallback when package manager queries don't return a version.
// Tries Go build info first, then PE version resources on Windows.
func FallbackDetect(path string) string {
	// Resolve Chocolatey shims to the real binary.
	if resolved := resolveChocoShim(path); resolved != "" {
		path = resolved
	}

	if ver := detectGoBuildInfo(path); ver != "" {
		slog.Debug("version via Go buildinfo", "path", path, "version", ver)
		return ver
	}

	if ver := detectPE(path); ver != "" {
		slog.Debug("version via PE resource", "path", path, "version", ver)
		return ver
	}

	slog.Debug("fallback detection found nothing", "path", path)
	return ""
}

// EnrichFallback fills in Version for any instance that still has an empty version
// after package manager queries, using PE metadata and Go buildinfo.
func EnrichFallback(tools []registry.Tool) {
	for i := range tools {
		EnrichOne(&tools[i])
	}
}

// EnrichOne fills in Version for a single tool's instances using fallback detection.
func EnrichOne(tool *registry.Tool) {
	for j := range tool.Instances {
		inst := &tool.Instances[j]
		if inst.Version == "" {
			inst.Version = FallbackDetect(inst.Path)
		}
	}
}

func detectGoBuildInfo(path string) string {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return ""
	}

	ver := info.Main.Version
	if ver == "" || ver == "(devel)" {
		return ""
	}

	ver = strings.TrimPrefix(ver, "v")

	// Discard Go pseudo-versions.
	if strings.HasPrefix(ver, "0.0.0-") {
		return ""
	}

	return ver
}

// resolveChocoShim is defined in detector_choco.go (or inline below for simplicity).
func resolveChocoShim(path string) string {
	lower := strings.ToLower(filepath.Clean(path))
	if !strings.Contains(lower, "chocolatey") {
		return ""
	}
	// Delegate to the platform-specific resolver.
	return resolveChocoShimPlatform(path)
}
