package registry

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
	BinaryNames []string
	Packages    PackageIDs
	Instances   []Instance
	Latest      string
	LatestFrom  string
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

// TruncatePath shortens a path for display, keeping the tail.
func TruncatePath(path string, maxLen int) string {
	if path == "" {
		return "—"
	}
	if len(path) <= maxLen {
		return path
	}
	return "..." + path[len(path)-maxLen+3:]
}
