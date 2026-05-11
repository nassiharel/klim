package pathbackup

import "os/exec"

// runCmd is a tiny wrapper around exec.CombinedOutput so we can stub
// it in tests if needed and keep the call site readable.
func runCmd(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}
