package teamfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nassiharel/clim/internal/registry"
)

func TestParseConstraint(t *testing.T) {
	tests := []struct {
		input  string
		wantOp string
		wantV  string
	}{
		{">=1.28", ">=", "1.28"},
		{">1.0", ">", "1.0"},
		{"<=2.0", "<=", "2.0"},
		{"<3.0", "<", "3.0"},
		{"!=1.5", "!=", "1.5"},
		{"=1.0", "=", "1.0"},
		{"1.28", ">=", "1.28"},
		{"", "", ""},
		{"  >=1.28  ", ">=", "1.28"},
	}
	for _, tt := range tests {
		op, ver := ParseConstraint(tt.input)
		if op != tt.wantOp || ver != tt.wantV {
			t.Errorf("ParseConstraint(%q) = (%q, %q), want (%q, %q)", tt.input, op, ver, tt.wantOp, tt.wantV)
		}
	}
}

func TestCheckConstraint(t *testing.T) {
	tests := []struct {
		op, installed, constraint string
		want                     bool
	}{
		{">=", "1.30", "1.28", true},
		{">=", "1.28", "1.28", true},
		{">=", "1.27", "1.28", false},
		{">", "1.29", "1.28", true},
		{">", "1.28", "1.28", false},
		{"<", "1.27", "1.28", true},
		{"<", "1.28", "1.28", false},
		{"=", "1.28", "1.28", true},
		{"=", "1.29", "1.28", false},
		{"!=", "1.29", "1.28", true},
		{"!=", "1.28", "1.28", false},
	}
	for _, tt := range tests {
		got := checkConstraint(tt.op, tt.installed, tt.constraint)
		if got != tt.want {
			t.Errorf("checkConstraint(%q, %q, %q) = %v, want %v", tt.op, tt.installed, tt.constraint, got, tt.want)
		}
	}
}

func TestFind(t *testing.T) {
	// Create temp dir with .clim.yaml.
	root := t.TempDir()
	sub := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	climFile := filepath.Join(root, ".clim.yaml")
	if err := os.WriteFile(climFile, []byte("tools:\n  - name: git\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Find from deep subdir.
	found := Find(sub)
	if found != climFile {
		t.Errorf("Find(%q) = %q, want %q", sub, found, climFile)
	}

	// Find from root itself.
	found = Find(root)
	if found != climFile {
		t.Errorf("Find(%q) = %q, want %q", root, found, climFile)
	}

	// Not found in empty dir.
	empty := t.TempDir()
	found = Find(empty)
	if found != "" {
		t.Errorf("Find(%q) = %q, want empty", empty, found)
	}
}

func TestParse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".clim.yaml")

	t.Run("valid", func(t *testing.T) {
		data := "name: test-project\ntools:\n  - name: git\n  - name: kubectl\n    version: \">=1.28\"\n"
		if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
		tf, err := Parse(path)
		if err != nil {
			t.Fatal(err)
		}
		if tf.Name != "test-project" {
			t.Errorf("name = %q, want test-project", tf.Name)
		}
		if len(tf.Tools) != 2 {
			t.Fatalf("tools = %d, want 2", len(tf.Tools))
		}
		if tf.Tools[1].Version != ">=1.28" {
			t.Errorf("version = %q, want >=1.28", tf.Tools[1].Version)
		}
	})

	t.Run("no tools", func(t *testing.T) {
		if err := os.WriteFile(path, []byte("name: empty\ntools: []\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Parse(path)
		if err == nil {
			t.Error("expected error for empty tools")
		}
	})

	t.Run("empty name", func(t *testing.T) {
		if err := os.WriteFile(path, []byte("tools:\n  - name: \"\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Parse(path)
		if err == nil {
			t.Error("expected error for empty tool name")
		}
	})
}

func TestCheck(t *testing.T) {
	tf := &TeamFile{
		Tools: []RequiredTool{
			{Name: "git"},
			{Name: "kubectl", Version: ">=1.28"},
			{Name: "docker"},
			{Name: "terraform", Version: ">=1.7"},
		},
	}

	tools := []registry.Tool{
		{Name: "git", Instances: []registry.Instance{{Version: "2.43.0", Source: "brew"}}},
		{Name: "kubectl", Instances: []registry.Instance{{Version: "1.33.3", Source: "choco"}}},
		// docker not in list → missing
		{Name: "terraform", Instances: []registry.Instance{{Version: "1.5.7", Source: "brew"}}},
	}

	results := Check(tf, tools)
	if len(results) != 4 {
		t.Fatalf("results = %d, want 4", len(results))
	}

	// git: OK (no constraint)
	if results[0].Status != StatusOK {
		t.Errorf("git: status = %d, want OK", results[0].Status)
	}
	// kubectl: OK (1.33.3 >= 1.28)
	if results[1].Status != StatusOK {
		t.Errorf("kubectl: status = %d, want OK", results[1].Status)
	}
	// docker: missing
	if results[2].Status != StatusMissing {
		t.Errorf("docker: status = %d, want Missing", results[2].Status)
	}
	// terraform: outdated (1.5.7 < 1.7)
	if results[3].Status != StatusOutdated {
		t.Errorf("terraform: status = %d, want Outdated", results[3].Status)
	}

	ok, missing, outdated := Summary(results)
	if ok != 2 || missing != 1 || outdated != 1 {
		t.Errorf("summary = %d/%d/%d, want 2/1/1", ok, missing, outdated)
	}

	if AllSatisfied(results) {
		t.Error("expected AllSatisfied=false")
	}
}

func TestGenerate(t *testing.T) {
	tools := []registry.Tool{
		{Name: "git", Instances: []registry.Instance{{Version: "2.43.0"}}},
		{Name: "kubectl", Instances: []registry.Instance{{Version: "1.33.3"}}},
		{Name: "docker"}, // not installed — should be excluded
	}

	tf := Generate(tools, false)
	if len(tf.Tools) != 2 {
		t.Fatalf("tools = %d, want 2", len(tf.Tools))
	}

	tfV := Generate(tools, true)
	if tfV.Tools[0].Version != ">=2.43.0" {
		t.Errorf("version = %q, want >=2.43.0", tfV.Tools[0].Version)
	}
}
