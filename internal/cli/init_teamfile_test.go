package cli

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/finder"
	"github.com/nassiharel/klim/internal/registry"
	"github.com/nassiharel/klim/internal/service"
	"github.com/nassiharel/klim/internal/teamfile"
)

// noopFinder satisfies finder.ToolFinder without touching PATH —
// runInit's scan path needs FindAll to succeed even when no tools are
// configured.
type noopFinder struct{}

func (noopFinder) FindAll(ctx context.Context, tools []registry.Tool) error { return nil }

// withInitCtx prepares a cobra.Command with a service that returns
// the supplied tools. Caller controls cwd + redirected user-config dir.
func withInitCtx(t *testing.T, tools ...registry.Tool) *cobra.Command {
	t.Helper()
	cat := &stubCatalogWithPacks{stubCatalog: stubCatalog{tools: tools}}
	svc := &service.ToolService{Catalog: cat, Finder: noopFinder{}}
	cmd := &cobra.Command{}
	cmd.SetContext(withCLICtx(context.Background(), &cliCtx{Svc: svc}))
	return cmd
}

// Compile-time guard that the no-op finder still satisfies the interface.
var _ finder.ToolFinder = noopFinder{}

// chdirWithExistingTeamfile cd's into a fresh temp dir, redirects the
// user-config dir, and creates an existing .klim.yaml so we can test
// the overwrite-refusal logic.
func chdirWithExistingTeamfile(t *testing.T, contents string) string {
	t.Helper()
	dir := chdirTemp(t)
	redirectConfig(t)
	if err := os.WriteFile(filepath.Join(dir, teamfile.FileName), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// resetInitFlags clears the package-level flag vars between tests so
// state from one test doesn't leak into the next.
func resetInitFlags(t *testing.T) {
	t.Helper()
	prevForce := initForceFlag
	prevAll := initAllFlag
	prevName := initNameFlag
	prevMinV := initMinVersionFlag
	t.Cleanup(func() {
		initForceFlag = prevForce
		initAllFlag = prevAll
		initNameFlag = prevName
		initMinVersionFlag = prevMinV
	})
	initForceFlag = false
	initAllFlag = false
	initNameFlag = ""
	initMinVersionFlag = false
}

func TestInit_RefusesToOverwriteByDefault(t *testing.T) {
	resetInitFlags(t)
	original := "tools:\n  - name: existing\n"
	dir := chdirWithExistingTeamfile(t, original)
	cmd := withInitCtx(t)

	err := runInit(cmd, nil)
	if err == nil {
		t.Fatal("expected error when .klim.yaml exists and --force is not set")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should mention --force, got: %v", err)
	}
	// File must be untouched.
	got, _ := os.ReadFile(filepath.Join(dir, teamfile.FileName))
	if string(got) != original {
		t.Errorf("existing manifest was modified despite the refusal:\n%s", got)
	}
}

func TestInit_ForceOverwritesExistingWithRealTools(t *testing.T) {
	resetInitFlags(t)
	original := "tools:\n  - name: stale\n"
	dir := chdirWithExistingTeamfile(t, original)

	// Catalog with one installed tool so runInit actually reaches
	// teamfile.Write (otherwise the empty-tools early return would
	// leave the file untouched and this test wouldn't exercise the
	// overwrite path it claims to).
	installed := registry.Tool{
		Name: "git",
		Instances: []registry.Instance{
			{Path: "/usr/bin/git", Version: "2.40.0", Source: registry.SourceApt},
		},
	}
	cmd := withInitCtx(t, installed)

	initForceFlag = true
	initAllFlag = true // skip project-detection so the test is self-contained

	if err := runInit(cmd, nil); err != nil {
		t.Fatalf("runInit with --force --all and one installed tool: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, teamfile.FileName))
	if err != nil {
		t.Fatalf("reading rewritten manifest: %v", err)
	}
	if !strings.Contains(string(got), "name: git") {
		t.Errorf("expected manifest to contain new tool 'git', got:\n%s", got)
	}
	if strings.Contains(string(got), "stale") {
		t.Errorf("manifest still contains 'stale' from the original file:\n%s", got)
	}
}

func TestInit_ForceWithoutToolsRefusesToBlankExistingFile(t *testing.T) {
	resetInitFlags(t)
	original := "tools:\n  - name: existing\n"
	dir := chdirWithExistingTeamfile(t, original)
	cmd := withInitCtx(t) // empty catalog → no detected tools

	initForceFlag = true
	initAllFlag = true

	err := runInit(cmd, nil)
	if err == nil {
		t.Fatal("expected error when --force given but no tools to write")
	}
	if !strings.Contains(err.Error(), "--force") || !strings.Contains(err.Error(), "no installed tools") {
		t.Errorf("error should mention --force AND no installed tools, got: %v", err)
	}
	// Existing file must be intact — never silently truncated.
	got, _ := os.ReadFile(filepath.Join(dir, teamfile.FileName))
	if string(got) != original {
		t.Errorf("--force with no tools must NOT touch the existing manifest:\n%s", got)
	}
}

func TestInit_ForceNoProjectFiles_RefusesToBlankExistingFile(t *testing.T) {
	resetInitFlags(t)
	original := "tools:\n  - name: existing\n"
	dir := chdirWithExistingTeamfile(t, original)
	cmd := withInitCtx(t)

	initForceFlag = true
	// Default project detection (no --all). The temp dir has only
	// .klim.yaml itself, no Dockerfile/go.mod/etc., so detection
	// returns 0 detected tools.

	err := runInit(cmd, nil)
	if err == nil {
		t.Fatal("expected error when --force given but no project files were detected")
	}
	got, _ := os.ReadFile(filepath.Join(dir, teamfile.FileName))
	if string(got) != original {
		t.Errorf("--force with no detected tools must NOT touch the existing manifest:\n%s", got)
	}
}

func TestInit_ForceFlag_NoShortHand(t *testing.T) {
	// Convention: -f is reserved for --file (CLI-CONVENTIONS.md).
	if f := initCmd.Flag("force"); f == nil {
		t.Fatal("expected --force flag")
	}
	if got := initCmd.Flag("force").Shorthand; got != "" {
		t.Errorf("--force should not have a shorthand (-f is reserved for --file), got %q", got)
	}
}

// TestInit_ForceProjectDetectedButNoneInstalled covers the empty-tools
// path on the project-detection branch when --all is NOT set. The
// reviewer flagged that the previous error message ("no tools
// detected") was inaccurate — detection found things, they just
// weren't installed. The new message must reflect that.
func TestInit_ForceProjectDetectedButNoneInstalled(t *testing.T) {
	resetInitFlags(t)
	dir := chdirTemp(t)
	redirectConfig(t)
	// Existing manifest so manifestExists is true.
	if err := os.WriteFile(filepath.Join(dir, teamfile.FileName), []byte("tools:\n  - name: existing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Project file so detection succeeds.
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module demo\n\ngo 1.22\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Empty installed set (catalog has 0 tools) — detection produces
	// matches, none of them are in the installed map.
	cmd := withInitCtx(t)
	initForceFlag = true

	err := runInit(cmd, nil)
	if err == nil {
		t.Fatal("expected error when --force given but no detected tools are installed")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Errorf("error should explain that detected tools aren't installed, got: %v", err)
	}
	// Existing file untouched.
	got, _ := os.ReadFile(filepath.Join(dir, teamfile.FileName))
	if !strings.Contains(string(got), "existing") {
		t.Errorf("existing manifest should be preserved, got: %s", got)
	}
}

// TestInit_DanglingSymlinkRequiresForce verifies that a dangling
// .klim.yaml symlink is recognised as "exists" by Lstat, so plain
// `klim project init` refuses to clobber it the same way it would refuse
// for a regular existing file.
func TestInit_DanglingSymlinkRequiresForce(t *testing.T) {
	resetInitFlags(t)
	dir := chdirTemp(t)
	redirectConfig(t)
	// Dangling symlink: .klim.yaml → /nonexistent/template.yaml.
	// Skip-on-permission-error so this exercises Windows CI when
	// developer mode is enabled and skips when it isn't.
	if err := tryCreateSymlink("/nonexistent/template.yaml", filepath.Join(dir, teamfile.FileName)); err != nil {
		if isSymlinkPermissionError(err) {
			t.Skipf("symlink creation requires elevated privileges: %v", err)
		}
		t.Fatal(err)
	}
	cmd := withInitCtx(t)

	err := runInit(cmd, nil)
	if err == nil {
		t.Fatal("expected error when dangling .klim.yaml symlink exists and --force is not set")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should mention --force, got: %v", err)
	}
	// Symlink itself must still be in place.
	info, err := os.Lstat(filepath.Join(dir, teamfile.FileName))
	if err != nil {
		t.Fatalf("symlink missing after init: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("dangling symlink was replaced despite the --force refusal")
	}
}

// tryCreateSymlink wraps os.Symlink for tests; on Windows CI without
// developer mode it returns ERROR_PRIVILEGE_NOT_HELD which the caller
// translates into t.Skip.
func tryCreateSymlink(oldname, newname string) error {
	return os.Symlink(oldname, newname)
}

// isSymlinkPermissionError reports whether err is the
// "privilege not held" error emitted by Windows when the user lacks
// the SeCreateSymbolicLinkPrivilege (i.e. not admin and developer mode
// is off). Matches both the wrapped fs.ErrPermission case and the
// raw "privilege not held" string.
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

// Compile-time assertion that registry.Tool type is still importable.
var _ = registry.Tool{}
