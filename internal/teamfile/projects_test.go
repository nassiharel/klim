package teamfile

import (
	"path/filepath"
	"testing"
)

func TestAddAndLoadProjects(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("HOME", tmp)
	t.Setenv("HOME", tmp)

	// Initially empty.
	projects, err := LoadProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Fatalf("expected 0 projects, got %d", len(projects))
	}

	// Add a project.
	if err := AddProject("/dev/myproject", "myproject", 5); err != nil {
		t.Fatal(err)
	}
	projects, err = LoadProjects()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
	if projects[0].Name != "myproject" {
		t.Errorf("name = %q, want myproject", projects[0].Name)
	}
	if projects[0].ToolCount != 5 {
		t.Errorf("toolCount = %d, want 5", projects[0].ToolCount)
	}

	// Update existing.
	if err := AddProject("/dev/myproject", "myproject-v2", 8); err != nil {
		t.Fatal(err)
	}
	projects, _ = LoadProjects()
	if len(projects) != 1 {
		t.Fatalf("expected 1 project after update, got %d", len(projects))
	}
	if projects[0].Name != "myproject-v2" {
		t.Errorf("name = %q, want myproject-v2", projects[0].Name)
	}

	// Add second.
	if err := AddProject("/dev/other", "other", 3); err != nil {
		t.Fatal(err)
	}
	projects, _ = LoadProjects()
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(projects))
	}

	// Remove.
	if err := RemoveProject("/dev/myproject"); err != nil {
		t.Fatal(err)
	}
	projects, _ = LoadProjects()
	if len(projects) != 1 {
		t.Fatalf("expected 1 project after remove, got %d", len(projects))
	}
	if projects[0].Name != "other" {
		t.Errorf("remaining = %q, want other", projects[0].Name)
	}
}

func TestProjectsPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("HOME", tmp)
	t.Setenv("HOME", tmp)

	path, err := ProjectsPath()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "projects.yaml" {
		t.Errorf("filename = %q, want projects.yaml", filepath.Base(path))
	}
}

func TestRemoveNonExistentProject(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("USERPROFILE", tmp)
	t.Setenv("HOME", tmp)
	t.Setenv("HOME", tmp)

	if err := RemoveProject("/dev/ghost"); err != nil {
		t.Fatalf("RemoveProject non-existent: %v", err)
	}
}
