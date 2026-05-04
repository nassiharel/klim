package web

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/service"
	"github.com/nassiharel/clim/internal/teamfile"
)

func startServerWithFixtures(t *testing.T, packs []registry.Pack) *httptest.Server {
	t.Helper()
	srv, err := New(Options{Service: service.New(), Bind: "127.0.0.1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	srv.loader = &fixtureLoader{
		tools: fixtureToolWithPackages(),
		favs:  map[string]bool{},
		packs: packs,
	}
	t.Cleanup(func() { _ = srv.Close() })
	ts := httptest.NewServer(srv.httpsrv.Handler)
	t.Cleanup(ts.Close)
	return ts
}

func TestPagePacks_ListsMarketplaceWithCounts(t *testing.T) {
	ts := startServerWithFixtures(t, []registry.Pack{
		{
			Name:        "k8s-starter",
			DisplayName: "Kubernetes Starter",
			Description: "Everything to get going with k8s",
			ToolNames:   []string{"kubectl", "terraform"},
		},
	})
	resp, body := get(t, ts.URL+"/packs")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, "Kubernetes Starter") {
		t.Fatalf("expected pack display name in HTML, got:\n%s", body)
	}
	// kubectl is installed in the fixture, terraform is not.
	if !strings.Contains(body, ">1<") {
		t.Fatalf("expected installed=1 badge for k8s-starter pack, got:\n%s", body)
	}
}

func TestPagePack_DetailShowsToolStatusAndInstallButton(t *testing.T) {
	ts := startServerWithFixtures(t, []registry.Pack{
		{Name: "mini", ToolNames: []string{"kubectl", "terraform"}},
	})
	resp, body := get(t, ts.URL+"/packs/mini")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, "kubectl") {
		t.Fatalf("missing kubectl row: %s", body)
	}
	// terraform is not installed in the fixture, so it should show an Install form.
	if !strings.Contains(body, `action="/tools/terraform/install"`) {
		t.Fatalf("expected Install form for terraform, got:\n%s", body)
	}
	// kubectl is already installed — should NOT have a per-row Install button.
	if strings.Contains(body, `action="/tools/kubectl/install"`) {
		t.Fatalf("kubectl is installed, should not show Install button")
	}
}

func TestPagePacks_LandingDistinguishesUnknownFromMissing(t *testing.T) {
	// Regression for the PR #48 review: buildPackRows used to count
	// every not-installed name (including catalog-unknowns) as
	// missing, which contradicted the per-pack detail page where
	// missing and unknown are tracked separately. The landing now
	// matches: kubectl is installed, terraform is missing-but-known,
	// ghost-tool is unknown.
	ts := startServerWithFixtures(t, []registry.Pack{
		{Name: "mixed", ToolNames: []string{"kubectl", "terraform", "ghost-tool"}},
	})
	resp, body := get(t, ts.URL+"/packs")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	// Header should now include both columns.
	for _, want := range []string{">Missing<", ">Unknown<"} {
		if !strings.Contains(body, want) {
			t.Errorf("packs landing missing column header %q", want)
		}
	}
	// Find the mixed pack's row and confirm the badge values: 1 installed,
	// 1 missing, 1 unknown.
	if !strings.Contains(body, `class="badge update">1<`) {
		t.Errorf("expected '1' in update badge (missing) for mixed pack; body:\n%s", body[:min(2000, len(body))])
	}
	if !strings.Contains(body, `class="badge muted">1<`) {
		t.Errorf("expected '1' in muted badge (unknown) for mixed pack")
	}
}

func TestBuildPackRows_UnknownTracking(t *testing.T) {
	tools := []registry.Tool{
		{Name: "kubectl", Instances: []registry.Instance{{Path: "/k"}}},
		{Name: "terraform"}, // catalog-known but not installed
	}
	installed := registry.InstalledSet(tools)
	known := knownToolSet(tools)
	rows := buildPackRows([]registry.Pack{
		{Name: "p", ToolNames: []string{"kubectl", "terraform", "ghost"}},
	}, installed, known, false)
	if len(rows) != 1 {
		t.Fatalf("got %d rows", len(rows))
	}
	r := rows[0]
	if r.Installed != 1 {
		t.Errorf("Installed=%d, want 1", r.Installed)
	}
	if r.Missing != 1 {
		t.Errorf("Missing=%d, want 1", r.Missing)
	}
	if r.Unknown != 1 {
		t.Errorf("Unknown=%d, want 1", r.Unknown)
	}
}
func TestPagePack_DistinguishesMissingFromUnknown(t *testing.T) {
	// kubectl is installed (fixtureToolWithPackages), terraform is in
	// the catalog but not installed, "ghost-tool" isn't in the
	// catalog at all.
	ts := startServerWithFixtures(t, []registry.Pack{
		{Name: "mixed", ToolNames: []string{"kubectl", "terraform", "ghost-tool"}},
	})
	resp, body := get(t, ts.URL+"/packs/mixed")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	// Expect "1 missing" (terraform) and "1 unknown" (ghost-tool).
	if !strings.Contains(body, "1 missing") {
		t.Errorf("expected '1 missing' in pack header, got:\n%s", body)
	}
	if !strings.Contains(body, "1 unknown") {
		t.Errorf("expected '1 unknown' in pack header (ghost-tool), got:\n%s", body)
	}
	// Ghost-tool row must render as 'not in catalog'.
	if !strings.Contains(body, "not in catalog") {
		t.Errorf("expected ghost-tool to render as 'not in catalog'")
	}
}

func TestPagePack_404OnUnknownPack(t *testing.T) {
	ts := startServerWithFixtures(t, nil)
	resp, _ := get(t, ts.URL+"/packs/this-pack-does-not-exist")
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestPageForYou_ScoresAndRendersInstallButton(t *testing.T) {
	// Build a fixture where kubectl is installed (with a "kubernetes"
	// tag) and a fresh tool "stern" shares that tag, so it should be
	// recommended.
	//
	// The pkg ID set spans every OS we test on (winget/brew/apt) so
	// HasAnyPackageForOS passes regardless of where the tests run.
	allOSPkgs := registry.PackageIDs{Brew: "x", Winget: "x", Apt: "x"}
	tools := []registry.Tool{
		{
			Name:      "kubectl",
			Tags:      []string{"kubernetes"},
			Latest:    "1.31.0",
			Instances: []registry.Instance{{Path: "/k", Version: "1.31.0", Source: registry.SourceBrew}},
			Packages:  allOSPkgs,
		},
		{
			Name:     "stern",
			Tags:     []string{"kubernetes"},
			Packages: allOSPkgs,
		},
		{
			// Unrelated tool with no overlap — should NOT appear.
			Name:     "ansible",
			Tags:     []string{"automation"},
			Packages: allOSPkgs,
		},
	}
	srv, err := New(Options{Service: service.New(), Bind: "127.0.0.1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	srv.loader = &fixtureLoader{tools: tools, favs: map[string]bool{}}
	t.Cleanup(func() { _ = srv.Close() })
	ts := httptest.NewServer(srv.httpsrv.Handler)
	t.Cleanup(ts.Close)

	resp, body := get(t, ts.URL+"/foryou")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, "stern") {
		t.Fatalf("expected stern in recommendations, got:\n%s", body)
	}
	if strings.Contains(body, "ansible") {
		t.Fatalf("ansible should not appear (no tag overlap)")
	}
	if !strings.Contains(body, `action="/tools/stern/install"`) {
		t.Fatalf("expected Install form for stern recommendation")
	}
}

func TestPageForYou_EmptyWhenNothingInstalled(t *testing.T) {
	srv, err := New(Options{Service: service.New(), Bind: "127.0.0.1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Fixture with only catalog-only tools (no Instances).
	srv.loader = &fixtureLoader{tools: []registry.Tool{
		{Name: "alpha", Packages: registry.PackageIDs{Brew: "alpha"}},
		{Name: "beta", Packages: registry.PackageIDs{Brew: "beta"}},
	}, favs: map[string]bool{}}
	t.Cleanup(func() { _ = srv.Close() })
	ts := httptest.NewServer(srv.httpsrv.Handler)
	t.Cleanup(ts.Close)

	resp, body := get(t, ts.URL+"/foryou")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, "No recommendations yet") {
		t.Fatalf("expected empty-state copy, got:\n%s", body)
	}
}

func TestPageProjects_EmptyState(t *testing.T) {
	// LoadProjects reads ~/.config/clim/projects.yaml. On a fresh test
	// host that file shouldn't exist, but be tolerant: the page must
	// render without erroring whether the registry is empty or has
	// real entries.
	ts, _ := startTestServer(t)
	resp, body := get(t, ts.URL+"/projects")
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, "Projects") {
		t.Fatalf("expected projects header, got:\n%s", body)
	}
}

func TestProjectCheckStatusName(t *testing.T) {
	cases := []struct {
		in   teamfile.CheckStatus
		want string
	}{
		{teamfile.StatusOK, "ok"},
		{teamfile.StatusMissing, "missing"},
		{teamfile.StatusOutdated, "outdated"},
		{teamfile.StatusUnknown, "unknown"},
	}
	for _, c := range cases {
		if got := projectCheckStatusName(c.in); got != c.want {
			t.Errorf("status %v: got %q, want %q", c.in, got, c.want)
		}
	}
}
