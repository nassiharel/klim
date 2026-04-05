package detector

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/nassiharel/clim/internal/registry"
)

// DetectVersion runs "binary --version" with a timeout and returns the first
// non-empty line of combined stdout+stderr output. Returns an empty string
// on any failure (timeout, non-zero exit, no output, etc.).
func DetectVersion(ctx context.Context, tool registry.Tool, timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, tool.Path, "--version")
	cmd.Stdin = nil // prevent interactive prompts from blocking

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Run(); err != nil {
		// Some tools write version info even on non-zero exit —
		// try to extract output anyway.
		if buf.Len() == 0 {
			return ""
		}
	}

	return firstLine(buf.String())
}

// DetectAll runs DetectVersion concurrently for all tools, mutating each
// tool's Version field in place. Concurrency is bounded by a semaphore
// to avoid overwhelming the system.
func DetectAll(ctx context.Context, tools []registry.Tool, timeout time.Duration, concurrency int) {
	if concurrency <= 0 {
		concurrency = runtime.NumCPU() * 2
	}

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for i := range tools {
		wg.Add(1)
		sem <- struct{}{} // acquire
		go func(t *registry.Tool) {
			defer wg.Done()
			defer func() { <-sem }() // release
			t.Version = DetectVersion(ctx, *t, timeout)
		}(&tools[i])
	}

	wg.Wait()
}

// firstLine extracts the first non-empty line from the given string.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		s = s[:idx]
	}
	return strings.TrimSpace(s)
}
