package doctor

import (
	"runtime"

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
	// clipboard. Used for "install this PM" commands and similar
	// copy-pasteable snippets. The TUI shows the literal command in
	// the confirmation hint so the user knows what they're copying.
	ActionCopyCommand ActionKind = "copy_command"
	// ActionRescan triggers the standard klim rescan (the same
	// effect as pressing `r`). Used for stale-cache and
	// unresolved-version issues.
	ActionRescan ActionKind = "rescan"
	// ActionJumpUpdates switches to My Tools → Updates.
	ActionJumpUpdates ActionKind = "jump_updates"
)

// Action is the structured remediation attached to a doctor Issue.
type Action struct {
	Kind    ActionKind `json:"kind,omitempty"`
	Label   string     `json:"label,omitempty"`
	Command string     `json:"command,omitempty"`
	// Target carries kind-specific context, e.g. a tool name or a
	// package-manager source for the relevant action.
	Target string `json:"target,omitempty"`
	// TouchesPATH is true when running this action will modify the
	// user's $PATH (or the persistent Windows User PATH). The TUI
	// uses it to snapshot the current PATH to ~/.klim/backups/path/
	// just before exec so the user can roll back from inside the
	// fix modal.
	TouchesPATH bool `json:"touches_path,omitempty"`
}

// installPMCommand returns a copy-pasteable command to install the
// given package manager, or "" when there's no sensible automated
// install (winget on non-Windows, apt on macOS, etc.).
func installPMCommand(pm registry.InstallSource) string {
	switch pm {
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
