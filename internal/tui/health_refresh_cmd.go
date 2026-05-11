package tui

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/nassiharel/klim/internal/registry"
	"github.com/nassiharel/klim/internal/service"
)

// healthPathRefreshedMsg carries the result of a post-PATH-fix
// lightweight refresh: re-read PATH from the OS, re-walk PATH for
// every catalog tool (no version resolution), and let the receiver
// re-diagnose. Tools are returned with refreshed Instances only;
// Version / Latest fields are untouched.
type healthPathRefreshedMsg struct {
	Tools   []registry.Tool
	Took    time.Duration
	NewPATH string // PATH value klim is now using (post-refresh)
	Err     error
}

// refreshAfterPathFixCmd is the fast post-PATH-fix replacement for a
// full startScan(). It:
//
//  1. Re-reads the effective PATH from the OS source so klim's
//     perceived $PATH reflects what the just-applied command did.
//     Currently a no-op on POSIX (the fix only mutated the child
//     shell's PATH); on Windows we pull HKLM + HKCU PATH out of the
//     registry via PowerShell.
//  2. Calls svc.RewalkPath — a Finder.FindAll pass that updates
//     tools[].Instances based on the new PATH. Cheap: file stats,
//     no subprocesses to package managers.
//  3. Returns the updated tool slice to the caller, which then
//     re-runs doctor.Diagnose locally (microseconds) and updates
//     m.doctorIssues / derived state.
//
// Wall-clock budget: tens of milliseconds vs. several seconds for
// the full startScan(). Version resolution and catalog reload are
// both skipped — the user just fixed PATH, not anything that would
// change a tool's version or its marketplace metadata.
func refreshAfterPathFixCmd(svc *service.ToolService, tools []registry.Tool) tea.Cmd {
	return func() tea.Msg {
		t0 := time.Now()
		newPATH := refreshProcessPATH()
		// Work on a copy so the model's old tools slice stays
		// untouched until the message reaches Update. Each tool is
		// shallow-copied; only Instances is rebuilt by FindAll, so
		// this avoids racing with anything else reading m.tools.
		clone := make([]registry.Tool, len(tools))
		copy(clone, tools)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := svc.RewalkPath(ctx, clone); err != nil {
			return healthPathRefreshedMsg{Tools: clone, Took: time.Since(t0), NewPATH: newPATH, Err: err}
		}
		return healthPathRefreshedMsg{Tools: clone, Took: time.Since(t0), NewPATH: newPATH}
	}
}

// refreshProcessPATH updates the current process' $PATH from the OS
// so subsequent PATH-walks and diagnostics reflect the fix the user
// just applied.
//
// On Windows the Effective process PATH at launch was the
// concatenation of the Machine and User PATH registry values. The
// PATH-fix commands klim emits write to the User PATH key. To pick
// up the change without restarting we read both keys back through
// PowerShell, concatenate them, and call os.Setenv. Errors are
// silent — the worst case is the next diagnostic still sees the
// stale PATH, which is no worse than before this refresh existed.
//
// On POSIX the snippet runs in a `sh -c` child and only changes
// that child's PATH; persisting requires editing the user's shell
// rc, which we deliberately don't do. We return the unchanged
// process PATH so the message carries something sensible.
func refreshProcessPATH() string {
	if runtime.GOOS != "windows" {
		return os.Getenv("PATH")
	}
	machine, _ := readEnvScope("Machine")
	user, _ := readEnvScope("User")
	newPATH := strings.TrimSpace(machine)
	if user = strings.TrimSpace(user); user != "" {
		if newPATH != "" && !strings.HasSuffix(newPATH, ";") {
			newPATH += ";"
		}
		newPATH += user
	}
	if newPATH == "" {
		return os.Getenv("PATH")
	}
	_ = os.Setenv("PATH", newPATH)
	return newPATH
}

func readEnvScope(scope string) (string, error) {
	out, err := exec.Command(
		"powershell", "-NoProfile", "-NonInteractive", "-Command",
		"[Environment]::GetEnvironmentVariable('PATH', '"+scope+"')",
	).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
