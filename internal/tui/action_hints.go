package tui

import (
	"errors"
	"os/exec"
	"strings"
)

// Known winget exit codes (from
// https://github.com/microsoft/winget-cli/blob/master/src/AppInstallerSharedLib/Public/AppInstallerSharedLib/AppInstallerErrors.h)
// that produce the unsigned form most users see when winget fails.
//
//	0x8A150014 (2316632084) — APPINSTALLER_CLI_ERROR_NO_APPLICATIONS_FOUND
//	0x8A150010 (2316632080) — APPINSTALLER_CLI_ERROR_NO_PACKAGES_AVAILABLE
const (
	wingetExitNotInstalled       = 2316632084
	wingetExitNoPackageAvailable = 2316632080
)

// actionFailureHint returns a human-readable hint for known
// well-meaning failures, or "" when no targeted hint applies. Called
// by toolActionCmd.Run after a non-zero exit so the user sees a
// useful next step instead of just a hex error code.
//
// Today the only handled cases are winget's "package not installed"
// (which clim repeatedly hit when its source-detection heuristic
// optimistically called Program Files binaries winget-managed even
// when winget had no record of them) and "no package available"
// (catalog id mismatch). 'where.exe' is used (rather than 'where')
// because PowerShell aliases 'where' to Where-Object.
func actionFailureHint(args []string, exitCode int) string {
	if len(args) == 0 {
		return ""
	}
	pm := strings.ToLower(args[0])
	if !strings.Contains(pm, "winget") {
		return ""
	}
	switch exitCode {
	case wingetExitNotInstalled:
		return "  ℹ winget reports this package isn't installed under that ID.\n" +
			"    The binary may have been installed by another method (manual\n" +
			"    download, Chocolatey, scoop, MSI installer). Try:\n" +
			"      winget list <name>     # what winget actually knows about\n" +
			"      where.exe <command>    # where the binary lives (use where.exe,\n" +
			"                             # not 'where' — that's a PowerShell alias)"
	case wingetExitNoPackageAvailable:
		return "  ℹ winget has no package matching that ID. The catalog entry\n" +
			"    may be stale or use a different ID than your local winget source.\n" +
			"    Try: winget search <name>"
	}
	return ""
}

// hintFromError returns a friendly hint by inspecting err for an
// *exec.ExitError and translating the exit code via
// actionFailureHint. Returns "" when err is nil, isn't an exit
// failure, or has no hint registered. Used by the pack and backup
// install/remove flows whose tea.ExecProcess callback only receives
// the wrapped error (not the exit code directly).
func hintFromError(args []string, err error) string {
	if err == nil {
		return ""
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return ""
	}
	return actionFailureHint(args, exitErr.ExitCode())
}

// errMsgWithHint is intentionally unused now: prior versions joined
// err.Error() with the multi-line hint via "\n", but pack/backup
// renderers treat errMsg as a single-line table cell and the
// embedded newline broke alignment. Callers now write err.Error()
// into errMsg and the multi-line hint into a separate item.hint
// field. The function is kept so external callers (and tests below)
// that want the joined form still have access; in-tree callers all
// route through hintFromError directly.
func errMsgWithHint(args []string, err error) string {
	if err == nil {
		return ""
	}
	out := err.Error()
	if hint := hintFromError(args, err); hint != "" {
		out += "\n" + hint
	}
	return out
}
