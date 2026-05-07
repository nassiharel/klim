package finder

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// registryPATH reads the User and System PATH values from the Windows registry
// and returns a merged, semicolon-separated string. This picks up directories
// added after the current process started (e.g. by winget portable installs).
func registryPATH() string {
	var parts []string

	// System PATH: HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Control\Session Manager\Environment`,
		registry.QUERY_VALUE); err == nil {
		if v, _, err := k.GetStringValue("Path"); err == nil {
			parts = append(parts, v)
		}
		_ = k.Close()
	}

	// User PATH: HKCU\Environment
	if k, err := registry.OpenKey(registry.CURRENT_USER,
		`Environment`, registry.QUERY_VALUE); err == nil {
		if v, _, err := k.GetStringValue("Path"); err == nil {
			parts = append(parts, v)
		}
		_ = k.Close()
	}

	return strings.Join(parts, string(os.PathListSeparator))
}

// extraInstallRoots returns Windows directories that commonly host
// winget-managed GUI applications whose binaries are NOT placed on
// PATH. These are scanned one level deep as a fallback so tools like
// Freelens (installed at %LOCALAPPDATA%\Programs\Freelens\Freelens.exe)
// are still detected.
//
// We deliberately exclude the global Program Files directories: they
// host hundreds of unrelated apps and the false-positive risk is
// material when a tool name like `bat` or `code` collides with a
// random vendor binary. %LOCALAPPDATA%\Programs is the per-user
// winget convention and far less crowded.
func extraInstallRoots() []string {
	var roots []string
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		roots = append(roots, filepath.Join(d, "Programs"))
	}
	return roots
}
