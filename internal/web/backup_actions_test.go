package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/registry"
	"github.com/nassiharel/klim/internal/service"
	"github.com/nassiharel/klim/internal/share"
)

// startBackupServer wires a Server with a fixture loader plus an
// override for the user-config dir so save/delete tests don't touch
// the real ~/.config/klim/backups/. paths.BackupsDir() reads the
// XDG / OS-specific config root via the paths package, which honours
// XDG_CONFIG_HOME on Linux and CLIM_CONFIG_HOME everywhere; we set
// the latter so tests stay isolated regardless of host OS.
func startBackupServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("CLIM_CONFIG_HOME", tmp)

	srv, err := New(Options{Service: service.New(), Bind: "127.0.0.1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tools := []registry.Tool{
		{
			Name:      "git",
			Latest:    "2.53.0",
			Instances: []registry.Instance{{Path: "/usr/bin/git", Version: "2.53.0", Source: registry.SourceWinget}},
			Packages:  registry.PackageIDs{Brew: "git", Winget: "Git.Git", Apt: "git"},
		},
		{
			Name:      "kubectl",
			Latest:    "1.31.0",
			Instances: []registry.Instance{{Path: "/k", Version: "1.28.4", Source: registry.SourceBrew}},
			Packages:  registry.PackageIDs{Brew: "kubectl", Winget: "Kubernetes.kubectl", Apt: "kubectl"},
		},
		{
			Name:     "terraform",
			Packages: registry.PackageIDs{Brew: "terraform", Winget: "Hashicorp.Terraform", Apt: "terraform"},
		},
	}
	srv.loader = &fixtureLoader{tools: tools, favs: map[string]bool{}}
	t.Cleanup(func() { _ = srv.Close() })
	ts := httptest.NewServer(srv.httpsrv.Handler)
	t.Cleanup(ts.Close)
	return ts, tmp
}

func TestBackupSave_WritesYAMLFileAndShowsFlash(t *testing.T) {
	ts, _ := startBackupServer(t)
	form := url.Values{"name": {"laptop-2026-05"}}
	resp, _ := postFormSameOrigin(t, ts.URL, "/backup/save", form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "flash=ok") || !strings.Contains(loc, "laptop-2026-05") {
		t.Errorf("redirect=%q", loc)
	}
	// Follow the redirect — the flash message should render and the
	// new backup should appear in the My Backups table.
	r2, body := get(t, ts.URL+loc)
	if r2.StatusCode != 200 {
		t.Fatalf("follow status: %d", r2.StatusCode)
	}
	if !strings.Contains(body, "laptop-2026-05.yaml") {
		t.Errorf("expected new backup file in My Backups list:\n%s", body)
	}
	if !strings.Contains(body, "Saved laptop-2026-05.yaml") {
		t.Errorf("expected flash banner")
	}
}

func TestBackupSave_RejectsBadName(t *testing.T) {
	ts, _ := startBackupServer(t)
	for _, bad := range []string{"", "../escape", ".hidden", "spaces here", strings.Repeat("a", 65)} {
		form := url.Values{"name": {bad}}
		resp, _ := postFormSameOrigin(t, ts.URL, "/backup/save", form)
		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("%q: status=%d, want 303", bad, resp.StatusCode)
			continue
		}
		loc := resp.Header.Get("Location")
		if !strings.Contains(loc, "flash=err") {
			t.Errorf("%q: expected error flash, got %q", bad, loc)
		}
	}
}

func TestBackupSavedDelete_RemovesFile(t *testing.T) {
	ts, _ := startBackupServer(t)
	// Create a backup first so we have something to delete.
	postFormSameOrigin(t, ts.URL, "/backup/save", url.Values{"name": {"to-delete"}})

	resp, _ := postFormSameOrigin(t, ts.URL, "/backup/saved/to-delete.yaml/delete", nil)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Location"), "Deleted") {
		t.Errorf("expected delete flash")
	}
	// A second delete should report not-found.
	resp2, _ := postFormSameOrigin(t, ts.URL, "/backup/saved/to-delete.yaml/delete", nil)
	if !strings.Contains(resp2.Header.Get("Location"), "not+found") {
		t.Errorf("expected not-found flash on second delete, got %q", resp2.Header.Get("Location"))
	}
}

func TestBackupPreview_ManifestYAML(t *testing.T) {
	ts, _ := startBackupServer(t)
	yaml := "tools:\n  - name: kubectl\n  - name: terraform\n  - name: bogus\n"
	form := url.Values{"manifest": {yaml}}
	resp, body := postFormSameOrigin(t, ts.URL, "/backup/preview", form)
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, "Manifest YAML") {
		t.Errorf("expected source label")
	}
	if !strings.Contains(body, ">kubectl<") || !strings.Contains(body, ">terraform<") {
		t.Errorf("expected kubectl + terraform rows")
	}
	if !strings.Contains(body, "not in catalog") {
		t.Errorf("expected bogus entry to render as not-in-catalog")
	}
	// terraform is not installed in the fixture, so it must offer an
	// Install button. kubectl IS installed, so it must NOT.
	if !strings.Contains(body, `action="/tools/terraform/install"`) {
		t.Errorf("expected install form for terraform")
	}
	if strings.Contains(body, `action="/tools/kubectl/install"`) {
		t.Errorf("kubectl is installed; should not show Install in preview")
	}
}

func TestBackupPreview_ShareToken(t *testing.T) {
	ts, _ := startBackupServer(t)
	token, err := share.Encode([]string{"terraform"})
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"token": {token}}
	resp, body := postFormSameOrigin(t, ts.URL, "/backup/preview", form)
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, "Share token") {
		t.Errorf("expected share-token source label")
	}
	if !strings.Contains(body, ">terraform<") {
		t.Errorf("expected terraform row in preview")
	}
}

func TestBackupPreview_RestoreSavedBackup(t *testing.T) {
	ts, _ := startBackupServer(t)
	postFormSameOrigin(t, ts.URL, "/backup/save", url.Values{"name": {"my-backup"}})

	resp, body := postFormSameOrigin(t, ts.URL, "/backup/preview", url.Values{"restore": {"my-backup.yaml"}})
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if !strings.Contains(body, "Saved backup: my-backup.yaml") {
		t.Errorf("expected saved-backup source label")
	}
	// git and kubectl are installed so they appear with installed badges;
	// the preview must list them either way.
	if !strings.Contains(body, ">git<") || !strings.Contains(body, ">kubectl<") {
		t.Errorf("expected git + kubectl rows")
	}
}

func TestBackupPreview_RejectsEmptyAndMalformed(t *testing.T) {
	ts, _ := startBackupServer(t)
	cases := []struct {
		name string
		form url.Values
		want string
	}{
		{"empty", url.Values{}, "paste a manifest"},
		{"malformed yaml", url.Values{"manifest": {"this is: not\n valid yaml: ["}}, "couldn%27t+parse+input"},
		{"bogus token", url.Values{"token": {"klim:v1:not-base64"}}, "couldn%27t+parse+input"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, _ := postFormSameOrigin(t, ts.URL, "/backup/preview", tc.form)
			if resp.StatusCode != http.StatusSeeOther {
				t.Fatalf("status: %d", resp.StatusCode)
			}
			loc := resp.Header.Get("Location")
			if !strings.Contains(loc, "flash=err") {
				t.Errorf("expected error flash, got %q", loc)
			}
		})
	}
}

func TestBackupPackCreate_SavesAndRedirectsToPack(t *testing.T) {
	ts, _ := startBackupServer(t)
	form := url.Values{
		"name":         {"my-stack"},
		"display_name": {"My Stack"},
		"description":  {"things i need"},
		"tools":        {"kubectl, terraform\nhelm"},
	}
	resp, _ := postFormSameOrigin(t, ts.URL, "/backup/packs/new", form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/packs/my-stack" {
		t.Errorf("redirect target: %q, want /packs/my-stack", loc)
	}
}

func TestBackupPackCreate_RejectsBadInputs(t *testing.T) {
	ts, _ := startBackupServer(t)
	cases := []url.Values{
		{"name": {""}, "tools": {"kubectl"}},
		{"name": {"with spaces"}, "tools": {"kubectl"}}, // spaces fail the slug regex
		{"name": {"-leading-dash"}, "tools": {"kubectl"}},
		{"name": {"ok"}, "tools": {""}},       // empty tools
		{"name": {"ok"}, "tools": {",,,, ,"}}, // only commas
	}
	for _, form := range cases {
		resp, _ := postFormSameOrigin(t, ts.URL, "/backup/packs/new", form)
		if resp.StatusCode != http.StatusSeeOther {
			t.Errorf("form=%v: status=%d, want 303", form, resp.StatusCode)
			continue
		}
		if !strings.Contains(resp.Header.Get("Location"), "flash=err") {
			t.Errorf("form=%v: expected error flash, got %q", form, resp.Header.Get("Location"))
		}
	}
}

func TestBackupPackCreate_LowercasesUppercaseSlug(t *testing.T) {
	// Server normalises uppercase to lowercase (forgiving UX, matches
	// what marketplace tooling does). Confirm a mixed-case slug ends
	// up at /packs/badcase, not /packs/BadCase or an error.
	ts, _ := startBackupServer(t)
	resp, _ := postFormSameOrigin(t, ts.URL, "/backup/packs/new", url.Values{
		"name":  {"BadCase"},
		"tools": {"kubectl"},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/packs/badcase" {
		t.Errorf("redirect=%q, want /packs/badcase", loc)
	}
}

func TestNormalisePackTools(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"kubectl, terraform", []string{"kubectl", "terraform"}},
		{"kubectl\nterraform\nhelm", []string{"kubectl", "terraform", "helm"}},
		{"  kubectl ,  terraform ,  ,  ", []string{"kubectl", "terraform"}},
		{"kubectl,kubectl,terraform", []string{"kubectl", "terraform"}},
		{"", nil},
	}
	for _, c := range cases {
		got := normalisePackTools(c.in)
		if len(got) != len(c.want) {
			t.Errorf("%q: got %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%q[%d]: got %q, want %q", c.in, i, got[i], c.want[i])
			}
		}
	}
}

func TestManifestToolNames_AcceptsBothShapes(t *testing.T) {
	wrapped := []byte("tools:\n  - name: kubectl\n  - name: terraform\n")
	flat := []byte("- kubectl\n- terraform\n")
	for _, body := range [][]byte{wrapped, flat} {
		got, err := manifestToolNames(body)
		if err != nil {
			t.Fatalf("body=%q: %v", body, err)
		}
		if len(got) != 2 || got[0] != "kubectl" || got[1] != "terraform" {
			t.Errorf("got %v", got)
		}
	}
	if _, err := manifestToolNames([]byte("not yaml: [")); err == nil {
		t.Error("expected error on malformed YAML")
	}
}
