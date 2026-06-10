//go:build !windows

package sessionstui

import "os/exec"

// shellExec wraps a shell snippet (e.g. `cd "/dev/klim" && claude --resume <id>`)
// into an *exec.Cmd that runs via /bin/sh. The dashboard uses this when
// handing control to the agent CLI via the `r` keybinding.
func shellExec(snippet string) *exec.Cmd {
	return exec.Command("/bin/sh", "-c", snippet)
}