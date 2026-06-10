//go:build windows

package sessionstui

import "os/exec"

// shellExec wraps a shell snippet for execution via cmd.exe so the
// `cd && cli` form in RestartCommand works as-is on Windows.
func shellExec(snippet string) *exec.Cmd {
	return exec.Command("cmd", "/c", snippet)
}
