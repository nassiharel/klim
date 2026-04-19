// Package livecheck contains the opt-in integration test that verifies
// each package ID declared in marketplace/tools/*.yaml resolves against
// the corresponding native package manager (winget, choco, scoop, brew,
// apt, snap, npm).
//
// The tests are guarded by the `integration` build tag so the normal
// `go test ./...` run stays fast and offline. To execute them:
//
//	go test -tags=integration -timeout=30m ./internal/marketplace/livecheck/...
//
// Per-tool/per-PM subtests are reported as Skip when the package
// manager binary is not installed on the current host, so the same
// command works on every CI runner (Windows, macOS, Linux) — each
// runner only exercises the package managers it natively supports.
package livecheck
