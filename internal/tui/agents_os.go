package tui

import "runtime"

// goos is wrapped so tests can override `runtime.GOOS` indirectly
// (set via a build helper rather than mutating a package-level var
// in production code).
var goos = runtime.GOOS
