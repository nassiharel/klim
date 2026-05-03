//go:build windows

package trail

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"

	"github.com/nassiharel/clim/internal/fileutil"
)

// LockFileEx flags (Windows).
const (
	lockfileExclusiveLock = 0x00000002
)

var (
	modkernel32      = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = modkernel32.NewProc("LockFileEx")
	procUnlockFileEx = modkernel32.NewProc("UnlockFileEx")
)

// acquireLock takes a lock on path using LockFileEx. The lock covers
// the entire file region.
//
// readOnly=true asks for a shared (non-exclusive) lock so multiple
// readers don't block each other; readOnly=false uses
// LOCKFILE_EXCLUSIVE_LOCK. In both cases the lock file is created
// with O_CREATE if it doesn't exist — that's required for cross-
// process correctness, because a read that found the file missing
// and skipped locking could race a concurrent first writer. The
// trade-off (read-only commands need write permission on the trail
// dir on first invocation) is intentional: race-free coordination
// wins over the read-only-config-dir edge case.
func acquireLock(path string, readOnly bool) (func(), error) {
	if err := fileutil.EnsureDir(path); err != nil {
		return nil, fmt.Errorf("creating lock dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // G302: advisory lock file
	if err != nil {
		return nil, fmt.Errorf("opening lock %s: %w", path, err)
	}

	lockFlags := uintptr(0)
	if !readOnly {
		lockFlags = lockfileExclusiveLock
	}
	var ol syscall.Overlapped
	r1, _, e1 := syscall.SyscallN(
		procLockFileEx.Addr(),
		f.Fd(),
		lockFlags,
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
