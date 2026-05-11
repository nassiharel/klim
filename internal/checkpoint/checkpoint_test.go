package checkpoint

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/registry"
)

func TestCapture_OnlyIncludesInstalled(t *testing.T) {
	tools := []registry.Tool{
		{
			Name:        "alpha",
			DisplayName: "Alpha",
			Instances:   []registry.Instance{{Path: "/usr/bin/alpha", Version: "1.2.3", Source: registry.SourceBrew}},
		},
		{
			Name:        "missing",
			DisplayName: "Missing",
		},
	}
	c := Capture("snap1", "test", tools)
	if len(c.Tools) != 1 {
		t.Fatalf("want 1 tool in checkpoint, got %d", len(c.Tools))
	}
	if c.Tools[0].Name != "alpha" {
		t.Errorf("expected alpha, got %q", c.Tools[0].Name)
	}
}

func TestSaveLoadList_RoundTrips(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())

	tools := []registry.Tool{{
		Name:      "kubectl",
		Instances: []registry.Instance{{Path: "/usr/bin/kubectl", Version: "1.31.0", Source: registry.SourceBrew}},
	}}
	c := Capture("before-upgrade", "pre-upgrade snapshot", tools)
	path, err := Save(c)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !strings.HasSuffix(path, ".yaml") {
		t.Errorf("expected .yaml file, got %s", path)
	}

	loaded, err := Load("before-upgrade")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Name != "before-upgrade" || loaded.Description != "pre-upgrade snapshot" {
		t.Errorf("metadata lost on round-trip: %+v", loaded)
	}
	if len(loaded.Tools) != 1 || loaded.Tools[0].Version != "1.31.0" {
		t.Errorf("tool state lost on round-trip: %+v", loaded.Tools)
	}

	list, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("want 1 checkpoint in list, got %d", len(list))
	}
}

func TestLoad_MissingReturnsErrNotExist(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	_, err := Load("does-not-exist")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("want ErrNotExist, got %v", err)
	}
}

func TestDelete_IdempotentOnMissing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	if err := Delete("does-not-exist"); err != nil {
		t.Errorf("Delete of missing checkpoint should be a no-op, got %v", err)
	}
}

func TestValidateName_RejectsTraversalAndInvalidChars(t *testing.T) {
	for _, bad := range []string{"", ".", "..", "../oops", "with/slash", `with\backslash`} {
		if err := validateName(bad); err == nil {
			t.Errorf("name %q should be rejected", bad)
		}
	}
	for _, good := range []string{"before-upgrade", "snap1", "v1.2.3", "team_release", "abc"} {
		if err := validateName(good); err != nil {
			t.Errorf("name %q should be valid, got %v", good, err)
		}
	}
}
