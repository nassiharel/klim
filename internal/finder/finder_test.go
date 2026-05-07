package finder

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/registry"
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

		// Apt / system packages — only detected as apt on Debian-based systems
		// where dpkg is available; otherwise falls back to manual.
		{
			"usr bin",
			"/usr/bin/git",
			usrBinSource(),
		},
		{
			"usr lib",
			"/usr/lib/python3/dist-packages/bin/python3",
			usrBinSource(),
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
		// Note: /usr/lib/node_modules — node_modules check comes before /usr/lib.
		{
			"usr lib node_modules matches npm",
			"/usr/lib/node_modules/.bin/prettier",
			registry.SourceNPM,
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

		// Program Files is predominantly winget territory on modern
		// Windows; classify as SourceWinget so the common case (a
		// winget MSI install) gets upgrade/remove actions. Non-
		// winget binaries that happen to live here surface a
		// friendly hint at remove time instead of a generic error
		// (see internal/tui/action_hints.go).
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

		// AppData\Local\Programs: same reasoning as Program Files —
		// winget per-user MSIs (VS Code, Azure Dev CLI) are the
		// common case; misattributed third-party installers get
		// the friendly hint at remove time.
		{
			"local programs",
			`C:\Users\user\AppData\Local\Programs\Microsoft VS Code\bin\code.cmd`,
			registry.SourceWinget,
		},

		// ProgramData / DockerDesktop / etc.: not a winget signal —
		// the path is shared by Docker, Chocolatey (caught earlier),
		// and various installers. We can't attribute it without more
		// context, so call it manual.
		{
			"programdata docker desktop",
			`C:\ProgramData\DockerDesktop\version-bin\docker.exe`,
			registry.SourceManual,
		},

		// WinGet — explicit packages dir under AppData.
		{
			"winget packages dir",
			`C:\Users\user\AppData\Local\Microsoft\WinGet\Packages\jqlang.jq_Microsoft.Winget.Source_8wekyb3d8bbwe\jq.exe`,
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
		// On Windows, registry PATH may still return directories even when
		// the process PATH is empty.
		if runtime.GOOS != "windows" && dirs != nil {
			t.Errorf("pathDirectories() with empty PATH = %v, want nil", dirs)
		}
	})
}

func TestBinaryCandidateNames(t *testing.T) {
	// binaryCandidateNames is platform-dependent. We test that it returns at least
	// the base name on any platform.
	candidates := binaryCandidateNames("git")
	if len(candidates) == 0 {
		t.Fatal("binaryCandidateNames returned 0 candidates")
	}

	// On all platforms, the list should contain "git" (the bare name).
	found := false
	for _, c := range candidates {
		if c == "git" {
			found = true
			break
		}
	}
	// On Windows, candidates will have .exe etc. plus the bare name.
	// On Unix, only the bare name is returned.
	if !found {
		t.Errorf("binaryCandidateNames(git) = %v, expected to contain 'git'", candidates)
	}
}

func TestNormaliseName(t *testing.T) {
	got := normaliseName("Git")
	if runtime.GOOS == "windows" {
		if got != "git" {
			t.Errorf("normaliseName(Git) on Windows = %q, want git", got)
		}
	} else {
		if got != "Git" {
			t.Errorf("normaliseName(Git) on Unix = %q, want Git (case-sensitive)", got)
		}
	}

	if got := normaliseName(""); got != "" {
		t.Errorf("normaliseName('') = %q, want empty", got)
	}
}

func TestDetectSource_WinGetPackagePaths(t *testing.T) {
	tests := []struct {
		name string
		path string
		want registry.InstallSource
	}{
		{
			"winget packages with 8wekyb3d8bbwe",
			`C:\Users\user\AppData\Local\Microsoft\WinGet\Packages\junegunn.fzf_Microsoft.Winget.Source_8wekyb3d8bbwe\fzf.exe`,
			registry.SourceWinget,
		},
		{
			"windows apps python",
			`C:\Users\user\AppData\Local\Microsoft\WindowsApps\python3.exe`,
			registry.SourceWinget,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectSource(tt.path)
			if got != tt.want {
				t.Errorf("detectSource(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestDetectSource_PipAndCargo(t *testing.T) {
	tests := []struct {
		name string
		path string
		want registry.InstallSource
	}{
		{"pip in local bin", "/home/user/.local/bin/httpie", registry.SourceManual},
		{"cargo", "/home/user/.cargo/bin/fd", registry.SourceCargo},
		{"go bin", "/home/user/go/bin/golangci-lint", registry.SourceGo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectSource(tt.path)
			if got != tt.want {
				t.Errorf("detectSource(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// usrBinSource returns SourceApt on Debian-based systems (where dpkg exists)
// and SourceManual everywhere else, matching detectSource's runtime behavior.
func usrBinSource() registry.InstallSource {
	if _, err := exec.LookPath("dpkg"); err == nil {
		return registry.SourceApt
	}
	return registry.SourceManual
}

// TestScanExtraInstallRoots covers the Phase 5 fallback that catches
// GUI apps installed by winget under %LOCALAPPDATA%\Programs (e.g.
// Freelens) which don't expose a binary on PATH and were previously
// reported as "not installed" even when `winget list` showed them.
func TestScanExtraInstallRoots(t *testing.T) {
	root := t.TempDir()

	// Lay out: <root>/Freelens/Freelens.exe and <root>/Other/whatever.exe.
	freelensDir := filepath.Join(root, "Freelens")
	if err := os.Mkdir(freelensDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	binName := "freelens" + exeSuffix()
	freelensBin := filepath.Join(freelensDir, binName)
	if err := os.WriteFile(freelensBin, []byte("\x00"), 0o755); err != nil {
		t.Fatalf("write bin: %v", err)
	}

	otherDir := filepath.Join(root, "Other")
	if err := os.Mkdir(otherDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(otherDir, "unrelated"+exeSuffix()), []byte{}, 0o755); err != nil {
		t.Fatalf("write unrelated: %v", err)
	}

	tools := []registry.Tool{
		{Name: "freelens", BinaryNames: []string{"freelens"}},
		{Name: "kubectl", BinaryNames: []string{"kubectl"}}, // not present, must stay empty
		{
			// Already-installed tool must not be touched.
			Name:        "git",
			BinaryNames: []string{"git"},
			Instances:   []registry.Instance{{Path: "/usr/bin/git", Source: registry.SourceManual}},
		},
	}

	scanExtraInstallRootsAt(tools, []string{root})

	if len(tools[0].Instances) != 1 {
		t.Fatalf("freelens expected 1 instance, got %d", len(tools[0].Instances))
	}
	gotPath := tools[0].Instances[0].Path
	resolvedWant, _ := filepath.EvalSymlinks(freelensBin)
	if gotPath != resolvedWant && gotPath != freelensBin {
		t.Errorf("freelens path = %q, want %q (or pre-resolved %q)", gotPath, resolvedWant, freelensBin)
	}
	if len(tools[1].Instances) != 0 {
		t.Errorf("kubectl should not have been found, got %v", tools[1].Instances)
	}
	if len(tools[2].Instances) != 1 || tools[2].Instances[0].Path != "/usr/bin/git" {
		t.Errorf("git instance was clobbered, got %+v", tools[2].Instances)
	}
}

func TestScanExtraInstallRoots_NoRoots(t *testing.T) {
	// Must be a no-op (and not panic) when no roots are configured —
	// matches the non-Windows case where extraInstallRoots() returns nil.
	tools := []registry.Tool{{Name: "freelens", BinaryNames: []string{"freelens"}}}
	scanExtraInstallRootsAt(tools, nil)
	if len(tools[0].Instances) != 0 {
		t.Errorf("expected no instances, got %v", tools[0].Instances)
	}
}

// TestScanExtraInstallRoots_BinaryNameOrder verifies that when a tool
// declares multiple BinaryNames (e.g. python/python3), Phase 5 picks
// the binary whose name appears first in BinaryNames — matching Phase 4's
// LookPath fallback semantics. Without explicit ordering the previous
// map-based implementation could return either match nondeterministically.
func TestScanExtraInstallRoots_BinaryNameOrder(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "Python")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Both binaries exist in the same subdir; BinaryNames lists "python"
	// first so that's the one that should be selected.
	for _, name := range []string{"python", "python3"} {
		if err := os.WriteFile(filepath.Join(dir, name+exeSuffix()), []byte{}, 0o755); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	// Run several times — map iteration order varies between runs in Go,
	// so a non-deterministic implementation would eventually flip.
	for i := 0; i < 25; i++ {
		tools := []registry.Tool{{Name: "py", BinaryNames: []string{"python", "python3"}}}
		scanExtraInstallRootsAt(tools, []string{root})
		if len(tools[0].Instances) != 1 {
			t.Fatalf("iter %d: expected 1 instance, got %d", i, len(tools[0].Instances))
		}
		got := filepath.Base(tools[0].Instances[0].Path)
		want := "python" + exeSuffix()
		if !strings.EqualFold(got, want) {
			t.Fatalf("iter %d: matched %q, expected %q (BinaryNames order)", i, got, want)
		}
	}
}

// TestScanExtraInstallRoots_StopsWhenAllResolved makes sure the scan
// short-circuits and doesn't keep walking subdirs once every pending tool
// has been resolved (Copilot review feedback on PR #71).
func TestScanExtraInstallRoots_StopsWhenAllResolved(t *testing.T) {
	root := t.TempDir()
	freelensDir := filepath.Join(root, "Freelens")
	if err := os.Mkdir(freelensDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(freelensDir, "freelens"+exeSuffix()), []byte{}, 0o755); err != nil {
		t.Fatalf("write bin: %v", err)
	}
	// A second copy in a parallel subdir — must NOT be picked up because
	// the tool already has an instance from the first match.
	otherDir := filepath.Join(root, "OtherFreelens")
	if err := os.Mkdir(otherDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(otherDir, "freelens"+exeSuffix()), []byte{}, 0o755); err != nil {
		t.Fatalf("write second: %v", err)
	}

	tools := []registry.Tool{{Name: "freelens", BinaryNames: []string{"freelens"}}}
	scanExtraInstallRootsAt(tools, []string{root})
	if len(tools[0].Instances) != 1 {
		t.Fatalf("expected exactly 1 instance, got %d: %+v", len(tools[0].Instances), tools[0].Instances)
	}
}

func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}
