//go:build !windows

package finder

// registryPATH is a no-op on non-Windows platforms.
func registryPATH() string { return "" }
