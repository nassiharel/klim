package registry

import (
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

// InstallSource identifies how a tool was installed.
type InstallSource string

// Supported install sources for tools.
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

// MarketplaceStatus indicates whether a tool was added or changed in
// the latest marketplace refresh.
type MarketplaceStatus string

// Marketplace status values set after a catalog refresh.
const (
	StatusUnchanged MarketplaceStatus = ""
	StatusNew       MarketplaceStatus = "new"
	StatusChanged   MarketplaceStatus = "changed"
)

// Tool represents a curated developer tool that clim tracks.
type Tool struct {
	Name              string
	DisplayName       string
	Category          string
	Tags              []string
	BinaryNames       []string
	Packages          PackageIDs
	Instances         []Instance
	Latest            string
	LatestFrom        string
	Info              *ToolInfo         // rich metadata, fetched lazily
	InfoFetched       bool              // true once info fetch completed (Info may still be nil)
	MarketplaceStatus MarketplaceStatus // set after a marketplace refresh
}

// Pack represents a curated bundle of tools that can be installed together.
type Pack struct {
	Name        string
	DisplayName string
	Description string
	ToolNames   []string // references to tool names in the catalog
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

// sourceCommands holds command arg templates for a package manager.
// The package ID is appended as the final argument (no shell interpolation).
type sourceCommands struct {
	install   []string // e.g. ["winget", "install", "--id"]
	upgrade   []string
	uninstall []string
}

// commandTemplates maps each package manager to its command arg templates.
// The package identifier is appended as the last argument at call time.
var commandTemplates = map[InstallSource]sourceCommands{
	SourceWinget: {
		install:   []string{"winget", "install", "--id"},
		upgrade:   []string{"winget", "upgrade", "--id"},
		uninstall: []string{"winget", "uninstall", "--id"},
	},
	SourceChoco: {
		install:   []string{"choco", "install"},
		upgrade:   []string{"choco", "upgrade"},
		uninstall: []string{"choco", "uninstall"},
	},
	SourceBrew: {
		install:   []string{"brew", "install"},
		upgrade:   []string{"brew", "upgrade"},
		uninstall: []string{"brew", "uninstall"},
	},
	SourceApt: {
		install:   []string{"sudo", "apt", "install"},
		upgrade:   []string{"sudo", "apt", "upgrade"},
		uninstall: []string{"sudo", "apt", "remove"},
	},
	SourceSnap: {
		install:   []string{"sudo", "snap", "install"},
		upgrade:   []string{"sudo", "snap", "refresh"},
		uninstall: []string{"sudo", "snap", "remove"},
	},
	SourceNPM: {
		install:   []string{"npm", "install", "-g"},
		upgrade:   []string{"npm", "update", "-g"},
		uninstall: []string{"npm", "uninstall", "-g"},
	},
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

// InstallArgs returns the command and arguments to install this tool.
// Returns nil if no package ID is available for the given source.
// The result is safe to pass directly to exec.Command(args[0], args[1:]...).
func (p PackageIDs) InstallArgs(source InstallSource) []string {
	if id := p.pkgID(source); id != "" {
		if tmpl, ok := commandTemplates[source]; ok {
			return append(append([]string{}, tmpl.install...), id)
		}
	}
	return nil
}

// InstallCmd returns the human-readable install command string for display.
func (p PackageIDs) InstallCmd(source InstallSource) string {
	return strings.Join(p.InstallArgs(source), " ")
}

// UpgradeArgs returns the command and arguments to upgrade this tool.
func (p PackageIDs) UpgradeArgs(source InstallSource) []string {
	if id := p.pkgID(source); id != "" {
		if tmpl, ok := commandTemplates[source]; ok {
			return append(append([]string{}, tmpl.upgrade...), id)
		}
	}
	return nil
}

// UpgradeCmd returns the human-readable upgrade command string for display.
func (p PackageIDs) UpgradeCmd(source InstallSource) string {
	return strings.Join(p.UpgradeArgs(source), " ")
}

// RemoveArgs returns the command and arguments to remove/uninstall this tool.
func (p PackageIDs) RemoveArgs(source InstallSource) []string {
	if id := p.pkgID(source); id != "" {
		if tmpl, ok := commandTemplates[source]; ok {
			return append(append([]string{}, tmpl.uninstall...), id)
		}
	}
	return nil
}

// RemoveCmd returns the human-readable remove command string for display.
func (p PackageIDs) RemoveCmd(source InstallSource) string {
	return strings.Join(p.RemoveArgs(source), " ")
}

// pmAvailable checks if a package manager binary is on PATH (cached).
// Override pmAvailableFunc in tests to control availability without real binaries.
var pmAvailability struct {
	once  sync.Once
	avail map[InstallSource]bool
}

// pmAvailableOverride holds an optional test stub for package manager availability.
// Use SetPMAvailableFunc to set it; atomic access avoids data races with concurrent resolvers.
var pmAvailableOverride atomic.Pointer[func(InstallSource) bool]

// SetPMAvailableFunc overrides the package manager availability check for testing.
// Pass nil to restore the default exec.LookPath behavior.
func SetPMAvailableFunc(fn func(InstallSource) bool) {
	if fn == nil {
		pmAvailableOverride.Store(nil)
	} else {
		pmAvailableOverride.Store(&fn)
	}
}

func pmAvailable(source InstallSource) bool {
	if fnp := pmAvailableOverride.Load(); fnp != nil {
		return (*fnp)(source)
	}
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

// HasAnyPackageForOS reports whether any package ID is defined for a package
// manager that is relevant to the current OS — regardless of whether that
// package manager is actually installed.
func (p PackageIDs) HasAnyPackageForOS() bool {
	for _, src := range sourcePriority() {
		if p.pkgID(src) != "" {
			return true
		}
	}
	return false
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

// PMStatus represents the availability of a package manager on this system.
type PMStatus struct {
	Source    InstallSource
	Available bool
}

// AllPMStatusForOS returns all package manager candidates for the current OS
// along with whether each is installed on the system.
func AllPMStatusForOS() []PMStatus {
	var all []InstallSource
	switch runtime.GOOS {
	case "windows":
		all = []InstallSource{SourceWinget, SourceChoco, SourceScoop, SourceNPM}
	case "darwin":
		all = []InstallSource{SourceBrew, SourceNPM}
	default: // linux
		all = []InstallSource{SourceApt, SourceSnap, SourceBrew, SourceNPM}
	}
	result := make([]PMStatus, len(all))
	for i, src := range all {
		result[i] = PMStatus{Source: src, Available: pmAvailable(src)}
	}
	return result
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
	if maxLen <= 3 || len(path) <= maxLen {
		return path
	}
	return "..." + path[len(path)-maxLen+3:]
}
