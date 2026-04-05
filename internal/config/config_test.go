package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingFile(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nonexistent.json"))
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if len(cfg.Include) != 0 || len(cfg.Exclude) != 0 || cfg.VersionTimeout != 0 {
		t.Fatalf("expected zero-value config, got: %+v", cfg)
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subdir", "config.json")

	original := Config{
		Include:        []string{"git", "go"},
		Exclude:        []string{"vim"},
		VersionTimeout: 3,
	}

	if err := Save(path, original); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(loaded.Include) != 2 || loaded.Include[0] != "git" || loaded.Include[1] != "go" {
		t.Errorf("Include mismatch: %v", loaded.Include)
	}
	if len(loaded.Exclude) != 1 || loaded.Exclude[0] != "vim" {
		t.Errorf("Exclude mismatch: %v", loaded.Exclude)
	}
	if loaded.VersionTimeout != 3 {
		t.Errorf("VersionTimeout mismatch: %d", loaded.VersionTimeout)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestApplyFilter_NoFilter(t *testing.T) {
	cfg := Config{}
	if !cfg.ApplyFilter("anything") {
		t.Error("empty config should allow everything")
	}
}

func TestApplyFilter_IncludeOnly(t *testing.T) {
	cfg := Config{Include: []string{"git", "go"}}

	if !cfg.ApplyFilter("git") {
		t.Error("git should be included")
	}
	if !cfg.ApplyFilter("Git") {
		t.Error("Git (case-insensitive) should be included")
	}
	if cfg.ApplyFilter("python") {
		t.Error("python should not be included")
	}
}

func TestApplyFilter_ExcludeOnly(t *testing.T) {
	cfg := Config{Exclude: []string{"VBoxManage", "dbus-daemon"}}

	if cfg.ApplyFilter("VBoxManage") {
		t.Error("VBoxManage should be excluded")
	}
	if cfg.ApplyFilter("vboxmanage") {
		t.Error("vboxmanage (case-insensitive) should be excluded")
	}
	if !cfg.ApplyFilter("git") {
		t.Error("git should be allowed")
	}
}

func TestApplyFilter_IncludeTakesPrecedence(t *testing.T) {
	cfg := Config{
		Include: []string{"git"},
		Exclude: []string{"git"}, // should be ignored
	}

	if !cfg.ApplyFilter("git") {
		t.Error("include should take precedence over exclude")
	}
	if cfg.ApplyFilter("python") {
		t.Error("python not in include list should be rejected")
	}
}

func TestEffectiveTimeout_Default(t *testing.T) {
	cfg := Config{}
	if cfg.EffectiveTimeout() != 5 {
		t.Errorf("expected default 5, got %d", cfg.EffectiveTimeout())
	}
}

func TestEffectiveTimeout_Custom(t *testing.T) {
	cfg := Config{VersionTimeout: 10}
	if cfg.EffectiveTimeout() != 10 {
		t.Errorf("expected 10, got %d", cfg.EffectiveTimeout())
	}
}
