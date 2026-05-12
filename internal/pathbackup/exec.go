package pathbackup

import "os/exec"

// runCmd is a tiny wrapper around exec.CombinedOutput. Declared as a
// package-level var so tests can replace it with a stub without
// reaching for build tags or linker tricks.
var runCmd = func(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}
