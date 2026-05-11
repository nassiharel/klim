package doctor

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nassiharel/klim/internal/registry"
)

// ActionKind enumerates the interactive remediations a diagnostic Issue
// can suggest. The TUI's Health → Issues view dispatches on Kind and
// executes the appropriate flow (copy command, jump tab, rescan, …).
// CLI consumers can ignore Action entirely — the legacy `Fix` text
// remains the human-readable summary.
type ActionKind string

// Action kinds. Keep these stable; they're emitted in JSON.
const (
	ActionNone ActionKind = ""
	// ActionCopyCommand copies Action.Command verbatim to the user's
	// clipboard. Used for PATH-cleanup snippets, "install this PM"
	// commands, etc. The TUI shows the literal command in the
	// confirmation hint so the user knows what they're copying.
	ActionCopyCommand ActionKind = "copy_command"
	// ActionRescan triggers the standard klim rescan (the same
	// effect as pressing `r`). Used for stale-cache and
	// unresolved-version issues.
	ActionRescan ActionKind = "rescan"
	// ActionJumpPathView switches to the Health → PATH sub-tab and
	// focuses the tool named in Target. Used for PATH-shadowing and
	// multi-install-conflict issues so the user lands directly on
	// the relevant row.
	ActionJumpPathView ActionKind = "jump_path"
	// ActionJumpUpdates switches to My Tools → Updates.
	ActionJumpUpdates ActionKind = "jump_updates"
)

// Action is the structured remediation attached to a doctor Issue.
type Action struct {
	Kind    ActionKind `json:"kind,omitempty"`
	Label   string     `json:"label,omitempty"`
	Command string     `json:"command,omitempty"`
	// Target carries kind-specific context: a tool name for
	// ActionJumpPathView, a PATH entry for ActionCopyCommand on a
	// PATH issue, etc.
	Target string `json:"target,omitempty"`
	// TouchesPATH is true when running this action will modify the
	// user's $PATH (or the persistent Windows User PATH). The TUI
	// uses it to snapshot the current PATH to ~/.klim/backups/path/
	// just before exec so the user can roll back from inside the
	// fix modal.
	TouchesPATH bool `json:"touches_path,omitempty"`
}

// removePathEntryCommand returns a shell snippet that removes a single
// directory from $PATH for the current shell session. Persisting the
// change is intentionally left to the user — we don't know which
// startup file owns their PATH and silently editing ~/.bashrc /
// ~/.zshrc / the Windows registry would be a footgun.
//
// On Windows the snippet additionally calls [Environment]::
// SetEnvironmentVariable so the change survives across sessions —
// that's safe because the persistent User PATH is well-defined and
// owned by the current user.
func removePathEntryCommand(entry string) string {
	if entry == "" {
		return ""
	}
	if runtime.GOOS == "windows" {
		// PowerShell. Use single quotes to avoid interpolation
		// surprises in paths that contain $ or `.
		esc := strings.ReplaceAll(entry, "'", "''")
		return fmt.Sprintf(`$new = ($env:PATH -split ';' | Where-Object { $_ -ne '%s' }) -join ';'; $env:PATH = $new; [Environment]::SetEnvironmentVariable('PATH', $new, 'User')`, esc)
	}
	// POSIX shells. awk + RS=: keeps ordering stable and tolerates
	// duplicates (every matching entry is dropped).
	esc := strings.ReplaceAll(entry, `"`, `\"`)
	return fmt.Sprintf(`export PATH="$(printf '%%s' "$PATH" | awk -v RS=: -v ORS=: '$0 != "%s"' | sed 's/:$//')"`, esc)
}

// reorderPathCommand returns a shell snippet that re-emits PATH with
// every system directory placed ahead of every user-writable dir.
// Best-effort and intentionally conservative — system entries are
// detected via the same list used by checkUserWritablePathOrder so
// the result is consistent with the warning.
func reorderPathCommand() string {
	if runtime.GOOS == "windows" {
		return `# Reorder Windows User PATH so system dirs precede user-writable dirs.
$systemDirs = @('C:\Windows', 'C:\Windows\System32', 'C:\Windows\SysWow64', 'C:\Program Files', 'C:\Program Files (x86)')
$entries = $env:PATH -split ';' | Where-Object { $_ -ne '' }
$systemFirst = @($entries | Where-Object { $d = $_; ($systemDirs | Where-Object { $d.ToLower().StartsWith($_.ToLower()) }).Count -gt 0 })
$userLast    = @($entries | Where-Object { $d = $_; ($systemDirs | Where-Object { $d.ToLower().StartsWith($_.ToLower()) }).Count -eq 0 })
$new = ($systemFirst + $userLast) -join ';'
$env:PATH = $new
[Environment]::SetEnvironmentVariable('PATH', $new, 'User')`
	}
	return `# Reorder PATH so system bin dirs precede user-writable ones for this shell session.
# Persist by appending the resulting "export PATH=…" line to ~/.bashrc / ~/.zshrc.
sys="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/opt/homebrew/bin:/opt/homebrew/sbin"
keep=""; rest=""
IFS=':'
for d in $PATH; do
  case ":$sys:" in
    *":$d:"*) keep="$keep:$d" ;;
    *)        rest="$rest:$d" ;;
  esac
done
unset IFS
export PATH="${keep#:}:${rest#:}"
echo "$PATH"`
}

// installPMCommand returns a copy-pasteable command to install the
// given package manager, or "" when there's no sensible automated
// install (winget on non-Windows, apt on macOS, etc.).
func installPMCommand(pm registry.InstallSource) string {	switch pm {
	case registry.SourceBrew:
		if runtime.GOOS == "windows" {
			return ""
		}
		return `/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`
	case registry.SourceScoop:
		if runtime.GOOS != "windows" {
			return ""
		}
		return `Set-ExecutionPolicy RemoteSigned -Scope CurrentUser; irm get.scoop.sh | iex`
	case registry.SourceChoco:
		if runtime.GOOS != "windows" {
			return ""
		}
		return `Set-ExecutionPolicy Bypass -Scope Process -Force; iex ((New-Object System.Net.WebClient).DownloadString('https://chocolatey.org/install.ps1'))`
	case registry.SourceNPM:
		switch runtime.GOOS {
		case "darwin":
			return "brew install node"
		case "windows":
			return "winget install OpenJS.NodeJS.LTS"
		default:
			return "sudo apt install -y nodejs npm"
		}
	case registry.SourceWinget:
		return "" // ships with Windows; missing winget means an older Windows version
	case registry.SourceApt, registry.SourceSnap:
		return "" // distro-specific install; no portable one-liner
	}
	return ""
}

// extractPATHEntry is unused — Action.Target carries the entry
// directly. Kept here as a private helper note for future consumers
// that consume serialized Issues (e.g. a web UI) and only have the
// rendered detail text. Defined as a no-op to document intent
// without paying the unused-symbol cost.
var _ = filepath.Clean // keep filepath imported for potential future helpers
