package pathbackup

import (
	"context"
	"os/exec"
)

// runCmd is a tiny wrapper around exec.CommandContext + CombinedOutput.
// Declared as a package-level var so tests can replace it with a stub
// without reaching for build tags or linker tricks. The context lets
// callers bound execution time, which matters because PowerShell on
// Windows can stall indefinitely on broken profiles or
// execution-policy prompts.
var runCmd = func(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(out), err
}
