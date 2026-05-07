//go:build !windows

package finder

// registryPATH is a no-op on non-Windows platforms.
func registryPATH() string { return "" }

// extraInstallRoots is a no-op on non-Windows platforms. The finder's
// PATH scan is sufficient on Linux/macOS where almost every developer
// tool ships a CLI that lands on PATH (or in /Applications/<App>.app
// where klim deliberately does not look — those are GUI apps not
// reasonably managed by klim's CLI-centric flows).
func extraInstallRoots() []string { return nil }
