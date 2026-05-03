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
	lockfileFailImmediate = 0x00000001 //nolint:unused // reserved for future try-lock paths
)

var (
	modkernel32      = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx   = modkernel32.NewProc("LockFileEx")
	procUnlockFileEx = modkernel32.NewProc("UnlockFileEx")
)

// acquireLock takes a lock on path using LockFileEx. The lock covers
// the entire file region; other processes attempting to lock the same
// file block until release.
//
// readOnly=true opens the lock file with O_RDONLY and asks for a
// shared (read) lock — multiple read paths can inspect the trail
// concurrently, and none of them need write permission on the trail
// dir or will materialise log.lock if it didn't exist already.
func acquireLock(path string, readOnly bool) (func(), error) {
	flags := os.O_CREATE | os.O_RDWR
	mode := os.FileMode(0o600)
	if readOnly {
		flags = os.O_RDONLY
	} else {
		if err := fileutil.EnsureDir(path); err != nil {
			return nil, fmt.Errorf("creating lock dir: %w", err)
		}
	}
	f, err := os.OpenFile(path, flags, mode) //nolint:gosec // G302: advisory lock file
	if err != nil {
		// No lock file yet on a read path = nothing to lock against.
		if readOnly && os.IsNotExist(err) {
			return func() {}, nil
		}
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
