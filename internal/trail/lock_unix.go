//go:build !windows

package trail

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"

	"github.com/nassiharel/clim/internal/fileutil"
)

// acquireLock takes an exclusive advisory lock on path. It blocks until the
// lock is held and returns a release func that unlocks and closes the FD.
//
// Uses flock(2) on Unix. Inherently advisory: cooperating processes only.
//
// readOnly=true opens the lock file with O_RDONLY and never tries to
// create it. Read paths (Log/Resolve/Show/Diff) pass readOnly=true so
// they don't need write permission on the trail directory and don't
// silently materialise log.lock on first read of a populated trail
// (which would otherwise break read-only / root-owned config dirs).
// Write paths (Capture/Prune) pass readOnly=false because they will
// create the lock if no other process has yet.
func acquireLock(path string, readOnly bool) (func(), error) {
	flags := os.O_CREATE | os.O_RDWR
	mode := os.FileMode(0o600)
	if readOnly {
		flags = os.O_RDONLY
	}
	if !readOnly {
		if err := fileutil.EnsureDir(path); err != nil {
			return nil, fmt.Errorf("creating lock dir: %w", err)
		}
	}
	f, err := os.OpenFile(path, flags, mode) //nolint:gosec // G302: advisory lock file, perms intentionally restrictive
	if err != nil {
		// Read-only inspection of a trail that was never written to
		// has nothing to lock against, so a missing log.lock is
		// benign — return a no-op release.
		if readOnly && os.IsNotExist(err) {
			return func() {}, nil
		}
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
