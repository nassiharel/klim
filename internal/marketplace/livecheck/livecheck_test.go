//go:build integration

package livecheck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/klim/internal/registry"
)

// cmdTimeout bounds a single package-manager subprocess call. Most are
// fast (metadata fetch), but winget/choco occasionally stall on network.
const cmdTimeout = 45 * time.Second

// toolsDir returns the absolute path to marketplace/tools resolved from
// the location of this test file so the test runs regardless of cwd.
func toolsDir(t *testing.T) string {
	t.Helper()
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/marketplace/livecheck/livecheck_test.go → repo root is 3 up.
	return filepath.Join(filepath.Dir(self), "..", "..", "..", "marketplace", "tools")
}

func loadTools(t *testing.T) []registry.ToolDef {
	t.Helper()
	dir := toolsDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read tools dir %s: %v", dir, err)
	}
	var defs []registry.ToolDef
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var td registry.ToolDef
		if err := yaml.Unmarshal(data, &td); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if td.Name == "" {
			t.Fatalf("%s: missing name", path)
		}
		defs = append(defs, td)
	}
	return defs
}

// pmAvailable reports whether the given package-manager binary is on PATH.
func pmAvailable(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}

// checkFn verifies a single package ID exists in its package manager.
// Implementations should return nil on "found", a non-nil error on
// "not found" / unreachable / protocol error. Timeouts bubble up as errors.
type checkFn func(ctx context.Context, id string) error

// packageManager bundles everything needed to run a liveness check for one PM.
type packageManager struct {
	source registry.InstallSource
	bin    string
	// supportedOS lists GOOS values on which this PM is meaningful to check.
	// Empty means "any OS" (e.g. npm works everywhere).
	supportedOS []string
	// pkgID extracts this manager's identifier from a tool definition.
	pkgID func(registry.PackageDef) string
	// check runs the existence probe for a given id.
	check checkFn
}

func matches(got string, list []string) bool {
	if len(list) == 0 {
		return true
	}
	for _, v := range list {
		if v == got {
			return true
		}
	}
	return false
}

// managers enumerates every package manager klim integrates with, along
// with the exact command used to prove an ID resolves.
var managers = []packageManager{
	{
		source:      registry.SourceWinget,
		bin:         "winget",
		supportedOS: []string{"windows"},
		pkgID:       func(p registry.PackageDef) string { return p.Winget },
		check:       checkWinget,
	},
	{
		source:      registry.SourceChoco,
		bin:         "choco",
		supportedOS: []string{"windows"},
		pkgID:       func(p registry.PackageDef) string { return p.Choco },
		check:       checkChoco,
	},
	{
		source:      registry.SourceScoop,
		bin:         "scoop",
		supportedOS: []string{"windows"},
		pkgID:       func(p registry.PackageDef) string { return p.Scoop },
		check:       checkScoop,
	},
	{
		source:      registry.SourceBrew,
		bin:         "brew",
		supportedOS: []string{"darwin", "linux"},
		pkgID:       func(p registry.PackageDef) string { return p.Brew },
		check:       checkBrew,
	},
	{
		source:      registry.SourceApt,
		bin:         "apt-cache",
		supportedOS: []string{"linux"},
		pkgID:       func(p registry.PackageDef) string { return p.Apt },
		check:       checkApt,
	},
	{
		source:      registry.SourceSnap,
		bin:         "snap",
		supportedOS: []string{"linux"},
		pkgID:       func(p registry.PackageDef) string { return p.Snap },
		check:       checkSnap,
	},
	{
		source: registry.SourceNPM,
		bin:    "npm",
		// npm is cross-platform — no OS restriction.
		pkgID: func(p registry.PackageDef) string { return p.NPM },
		check: checkNPM,
	},
}

// TestMarketplacePackageIDsResolve walks every tool YAML and, for every
// package manager supported on the current host, verifies the declared
// package ID resolves to a real package. Each package manager runs as a
// subtest and calls t.Skipf when its binary is not on PATH, so skips
// show up explicitly in the test output. If no package manager is
// available on this host the parent test is marked skipped.
func TestMarketplacePackageIDsResolve(t *testing.T) {
	defs := loadTools(t)
	if len(defs) == 0 {
		t.Fatal("no tool YAMLs found")
	}

	var okCount, failCount, attempted int
	for _, pm := range managers {
		pm := pm
		t.Run(string(pm.source), func(t *testing.T) {
			if !matches(runtime.GOOS, pm.supportedOS) {
				t.Skipf("%s not applicable on %s", pm.source, runtime.GOOS)
			}
			if !pmAvailable(pm.bin) {
				t.Skipf("%s binary %q not on PATH", pm.source, pm.bin)
			}
			attempted++
			for _, d := range defs {
				d := d
				id := pm.pkgID(d.Packages)
				if id == "" {
					continue
				}
				t.Run(d.Name, func(t *testing.T) {
					// Subtests run sequentially within a PM group so we
					// don't hammer any one service with parallel queries
					// (winget/choco/apt-cache are slow and rate-limitable).
					ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
					defer cancel()
					if err := pm.check(ctx, id); err != nil {
						failCount++
						t.Errorf("%s %q not found: %v", pm.source, id, err)
						return
					}
					okCount++
				})
			}
		})
	}

	if attempted == 0 {
		t.Skip("no supported package manager available on this host; nothing probed")
	}

	t.Logf("livecheck summary: %d resolved, %d failed (%d tools, %d managers attempted)",
		okCount, failCount, len(defs), attempted)
}

// --- per-package-manager probes ---

// runCmd runs the given command and returns (stdout, exit error). stderr
// is discarded — the reason an ID doesn't exist is encoded in the exit
// code plus sometimes stdout; stderr is usually progress noise.
func runCmd(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil
	cmd.Stdin = nil
	err := cmd.Run()
	return stdout.String(), err
}

func checkWinget(ctx context.Context, id string) error {
	// winget show exits 0 on found, non-zero on not found.
	_, err := runCmd(ctx, "winget", "show", "--id", id, "--exact",
		"--accept-source-agreements", "--disable-interactivity")
	return err
}

func checkChoco(ctx context.Context, id string) error {
	// choco search always exits 0; presence is determined by "name|version" output.
	out, err := runCmd(ctx, "choco", "search", "--exact", "--limit-output", id)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], id) {
			return nil
		}
	}
	return fmt.Errorf("not present in choco search output")
}

func checkScoop(ctx context.Context, id string) error {
	// scoop info exits non-zero (or prints "Could not find manifest") when missing.
	out, err := runCmd(ctx, "scoop", "info", id)
	if err != nil {
		return err
	}
	if strings.Contains(strings.ToLower(out), "could not find manifest") {
		return fmt.Errorf("scoop reports no manifest")
	}
	return nil
}

func checkBrew(ctx context.Context, id string) error {
	// brew info --json=v2 returns exit 0 + populated formulae/casks on found.
	out, err := runCmd(ctx, "brew", "info", "--json=v2", id)
	if err != nil {
		return err
	}
	var result struct {
		Formulae []any `json:"formulae"`
		Casks    []any `json:"casks"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		return fmt.Errorf("brew info JSON parse: %w", err)
	}
	if len(result.Formulae) == 0 && len(result.Casks) == 0 {
		return fmt.Errorf("no formula or cask returned")
	}
	return nil
}

func checkApt(ctx context.Context, id string) error {
	// apt-cache policy always exits 0; presence = a real Candidate line.
	out, err := runCmd(ctx, "apt-cache", "policy", id)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Candidate:") {
			continue
		}
		cand := strings.TrimSpace(strings.TrimPrefix(line, "Candidate:"))
		if cand == "" || cand == "(none)" {
			return fmt.Errorf("apt candidate is (none)")
		}
		return nil
	}
	return fmt.Errorf("no Candidate line in apt-cache policy output")
}

func checkSnap(ctx context.Context, id string) error {
	// snap info exits non-zero when the snap isn't in any channel.
	_, err := runCmd(ctx, "snap", "info", id)
	return err
}

func checkNPM(ctx context.Context, id string) error {
	// npm view <pkg> version exits non-zero if the package doesn't exist.
	_, err := runCmd(ctx, "npm", "view", id, "version")
	return err
}
