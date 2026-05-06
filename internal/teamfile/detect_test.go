package teamfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectFromProject(t *testing.T) {
	tests := []struct {
		name     string
		files    map[string]string // relative path → content
		dirs     []string          // directories to create
		wantTool string            // tool name expected in results
	}{
		{
			name:     "go.mod detects go",
			files:    map[string]string{"go.mod": "module example.com\n"},
			wantTool: "go",
		},
		{
			name:     "golangci config detects golangci-lint",
			files:    map[string]string{".golangci.yml": "linters:\n"},
			wantTool: "golangci-lint",
		},
		{
			name:     "package.json detects node",
			files:    map[string]string{"package.json": "{}"},
			wantTool: "node",
		},
		{
			name:     "Dockerfile detects docker",
			files:    map[string]string{"Dockerfile": "FROM alpine\n"},
			wantTool: "docker",
		},
		{
			name:     "Dockerfile FROM golang detects go",
			files:    map[string]string{"Dockerfile": "FROM golang:1.21\nRUN go build\n"},
			wantTool: "go",
		},
		{
			name:     "terraform dir detects terraform",
			dirs:     []string{"terraform"},
			wantTool: "terraform",
		},
		{
			name:     ".tf file detects terraform",
			files:    map[string]string{"main.tf": "resource {}\n"},
			wantTool: "terraform",
		},
		{
			name:     "helm dir detects helm",
			dirs:     []string{"helm"},
			wantTool: "helm",
		},
		{
			name:     ".github dir detects gh",
			dirs:     []string{".github"},
			wantTool: "gh",
		},
		{
			name:     "Cargo.toml detects cargo",
			files:    map[string]string{"Cargo.toml": "[package]\n"},
			wantTool: "cargo",
		},
		{
			name:     "pyproject.toml detects python3",
			files:    map[string]string{"pyproject.toml": "[project]\n"},
			wantTool: "python3",
		},
		{
			name:     "ansible.cfg detects ansible",
			files:    map[string]string{"ansible.cfg": "[defaults]\n"},
			wantTool: "ansible",
		},
		{
			name:     "CI file with kubectl keyword",
			files:    map[string]string{".github/workflows/deploy.yml": "steps:\n  - run: kubectl apply -f k8s/\n"},
			dirs:     []string{".github/workflows"},
			wantTool: "kubectl",
		},
		{
			name:     "dependabot in .github",
			files:    map[string]string{".github/dependabot.yml": "version: 2\n"},
			dirs:     []string{".github"},
			wantTool: "git",
		},
		{
			name:     "Justfile detects just",
			files:    map[string]string{"Justfile": "build:\n  go build\n"},
			wantTool: "just",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			// Create directories.
			for _, d := range tt.dirs {
				if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
					t.Fatalf("MkdirAll(%s): %v", d, err)
				}
			}

			// Create files.
			for path, content := range tt.files {
				full := filepath.Join(dir, path)
				if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
					t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
				}
				if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
					t.Fatalf("WriteFile(%s): %v", path, err)
				}
			}

			result := DetectFromProject(dir)

			found := false
			for _, r := range result.Tools {
				if r.Name == tt.wantTool {
					found = true
					break
				}
			}
			if !found {
				names := make([]string, len(result.Tools))
				for i, r := range result.Tools {
					names[i] = r.Name
				}
				t.Errorf("expected tool %q in results, got %v", tt.wantTool, names)
			}
		})
	}
}

func TestDetectDeterministic(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".github"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Run detection multiple times and verify order is stable.
	first := DetectFromProject(dir)
	for i := 0; i < 10; i++ {
		again := DetectFromProject(dir)
		if len(again.Tools) != len(first.Tools) {
			t.Fatalf("run %d: length changed %d → %d", i, len(first.Tools), len(again.Tools))
		}
		for j := range first.Tools {
			if first.Tools[j].Name != again.Tools[j].Name {
				t.Fatalf("run %d: order changed at %d: %q → %q", i, j, first.Tools[j].Name, again.Tools[j].Name)
			}
		}
	}
}

func TestDetectEmpty(t *testing.T) {
	dir := t.TempDir()
	result := DetectFromProject(dir)
	if len(result.Tools) != 0 {
		t.Errorf("expected 0 results for empty dir, got %d", len(result.Tools))
	}
}
