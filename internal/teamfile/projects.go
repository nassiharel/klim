package teamfile

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/paths"
)

// ProjectEntry represents a registered project.
type ProjectEntry struct {
	Path        string    `yaml:"path"`
	Name        string    `yaml:"name"`
	LastChecked time.Time `yaml:"last_checked,omitempty"`
	ToolCount   int       `yaml:"tool_count,omitempty"`
}

type projectsFile struct {
	Projects []ProjectEntry `yaml:"projects"`
}

// ProjectsPath returns the path to the projects registry file.
func ProjectsPath() (string, error) {
	return paths.Join("projects", "projects.yaml")
}

// LoadProjects reads all registered projects from disk.
func LoadProjects() ([]ProjectEntry, error) {
	path, err := ProjectsPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var f projectsFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing projects.yaml: %w", err)
	}
	return f.Projects, nil
}

// SaveProjects writes all registered projects to disk.
func SaveProjects(projects []ProjectEntry) error {
	path, err := ProjectsPath()
	if err != nil {
		return err
	}
	f := projectsFile{Projects: projects}
	data, err := yaml.Marshal(&f)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// AddProject registers a project (or updates it if path already exists).
func AddProject(projectPath, name string, toolCount int) error {
	projects, err := LoadProjects()
	if err != nil {
		return err
	}

	abs, err := filepath.Abs(projectPath)
	if err != nil {
		abs = projectPath
	}

	now := time.Now()
	found := false
	for i := range projects {
		if projects[i].Path == abs {
			projects[i].Name = name
			projects[i].LastChecked = now
			projects[i].ToolCount = toolCount
			found = true
			break
		}
	}
	if !found {
		projects = append(projects, ProjectEntry{
			Path:        abs,
			Name:        name,
			LastChecked: now,
			ToolCount:   toolCount,
		})
	}

	return SaveProjects(projects)
}

// RemoveProject unregisters a project by path.
func RemoveProject(projectPath string) error {
	projects, err := LoadProjects()
	if err != nil {
		return err
	}

	abs, _ := filepath.Abs(projectPath)
	filtered := projects[:0]
	for _, p := range projects {
		if p.Path != abs {
			filtered = append(filtered, p)
		}
	}
	return SaveProjects(filtered)
}
