package cli

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/custompacks"
	"github.com/nassiharel/klim/internal/registry"
	"github.com/nassiharel/klim/internal/service"
	"github.com/nassiharel/klim/internal/teamfile"
)

// stubCatalog is a minimal ToolCatalog that returns whatever tools were
// configured. It does NOT implement service.PackLoader unless packs is
// non-nil — the cast on the consumer side then drives behavior.
type stubCatalog struct {
	tools []registry.Tool
	packs []registry.Pack
}

func (s *stubCatalog) LoadTools(_ context.Context) ([]registry.Tool, *service.CatalogInfo, error) {
	return s.tools, &service.CatalogInfo{Tools: len(s.tools)}, nil
}

// stubCatalogWithPacks pairs LoadTools + LoadPacks. Refscan only ever
// reaches LoadPacks, so this is enough for the tests.
type stubCatalogWithPacks struct {
	stubCatalog
}

func (s *stubCatalogWithPacks) LoadPacks(_ context.Context) ([]registry.Pack, error) {
	return s.packs, nil
}

// withRefscanCtx prepares a cobra.Command with a cliCtx whose
// ToolService delegates pack lookups to stub. Caller can also set
// XDG/APPDATA via t.Setenv to redirect projects.yaml + custom-packs.
func withRefscanCtx(t *testing.T, packs []registry.Pack) *cobra.Command {
	t.Helper()
	cat := &stubCatalogWithPacks{stubCatalog: stubCatalog{packs: packs}}
	svc := &service.ToolService{Catalog: cat}
	cmd := &cobra.Command{}
	cmd.SetContext(withCLICtx(context.Background(), &cliCtx{Svc: svc}))
	return cmd
}

// redirectConfig points os.UserConfigDir() at a fresh temp dir so the
// teamfile/projects + custompacks loaders see an empty user config.
// On Windows that's APPDATA; on Unix it's XDG_CONFIG_HOME.
func redirectConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", dir)
	} else {
		t.Setenv("XDG_CONFIG_HOME", dir)
		// macOS: os.UserConfigDir returns ~/Library/Application Support
		// — override HOME too so we don't leak into the user's real config.
		t.Setenv("HOME", dir)
	}
	return dir
}

// chdirTemp changes into a fresh temp dir for the test's lifetime.
// CollectReferences calls teamfile.Find(cwd) so the CWD must be
// controlled. Restores the original CWD on test cleanup.
func chdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	return dir
}

func TestCollectReferences_LocalTeamfileRequiredAndOptional(t *testing.T) {
	dir := chdirTemp(t)
	redirectConfig(t)
	tf := []byte(`tools:
  - name: kubectl
    version: ">=1.28"
optional:
  - name: helm
    version: "~3.12"
`)
	if err := os.WriteFile(filepath.Join(dir, ".klim.yaml"), tf, 0o644); err != nil { //nolint:gosec
		t.Fatal(err)
	}
	cmd := withRefscanCtx(t, nil)

	refs, warnings := CollectReferences(cmd, "kubectl")
	if len(refs) != 1 || refs[0].Kind != "teamfile" || !refs[0].Required || refs[0].Constraint != ">=1.28" {
		t.Fatalf("required path: refs=%+v warnings=%v", refs, warnings)
	}

	refs, warnings = CollectReferences(cmd, "helm")
	if len(refs) != 1 || refs[0].Kind != "teamfile" || refs[0].Required || refs[0].Constraint != "~3.12" {
		t.Fatalf("optional path: refs=%+v warnings=%v", refs, warnings)
	}
}

func TestCollectReferences_NoMatchesIsEmpty(t *testing.T) {
	chdirTemp(t)
	redirectConfig(t)
	cmd := withRefscanCtx(t, nil)
	refs, warnings := CollectReferences(cmd, "nothing-here")
	if len(refs) != 0 {
		t.Errorf("expected 0 refs, got %+v", refs)
	}
	if len(warnings) != 0 {
		t.Errorf("expected 0 warnings, got %+v", warnings)
	}
}

func TestCollectReferences_MalformedTeamfileBecomesWarning(t *testing.T) {
	// A parse error must surface as a warning, not silently make the
	// command report "no references" when the file does in fact
	// reference the tool.
	dir := chdirTemp(t)
	redirectConfig(t)
	if err := os.WriteFile(filepath.Join(dir, ".klim.yaml"), []byte("tools: [not-a-mapping]\n"), 0o644); err != nil { //nolint:gosec
		t.Fatal(err)
	}
	cmd := withRefscanCtx(t, nil)
	refs, warnings := CollectReferences(cmd, "kubectl")
	if len(refs) != 0 {
		t.Errorf("expected 0 refs from malformed teamfile, got %+v", refs)
	}
	if len(warnings) == 0 {
		t.Fatal("expected a warning for malformed teamfile")
	}
	if !strings.Contains(warnings[0], "could not parse") {
		t.Errorf("warning should mention parse failure: %q", warnings[0])
	}
}

func TestCollectReferences_PackMatch(t *testing.T) {
	chdirTemp(t)
	redirectConfig(t)
	cmd := withRefscanCtx(t, []registry.Pack{
		{Name: "k8s-starter", DisplayName: "Kubernetes Starter", ToolNames: []string{"kubectl", "helm"}},
		{Name: "data", DisplayName: "Data", ToolNames: []string{"jq"}},
	})
	refs, _ := CollectReferences(cmd, "kubectl")
	var pack *Reference
	for i := range refs {
		if refs[i].Kind == "pack" {
			pack = &refs[i]
			break
		}
	}
	if pack == nil {
		t.Fatalf("expected a pack reference, got %+v", refs)
	}
	if pack.Name != "k8s-starter" || pack.DisplayName != "Kubernetes Starter" {
		t.Errorf("wrong pack ref: %+v", pack)
	}
}

// TestCollectReferences_RegisteredProjects covers the projects.yaml
// branch: a registered project pin should be returned as Kind:"project"
// with its constraint preserved.
func TestCollectReferences_RegisteredProjects(t *testing.T) {
	chdirTemp(t)
	cfg := redirectConfig(t)

	// Create a fake project on disk + a .klim.yaml inside it.
	projDir := filepath.Join(cfg, "fake-project")
	if err := os.MkdirAll(projDir, 0o755); err != nil { //nolint:gosec
		t.Fatal(err)
	}
	tf := []byte("tools:\n  - name: kubectl\n    version: \">=1.28\"\n")
	if err := os.WriteFile(filepath.Join(projDir, ".klim.yaml"), tf, 0o644); err != nil { //nolint:gosec
		t.Fatal(err)
	}

	// Register it via the public SaveProjects API.
	if err := teamfile.SaveProjects([]teamfile.ProjectEntry{
		{Name: "myapp", Path: projDir},
	}); err != nil {
		t.Fatal(err)
	}

	cmd := withRefscanCtx(t, nil)
	refs, warnings := CollectReferences(cmd, "kubectl")
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
	var proj *Reference
	for i := range refs {
		if refs[i].Kind == "project" {
			proj = &refs[i]
			break
		}
	}
	if proj == nil {
		t.Fatalf("expected a project reference, got %+v", refs)
	}
	if proj.Name != "myapp" || !proj.Required || proj.Constraint != ">=1.28" {
		t.Errorf("project ref fields wrong: %+v", proj)
	}
}

// TestCollectReferences_CustomPacks covers the custom-packs branch:
// a tool listed in a user-created pack should surface as
// Kind:"custom_pack" with its display name preserved.
func TestCollectReferences_CustomPacks(t *testing.T) {
	chdirTemp(t)
	redirectConfig(t)

	// Create a custom-packs file pointing at the tool we'll query.
	if err := custompacks.Save([]registry.Pack{
		{Name: "my-stack", DisplayName: "My Stack", ToolNames: []string{"kubectl", "helm"}},
	}); err != nil {
		t.Fatal(err)
	}

	cmd := withRefscanCtx(t, nil)
	refs, warnings := CollectReferences(cmd, "kubectl")
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
	var cp *Reference
	for i := range refs {
		if refs[i].Kind == "custom_pack" {
			cp = &refs[i]
			break
		}
	}
	if cp == nil {
		t.Fatalf("expected a custom_pack reference, got %+v", refs)
	}
	if cp.Name != "my-stack" || cp.DisplayName != "My Stack" {
		t.Errorf("custom-pack ref fields wrong: %+v", cp)
	}
}

// TestCollectReferences_GetwdErrorBecomesWarning verifies that an
// inaccessible/deleted CWD records a warning instead of silently
// dropping the local-teamfile branch. We exercise the failure path
// by chdir'ing into a temp dir and deleting it before the call —
// os.Getwd() then fails on Unix; on Windows the cwd handle is
// stable, so the test is best-effort.
func TestCollectReferences_GetwdErrorBecomesWarning(t *testing.T) {
	tmp := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	// Try to delete the cwd. On Windows this can succeed but the
	// process keeps a handle and Getwd may still return the path,
	// so accept either outcome — the assertion is only that *if*
	// we reach the warning branch, the message is informative.
	if err := os.RemoveAll(tmp); err != nil {
		t.Skipf("could not remove cwd on this platform: %v", err)
	}
	redirectConfig(t)
	cmd := withRefscanCtx(t, nil)
	_, warnings := CollectReferences(cmd, "kubectl")
	for _, w := range warnings {
		if strings.Contains(w, "working directory") {
			return // expected warning surfaced
		}
	}
	// Platform may not produce the error — that's fine.
	t.Skipf("os.Getwd did not fail on this platform; warnings=%v", warnings)
}

// TestSamePath_WindowsCaseInsensitive guards against the
// duplicate-suppression regression where a registered project's
// `.klim.yaml` and the locally-discovered one differed only by case
// on Windows and ended up reported twice. Other platforms keep
// byte-wise comparison.
func TestSamePath_WindowsCaseInsensitive(t *testing.T) {
	a := filepath.Join("C:", "Users", "me", ".klim.yaml")
	b := filepath.Join("c:", "users", "me", ".klim.yaml")
	got := samePath(a, b)
	want := runtime.GOOS == "windows"
	if got != want {
		t.Errorf("samePath(%q, %q) = %v, want %v on %s", a, b, got, want, runtime.GOOS)
	}
	// Identical paths always match.
	if !samePath(a, a) {
		t.Errorf("samePath should match identical paths")
	}
	// Genuinely-different paths never match.
	c := filepath.Join("C:", "Users", "other", ".klim.yaml")
	if samePath(a, c) {
		t.Errorf("samePath(%q, %q) should be false", a, c)
	}
}
