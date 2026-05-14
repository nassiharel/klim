//go:build !windows

package cli

import "syscall"

func init() {
	syscallExec = syscall.Exec
}
