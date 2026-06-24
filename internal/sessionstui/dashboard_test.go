package sessionstui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/agents/costs"
	"github.com/nassiharel/klim/internal/agents/search"
)

func TestRebuildView(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	m := &Model{
		svc:     nil,
		now:     func() time.Time { return now },
		tab:     tabActive,
		groupBy: groupByProject,
		snapshot: &agents.Snapshot{Sessions: []agents.Session{
			{ID: "a", Status: agents.SessionStatusActive, LiveState: agents.StateWorking, LastModified: now.Add(-1 * time.Minute), Title: "alpha", ProjectPath: "/dev/klim"},
			{ID: "b", Status: agents.SessionStatusCompleted, LiveState: agents.StateIdle, LastModified: now.Add(-1 * time.Hour), Title: "beta"},
			{ID: "c", Status: agents.SessionStatusActive, LiveState: agents.StateWaiting, LastModified: now.Add(-2 * time.Minute), Title: "gamma", ProjectPath: "/dev/other", Starred: true},
		}},
	}

	m.rebuildView()
	// Active tab excludes the completed one and orders by:
	// starred-first, then state-rank, then mtime.
	if len(m.flat) != 2 {
		t.Fatalf("active count = %d, want 2", len(m.flat))
	}
	if m.flat[0].ID != "c" {
		t.Errorf("expected starred 'c' first, got %q", m.flat[0].ID)
	}

	m.tab = tabPrevious
	m.rebuildView()
	if len(m.flat) != 1 || m.flat[0].ID != "b" {
		t.Errorf("previous tab: got %d sessions, want [b]", len(m.flat))
	}

	m.tab = tabActive
	m.search = "alpha"
	m.rebuildView()
	if len(m.flat) != 1 || m.flat[0].ID != "a" {
		t.Errorf("search alpha: got %v, want [a]", idsOf(m.flat))
	}
}

func TestNextGroupBy(t *testing.T) {
	t.Parallel()
	if next := nextGroupBy(groupByProject); next != groupByProvider {
		t.Errorf("project → %s, want %s", next, groupByProvider)
	}
	if next := nextGroupBy(groupByProvider); next != groupByNone {
		t.Errorf("provider → %s, want %s", next, groupByNone)
	}
	if next := nextGroupBy(groupByNone); next != groupByProject {
		t.Errorf("none → %s, want %s", next, groupByProject)
	}
}

func TestProviderForSessionID(t *testing.T) {
	t.Parallel()
	tests := map[string]agents.ProviderID{
		"claude:abc":  agents.ProviderClaudeCode,
		"copilot:abc": agents.ProviderCopilotCLI,
		"bare":        "",
	}
	for in, want := range tests {
		if got := providerForSessionID(in); got != want {
			t.Errorf("providerForSessionID(%q) = %q, want %q", in, got, want)
		}
	}
}

func idsOf(in []agents.Session) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = s.ID
	}
	return out
}

// fakeProvider is the minimal stub needed to exercise the resume
// execution path. BuildLaunch returns a fixed (bin, args, cwd)
// triple so the test can pin that the dashboard executes the
// binary directly — never via a shell — regardless of how dirty
// the session's ProjectPath might be.
type fakeProvider struct {
	id          agents.ProviderID
	launchBin   string
	launchArgs  []string
	launchErr   error
	captureSpec *agents.LaunchSpec
	respectCwd  bool // when true, BuildLaunch echoes spec.Cwd into the plan
}

func (f *fakeProvider) ID() agents.ProviderID                                        { return f.id }
func (f *fakeProvider) DisplayName() string                                          { return string(f.id) }
func (f *fakeProvider) Detect(_ context.Context) agents.Status                       { return agents.Status{Installed: true} }
func (f *fakeProvider) Marketplaces(_ context.Context) ([]agents.Marketplace, error) { return nil, nil }
func (f *fakeProvider) Plugins(_ context.Context) ([]agents.Plugin, error)           { return nil, nil }
func (f *fakeProvider) Skills(_ context.Context) ([]agents.Skill, error)             { return nil, nil }
func (f *fakeProvider) MCPs(_ context.Context) ([]agents.MCP, error)                 { return nil, nil }
func (f *fakeProvider) Sessions(_ context.Context) ([]agents.Session, error)         { return nil, nil }
func (f *fakeProvider) AddMarketplace(_ context.Context, _ string) error             { return nil }
func (f *fakeProvider) RemoveMarketplace(_ context.Context, _ string) error          { return nil }
func (f *fakeProvider) InstallPlugin(_ context.Context, _ agents.PluginRef) error    { return nil }
func (f *fakeProvider) UninstallPlugin(_ context.Context, _ string) error            { return nil }
func (f *fakeProvider) EnablePlugin(_ context.Context, _ string, _ bool) error       { return nil }
func (f *fakeProvider) UpdatePlugin(_ context.Context, _ string) error               { return nil }
func (f *fakeProvider) TokenSamples(_ context.Context, _ costs.ScanInput) (costs.ScanResult, error) {
	return costs.ScanResult{}, nil
}
func (f *fakeProvider) SessionTexts(_ context.Context) ([]search.SessionText, error) {
	return nil, nil
}
func (f *fakeProvider) AddMCP(_ context.Context, _ agents.MCPSpec) error    { return nil }
func (f *fakeProvider) RemoveMCP(_ context.Context, _ string) error         { return nil }
func (f *fakeProvider) EnableMCP(_ context.Context, _ string, _ bool) error { return nil }
func (f *fakeProvider) DeleteSession(_ context.Context, _ string) error     { return nil }
func (f *fakeProvider) BuildLaunch(spec agents.LaunchSpec) (agents.ExecPlan, error) {
	if f.captureSpec != nil {
		*f.captureSpec = spec
	}
	if f.launchErr != nil {
		return agents.ExecPlan{}, f.launchErr
	}
	plan := agents.ExecPlan{Bin: f.launchBin, Args: append([]string(nil), f.launchArgs...)}
	if f.respectCwd {
		plan.Cwd = spec.Cwd
	}
	return plan, nil
}

// TestBuildResumeExec_NoShellInjection pins the security fix for
// PR #93 review: resuming a session must NOT pipe RestartCommand
// through /bin/sh -c (or cmd.exe /c) — that path made any shell
// metacharacter in ProjectPath a command-injection vector. The
// dashboard now uses Provider.BuildLaunch to get a (bin, args, cwd)
// triple and execs the binary directly. This test asserts:
//
//  1. cmd.Path is the agent binary, NEVER a shell.
//  2. cmd.Args[0] is the agent binary, not "sh"/"cmd".
//  3. cmd.Dir is set to the session's ProjectPath (so the agent
//     CLI's cwd is correct without any "cd ... &&" prefix).
//  4. A ProjectPath full of shell metacharacters round-trips
//     verbatim into cmd.Dir — it never reaches a shell that could
//     interpret it.
func TestBuildResumeExec_NoShellInjection(t *testing.T) {
	t.Parallel()

	const evilPath = `/tmp/foo; rm -rf $HOME && touch /tmp/pwn ` + "`whoami`"

	stub := &fakeProvider{
		id:         agents.ProviderClaudeCode,
		launchBin:  "/usr/local/bin/claude",
		launchArgs: []string{"-r", "uuid-123"},
		respectCwd: true,
	}
	svc := agents.NewService(2, stub)
	m := &Model{svc: svc}

	cmd, err := m.buildResumeExec(agents.Session{
		ID:          "claude:project-dir",
		Provider:    agents.ProviderClaudeCode,
		ProjectPath: evilPath,
	})
	if err != nil {
		t.Fatalf("buildResumeExec: %v", err)
	}

	// (1) Path must be the agent binary, not a shell.
	if cmd.Path != "/usr/local/bin/claude" {
		t.Errorf("cmd.Path = %q, want the agent binary", cmd.Path)
	}
	for _, banned := range []string{"/bin/sh", "/bin/bash", "/usr/bin/sh", "cmd.exe", "cmd"} {
		if strings.HasSuffix(cmd.Path, banned) {
			t.Errorf("cmd.Path = %q — must NOT be a shell", cmd.Path)
		}
	}

	// (2) Args[0] must be the binary, not a shell argv0.
	if len(cmd.Args) == 0 || cmd.Args[0] != "/usr/local/bin/claude" {
		t.Errorf("cmd.Args[0] = %q, want the agent binary", cmd.Args)
	}

	// (3) + (4) Cwd is the verbatim ProjectPath. The shell
	// metacharacters in evilPath never reach a shell.
	if cmd.Dir != evilPath {
		t.Errorf("cmd.Dir = %q\nwant: %q", cmd.Dir, evilPath)
	}
}

// TestBuildResumeExec_UnknownProviderReturnsError exercises the
// argument-validation branches so a misconfigured session id
// surfaces as a useful error instead of a nil cmd.
func TestBuildResumeExec_UnknownProviderReturnsError(t *testing.T) {
	t.Parallel()
	m := &Model{svc: agents.NewService(2)}
	_, err := m.buildResumeExec(agents.Session{ID: "bare-no-prefix"})
	if err == nil || !strings.Contains(err.Error(), "infer provider") {
		t.Errorf("want infer-provider error, got %v", err)
	}
}
