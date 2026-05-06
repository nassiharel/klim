package web

import (
	"os/exec"
	"runtime"
)

// OpenBrowser tries to launch the user's default browser pointed at
// url. Returns the underlying exec error if the platform's open
// command fails. Callers should treat any failure as best-effort —
// always print the URL too so the user can copy-paste it manually.
func OpenBrowser(url string) error {
	cmd, args := openCommand(url)
	if cmd == "" {
		return nil
	}
	c := exec.Command(cmd, args...)
	// Detach: we don't care about the browser's exit code, and Wait
	// would block this CLI process for the lifetime of the browser.
	return c.Start()
}

// openCommand returns the platform-specific launcher invocation. The
// command is empty on unknown platforms; we treat that as a no-op
// rather than guessing.
func openCommand(url string) (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{url}
	case "windows":
		// rundll32 is robust against PowerShell quoting and works on
		// minimal Windows installs that don't have `start` in PATH.
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	case "linux", "freebsd", "openbsd", "netbsd":
		return "xdg-open", []string{url}
	}
	return "", nil
}
