//go:build !windows

package trail

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"

	"github.com/nassiharel/klim/internal/fileutil"
)

// acquireLock takes an advisory lock on path. It blocks until the lock
// is held and returns a release func that unlocks and closes the FD.
//
// Uses flock(2) on Unix. Inherently advisory: cooperating processes only.
//
// readOnly=true asks for a shared (LOCK_SH) lock so multiple readers
// don't block each other; readOnly=false uses LOCK_EX. In both cases
// the lock file is created with O_CREATE if it doesn't exist — that's
// required for cross-process correctness, because a read that found
// the file missing and skipped locking could race a concurrent first
// writer. The trade-off (read-only commands need write permission on
// the trail dir on first invocation) is intentional: race-free
// coordination wins over the read-only-config-dir edge case.
func acquireLock(path string, readOnly bool) (func(), error) {
	if err := fileutil.EnsureDir(path); err != nil {
		return nil, fmt.Errorf("creating lock dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // G302: advisory lock file
	if err != nil {
		return nil, fmt.Errorf("opening lock %s: %w", path, err)
	}
	lockOp := unix.LOCK_EX
	if readOnly {
		lockOp = unix.LOCK_SH
	}
	for {
		err := unix.Flock(int(f.Fd()), lockOp)
		if err == nil {
			break
		}
		if errors.Is(err, unix.EINTR) {
			continue
		}
		_ = f.Close()
		return nil, fmt.Errorf("acquiring lock %s: %w", path, err)
	}
	return func() {
		_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
		_ = f.Close()
	}, nil
}
