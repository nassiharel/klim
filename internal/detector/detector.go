package detector

import (
	"debug/buildinfo"
	"runtime"
	"strings"
	"sync"

	"github.com/nassiharel/clim/internal/registry"
)

// Detect reads version information from a binary file without executing it.
// It tries Go build info first (covers Go-compiled tools), then PE version
// resources on Windows (covers C/C++ tools). Returns "" if no version found.
func Detect(path string) string {
	// Layer 1: Go build info (works cross-platform).
	if ver := detectGoBuildInfo(path); ver != "" {
		return ver
	}

	// Layer 2: PE version info (Windows only).
	if ver := detectPE(path); ver != "" {
		return ver
	}

	return ""
}

// DetectAll runs Detect concurrently for all tools, populating each tool's
// Version field in place. Concurrency is bounded by a semaphore.
func DetectAll(tools []registry.Tool, concurrency int) {
	if concurrency <= 0 {
		concurrency = runtime.NumCPU() * 2
	}

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i := range tools {
		wg.Add(1)
		sem <- struct{}{}
		go func(t *registry.Tool) {
			defer wg.Done()
			defer func() { <-sem }()
			t.Version = Detect(t.Path)
		}(&tools[i])
	}

	wg.Wait()
}

// detectGoBuildInfo reads Go module version info embedded in a Go binary.
// Returns the main module version (e.g. "0.21.1") or "" if not a Go binary
// or version is "(devel)".
func detectGoBuildInfo(path string) string {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return ""
	}

	ver := info.Main.Version
	if ver == "" || ver == "(devel)" {
		return ""
	}

	// Strip leading "v" prefix (e.g. "v0.21.1" → "0.21.1").
	ver = strings.TrimPrefix(ver, "v")
	return ver
}
