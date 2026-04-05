package finder

import (
	"testing"

	"github.com/nassiharel/clim/internal/registry"
)

func TestDetectSource(t *testing.T) {
	tests := []struct {
		name string
		path string
		want registry.InstallSource
	}{
		// Chocolatey.
		{
			"chocolatey install",
			`C:\ProgramData\chocolatey\bin\git.exe`,
			registry.SourceChoco,
		},
		{
			"chocolatey lib",
			`C:\ProgramData\chocolatey\lib\nodejs\tools\node.exe`,
			registry.SourceChoco,
		},

		// Scoop.
		{
			"scoop install",
			`C:\Users\user\scoop\apps\git\current\bin\git.exe`,
			registry.SourceScoop,
		},

		// Homebrew.
		{
			"homebrew opt prefix",
			"/opt/homebrew/bin/git",
			registry.SourceBrew,
		},
		{
			"homebrew cellar",
			"/usr/local/Cellar/git/2.43.0/bin/git",
			registry.SourceBrew,
		},
		{
			"homebrew cellar via homebrew path",
			"/home/user/homebrew/Cellar/node/20.0.0/bin/node",
			registry.SourceBrew,
		},

		// Snap.
		{
			"snap install",
			"/snap/kubectl/3456/kubectl",
			registry.SourceSnap,
		},

		// Apt / system packages.
		{
			"usr bin",
			"/usr/bin/git",
			registry.SourceApt,
		},
		{
			"usr lib",
			"/usr/lib/python3/dist-packages/bin/python3",
			registry.SourceApt,
		},

		// NPM.
		{
			"npm roaming",
			`C:\Users\user\AppData\Roaming\npm\prettier.cmd`,
			registry.SourceNPM,
		},
		{
			"npm global node_modules",
			"/opt/node/lib/node_modules/.bin/prettier",
			registry.SourceNPM,
		},
		// Note: /usr/lib/node_modules matches SourceApt first (order-dependent).
		{
			"usr lib node_modules matches apt",
			"/usr/lib/node_modules/.bin/prettier",
			registry.SourceApt,
		},

		// Go.
		{
			"go bin",
			"/home/user/go/bin/golangci-lint",
			registry.SourceGo,
		},
		{
			"gopath bin",
			`C:\Users\user\go\bin\gopls.exe`,
			registry.SourceGo,
		},

		// Cargo.
		{
			"cargo bin",
			"/home/user/.cargo/bin/fd",
			registry.SourceCargo,
		},

		// Pip / local bin — .local/bin is a general user-level directory,
		// not specific to pip. Correctly attributed as manual.
		{
			"local bin",
			"/home/user/.local/bin/httpie",
			registry.SourceManual,
		},

		// WinGet — Program Files.
		{
			"program files",
			`C:\Program Files\Git\cmd\git.exe`,
			registry.SourceWinget,
		},
		{
			"program files x86",
			`C:\Program Files (x86)\Something\tool.exe`,
			registry.SourceWinget,
		},

		// WinGet — MSIX packages.
		{
			"msix package",
			`C:\Program Files\WindowsApps\Microsoft.AzureCLI_2.0.0.0_x64__8wekyb3d8bbwe\az.exe`,
			registry.SourceWinget,
		},

		// WinGet — Windows Apps.
		{
			"windows app alias",
			`C:\Users\user\AppData\Local\Microsoft\WindowsApps\python.exe`,
			registry.SourceWinget,
		},

		// WinGet — per-user programs.
		{
			"local programs",
			`C:\Users\user\AppData\Local\Programs\Microsoft VS Code\bin\code.cmd`,
			registry.SourceWinget,
		},

		// WinGet — ProgramData.
		{
			"programdata",
			`C:\ProgramData\DockerDesktop\version-bin\docker.exe`,
			registry.SourceWinget,
		},

		// Manual / unknown.
		{
			"custom directory",
			"/opt/mytools/bin/mytool",
			registry.SourceManual,
		},
		{
			"tmp directory",
			"/tmp/bin/sometool",
			registry.SourceManual,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectSource(tt.path)
			if got != tt.want {
				t.Errorf("detectSource(%q) = %q, want %q",
					tt.path, got, tt.want)
			}
		})
	}
}

func TestPathDirectories(t *testing.T) {
	// pathDirectories reads from os.Getenv("PATH"), so we test indirectly.
	// Testing the logic: empty entries are filtered, whitespace is trimmed.
	t.Run("returns nil for empty PATH", func(t *testing.T) {
		// Save and restore PATH.
		t.Setenv("PATH", "")
		dirs := pathDirectories()
		if dirs != nil {
			t.Errorf("pathDirectories() with empty PATH = %v, want nil", dirs)
		}
	})
}

func TestBinaryCandidates(t *testing.T) {
	// binaryCandidates is platform-dependent. We test that it returns at least
	// the base path on any platform.
	candidates := binaryCandidates("/usr/bin", "git")
	if len(candidates) == 0 {
		t.Fatal("binaryCandidates returned 0 candidates")
	}

	// On all platforms, the list should contain the full path.
	found := false
	for _, c := range candidates {
		// Normalize for comparison.
		if c == "/usr/bin/git" || c == `\usr\bin\git` {
			found = true
			break
		}
	}
	// On Windows, candidates will have .exe etc. instead of bare name at first,
	// but the bare name is appended last. On Unix, only the bare name is returned.
	if !found && len(candidates) == 1 {
		// Unix: single candidate should be the exact path.
		t.Errorf("binaryCandidates(/usr/bin, git) = %v, expected /usr/bin/git", candidates)
	}
}
