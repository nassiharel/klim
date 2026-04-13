package detector

import (
	"context"
	"debug/buildinfo"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nassiharel/clim/internal/registry"
)

// FallbackDetect reads version information from a binary file without executing it.
// Used as a fallback when package manager queries don't return a version.
// Tries Go build info first, then PE version resources on Windows,
// then executes the binary with --version as a last resort.
func FallbackDetect(path string) string {
	// Resolve Chocolatey shims to the real binary.
	if resolved := resolveChocoShim(path); resolved != "" {
		path = resolved
	}

	if ver := detectGoBuildInfo(path); ver != "" {
		return ver
	}

	if ver := detectPE(path); ver != "" {
		return ver
	}

	if ver := detectCLIVersion(path); ver != "" {
		return ver
	}

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

// versionRe matches semver-like version strings: 1.2.3, 0.71.0, 2.53.0.windows.1, etc.
var versionRe = regexp.MustCompile(`\b(\d+\.\d+(?:\.\d+)?(?:[.\-]\w+)*)\b`)

// detectCLIVersion executes the binary with --version and parses the output.
// This is a last-resort fallback — only called when static detection fails.
func detectCLIVersion(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "--version")
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Take only the first line to avoid parsing multi-line output.
	first := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if first == "" {
		return ""
	}

	// Extract the first semver-like version from the line.
	if m := versionRe.FindString(first); m != "" {
		return m
	}

	return ""
}
