package teamfile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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
		want                      bool
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
	// docker: not in catalog → unknown
	if results[2].Status != StatusUnknown {
		t.Errorf("docker: status = %d, want Unknown", results[2].Status)
	}
	// terraform: outdated (1.5.7 < 1.7)
	if results[3].Status != StatusOutdated {
		t.Errorf("terraform: status = %d, want Outdated", results[3].Status)
	}

	ok, missing, outdated, unknown := Summary(results)
	if ok != 2 || missing != 0 || outdated != 1 || unknown != 1 {
		t.Errorf("summary = %d/%d/%d/%d, want 2/0/1/1", ok, missing, outdated, unknown)
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

// TestWrite_PreservesInodeAndMode guards the contract that
// teamfile.Write rewrites in place: the inode of an existing
// .clim.yaml stays stable across a Write call (so hardlinks and
// rich metadata like ACLs / xattrs survive — atomic temp+rename
// would replace the inode and drop them) and a manually-set mode
// is preserved by os.WriteFile's overwrite semantics.
func TestWrite_PreservesInodeAndMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Windows file modes don't map to POSIX bits; inode IDs
		// returned by Stat are zeroed in many configurations. The
		// inode-preservation guarantee still holds (truncate-in-place
		// keeps the same handle) but it can't be asserted portably
		// here.
		t.Skip("POSIX-only metadata preservation assertions")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)

	// Seed an existing manifest with a tightened mode.
	if err := os.WriteFile(path, []byte("tools:\n  - name: git\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	infoBefore, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	// Rewrite via teamfile.Write.
	tf := &TeamFile{Tools: []RequiredTool{{Name: "kubectl"}}}
	if err := Write(tf, path); err != nil {
		t.Fatalf("Write: %v", err)
	}

	infoAfter, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(infoBefore, infoAfter) {
		t.Errorf("Write replaced the inode; hardlinks / ACLs / xattrs would be lost")
	}
	if got := infoAfter.Mode().Perm(); got != 0o600 {
		t.Errorf("mode after Write = %o, want 0600 (existing perms must be preserved on overwrite)", got)
	}
}

// TestWrite_FollowsSymlinkAndPreservesIt guards that a .clim.yaml
// symlink points to a shared template stays a symlink after Write,
// and the new contents land at the target.
func TestWrite_FollowsSymlinkAndPreservesIt(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "template.yaml")
	if err := os.WriteFile(target, []byte("tools:\n  - name: stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, FileName)
	if err := os.Symlink(target, link); err != nil {
		// Permission errors (Windows without admin/dev-mode) are the
		// expected skip case. Anything else — path-handling
		// regressions, fs-specific bugs — must fail so a Windows
		// regression in this contract surfaces in CI.
		if isSymlinkPermissionError(err) {
			t.Skipf("symlink creation requires elevated privileges on this host: %v", err)
		}
		t.Fatalf("os.Symlink(%q, %q): %v", target, link, err)
	}

	tf := &TeamFile{Tools: []RequiredTool{{Name: "git"}}}
	if err := Write(tf, link); err != nil {
		t.Fatalf("Write through symlink: %v", err)
	}

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("Write replaced the symlink with a regular file")
	}
	got, _ := os.ReadFile(target)
	if !bytesContains(got, []byte("name: git")) {
		t.Errorf("target not updated through symlink: %s", got)
	}
}

// isSymlinkPermissionError reports whether err is the Windows
// "privilege not held" error or a generic POSIX permission error. We
// only skip the test on these — every other failure is real and must
// fail loudly. Otherwise a regression in symlink handling could
// silently turn into a skip and hide the bug.
func isSymlinkPermissionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, fs.ErrPermission) {
		return true
	}
	if runtime.GOOS != "windows" {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "privilege") || strings.Contains(msg, "not held")
}

func bytesContains(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		ok := true
		for j := range needle {
			if haystack[i+j] != needle[j] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
