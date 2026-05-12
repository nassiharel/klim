// Package pathbackup captures the value of $PATH (and the persistent
// User PATH on Windows) before klim applies any PATH-modifying fix,
// and produces shell-specific restore commands so the user can roll
// back without leaving the TUI.
//
// Backups land at ~/.klim/backups/path/path-YYYYMMDD-HHMMSS.yaml
// (UTC timestamp, no timezone offset). We keep the format human-
// readable on purpose — a user who wants to inspect or restore
// manually can cat the file and copy the `path:` value.
package pathbackup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/nassiharel/klim/internal/fileutil"
	"github.com/nassiharel/klim/internal/paths"
)

// Backup is one captured snapshot of PATH plus the context that
// triggered the capture.
type Backup struct {
	Timestamp time.Time `yaml:"timestamp"`
	Trigger   string    `yaml:"trigger"`             // e.g. "doctor.fix"
	Issue     string    `yaml:"issue,omitempty"`     // human title of the issue that prompted the backup
	GOOS      string    `yaml:"goos"`                // runtime.GOOS at capture time — restore commands depend on this
	PATH      string    `yaml:"path"`                // the contents of $PATH (or %PATH%) at capture time
	UserPATH  string    `yaml:"user_path,omitempty"` // Windows-only: persistent User PATH registry value
	Command   string    `yaml:"command,omitempty"`   // the command that was about to be run (for audit)
	// File is filled in by Save (and List) — it's the absolute path
	// of the YAML file on disk. Not serialized.
	File string `yaml:"-"`
}

// Capture snapshots the current PATH for the given trigger. On
// Windows it also reads the persistent User PATH from the registry
// (best-effort — falls back to empty string when reading fails).
func Capture(trigger, issue, command string) Backup {
	b := Backup{
		Timestamp: time.Now().UTC(),
		Trigger:   trigger,
		Issue:     issue,
		GOOS:      runtime.GOOS,
		PATH:      os.Getenv("PATH"),
		Command:   command,
	}
	if runtime.GOOS == "windows" {
		b.UserPATH = readUserPATH()
	}
	return b
}

// Save writes the backup to ~/.klim/backups/path/ and returns the
// resulting absolute path. The file is named after the timestamp;
// collisions (same second) are resolved by appending -2, -3, etc.
func Save(b Backup) (string, error) {
	dir, err := paths.PathBackupsDir()
	if err != nil {
		return "", err
	}
	stamp := b.Timestamp.UTC().Format("20060102-150405")
	path := filepath.Join(dir, "path-"+stamp+".yaml")
	for i := 2; fileExists(path); i++ {
		path = filepath.Join(dir, fmt.Sprintf("path-%s-%d.yaml", stamp, i))
	}
	header := "# klim PATH backup — created before applying a Health fix.\n" +
		"# Show:   klim health path-backups show " + strings.TrimSuffix(filepath.Base(path), ".yaml") + "\n" +
		"# Restore: klim health path-backups restore-cmd " + strings.TrimSuffix(filepath.Base(path), ".yaml") + "\n"
	if err := fileutil.WriteYAML(path, &b, header); err != nil {
		return "", err
	}
	return path, nil
}

// List returns every backup in the backups dir, sorted newest first.
// Missing dir is not an error — we just return an empty slice so the
// caller can render a clean "no backups yet" state.
func List() ([]Backup, error) {
	dir, err := paths.PathBackupsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Backup
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		var b Backup
		full := filepath.Join(dir, e.Name())
		if _, err := fileutil.ReadYAML(full, &b); err != nil {
			continue
		}
		b.File = full
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp.After(out[j].Timestamp) })
	return out, nil
}

// RestoreCommand returns a shell-specific command that, when executed
// in the user's interactive shell, sets PATH back to the captured
// value. On Windows the User PATH registry entry is also restored
// when it was captured.
//
// The function is platform-agnostic in that it inspects b.GOOS rather
// than runtime.GOOS — so a backup captured on Windows still produces
// the right command if someone reviews it on Linux. The command is
// what gets *displayed* and *executed*; the user always sees what's
// about to run before it does.
//
// Quoting strategy:
//
//   - Windows: PowerShell single-quoted strings; embedded single
//     quotes are doubled (PowerShell's literal-string escape rule).
//   - POSIX:   Bourne single-quoted strings; embedded single quotes
//     use the canonical close-escape-reopen sequence. An embedded
//     single quote becomes four characters in sequence: end-quote,
//     backslash, single-quote, re-open-quote. See shSingleQuote()
//     for the implementation. This is the only fully-safe approach
//     because POSIX shells
//     perform parameter and command expansion inside double quotes,
//     and a PATH value containing `$(...)` or backticks would
//     otherwise be interpreted as a command substitution.
func RestoreCommand(b Backup) string {
	switch b.GOOS {
	case "windows":
		cmd := fmt.Sprintf(`$env:PATH = '%s'`, psSingleQuote(b.PATH))
		if b.UserPATH != "" {
			cmd += fmt.Sprintf(`; [Environment]::SetEnvironmentVariable('PATH', '%s', 'User')`, psSingleQuote(b.UserPATH))
		}
		return cmd
	default:
		return `export PATH=` + shSingleQuote(b.PATH)
	}
}

// shSingleQuote wraps s in Bourne-shell single quotes. Embedded
// single quotes use the canonical close-escape-reopen sequence
// (an inner single quote becomes the four characters
// end-quote / backslash / single-quote / re-open-quote). The result
// is safe to paste into bash / zsh / sh without any interpretation
// of $, backticks, or other shell metacharacters.
func shSingleQuote(s string) string {
	return `'` + strings.ReplaceAll(s, `'`, `'\''`) + `'`
}

// psSingleQuote wraps s in PowerShell single quotes, escaping
// embedded single quotes by doubling them — PowerShell's literal
// string rule.
func psSingleQuote(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// readUserPATH returns the Windows User PATH from the registry, or ""
// on any failure. Implemented via PowerShell so we don't pull a
// platform-specific dependency (golang.org/x/sys/windows/registry)
// into the build for one read; the cost of spawning a shell is fine
// for a single capture per fix.
//
// Defined on all platforms but only called when runtime.GOOS ==
// "windows"; the empty-string fallback is the right answer
// everywhere else.
func readUserPATH() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	// Cap PowerShell at five seconds — a hung profile or
	// execution-policy prompt must not block a PATH backup
	// capture (which gates every Health-fix action).
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := runCmd(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command",
		`[Environment]::GetEnvironmentVariable('PATH', 'User')`)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}
