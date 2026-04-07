package finder

import (
	"os"
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
		k.Close()
	}

	// User PATH: HKCU\Environment
	if k, err := registry.OpenKey(registry.CURRENT_USER,
		`Environment`, registry.QUERY_VALUE); err == nil {
		if v, _, err := k.GetStringValue("Path"); err == nil {
			parts = append(parts, v)
		}
		k.Close()
	}

	return strings.Join(parts, string(os.PathListSeparator))
}
