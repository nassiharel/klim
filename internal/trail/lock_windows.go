//go:build windows

package trail

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"github.com/nassiharel/clim/internal/fileutil"
)

// LockFileEx flag (Windows).
const lockfileExclusiveLock = 0x00000002

var (
	modkernel32      = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = modkernel32.NewProc("LockFileEx")
	procUnlockFileEx = modkernel32.NewProc("UnlockFileEx")
)

// acquireLock takes an exclusive lock on path using LockFileEx. The lock
// covers the entire file region; other processes attempting to lock the
// same file block until release.
func acquireLock(path string) (func(), error) {
	if err := fileutil.EnsureDir(path); err != nil {
		return nil, fmt.Errorf("creating lock dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // G302: 0o644 is fine for advisory lock files; lower in case lint demands it
	if err != nil {
		return nil, fmt.Errorf("opening lock %s: %w", path, err)
	}

	var ol syscall.Overlapped
	r1, _, e1 := syscall.SyscallN(
		procLockFileEx.Addr(),
		f.Fd(),
		uintptr(lockfileExclusiveLock),
		0,
		^uintptr(0),
		^uintptr(0),
		uintptr(unsafe.Pointer(&ol)),
	)
	if r1 == 0 {
		_ = f.Close()
		return nil, fmt.Errorf("acquiring lock %s: %w", path, e1)
	}
	return func() {
		var ol syscall.Overlapped
		_, _, _ = syscall.SyscallN(
			procUnlockFileEx.Addr(),
			f.Fd(),
			0,
			^uintptr(0),
			^uintptr(0),
			uintptr(unsafe.Pointer(&ol)),
		)
		_ = f.Close()
	}, nil
}
