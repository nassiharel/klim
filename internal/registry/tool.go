package registry

// Tool represents a discovered executable on the system PATH.
type Tool struct {
	// Name is the basename of the binary (e.g., "git", "python3").
	Name string

	// DisplayName is the human-friendly name (e.g., "Git", "Python").
	// Set from the known tools registry; defaults to Name if not known.
	DisplayName string

	// Path is the absolute resolved path to the binary.
	Path string

	// Version is the detected installed version (from PE metadata or Go
	// buildinfo). Empty if detection failed or was not attempted.
	Version string

	// LatestVersion is the latest available version from upstream
	// (GitHub/PyPI/NPM). Only populated for known tools. Empty otherwise.
	LatestVersion string
}
