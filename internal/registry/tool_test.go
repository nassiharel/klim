package registry

import (
	"runtime"
	"testing"
)

func TestInstallCmd(t *testing.T) {
	pkgs := PackageIDs{
		Winget: "Git.Git",
		Choco:  "git",
		Scoop:  "git",
		Brew:   "git",
		Apt:    "git",
		Snap:   "git",
		NPM:    "git",
	}

	tests := []struct {
		source InstallSource
		want   string
	}{
		{SourceWinget, "winget install --id Git.Git"},
		{SourceChoco, "choco install git"},
		{SourceScoop, "scoop install git"},
		{SourceBrew, "brew install git"},
		{SourceApt, "sudo apt install git"},
		{SourceSnap, "sudo snap install git"},
		{SourceNPM, "npm install -g git"},
		{SourceGo, ""},
		{SourceCargo, ""},
		{SourcePip, ""},
		{SourceManual, ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.source), func(t *testing.T) {
			got := pkgs.InstallCmd(tt.source)
			if got != tt.want {
				t.Errorf("InstallCmd(%s) = %q, want %q", tt.source, got, tt.want)
			}
		})
	}
}

func TestUpgradeCmd(t *testing.T) {
	pkgs := PackageIDs{
		Winget: "Git.Git",
		Choco:  "git",
		Scoop:  "git",
		Brew:   "git",
		Apt:    "git",
		Snap:   "git",
		NPM:    "git",
	}

	tests := []struct {
		source InstallSource
		want   string
	}{
		{SourceWinget, "winget upgrade --id Git.Git"},
		{SourceChoco, "choco upgrade git"},
		{SourceScoop, "scoop update git"},
		{SourceBrew, "brew upgrade git"},
		{SourceApt, "sudo apt upgrade git"},
		{SourceSnap, "sudo snap refresh git"},
		{SourceNPM, "npm update -g git"},
		{SourceGo, ""},
		{SourceManual, ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.source), func(t *testing.T) {
			got := pkgs.UpgradeCmd(tt.source)
			if got != tt.want {
				t.Errorf("UpgradeCmd(%s) = %q, want %q", tt.source, got, tt.want)
			}
		})
	}
}

func TestRemoveCmd(t *testing.T) {
	pkgs := PackageIDs{
		Winget: "Git.Git",
		Choco:  "git",
		Scoop:  "git",
		Brew:   "git",
		Apt:    "git",
		Snap:   "git",
		NPM:    "git",
	}

	tests := []struct {
		source InstallSource
		want   string
	}{
		{SourceWinget, "winget uninstall --id Git.Git"},
		{SourceChoco, "choco uninstall git"},
		{SourceScoop, "scoop uninstall git"},
		{SourceBrew, "brew uninstall git"},
		{SourceApt, "sudo apt remove git"},
		{SourceSnap, "sudo snap remove git"},
		{SourceNPM, "npm uninstall -g git"},
		{SourceGo, ""},
		{SourceManual, ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.source), func(t *testing.T) {
			got := pkgs.RemoveCmd(tt.source)
			if got != tt.want {
				t.Errorf("RemoveCmd(%s) = %q, want %q", tt.source, got, tt.want)
			}
		})
	}
}

func TestCommandsWithEmptyPackageIDs(t *testing.T) {
	empty := PackageIDs{}
	sources := []InstallSource{
		SourceWinget, SourceChoco, SourceScoop, SourceBrew,
		SourceApt, SourceSnap, SourceNPM,
	}

	for _, source := range sources {
		if got := empty.InstallCmd(source); got != "" {
			t.Errorf("empty.InstallCmd(%s) = %q, want empty", source, got)
		}
		if got := empty.UpgradeCmd(source); got != "" {
			t.Errorf("empty.UpgradeCmd(%s) = %q, want empty", source, got)
		}
		if got := empty.RemoveCmd(source); got != "" {
			t.Errorf("empty.RemoveCmd(%s) = %q, want empty", source, got)
		}
	}
}

func TestInstallArgs_StructuredArgs(t *testing.T) {
	pkgs := PackageIDs{Winget: "Git.Git", Brew: "git"}

	// Winget: binary + flags + package ID as separate args.
	args := pkgs.InstallArgs(SourceWinget)
	if len(args) != 4 || args[0] != "winget" || args[3] != "Git.Git" {
		t.Errorf("InstallArgs(winget) = %v, want [winget install --id Git.Git]", args)
	}

	// Brew: binary + subcommand + package ID.
	args = pkgs.InstallArgs(SourceBrew)
	if len(args) != 3 || args[0] != "brew" || args[2] != "git" {
		t.Errorf("InstallArgs(brew) = %v, want [brew install git]", args)
	}

	// Unsupported source returns nil.
	if args := pkgs.InstallArgs(SourceCargo); args != nil {
		t.Errorf("InstallArgs(cargo) = %v, want nil", args)
	}

	// Empty ID returns nil.
	empty := PackageIDs{}
	if args := empty.InstallArgs(SourceWinget); args != nil {
		t.Errorf("empty.InstallArgs(winget) = %v, want nil", args)
	}
}

func TestInstallArgs_ShellMetacharsStaySingleArg(t *testing.T) {
	// A malicious package ID with shell metacharacters must remain as one
	// argument — exec.Command passes it directly to the process, not a shell.
	pkgs := PackageIDs{Brew: "legit; rm -rf /"}
	args := pkgs.InstallArgs(SourceBrew)
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d: %v", len(args), args)
	}
	// The malicious string is a single element, not split by the shell.
	if args[2] != "legit; rm -rf /" {
		t.Errorf("package ID was modified: got %q", args[2])
	}
}

func TestTruncatePath(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		maxLen int
		want   string
	}{
		{"empty", "", 40, "—"},
		{"short path", "/usr/bin/git", 40, "/usr/bin/git"},
		{"exact fit", "/usr/bin/git", 12, "/usr/bin/git"},
		{"truncated", "/home/user/.local/bin/tool", 15, "...cal/bin/tool"},
		{"maxLen zero", "/usr/bin/git", 0, "/usr/bin/git"},
		{"maxLen negative", "/usr/bin/git", -5, "/usr/bin/git"},
		{"maxLen 1", "/usr/bin/git", 1, "/usr/bin/git"},
		{"maxLen 3", "/usr/bin/git", 3, "/usr/bin/git"},
		{"maxLen 4", "/usr/bin/git", 4, "...t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncatePath(tt.path, tt.maxLen)
			if got != tt.want {
				t.Errorf("TruncatePath(%q, %d) = %q, want %q",
					tt.path, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestStatusString(t *testing.T) {
	tests := []struct {
		name      string
		installed string
		latest    string
		want      string
	}{
		{"up to date", "1.2.3", "1.2.3", "✓ up to date"},
		{"update available", "1.2.3", "1.2.4", "⬆ update"},
		{"no installed", "", "1.2.3", "?"},
		{"no latest", "1.2.3", "", ""},
		{"both empty", "", "", ""},
		{"installed newer than latest", "10.0.426", "8.0.419", "✓ up to date"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StatusString(tt.installed, tt.latest)
			if got != tt.want {
				t.Errorf("StatusString(%q, %q) = %q, want %q",
					tt.installed, tt.latest, got, tt.want)
			}
		})
	}
}

func TestPrimaryInstance(t *testing.T) {
	t.Run("no instances", func(t *testing.T) {
		tool := Tool{}
		if got := tool.PrimaryInstance(); got != nil {
			t.Errorf("PrimaryInstance() = %v, want nil", got)
		}
	})

	t.Run("with instances", func(t *testing.T) {
		tool := Tool{
			Instances: []Instance{
				{Path: "/usr/bin/git", Version: "2.43.0", Source: SourceApt},
				{Path: "/usr/local/bin/git", Version: "2.44.0", Source: SourceBrew},
			},
		}
		got := tool.PrimaryInstance()
		if got == nil {
			t.Fatal("PrimaryInstance() = nil, want non-nil")
		}
		if got.Path != "/usr/bin/git" {
			t.Errorf("PrimaryInstance().Path = %q, want /usr/bin/git", got.Path)
		}
	})
}

func TestInstalledVersion(t *testing.T) {
	t.Run("no instances", func(t *testing.T) {
		tool := Tool{}
		if got := tool.InstalledVersion(); got != "" {
			t.Errorf("InstalledVersion() = %q, want empty", got)
		}
	})

	t.Run("with version", func(t *testing.T) {
		tool := Tool{
			Instances: []Instance{{Version: "2.43.0"}},
		}
		if got := tool.InstalledVersion(); got != "2.43.0" {
			t.Errorf("InstalledVersion() = %q, want 2.43.0", got)
		}
	})
}

func TestHasUpdate(t *testing.T) {
	tests := []struct {
		name    string
		version string
		latest  string
		want    bool
	}{
		{"update available", "1.2.3", "1.2.4", true},
		{"up to date", "1.2.3", "1.2.3", false},
		{"no version", "", "1.2.3", false},
		{"no latest", "1.2.3", "", false},
		{"installed newer than latest", "10.0.426", "8.0.419", false},
		{"installed newer preview", "2.0.0", "1.9.5", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := Tool{
				Instances: []Instance{{Version: tt.version}},
				Latest:    tt.latest,
			}
			if got := tool.HasUpdate(); got != tt.want {
				t.Errorf("HasUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBestInstallSource(t *testing.T) {
	// Stub all package managers as available so the test is deterministic
	// regardless of what's installed on the host.
	SetPMAvailableFunc(func(_ InstallSource) bool { return true })
	t.Cleanup(func() { SetPMAvailableFunc(nil) })

	pkgs := PackageIDs{
		Winget: "Git.Git",
		Choco:  "git",
		Brew:   "git",
		Apt:    "git",
		Snap:   "git",
		NPM:    "git",
	}

	got := pkgs.BestInstallSource()

	// Platform-dependent expectation (priority order, all available).
	switch runtime.GOOS {
	case "windows":
		if got != SourceWinget {
			t.Errorf("BestInstallSource() on Windows = %q, want winget", got)
		}
	case "darwin":
		if got != SourceBrew {
			t.Errorf("BestInstallSource() on macOS = %q, want brew", got)
		}
	default:
		if got != SourceApt {
			t.Errorf("BestInstallSource() on Linux = %q, want apt", got)
		}
	}
}

func TestBestInstallSource_NPMFallback(t *testing.T) {
	SetPMAvailableFunc(func(_ InstallSource) bool { return true })
	t.Cleanup(func() { SetPMAvailableFunc(nil) })

	pkgs := PackageIDs{NPM: "prettier"}
	got := pkgs.BestInstallSource()
	if got != SourceNPM {
		t.Errorf("BestInstallSource() with only NPM = %q, want npm", got)
	}
}

func TestBestInstallSource_Empty(t *testing.T) {
	SetPMAvailableFunc(func(_ InstallSource) bool { return true })
	t.Cleanup(func() { SetPMAvailableFunc(nil) })

	pkgs := PackageIDs{}
	got := pkgs.BestInstallSource()
	if got != "" {
		t.Errorf("BestInstallSource() with no packages = %q, want empty", got)
	}
}

func TestBestInstallSource_NoPMInstalled(t *testing.T) {
	// When no package manager is installed, BestInstallSource returns "".
	SetPMAvailableFunc(func(_ InstallSource) bool { return false })
	t.Cleanup(func() { SetPMAvailableFunc(nil) })

	pkgs := PackageIDs{Winget: "Git.Git", Brew: "git", Apt: "git"}
	got := pkgs.BestInstallSource()
	if got != "" {
		t.Errorf("BestInstallSource() with no PMs = %q, want empty", got)
	}
}
