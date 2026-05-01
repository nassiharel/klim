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
// Each package manager is run as a subtest; when its binary is not on
// PATH the subtest calls t.Skipf, so skips appear explicitly in the
// test log and machine-readable output (SKIP rather than silent no-op).
// If none of the supported package managers are available on the host
// the whole test is marked skipped — catching minimal runners where the
// job would otherwise pass without probing anything.
package livecheck
