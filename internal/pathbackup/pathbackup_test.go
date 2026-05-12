package pathbackup

import (
	"runtime"
	"strings"
	"testing"
)

func TestCapture_PopulatesTimestampAndPATH(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")
	b := Capture("doctor.fix", "Duplicate PATH entry", "echo hi")
	if b.PATH != "/usr/bin:/bin" {
		t.Errorf("PATH = %q, want %q", b.PATH, "/usr/bin:/bin")
	}
	if b.Trigger != "doctor.fix" {
		t.Errorf("Trigger not preserved")
	}
	if b.GOOS != runtime.GOOS {
		t.Errorf("GOOS = %q, want %q", b.GOOS, runtime.GOOS)
	}
	if b.Timestamp.IsZero() {
		t.Errorf("Timestamp should be set")
	}
}

func TestSaveAndList_RoundTrips(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	t.Setenv("PATH", "/a:/b")

	b := Capture("doctor.fix", "Test", "echo hi")
	file, err := Save(b)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !strings.HasSuffix(file, ".yaml") {
		t.Errorf("expected .yaml file, got %s", file)
	}

	list, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 backup, got %d", len(list))
	}
	if list[0].PATH != "/a:/b" {
		t.Errorf("backup PATH = %q, want %q", list[0].PATH, "/a:/b")
	}
}

func TestRestoreCommand_POSIX(t *testing.T) {
	b := Backup{GOOS: "linux", PATH: `/usr/bin:/bin`}
	got := RestoreCommand(b)
	if got != `export PATH='/usr/bin:/bin'` {
		t.Errorf("got %q", got)
	}
}

func TestRestoreCommand_POSIXResistsShellExpansion(t *testing.T) {
	// PATH containing characters that double quotes would interpret
	// must NOT expand when the command is pasted into a shell. The
	// single-quote wrapping is the only safe choice — confirm the
	// generated command contains the dangerous sequence verbatim.
	b := Backup{GOOS: "linux", PATH: `/tmp:$(rm -rf /):${HOME}`}
	got := RestoreCommand(b)
	if !strings.Contains(got, `$(rm -rf /)`) || !strings.Contains(got, `${HOME}`) {
		t.Errorf("dangerous sequences should appear literally inside single quotes: %q", got)
	}
	// And the outer wrapping must be single quotes, not double.
	if !strings.HasPrefix(got, `export PATH='`) {
		t.Errorf("POSIX restore must single-quote, got %q", got)
	}
}

func TestRestoreCommand_POSIXEscapesSingleQuote(t *testing.T) {
	b := Backup{GOOS: "linux", PATH: `/it's/a/path`}
	got := RestoreCommand(b)
	// Embedded single quote must close the literal, escape with a
	// backslash, then reopen the literal.
	if !strings.Contains(got, `'\''`) {
		t.Errorf("single quote not escaped via close/escape/reopen: %q", got)
	}
}

func TestRestoreCommand_WindowsIncludesUserPATH(t *testing.T) {
	b := Backup{GOOS: "windows", PATH: `C:\bin`, UserPATH: `C:\user\bin`}
	got := RestoreCommand(b)
	if !strings.Contains(got, `$env:PATH = 'C:\bin'`) {
		t.Errorf("missing session PATH: %q", got)
	}
	if !strings.Contains(got, `SetEnvironmentVariable('PATH', 'C:\user\bin', 'User')`) {
		t.Errorf("missing User PATH restore: %q", got)
	}
}

func TestRestoreCommand_WindowsEscapesQuotes(t *testing.T) {
	b := Backup{GOOS: "windows", PATH: `C:\it's\bin`}
	got := RestoreCommand(b)
	if !strings.Contains(got, `C:\it''s\bin`) {
		t.Errorf("single quote not doubled for PowerShell: %q", got)
	}
}
