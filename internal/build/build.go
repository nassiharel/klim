package build

import (
	"fmt"
	"runtime/debug"
)

// Version, Commit, and Date are populated at build time via ldflags:
//
//	-X github.com/nassiharel/clim/internal/build.Version=v1.0.0
//	-X github.com/nassiharel/clim/internal/build.Commit=abc1234
//	-X github.com/nassiharel/clim/internal/build.Date=2024-01-01T00:00:00Z
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

func init() {
	// Fallback: if not set via ldflags, try module metadata
	// (works when installed via `go install`)
	if Version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			Version = info.Main.Version
		}
	}
}

// Info returns a formatted version string.
func Info() string {
	return fmt.Sprintf("clim %s (commit: %s, built: %s)", Version, Commit, Date)
}
