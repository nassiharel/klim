package registry

import "runtime"

// InstallSource identifies how a tool was installed.
type InstallSource string

const (
	SourceWinget InstallSource = "winget"
	SourceChoco  InstallSource = "choco"
	SourceScoop  InstallSource = "scoop"
	SourceBrew   InstallSource = "brew"
	SourceApt    InstallSource = "apt"
	SourceSnap   InstallSource = "snap"
	SourceNPM    InstallSource = "npm"
	SourceGo     InstallSource = "go"
	SourceCargo  InstallSource = "cargo"
	SourcePip    InstallSource = "pip"
	SourceManual InstallSource = "manual"
)

// Tool represents a curated developer tool that clim tracks.
type Tool struct {
	Name        string
	DisplayName string
	Category    string
	BinaryNames []string
	Packages    PackageIDs
	Instances   []Instance
	Latest      string
	LatestFrom  string
	Disabled    bool
}

// Instance represents a single installation of a tool found on PATH.
type Instance struct {
	Path    string
	Version string
	Source  InstallSource
}

// PackageIDs maps package manager names to their package identifiers.
type PackageIDs struct {
	Winget string
	Choco  string
	Brew   string
	Apt    string
	Snap   string
	NPM    string
}

// PrimaryInstance returns the first (PATH-precedence) instance, or nil.
func (t *Tool) PrimaryInstance() *Instance {
	if len(t.Instances) > 0 {
		return &t.Instances[0]
	}
	return nil
}

// InstalledVersion returns the version of the primary instance.
func (t *Tool) InstalledVersion() string {
	if inst := t.PrimaryInstance(); inst != nil {
		return inst.Version
	}
	return ""
}

// IsInstalled returns true if at least one instance was found.
func (t *Tool) IsInstalled() bool {
	return len(t.Instances) > 0
}

// HasUpdate returns true if a newer version is available.
func (t *Tool) HasUpdate() bool {
	ver := t.InstalledVersion()
	return ver != "" && t.Latest != "" && !VersionsMatch(ver, t.Latest)
}

// InstallCmd returns the shell command to install this tool using the given source.
func (p PackageIDs) InstallCmd(source InstallSource) string {
	switch source {
	case SourceWinget:
		if p.Winget != "" {
			return "winget install --id " + p.Winget
		}
	case SourceChoco:
		if p.Choco != "" {
			return "choco install " + p.Choco
		}
	case SourceBrew:
		if p.Brew != "" {
			return "brew install " + p.Brew
		}
	case SourceApt:
		if p.Apt != "" {
			return "sudo apt install " + p.Apt
		}
	case SourceSnap:
		if p.Snap != "" {
			return "sudo snap install " + p.Snap
		}
	case SourceNPM:
		if p.NPM != "" {
			return "npm install -g " + p.NPM
		}
	}
	return ""
}

// UpgradeCmd returns the shell command to upgrade this tool using the given source.
func (p PackageIDs) UpgradeCmd(source InstallSource) string {
	switch source {
	case SourceWinget:
		if p.Winget != "" {
			return "winget upgrade --id " + p.Winget
		}
	case SourceChoco:
		if p.Choco != "" {
			return "choco upgrade " + p.Choco
		}
	case SourceBrew:
		if p.Brew != "" {
			return "brew upgrade " + p.Brew
		}
	case SourceApt:
		if p.Apt != "" {
			return "sudo apt upgrade " + p.Apt
		}
	case SourceSnap:
		if p.Snap != "" {
			return "sudo snap refresh " + p.Snap
		}
	case SourceNPM:
		if p.NPM != "" {
			return "npm update -g " + p.NPM
		}
	}
	return ""
}

// RemoveCmd returns the shell command to remove/uninstall this tool.
func (p PackageIDs) RemoveCmd(source InstallSource) string {
	switch source {
	case SourceWinget:
		if p.Winget != "" {
			return "winget uninstall --id " + p.Winget
		}
	case SourceChoco:
		if p.Choco != "" {
			return "choco uninstall " + p.Choco
		}
	case SourceBrew:
		if p.Brew != "" {
			return "brew uninstall " + p.Brew
		}
	case SourceApt:
		if p.Apt != "" {
			return "sudo apt remove " + p.Apt
		}
	case SourceSnap:
		if p.Snap != "" {
			return "sudo snap remove " + p.Snap
		}
	case SourceNPM:
		if p.NPM != "" {
			return "npm uninstall -g " + p.NPM
		}
	}
	return ""
}

// BestInstallSource returns the recommended package manager for the current OS.
// Priority: Windows: winget→choco→npm, macOS: brew→npm, Linux: apt→snap→brew→npm.
func (p PackageIDs) BestInstallSource() InstallSource {
	switch runtime.GOOS {
	case "windows":
		if p.Winget != "" {
			return SourceWinget
		}
		if p.Choco != "" {
			return SourceChoco
		}
	case "darwin":
		if p.Brew != "" {
			return SourceBrew
		}
	default: // linux
		if p.Apt != "" {
			return SourceApt
		}
		if p.Snap != "" {
			return SourceSnap
		}
		if p.Brew != "" {
			return SourceBrew
		}
	}
	if p.NPM != "" {
		return SourceNPM
	}
	return ""
}

// BestInstallCmd returns the install command using the best source for this OS.
func (p PackageIDs) BestInstallCmd() string {
	return p.InstallCmd(p.BestInstallSource())
}

// SourcesForOS returns the package manager sources relevant to the current OS.
func SourcesForOS() []InstallSource {
	switch runtime.GOOS {
	case "windows":
		return []InstallSource{SourceWinget, SourceChoco, SourceScoop, SourceNPM}
	case "darwin":
		return []InstallSource{SourceBrew, SourceNPM}
	default: // linux
		return []InstallSource{SourceApt, SourceSnap, SourceBrew, SourceNPM}
	}
}

// StatusString compares installed vs latest and returns a display string.
func StatusString(installed, latest string) string {
	if installed == "" {
		if latest != "" {
			return "?"
		}
		return ""
	}
	if latest == "" {
		return ""
	}
	if VersionsMatch(installed, latest) {
		return "✓ up to date"
	}
	return "⬆ update"
}

// TruncatePath shortens a path for display.
func TruncatePath(path string, maxLen int) string {
	if path == "" {
		return "—"
	}
	if len(path) <= maxLen {
		return path
	}
	return "..." + path[len(path)-maxLen+3:]
}
