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

// withInitCtx prepares a cobra.Command with a service that returns no
// catalog tools and does no PATH scanning. Caller controls cwd +
// redirected user-config dir.
func withInitCtx(t *testing.T) *cobra.Command {
	t.Helper()
	cat := &stubCatalogWithPacks{stubCatalog: stubCatalog{}}
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

func TestInit_ForceOverwritesExisting(t *testing.T) {
	resetInitFlags(t)
	chdirWithExistingTeamfile(t, "tools:\n  - name: existing\n")
	cmd := withInitCtx(t)

	initForceFlag = true
	initAllFlag = true // skip project-detection so the test is self-contained

	// runInit may complete successfully or with "no tools to include"
	// depending on whether anything is on PATH; either is fine — we
	// only assert the existence-check no longer blocks.
	if err := runInit(cmd, nil); err != nil {
		// The only error we should NOT see is the existence-check
		// refusal we removed via --force.
		if strings.Contains(err.Error(), "already exists") {
			t.Fatalf("--force should bypass existence check, got: %v", err)
		}
	}
}

func TestInit_ForceShortFlagRegistered(t *testing.T) {
	if f := initCmd.Flag("force"); f == nil {
		t.Fatal("expected --force flag")
	}
	if got := initCmd.Flag("force").Shorthand; got != "f" {
		t.Errorf("expected -f shorthand, got %q", got)
	}
}

// Compile-time assertion that registry.Tool type is still importable.
var _ = registry.Tool{}
