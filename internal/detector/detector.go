package detector

import (
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"sync"
	"time"

	"github.com/nassiharel/clim/internal/registry"
)

// DetectionResult holds the outcome of detecting a single CLI tool.
type DetectionResult struct {
	Found   bool
	Version string // Parsed version string, empty if not parseable.
	Path    string // Absolute path to the binary.
	Error   error  // Non-fatal error (e.g., version parse failure).
}

// DetectAll runs detection for all tools concurrently.
// Returns results in the same order as the input slice.
func DetectAll(ctx context.Context, tools []registry.Tool) []DetectionResult {
	results := make([]DetectionResult, len(tools))
	var wg sync.WaitGroup

	for i, tool := range tools {
		wg.Add(1)
		go func(idx int, t registry.Tool) {
			defer wg.Done()
			results[idx] = DetectOne(ctx, t)
		}(i, tool)
	}

	wg.Wait()
	return results
}

// DetectOne detects a single tool: finds the binary, runs the version command,
// and parses the version from the output.
func DetectOne(ctx context.Context, tool registry.Tool) DetectionResult {
	// Find the binary — try each name in order.
	var binPath string
	var binName string
	for _, name := range tool.BinaryNames {
		path, err := exec.LookPath(name)
		if err == nil {
			binPath = path
			binName = name
			break
		}
	}

	if binPath == "" {
		return DetectionResult{Found: false}
	}

	// Run the version command with a timeout.
	timeout := 10 * time.Second
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := tool.VersionArgs
	cmd := exec.CommandContext(cmdCtx, binName, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Some tools write version info even on non-zero exit.
	// Combine stdout and stderr for regex matching.
	combined := stdout.String() + stderr.String()

	if err != nil && combined == "" {
		return DetectionResult{
			Found: true,
			Path:  binPath,
			Error: err,
		}
	}

	// Parse version from the output.
	version, parseErr := ParseVersion(combined, tool.VersionRegex)
	if parseErr != nil {
		return DetectionResult{
			Found: true,
			Path:  binPath,
			Error: parseErr,
		}
	}

	return DetectionResult{
		Found:   true,
		Version: version,
		Path:    binPath,
	}
}

// ParseVersion extracts a version string from the given output using the regex.
// The regex must contain exactly one capture group.
func ParseVersion(output, pattern string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}

	matches := re.FindStringSubmatch(output)
	if len(matches) < 2 {
		return "", nil // No match — version not parseable but not an error.
	}

	return matches[1], nil
}
