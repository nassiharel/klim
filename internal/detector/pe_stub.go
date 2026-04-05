//go:build !windows

package detector

// detectPE is a no-op on non-Windows platforms.
// PE version resources are a Windows-only concept.
func detectPE(_ string) string {
	return ""
}
