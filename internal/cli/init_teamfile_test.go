package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/finder"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/service"
	"github.com/nassiharel/clim/internal/teamfile"
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
// user-config dir, and creates an existing .clim.yaml so we can test
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
		t.Fatal("expected error when .clim.yaml exists and --force is not set")
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
	if !strings.Contains(err.Error(), "--force") || !strings.Contains(err.Error(), "no tools") {
		t.Errorf("error should mention --force AND no tools, got: %v", err)
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
	// .clim.yaml itself, no Dockerfile/go.mod/etc., so detection
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
	// Convention: -f is reserved for --file (docs/cli-conventions.md).
	if f := initCmd.Flag("force"); f == nil {
		t.Fatal("expected --force flag")
	}
	if got := initCmd.Flag("force").Shorthand; got != "" {
		t.Errorf("--force should not have a shorthand (-f is reserved for --file), got %q", got)
	}
}

// Compile-time assertion that registry.Tool type is still importable.
var _ = registry.Tool{}
