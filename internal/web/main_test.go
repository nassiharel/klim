package web

import (
	"os"
	"testing"

	"github.com/nassiharel/klim/internal/registry"
)

// TestMain pins the package-manager availability override for the
// duration of the web test binary so jobs/handler tests don't depend
// on what the runner happens to have on PATH.
//
// Without this, tests that resolve install commands via
// registry.PackageIDs.BestInstallSource (e.g. TestJobs_StartAndComplete)
// fail on Linux/macOS runners that lack winget/brew but have apt — and
// on Windows runners with only winget but the fixture tool only
// declares brew/apt. Web tests assert HTTP/handler behaviour, not real
// PM detection, so a hermetic "all PMs available" stub is the right
// scope.
//
// Tests that explicitly want to assert PM-availability behaviour can
// still call SetPMAvailableFunc with their own stub and use
// t.Cleanup to restore it (the override is goroutine-safe via
// atomic.Pointer).
func TestMain(m *testing.M) {
	registry.SetPMAvailableFunc(func(_ registry.InstallSource) bool { return true })
	code := m.Run()
	// Reset before exit so a future move to a TestMain that calls
	// other test entrypoints doesn't carry the override over.
	registry.SetPMAvailableFunc(nil)
	os.Exit(code)
}
