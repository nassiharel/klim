// Package snapshot manages environment snapshots and named profiles.
// Snapshots are timestamped manifests stored under ~/.config/clim/snapshots/.
// Profiles are named snapshots stored under ~/.config/clim/profiles/.
package snapshot

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/fileutil"
	"github.com/nassiharel/clim/internal/manifest"
	"github.com/nassiharel/clim/internal/paths"
	"github.com/nassiharel/clim/internal/registry"
)

// Snapshot extends a manifest with snapshot-specific metadata.
type Snapshot struct {
	manifest.Manifest `yaml:",inline"`
	Name              string `yaml:"name,omitempty"`
	CreatedAt         string `yaml:"created_at"`
}

// snapshotsDir returns the path to the snapshots directory.
func snapshotsDir() (string, error) {
	return paths.Join("snapshots")
}

// profilesDir returns the path to the profiles directory.
func profilesDir() (string, error) {
	return paths.Join("profiles")
}

// Save creates a timestamped snapshot of the given tools.
func Save(tools []registry.Tool, label string) (string, error) {
	dir, err := snapshotsDir()
	if err != nil {
		return "", err
	}

	snap := buildSnapshot(tools, label)

	ts := time.Now().Format("2006-01-02T150405")
	name := ts
	if label != "" {
		safe := sanitizeName(label)
		name = ts + "-" + safe
	}
	filename := name + ".yaml"
	path := filepath.Join(dir, filename)

	if err := writeSnapshot(path, snap); err != nil {
		return "", err
	}
	return path, nil
}

// List returns all saved snapshots sorted by creation time (newest first).
func List() ([]Entry, error) {
	dir, err := snapshotsDir()
	if err != nil {
		return nil, err
	}
	return listDir(dir)
}

// Load reads a snapshot by filename or path.
func Load(nameOrPath string) (*Snapshot, error) {
	path, err := resolveSnapshotPath(nameOrPath)
	if err != nil {
		return nil, err
	}
	return readSnapshot(path)
}

// Delete removes a snapshot by filename or path.
func Delete(nameOrPath string) error {
	path, err := resolveSnapshotPath(nameOrPath)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// SaveProfile saves a named profile (overwriting if it exists).
func SaveProfile(tools []registry.Tool, name string) (string, error) {
	dir, err := profilesDir()
	if err != nil {
		return "", err
	}

	snap := buildSnapshot(tools, name)
	safe := sanitizeName(name)
	if safe == "" {
		return "", fmt.Errorf("invalid profile name: %q", name)
	}
	path := filepath.Join(dir, safe+".yaml")

	if err := writeSnapshot(path, snap); err != nil {
		return "", err
	}
	return path, nil
}

// LoadProfile reads a named profile.
func LoadProfile(name string) (*Snapshot, error) {
	dir, err := profilesDir()
	if err != nil {
		return nil, err
	}
	safe := sanitizeName(name)
	if safe == "" {
		return nil, fmt.Errorf("invalid profile name: %q", name)
	}
	path := filepath.Join(dir, safe+".yaml")
	return readSnapshot(path)
}

// ListProfiles returns all saved profiles.
func ListProfiles() ([]Entry, error) {
	dir, err := profilesDir()
	if err != nil {
		return nil, err
	}
	return listDir(dir)
}

// DeleteProfile removes a named profile.
func DeleteProfile(name string) error {
	dir, err := profilesDir()
	if err != nil {
		return err
	}
	safe := sanitizeName(name)
	if safe == "" {
		return fmt.Errorf("invalid profile name: %q", name)
	}
	return os.Remove(filepath.Join(dir, safe+".yaml"))
}

// Entry represents a snapshot or profile listing entry.
type Entry struct {
	Name      string
	Path      string
	CreatedAt time.Time
	ToolCount int
}

// --- helpers ---

func buildSnapshot(tools []registry.Tool, label string) Snapshot {
	var exported []manifest.Tool
	for _, t := range tools {
		if t.IsInstalled() {
			exported = append(exported, manifest.FromRegistryTool(t))
		}
	}
	return Snapshot{
		Manifest: manifest.Manifest{
			GeneratedBy: "clim snapshot",
			OS:          runtime.GOOS,
			Arch:        runtime.GOARCH,
			Tools:       exported,
		},
		Name:      label,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

func writeSnapshot(path string, snap Snapshot) error {
	if err := fileutil.EnsureDir(path); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	data, err := yaml.Marshal(&snap)
	if err != nil {
		return fmt.Errorf("marshalling snapshot: %w", err)
	}
	return fileutil.AtomicWrite(path, data, 0o644)
}

// maxSnapshotSize limits snapshot files to prevent memory exhaustion.
const maxSnapshotSize = 10 << 20 // 10 MB

func readSnapshot(path string) (*Snapshot, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("reading snapshot %s: %w", path, err)
	}
	if info.Size() > maxSnapshotSize {
		return nil, fmt.Errorf("snapshot %s too large (%d bytes, max %d)", path, info.Size(), maxSnapshotSize)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading snapshot %s: %w", path, err)
	}
	var snap Snapshot
	if err := yaml.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("parsing snapshot %s: %w", path, err)
	}
	return &snap, nil
}

func listDir(dir string) ([]Entry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var result []Entry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}

		// Quick-parse to get tool count and name.
		snap, readErr := readSnapshot(path)
		toolCount := 0
		name := strings.TrimSuffix(e.Name(), ".yaml")
		if readErr == nil && snap != nil {
			toolCount = len(snap.Tools)
			if snap.Name != "" {
				name = snap.Name
			}
		}

		result = append(result, Entry{
			Name:      name,
			Path:      path,
			CreatedAt: info.ModTime(),
			ToolCount: toolCount,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}

func resolveSnapshotPath(nameOrPath string) (string, error) {
	// Reject path traversal and dangerous characters.
	if strings.Contains(nameOrPath, "..") || strings.ContainsAny(nameOrPath, "/\\\x00") {
		return "", fmt.Errorf("invalid snapshot name: %q", nameOrPath)
	}

	dir, err := snapshotsDir()
	if err != nil {
		return "", err
	}

	// Try exact match (with .yaml).
	name := nameOrPath
	if !strings.HasSuffix(name, ".yaml") {
		name += ".yaml"
	}
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	// Try matching by prefix, suffix, or substring.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("snapshot %q not found", nameOrPath)
	}
	var matches []string
	for _, e := range entries {
		base := strings.TrimSuffix(e.Name(), ".yaml")
		if strings.HasPrefix(base, nameOrPath) || strings.HasSuffix(base, nameOrPath) || strings.Contains(base, nameOrPath) {
			matches = append(matches, e.Name())
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("snapshot %q not found", nameOrPath)
	case 1:
		return filepath.Join(dir, matches[0]), nil
	default:
		return "", fmt.Errorf("ambiguous snapshot %q — matches %d files: %s", nameOrPath, len(matches), strings.Join(matches, ", "))
	}
}

func sanitizeName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, name)
	return strings.ToLower(name)
}
