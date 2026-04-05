package registry

// Tool represents a discovered executable on the system PATH.
type Tool struct {
	// Name is the basename of the binary (e.g., "git", "python3").
	Name string

	// Path is the absolute resolved path to the binary (e.g., "/usr/bin/git").
	Path string
}
