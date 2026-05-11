package postcheck

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nassiharel/klim/internal/registry"
)

// seedBinary writes a no-op executable into dir/name and returns its
// full path. On POSIX the file is chmod +x; on Windows we just give
// it the .exe extension (no chmod needed for stat to find it).
func seedBinary(t *testing.T, dir, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" && !strings.HasSuffix(name, ".exe") {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	body := "#!/bin/sh\nexit 0\n"
	if runtime.GOOS == "windows" {
		body = "@echo off\r\nexit /b 0\r\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("seed binary %q: %v", name, err)
	}
	return path
}

func TestRun_EmptyInputIsClean(t *testing.T) {
	r := Run(nil, nil, Options{SkipManagerIntegrity: true})
	if r.Failed {
		t.Fatalf("empty input should not fail; got %+v", r)
	}
	if r.Took <= 0 {
		t.Errorf("Result.Took should be measured")
	}
}

func TestRun_PreExistingFailureBecomesWarnNotFail(t *testing.T) {
	// Realistic pre-existing failure: the tool isn't in the
	// before-snapshot at all (finder wouldn't have recorded an
	// Instance for a binary that doesn't exist), but somehow shows
	// up in the after-snapshot — typically because the user
	// hand-edited a manifest or because we're rescanning after a
	// failed install. Either way it's not a regression vs.
	// pre-apply, so postcheck should warn rather than fail.
	t.Setenv("PATH", t.TempDir())
	bad := registry.Tool{
		Name:        "ghost",
		BinaryNames: []string{"definitely-not-on-path"},
		Instances:   []registry.Instance{{Path: "/no/such/path", Version: "1", Source: registry.SourceBrew}},
	}
	r := Run(
		[]registry.Tool{},    // before — ghost was NOT installed
		[]registry.Tool{bad}, // after  — ghost shows up "installed" but its binary is missing
		Options{SkipManagerIntegrity: true, PerProbeTimeout: 200 * time.Millisecond},
	)
	if r.Failed {
		t.Errorf("pre-existing-style failure should not flip Failed=true: %+v", r.Checks)
	}
	for _, c := range r.Checks {
		if c.Name == "shell resolution" && c.Status == StatusFail {
			t.Errorf("shell resolution should be Warn for pre-existing miss, got %q", c.Status)
		}
	}
}

func TestRun_RegressionIsFlaggedAsFail(t *testing.T) {
	// Before: binary exists on PATH. After: PATH points elsewhere
	// so the same tool no longer resolves. That's a regression.
	dir := t.TempDir()
	bin := seedBinary(t, dir, "demo")
	good := registry.Tool{
		Name:        "demo",
		BinaryNames: []string{"demo"},
		Instances:   []registry.Instance{{Path: bin, Version: "1", Source: registry.SourceBrew}},
	}

	// Pre-apply: binary is on PATH so it resolves cleanly.
	t.Setenv("PATH", dir)
	before := Run([]registry.Tool{good}, []registry.Tool{good}, Options{SkipManagerIntegrity: true})
	if before.Failed {
		t.Fatalf("pre-apply should be clean: %+v", before.Checks)
	}

	// Post-apply: PATH points to an empty dir, so resolution fails.
	t.Setenv("PATH", t.TempDir())
	after := Run([]registry.Tool{good}, []registry.Tool{good}, Options{SkipManagerIntegrity: true, PerProbeTimeout: 200 * time.Millisecond})
	if !after.Failed {
		t.Errorf("regression should flip Failed=true: %+v", after.Checks)
	}
	found := false
	for _, r := range after.Regressions {
		if r == "demo" {
			found = true
		}
	}
	if !found {
		t.Errorf("regressions list should mention demo: %+v", after.Regressions)
	}
}

func TestBinaryValidation_MissingFileIsBroken(t *testing.T) {
	tools := []registry.Tool{{
		Name:        "ghost",
		BinaryNames: []string{"ghost"},
		Instances:   []registry.Instance{{Path: filepath.Join(t.TempDir(), "absent"), Version: "1", Source: registry.SourceBrew}},
	}}
	r := Run(nil, tools, Options{SkipManagerIntegrity: true, PerProbeTimeout: 200 * time.Millisecond})
	if !r.Failed {
		t.Errorf("missing binary should fail: %+v", r.Checks)
	}
}

func TestBinaryValidation_RespectsConcurrencyCap(t *testing.T) {
	// Seed 20 binaries and probe with concurrency=1 — the test
	// just verifies the worker pool completes without panic.
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	var tools []registry.Tool
	for i := 0; i < 20; i++ {
		name := "demo" + strings.Repeat("x", i+1)
		bin := seedBinary(t, dir, name)
		tools = append(tools, registry.Tool{
			Name:        name,
			BinaryNames: []string{name},
			Instances:   []registry.Instance{{Path: bin, Version: "1", Source: registry.SourceBrew}},
		})
	}
	r := Run(tools, tools, Options{
		SkipManagerIntegrity: true,
		Concurrency:          1,
		PerProbeTimeout:      400 * time.Millisecond,
		WallClockBudget:      30 * time.Second,
	})
	// On POSIX our seed binaries are real shell scripts so they
	// pass the probe. On Windows the .bat probe may not work for
	// every flag we try, but the binary at least stats successfully
	// — that's enough for the test (no panic, no hang).
	if len(r.Checks) == 0 {
		t.Errorf("no checks produced")
	}
}

func TestPATHConsistency_FlagsDuplicateAsWarn(t *testing.T) {
	dir := t.TempDir()
	sep := ":"
	if runtime.GOOS == "windows" {
		sep = ";"
	}
	t.Setenv("PATH", strings.Join([]string{dir, dir}, sep))
	r := Run(nil, nil, Options{SkipManagerIntegrity: true})
	for _, c := range r.Checks {
		if c.Name == "PATH consistency" {
			if c.Status != StatusWarn {
				t.Errorf("duplicate PATH should warn, got %q (items=%v)", c.Status, c.Items)
			}
			return
		}
	}
	t.Errorf("PATH consistency check not present")
}

func TestManagerIntegrity_SkipsMissingManagers(t *testing.T) {
	// Tool installed via a PM that almost certainly isn't on PATH
	// in the test environment (`scoop` on Linux, `apt` on Windows).
	var src registry.InstallSource
	switch runtime.GOOS {
	case "linux", "darwin":
		src = registry.SourceScoop
	default:
		src = registry.SourceApt
	}
	tools := []registry.Tool{{
		Name:      "demo",
		Instances: []registry.Instance{{Path: filepath.Join(t.TempDir(), "demo"), Version: "1", Source: src}},
	}}
	r := Run(tools, tools, Options{PerProbeTimeout: 500 * time.Millisecond})
	for _, c := range r.Checks {
		if c.Name == "manager integrity" {
			if c.Status == StatusFail {
				t.Errorf("missing PM should be Skip, not Fail (was %s)", c.Status)
			}
			return
		}
	}
}

func TestRun_WallClockBudgetSurfacedAsSkipNotPanic(t *testing.T) {
	// 1ms budget — every probe times out, but Run should still
	// return cleanly with checks reporting Skip / partial results.
	r := Run(nil, nil, Options{
		SkipManagerIntegrity: true,
		WallClockBudget:      time.Millisecond,
		PerProbeTimeout:      time.Microsecond,
	})
	if len(r.Checks) == 0 {
		t.Errorf("Run should still produce checks even at zero budget")
	}
}

func TestCandidateBinaries_AddsWindowsExtensions(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only behaviour")
	}
	got := candidateBinaries(registry.Tool{
		Name:        "foo",
		BinaryNames: []string{"foo"},
	})
	wantContains := []string{"foo", "foo.exe", "foo.cmd"}
	for _, w := range wantContains {
		found := false
		for _, g := range got {
			if strings.EqualFold(g, w) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("candidate list missing %q: %v", w, got)
		}
	}
}

func TestRun_PerCheckTookIsRecorded(t *testing.T) {
	r := Run(nil, nil, Options{SkipManagerIntegrity: true})
	for _, c := range r.Checks {
		if c.Took < 0 {
			t.Errorf("check %q has negative Took", c.Name)
		}
	}
}
