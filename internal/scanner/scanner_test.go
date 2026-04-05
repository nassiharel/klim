package scanner

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nassiharel/clim/internal/config"
	"github.com/nassiharel/clim/internal/registry"
)

func TestScanPATH_BasicDiscovery(t *testing.T) {
	dir := t.TempDir()

	// Create fake executables.
	createFakeExecutable(t, dir, "mytool")
	createFakeExecutable(t, dir, "othertool")

	// Set PATH to just our temp dir.
	t.Setenv("PATH", dir)

	cfg := config.Config{}
	tools, err := ScanPATH(cfg)
	if err != nil {
		t.Fatalf("ScanPATH failed: %v", err)
	}

	names := toolNames(tools)
	if !contains(names, "mytool") {
		t.Errorf("expected mytool in results, got: %v", names)
	}
	if !contains(names, "othertool") {
		t.Errorf("expected othertool in results, got: %v", names)
	}
}

func TestScanPATH_Deduplication(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	createFakeExecutable(t, dir1, "duped")
	createFakeExecutable(t, dir2, "duped")

	t.Setenv("PATH", dir1+string(os.PathListSeparator)+dir2)

	cfg := config.Config{}
	tools, err := ScanPATH(cfg)
	if err != nil {
		t.Fatalf("ScanPATH failed: %v", err)
	}

	count := 0
	for _, tool := range tools {
		if tool.Name == "duped" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 'duped', got %d", count)
	}

	// First dir should win (PATH precedence).
	for _, tool := range tools {
		if tool.Name == "duped" {
			if !pathStartsWith(tool.Path, dir1) {
				t.Errorf("expected path in dir1 (%s), got %s", dir1, tool.Path)
			}
		}
	}
}

func TestScanPATH_ExcludeFilter(t *testing.T) {
	dir := t.TempDir()
	createFakeExecutable(t, dir, "keepme")
	createFakeExecutable(t, dir, "hideme")

	t.Setenv("PATH", dir)

	cfg := config.Config{Exclude: []string{"hideme"}}
	tools, err := ScanPATH(cfg)
	if err != nil {
		t.Fatalf("ScanPATH failed: %v", err)
	}

	names := toolNames(tools)
	if !contains(names, "keepme") {
		t.Errorf("keepme should be in results: %v", names)
	}
	if contains(names, "hideme") {
		t.Errorf("hideme should not be in results: %v", names)
	}
}

func TestScanPATH_IncludeFilter(t *testing.T) {
	dir := t.TempDir()
	createFakeExecutable(t, dir, "wanted")
	createFakeExecutable(t, dir, "unwanted")

	t.Setenv("PATH", dir)

	cfg := config.Config{Include: []string{"wanted"}}
	tools, err := ScanPATH(cfg)
	if err != nil {
		t.Fatalf("ScanPATH failed: %v", err)
	}

	names := toolNames(tools)
	if !contains(names, "wanted") {
		t.Errorf("wanted should be in results: %v", names)
	}
	if contains(names, "unwanted") {
		t.Errorf("unwanted should not be in results: %v", names)
	}
}

func TestScanPATH_SkipsDirectories(t *testing.T) {
	dir := t.TempDir()
	createFakeExecutable(t, dir, "real")

	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", dir)

	cfg := config.Config{}
	tools, err := ScanPATH(cfg)
	if err != nil {
		t.Fatalf("ScanPATH failed: %v", err)
	}

	names := toolNames(tools)
	if contains(names, "subdir") {
		t.Error("directories should not appear in results")
	}
}

func TestScanPATH_SkipsNonExistentDirs(t *testing.T) {
	dir := t.TempDir()
	createFakeExecutable(t, dir, "exists")

	bogus := filepath.Join(t.TempDir(), "no-such-dir")
	t.Setenv("PATH", bogus+string(os.PathListSeparator)+dir)

	cfg := config.Config{}
	tools, err := ScanPATH(cfg)
	if err != nil {
		t.Fatalf("ScanPATH failed: %v", err)
	}

	if len(tools) == 0 {
		t.Error("should still find tools from valid directories")
	}
}

func TestScanPATH_EmptyPATH(t *testing.T) {
	t.Setenv("PATH", "")

	cfg := config.Config{}
	tools, err := ScanPATH(cfg)
	if err != nil {
		t.Fatalf("ScanPATH failed: %v", err)
	}
	if len(tools) != 0 {
		t.Errorf("expected 0 tools for empty PATH, got %d", len(tools))
	}
}

func TestScanPATH_SortedAlphabetically(t *testing.T) {
	dir := t.TempDir()
	createFakeExecutable(t, dir, "zebra")
	createFakeExecutable(t, dir, "alpha")
	createFakeExecutable(t, dir, "middle")

	t.Setenv("PATH", dir)

	cfg := config.Config{}
	tools, err := ScanPATH(cfg)
	if err != nil {
		t.Fatalf("ScanPATH failed: %v", err)
	}

	names := toolNames(tools)
	if len(names) < 3 {
		t.Fatalf("expected at least 3 tools, got %d", len(names))
	}
	if names[0] != "alpha" || names[1] != "middle" || names[2] != "zebra" {
		t.Errorf("expected alphabetical order, got: %v", names)
	}
}

// --- helpers ---

func createFakeExecutable(t *testing.T, dir, name string) {
	t.Helper()

	var path string
	var content []byte

	if runtime.GOOS == "windows" {
		path = filepath.Join(dir, name+".exe")
		// Minimal PE-like file (just needs to exist for our tests).
		content = []byte("fake")
	} else {
		path = filepath.Join(dir, name)
		content = []byte("#!/bin/sh\necho fake\n")
	}

	if err := os.WriteFile(path, content, 0o755); err != nil {
		t.Fatal(err)
	}
}

func toolNames(tools []registry.Tool) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

func pathStartsWith(path, prefix string) bool {
	// Resolve symlinks and short names (e.g., Windows 8.3 names) for consistent comparison.
	resolved1, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolved1 = path
	}
	resolved2, err := filepath.EvalSymlinks(prefix)
	if err != nil {
		resolved2 = prefix
	}
	abs1, _ := filepath.Abs(resolved1)
	abs2, _ := filepath.Abs(resolved2)
	return len(abs1) >= len(abs2) && abs1[:len(abs2)] == abs2
}
