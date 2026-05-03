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
func acquireLock(path string) (func(), error) {
	if err := fileutil.EnsureDir(path); err != nil {
		return nil, fmt.Errorf("creating lock dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // G302: advisory lock file, perms intentionally restrictive
	if err != nil {
		return nil, fmt.Errorf("opening lock %s: %w", path, err)
	}
	for {
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX)
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
