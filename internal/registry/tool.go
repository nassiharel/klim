package registry

// Tool represents a curated developer tool that clim tracks.
type Tool struct {
	// Name is the short identifier (e.g., "kubectl").
	Name string

	// DisplayName is the human-friendly name (e.g., "kubectl").
	DisplayName string

	// BinaryNames are executable names to search for, tried in order.
	// e.g., ["python3", "python"] — tries python3 first.
	BinaryNames []string

	// Packages maps package manager names to their package identifiers.
	Packages PackageIDs

	// Instances holds all found installations across PATH.
	// Populated by the finder. First entry is the primary (PATH precedence).
	Instances []Instance

	// Latest is the latest available version from upstream.
	Latest string

	// LatestFrom is which package manager or source reported the latest version.
	LatestFrom string
}

// Instance represents a single installation of a tool found on PATH.
type Instance struct {
	// Path is the absolute resolved path to the binary.
	Path string

	// Version is the detected installed version.
	Version string

	// Source is the detected install source ("winget", "choco", "brew", "apt", etc.).
	Source string

	// IsPrimary is true if this instance takes PATH precedence (first found).
	IsPrimary bool
}

// PackageIDs maps package manager names to their package identifiers for this tool.
type PackageIDs struct {
	Winget string // winget package ID, e.g. "Git.Git"
	Choco  string // chocolatey package name, e.g. "git"
	Brew   string // homebrew formula, e.g. "git"
	Apt    string // apt/dpkg package name, e.g. "git"
	Snap   string // snap package name
	NPM    string // npm package name
}

// PrimaryInstance returns the first (PATH-precedence) instance, or nil.
func (t *Tool) PrimaryInstance() *Instance {
	for i := range t.Instances {
		if t.Instances[i].IsPrimary {
			return &t.Instances[i]
		}
	}
	if len(t.Instances) > 0 {
		return &t.Instances[0]
	}
	return nil
}

// InstalledVersion returns the version of the primary instance, or "".
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
