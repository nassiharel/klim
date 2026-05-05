package tui

import "strings"

// Known winget exit codes (from
// https://github.com/microsoft/winget-cli/blob/master/src/AppInstallerSharedLib/Public/AppInstallerSharedLib/AppInstallerErrors.h)
// that produce the unsigned form most users see when winget fails.
//
//   0x8A150014 (2316632084) — APPINSTALLER_CLI_ERROR_NO_APPLICATIONS_FOUND
//   0x8A150010 (2316632080) — APPINSTALLER_CLI_ERROR_NO_PACKAGES_AVAILABLE
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
// (catalog id mismatch).
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
			"      winget list <name>     # check what winget knows about\n" +
			"      where <command>        # find the actual binary location"
	case wingetExitNoPackageAvailable:
		return "  ℹ winget has no package matching that ID. The catalog entry\n" +
			"    may be stale or use a different ID than your local winget source.\n" +
			"    Try: winget search <name>"
	}
	return ""
}
