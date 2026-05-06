package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/config"
)

// withComplianceCmdCtx wires a cobra.Command with a cliCtx whose Cfg is
// the supplied compliance config. Tests pass via cobra.Command.Context()
// the same way the real PersistentPreRun does.
func withComplianceCmdCtx(t *testing.T, cmpCfg config.ComplianceConfig) *cobra.Command {
	t.Helper()
	cfg := config.Default()
	cfg.Compliance = cmpCfg
	cmd := &cobra.Command{}
	cmd.SetContext(withCLICtx(context.Background(), &cliCtx{Cfg: cfg}))
	return cmd
}

// resetComplianceFlags clears the package-level flag vars between tests.
// Cobra leaves them populated after a previous test, which would leak
// into the next one's loadPolicyForCmd call.
func resetComplianceFlags(t *testing.T) {
	t.Helper()
	prevPolicy := compliancePolicyFlag
	prevURL := complianceURLFlag
	t.Cleanup(func() {
		compliancePolicyFlag = prevPolicy
		complianceURLFlag = prevURL
	})
	compliancePolicyFlag = ""
	complianceURLFlag = ""
}

// redirectClimConfig redirects the OS-specific user config dir to a
// temp dir so paths.* helpers don't touch the developer's real config.
func redirectClimConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("AppData", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("HOME", dir)
	return dir
}

func TestLoadPolicyForCmd_PolicyFlagBeatsURLAndConfig(t *testing.T) {
	resetComplianceFlags(t)
	redirectClimConfig(t)

	// Local policy on disk.
	tmp := t.TempDir()
	local := filepath.Join(tmp, "local.yaml")
	if err := os.WriteFile(local, []byte("name: local-policy\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	// HTTP server that should NOT be hit.
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		_, _ = w.Write([]byte("name: remote-policy\n"))
	}))
	defer srv.Close()

	compliancePolicyFlag = local
	cmd := withComplianceCmdCtx(t, config.ComplianceConfig{URL: srv.URL})

	p, src, err := loadPolicyForCmd(cmd, false)
	if err != nil {
		t.Fatalf("loadPolicyForCmd: %v", err)
	}
	if p == nil || p.Name != "local-policy" {
		t.Errorf("expected local-policy, got %+v", p)
	}
	if src != local {
		t.Errorf("expected source %s, got %s", local, src)
	}
	if hit {
		t.Error("--policy flag must beat URL — server should not have been hit")
	}
}

func TestLoadPolicyForCmd_URLFlagBeatsConfigURL(t *testing.T) {
	resetComplianceFlags(t)
	redirectClimConfig(t)

	configURLHit := false
	configSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		configURLHit = true
		_, _ = w.Write([]byte("name: config-url-policy\n"))
	}))
	defer configSrv.Close()

	flagSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("name: flag-url-policy\n"))
	}))
	defer flagSrv.Close()

	complianceURLFlag = flagSrv.URL
	cmd := withComplianceCmdCtx(t, config.ComplianceConfig{URL: configSrv.URL})

	p, _, err := loadPolicyForCmd(cmd, true) // force refresh so we always fetch
	if err != nil {
		t.Fatalf("loadPolicyForCmd: %v", err)
	}
	if p == nil || p.Name != "flag-url-policy" {
		t.Errorf("expected flag-url-policy, got %+v", p)
	}
	if configURLHit {
		t.Error("--url flag must beat compliance.url; config URL should not have been hit")
	}
}

func TestLoadPolicyForCmd_ConfigURLBeatsConfigPolicyFile(t *testing.T) {
	resetComplianceFlags(t)
	redirectClimConfig(t)

	tmp := t.TempDir()
	local := filepath.Join(tmp, "local.yaml")
	if err := os.WriteFile(local, []byte("name: local-file-policy\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("name: remote-policy\n"))
	}))
	defer srv.Close()

	cmd := withComplianceCmdCtx(t, config.ComplianceConfig{
		URL:    srv.URL,
		Policy: local,
	})

	p, _, err := loadPolicyForCmd(cmd, true)
	if err != nil {
		t.Fatalf("loadPolicyForCmd: %v", err)
	}
	if p == nil || p.Name != "remote-policy" {
		t.Errorf("expected remote-policy (URL > Policy file), got %+v", p)
	}
}

func TestLoadPolicyForCmd_NoSource_ReturnsHelpfulError(t *testing.T) {
	resetComplianceFlags(t)
	redirectClimConfig(t)

	cmd := withComplianceCmdCtx(t, config.ComplianceConfig{})

	_, _, err := loadPolicyForCmd(cmd, false)
	if err == nil {
		t.Fatal("expected error when nothing is configured")
	}
	if !strings.Contains(err.Error(), "klim security compliance init") {
		t.Errorf("error should suggest `klim compliance init`, got: %v", err)
	}
}

func TestComplianceRefreshCmd_HasURLFlag(t *testing.T) {
	// Regression: the previous version registered --url only on the
	// `check` subcommand, so `refresh --url …` failed with
	// "unknown flag --url".
	if f := complianceRefreshCmd.Flag("url"); f == nil {
		t.Fatal("complianceRefreshCmd should have a --url flag")
	}
}

func TestComplianceCmd_LongHelp_DocumentsURLAndRefresh(t *testing.T) {
	long := complianceCmd.Long
	for _, want := range []string{"--url", "compliance.url", "refresh", "compliance.policy"} {
		if !strings.Contains(long, want) {
			t.Errorf("complianceCmd.Long should mention %q, got:\n%s", want, long)
		}
	}
}
