package registry

import (
	"fmt"
	"os/exec"
	"runtime"
	"sync"
)

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
	Info        *ToolInfo // rich metadata, fetched lazily
}

// ToolInfo holds rich metadata about a tool, fetched from package managers.
type ToolInfo struct {
	Description string
	Publisher   string
	Homepage    string
	License     string
	ReleaseDate string
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
// Returns false if the installed version is already newer than the latest
// reported by the package manager (e.g. preview/RC vs stable channel).
func (t *Tool) HasUpdate() bool {
	ver := t.InstalledVersion()
	if ver == "" || t.Latest == "" {
		return false
	}
	if VersionsMatch(ver, t.Latest) {
		return false
	}
	// Only flag as update if latest is actually newer than installed.
	return CompareVersions(t.Latest, ver) > 0
}

// sourceCommands holds command templates for a package manager.
// Each template uses %s as a placeholder for the package identifier.
type sourceCommands struct {
	install   string
	upgrade   string
	uninstall string
}

// commandTemplates maps each package manager to its command templates.
var commandTemplates = map[InstallSource]sourceCommands{
	SourceWinget: {"winget install --id %s", "winget upgrade --id %s", "winget uninstall --id %s"},
	SourceChoco:  {"choco install %s", "choco upgrade %s", "choco uninstall %s"},
	SourceBrew:   {"brew install %s", "brew upgrade %s", "brew uninstall %s"},
	SourceApt:    {"sudo apt install %s", "sudo apt upgrade %s", "sudo apt remove %s"},
	SourceSnap:   {"sudo snap install %s", "sudo snap refresh %s", "sudo snap remove %s"},
	SourceNPM:    {"npm install -g %s", "npm update -g %s", "npm uninstall -g %s"},
}

// pkgID returns the package identifier for the given source, or "".
func (p PackageIDs) pkgID(source InstallSource) string {
	switch source {
	case SourceWinget:
		return p.Winget
	case SourceChoco:
		return p.Choco
	case SourceBrew:
		return p.Brew
	case SourceApt:
		return p.Apt
	case SourceSnap:
		return p.Snap
	case SourceNPM:
		return p.NPM
	}
	return ""
}

// InstallCmd returns the shell command to install this tool using the given source.
func (p PackageIDs) InstallCmd(source InstallSource) string {
	if id := p.pkgID(source); id != "" {
		if tmpl, ok := commandTemplates[source]; ok {
			return fmt.Sprintf(tmpl.install, id)
		}
	}
	return ""
}

// UpgradeCmd returns the shell command to upgrade this tool using the given source.
func (p PackageIDs) UpgradeCmd(source InstallSource) string {
	if id := p.pkgID(source); id != "" {
		if tmpl, ok := commandTemplates[source]; ok {
			return fmt.Sprintf(tmpl.upgrade, id)
		}
	}
	return ""
}

// RemoveCmd returns the shell command to remove/uninstall this tool.
func (p PackageIDs) RemoveCmd(source InstallSource) string {
	if id := p.pkgID(source); id != "" {
		if tmpl, ok := commandTemplates[source]; ok {
			return fmt.Sprintf(tmpl.uninstall, id)
		}
	}
	return ""
}

// pmAvailable checks if a package manager binary is on PATH (cached).
var pmAvailability struct {
	once sync.Once
	avail map[InstallSource]bool
}

func pmAvailable(source InstallSource) bool {
	pmAvailability.once.Do(func() {
		pmAvailability.avail = make(map[InstallSource]bool)
		checks := map[InstallSource]string{
			SourceWinget: "winget",
			SourceChoco:  "choco",
			SourceScoop:  "scoop",
			SourceBrew:   "brew",
			SourceApt:    "apt",
			SourceSnap:   "snap",
			SourceNPM:    "npm",
		}
		for src, bin := range checks {
			_, err := exec.LookPath(bin)
			pmAvailability.avail[src] = err == nil
		}
	})
	return pmAvailability.avail[source]
}

// BestInstallSource returns the recommended package manager for the current OS.
// Only suggests package managers that are actually installed on the system.
// Priority: Windows: winget→choco→npm, macOS: brew→npm, Linux: apt→snap→brew→npm.
func (p PackageIDs) BestInstallSource() InstallSource {
	candidates := sourcePriority()
	for _, src := range candidates {
		if p.pkgID(src) != "" && pmAvailable(src) {
			return src
		}
	}
	return ""
}

// sourcePriority returns the preferred package manager order for the current OS.
func sourcePriority() []InstallSource {
	switch runtime.GOOS {
	case "windows":
		return []InstallSource{SourceWinget, SourceChoco, SourceNPM}
	case "darwin":
		return []InstallSource{SourceBrew, SourceNPM}
	default: // linux
		return []InstallSource{SourceApt, SourceSnap, SourceBrew, SourceNPM}
	}
}

// BestInstallCmd returns the install command using the best source for this OS.
func (p PackageIDs) BestInstallCmd() string {
	return p.InstallCmd(p.BestInstallSource())
}

// SourcesForOS returns the package manager sources available on the current OS.
// Only includes package managers that are actually installed.
func SourcesForOS() []InstallSource {
	var all []InstallSource
	switch runtime.GOOS {
	case "windows":
		all = []InstallSource{SourceWinget, SourceChoco, SourceScoop, SourceNPM}
	case "darwin":
		all = []InstallSource{SourceBrew, SourceNPM}
	default: // linux
		all = []InstallSource{SourceApt, SourceSnap, SourceBrew, SourceNPM}
	}
	var available []InstallSource
	for _, src := range all {
		if pmAvailable(src) {
			available = append(available, src)
		}
	}
	return available
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
	// Only show update arrow if latest is actually newer.
	if CompareVersions(latest, installed) > 0 {
		return "⬆ update"
	}
	return "✓ up to date"
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
