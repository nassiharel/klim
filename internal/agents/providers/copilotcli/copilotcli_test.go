package copilotcli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/agents"
)

func fixture(t *testing.T) (home, cwd string) {
	t.Helper()
	root := t.TempDir()
	home = filepath.Join(root, "copilot-home")
	cwd = filepath.Join(root, "proj")
	for _, d := range []string{home, cwd} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	writeFile(t, filepath.Join(home, "mcp-config.json"), `{
  "mcpServers": {
    "playwright": {"type": "local", "command": "npx", "args": ["@playwright/mcp@latest"]},
    "context7": {"type": "http", "url": "https://mcp.context7.com/mcp"}
  }
}`)
	writeFile(t, filepath.Join(cwd, ".github", "mcp.json"), `{
  "mcpServers": {"projectsrv": {"type": "local", "command": "/bin/proj"}}
}`)
	writeFile(t, filepath.Join(home, "skills", "personal-tip", "SKILL.md"),
		"---\nname: personal-tip\ndescription: A personal tip\n---\n")
	writeFile(t, filepath.Join(cwd, ".github", "skills", "ci-debug", "SKILL.md"),
		"---\nname: ci-debug\ndescription: Debug CI\n---\n")

	pluginDir := filepath.Join(home, "installed-plugins", "copilot-plugins", "workiq")
	writeFile(t, filepath.Join(pluginDir, "plugin.json"), `{"name": "workiq", "description": "Work IQ", "version": "1.0.0"}`)
	writeFile(t, filepath.Join(pluginDir, "skills", "do-work", "SKILL.md"),
		"---\nname: do-work\ndescription: Do work\n---\n")
	return home, cwd
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestProvider_Skills(t *testing.T) {
	home, cwd := fixture(t)
	p := &Provider{HomeOverride: home, CwdOverride: cwd}
	skills, err := p.Skills(context.Background())
	if err != nil {
		t.Fatalf("Skills: %v", err)
	}
	names := map[string]bool{}
	for _, s := range skills {
		names[s.Name] = true
	}
	for _, want := range []string{"personal-tip", "ci-debug", "do-work"} {
		if !names[want] {
			t.Errorf("missing skill %q in %v", want, names)
		}
	}
}

func TestProvider_MCPs(t *testing.T) {
	home, cwd := fixture(t)
	p := &Provider{HomeOverride: home, CwdOverride: cwd}
	mcps, err := p.MCPs(context.Background())
	if err != nil {
		t.Fatalf("MCPs: %v", err)
	}
	if len(mcps) != 3 {
		t.Fatalf("expected 3 MCPs, got %d: %+v", len(mcps), mcps)
	}
	transports := map[string]string{}
	for _, m := range mcps {
		transports[m.Name] = m.Transport
	}
	if transports["playwright"] != "stdio" {
		t.Errorf("playwright transport = %q, want stdio (local→stdio normalization)", transports["playwright"])
	}
	if transports["context7"] != "http" {
		t.Errorf("context7 transport = %q, want http", transports["context7"])
	}
}

func TestProvider_Plugins(t *testing.T) {
	home, cwd := fixture(t)
	p := &Provider{HomeOverride: home, CwdOverride: cwd}
	plugins, err := p.Plugins(context.Background())
	if err != nil {
		t.Fatalf("Plugins: %v", err)
	}
	if len(plugins) != 1 || plugins[0].Name != "workiq" {
		t.Fatalf("plugins = %+v", plugins)
	}
}

func TestProvider_BuildLaunch(t *testing.T) {
	p := &Provider{BinaryOverride: "/usr/bin/copilot"}
	plan, err := p.BuildLaunch(agents.LaunchSpec{SessionID: "copilot:abc"})
	if err != nil {
		t.Fatalf("BuildLaunch: %v", err)
	}
	if len(plan.Args) != 1 || plan.Args[0] != "--resume=abc" {
		t.Errorf("Args = %v, want [--resume=abc]", plan.Args)
	}
}

// TestProvider_Sessions_RestartCommandUsesEqualsResumeForm pins the
// RestartCommand string form to use `--resume=<id>` (the equals
// form documented in `copilot --help`). Earlier code emitted
// `--resume <id>` (space-separated) which is NOT a documented form
// for Copilot CLI 1.x; a user pasting the snippet into a shell
// would hit "unknown argument" while BuildLaunch's exec-path resume
// (which already uses `--resume=`) keeps working. The two surfaces
// must agree.
func TestProvider_Sessions_RestartCommandUsesEqualsResumeForm(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "copilot-home")
	sid := "11111111-2222-3333-4444-555555555555"
	dir := filepath.Join(home, "session-state", sid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Two fixtures: one with cwd (RestartCommand prefixed by `cd …`)
	// and one without (bare `copilot --resume=…`). Both must use the
	// equals form.
	events := `{"type":"session.start","data":{"sessionId":"` + sid + `","startTime":"2026-06-12T08:00:00.000Z","context":{"cwd":"/dev/foo"}},"id":"e1","timestamp":"2026-06-12T08:00:00.001Z","parentId":null}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(events), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	p := &Provider{HomeOverride: home}
	sessions, err := p.Sessions(context.Background())
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	want := "--resume=" + sid
	if !strings.Contains(sessions[0].RestartCommand, want) {
		t.Errorf("RestartCommand = %q, want substring %q (equals form, matching BuildLaunch)",
			sessions[0].RestartCommand, want)
	}
	if strings.Contains(sessions[0].RestartCommand, "--resume "+sid) {
		t.Errorf("RestartCommand uses space-separated form %q which copilot --help does not document; "+
			"the only documented surface is --resume[=value]",
			sessions[0].RestartCommand)
	}
}

func TestProvider_Sessions_RealLayout(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "copilot-home")
	sid := "0596511c-4387-4cc2-8d08-4302511cc586"
	dir := filepath.Join(home, "session-state", sid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// First-2-lines snippet that mirrors a real Copilot CLI events.jsonl.
	events := `{"data":{"copilotVersion":"1.0.43","producer":"agency","sessionId":"` + sid + `","startTime":"2026-05-13T12:55:58.438Z","version":1},"id":"x","parentId":null,"timestamp":"2026-05-13T12:55:58.438Z","type":"session.start"}
{"type":"session.resume","data":{"resumeTime":"2026-05-13T12:57:08.765Z","eventCount":1,"context":{"cwd":"C:\\dev\\VideoIndexer-CLI","gitRoot":"C:\\dev\\VideoIndexer-CLI","repository":"DefaultCollection/One/VideoIndexer-CLI","hostType":"ado","repositoryHost":"msazure.visualstudio.com"}},"id":"y","timestamp":"2026-05-13T12:57:08.765Z","parentId":"x"}
`
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(events), 0o644); err != nil {
		t.Fatalf("write events: %v", err)
	}

	p := &Provider{HomeOverride: home}
	sessions, err := p.Sessions(context.Background())
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1: %+v", len(sessions), sessions)
	}
	s := sessions[0]
	if s.ID != "copilot:"+sid {
		t.Errorf("ID = %q, want copilot:%s", s.ID, sid)
	}
	if s.ProjectPath != `C:\dev\VideoIndexer-CLI` {
		t.Errorf("ProjectPath = %q, want C:\\dev\\VideoIndexer-CLI", s.ProjectPath)
	}
	if s.Title != "DefaultCollection/One/VideoIndexer-CLI" {
		t.Errorf("Title = %q", s.Title)
	}
	if s.LastModified.IsZero() {
		t.Error("LastModified should be set from session.start startTime")
	}
}

// TestProvider_Sessions_CopilotCLI_1_0_61 covers the event-type
// vocabulary actually emitted by Copilot CLI 1.0.x: tool events use
// `tool.execution_start` / `tool.execution_complete` (NOT `tool.start`),
// per-turn boundaries use `assistant.turn_start` / `assistant.turn_end`
// (NOT `turn.start`), tool name lives at `data.toolName` (NOT
// `data.tool.name`), and the session-context block carries a `branch`
// field. Earlier vocabulary checks left ToolCounts / Branch / TurnCount
// empty, which surfaced as a uniformly-empty Sessions sub-tab in the
// TUI even though the directories were being discovered.
func TestProvider_Sessions_CopilotCLI_1_0_61(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "copilot-home")
	sid := "0ad57e31-6530-43c3-bf9c-afdc40385c93"
	dir := filepath.Join(home, "session-state", sid)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Real-shape snippet captured from a Copilot CLI 1.0.61 transcript.
	// Two assistant turns; the first turn invokes two tools (one
	// completes, one is left pending so LiveState should be "working"
	// when `now` is close to the last event timestamp).
	events := `{"type":"session.start","data":{"sessionId":"` + sid + `","version":1,"producer":"copilot-agent","copilotVersion":"1.0.61","startTime":"2026-06-11T15:29:59.225Z","context":{"cwd":"C:\\dev\\Foo","gitRoot":"C:\\dev\\Foo","branch":"nassiharel/feature-x","repository":"DefaultCollection/Org/Foo","hostType":"ado"}},"id":"e1","timestamp":"2026-06-11T15:30:00.102Z","parentId":null}
{"type":"user.message","data":{"sessionId":"` + sid + `","text":"hi"},"id":"e2","timestamp":"2026-06-11T15:30:05.000Z","parentId":"e1"}
{"type":"assistant.turn_start","data":{"sessionId":"` + sid + `","turnId":"0"},"id":"e3","timestamp":"2026-06-11T15:30:06.000Z","parentId":"e2"}
{"type":"tool.execution_start","data":{"toolCallId":"tc-1","toolName":"glob","arguments":{"pattern":"**/AGENTS.md"},"turnId":"0"},"id":"e4","timestamp":"2026-06-11T15:30:07.000Z","parentId":"e3"}
{"type":"tool.execution_start","data":{"toolCallId":"tc-2","toolName":"glob","arguments":{"pattern":"*.md"},"turnId":"0"},"id":"e5","timestamp":"2026-06-11T15:30:07.100Z","parentId":"e4"}
{"type":"tool.execution_complete","data":{"toolCallId":"tc-1","toolName":"glob","turnId":"0"},"id":"e6","timestamp":"2026-06-11T15:30:08.000Z","parentId":"e5"}
`
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(events), 0o644); err != nil {
		t.Fatalf("write events: %v", err)
	}

	p := &Provider{HomeOverride: home}
	sessions, err := p.Sessions(context.Background())
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	s := sessions[0]

	// Branch (from data.context.branch) drives the branch pill in
	// the tile renderer. Empty here was the most visible "looks
	// empty" symptom.
	if s.Branch != "nassiharel/feature-x" {
		t.Errorf("Branch = %q, want nassiharel/feature-x", s.Branch)
	}

	// ToolCounts populates the 🔌 chip and the Stats tab bar chart.
	if got := s.ToolCounts["glob"]; got != 2 {
		t.Errorf("ToolCounts[glob] = %d, want 2 (got map = %+v)", got, s.ToolCounts)
	}

	// One user.message → TurnCount should be 1 (turn.start aliases
	// are also counted but assistant.turn_start should NOT double-
	// count, since that would diverge from Claude's per-user-prompt
	// semantics).
	if s.TurnCount != 1 {
		t.Errorf("TurnCount = %d, want 1", s.TurnCount)
	}

	// One tool started but never completed → pendingTools > 0 →
	// StateWorking (when within the stale threshold). The stale
	// threshold is 60s; the last event in the fixture is at
	// 15:30:08, so checking against StateWorking OR StateIdle keeps
	// this test independent of wall clock. The important assertion
	// is that LiveState is NOT empty.
	if s.LiveState == "" {
		t.Errorf("LiveState empty; expected derived state from translated tool events")
	}
}

func TestProvider_UpdatePlugin_UnsupportedSurfacesClearError(t *testing.T) {
	p := &Provider{BinaryOverride: "/usr/bin/copilot"}
	prev := PluginUpdateProbe
	PluginUpdateProbe = func(context.Context, string) bool { return false }
	defer func() { PluginUpdateProbe = prev }()

	err := p.UpdatePlugin(context.Background(), "anything")
	if err == nil || !strings.Contains(err.Error(), "update not supported by copilot-cli") {
		t.Errorf("got err=%v, want 'update not supported by copilot-cli'", err)
	}
}

// TestProvider_DeleteSession_RemovesDirectory verifies the Copilot
// DeleteSession path: Copilot CLI 1.x has no `session delete`
// subcommand, so we delete the on-disk session-state dir directly
// instead of shelling out (which would print "Invalid command format"
// to the TUI's inherited stderr and leave the session intact).
//
// Covers all three layout candidates Sessions() supports, plus the
// "not found" error path so callers see a clean message instead of
// silent success.
func TestProvider_DeleteSession_RemovesDirectory(t *testing.T) {
	for _, layout := range []string{"session-state", "sessions", "state"} {
		t.Run(layout, func(t *testing.T) {
			home := t.TempDir()
			sid := "0596511c-4387-4cc2-8d08-4302511cc586"
			dir := filepath.Join(home, layout, sid)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte("{}\n"), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			p := &Provider{HomeOverride: home}

			if err := p.DeleteSession(context.Background(), "copilot:"+sid); err != nil {
				t.Fatalf("DeleteSession: %v", err)
			}
			if _, err := os.Stat(dir); !os.IsNotExist(err) {
				t.Errorf("expected dir removed, got stat err=%v", err)
			}
		})
	}
}

func TestProvider_DeleteSession_MissingReturnsError(t *testing.T) {
	home := t.TempDir()
	p := &Provider{HomeOverride: home}
	err := p.DeleteSession(context.Background(), "copilot:does-not-exist")
	if err == nil {
		t.Fatal("expected error for missing session, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("err=%v, want 'not found'", err)
	}
}

func TestProvider_DeleteSession_EmptyIDIsError(t *testing.T) {
	p := &Provider{HomeOverride: t.TempDir()}
	err := p.DeleteSession(context.Background(), "copilot:")
	if err == nil {
		t.Fatal("expected error for empty id, got nil")
	}
}

// TestProvider_DeleteSession_LeavesOtherSessions covers the
// blast-radius worry: removing one session must NOT touch others
// that happen to share the parent directory. The most common failure
// mode would be a path-construction bug that lands on the parent
// itself.
func TestProvider_DeleteSession_LeavesOtherSessions(t *testing.T) {
	home := t.TempDir()
	keep := filepath.Join(home, "session-state", "keep-me")
	doomed := filepath.Join(home, "session-state", "doomed")
	for _, d := range []string{keep, doomed} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	p := &Provider{HomeOverride: home}

	if err := p.DeleteSession(context.Background(), "copilot:doomed"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	if _, err := os.Stat(doomed); !os.IsNotExist(err) {
		t.Errorf("doomed dir not removed: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("kept dir was unexpectedly removed: %v", err)
	}
}

// TestProvider_DeleteSession_RejectsPathTraversal pins the
// path-traversal guard added in response to PR #93 review. An id
// containing a path separator, "..", or a non-canonical form must
// be rejected BEFORE filepath.Join + os.RemoveAll get a chance to
// escape the Copilot home root.
func TestProvider_DeleteSession_RejectsPathTraversal(t *testing.T) {
	home := t.TempDir()
	// A canary directory OUTSIDE the Copilot home that a successful
	// traversal would delete. If the guard fails, this disappears.
	canary := filepath.Join(filepath.Dir(home), "canary")
	if err := os.MkdirAll(canary, 0o755); err != nil {
		t.Fatalf("mkdir canary: %v", err)
	}
	p := &Provider{HomeOverride: home}

	cases := []struct {
		name string
		id   string
	}{
		{"parent dir", ".."},
		{"current dir", "."},
		{"forward slash", "foo/bar"},
		{"backslash", `foo\bar`},
		{"escaping slash", "../canary"},
		{"escaping backslash", `..\canary`},
		{"deep escape", "../../etc/passwd"},
		{"trailing slash", "foo/"},
		{"leading slash", "/abs/path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := p.DeleteSession(context.Background(), "copilot:"+tc.id)
			if err == nil {
				t.Fatalf("expected error for id %q, got nil", tc.id)
			}
			if !strings.Contains(err.Error(), "invalid") {
				t.Errorf("err=%v, want 'invalid' in message", err)
			}
		})
	}
	// Canary must still exist after every rejection.
	if _, err := os.Stat(canary); err != nil {
		t.Fatalf("canary was unexpectedly removed: %v", err)
	}
}

// TestQuoteForShell pins the POSIX single-quote escaping that
// quoteForShell uses for paths embedded in RestartCommand. The
// previous double-quote approach allowed `$`, backticks, and
// `$(...)` to expand when pasted into a POSIX shell — a
// command-injection vector when ProjectPath was attacker-controlled.
// Single quotes inhibit ALL expansion so the snippet is safe to
// paste even with hostile metacharacters.
func TestQuoteForShell(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", `''`},
		{"plain", "abc", "abc"},
		{"absolute path no spaces", "/home/user/repo", "/home/user/repo"},
		{"with space", "my repo", `'my repo'`},
		{"with dollar sign", "$HOME/x", `'$HOME/x'`},
		{"with backtick", "a`whoami`b", `'a` + "`whoami`" + `b'`},
		{"with cmd subst", "x$(rm -rf /)y", `'x$(rm -rf /)y'`},
		{"with semicolon", "a;rm -rf /;b", `'a;rm -rf /;b'`},
		{"with pipe", "a|b", `'a|b'`},
		{"with ampersand", "a&b", `'a&b'`},
		{"with single quote", "it's", `'it'\''s'`},
		{"with double quote", `say "hi"`, `'say "hi"'`},
		{"with newline", "a\nb", "'a\nb'"},
		{"with glob", "*.go", `'*.go'`},
		// Bash with interactive history expansion (the default on
		// interactive prompts) and zsh both expand unquoted `!`
		// sequences when the snippet is pasted at a prompt — exactly
		// the use case for RestartCommand. Single-quote wrapping
		// (the slow path) suppresses the expansion across both
		// shells; unquoted, `!!` becomes the previous command and
		// can change the pasted line's meaning entirely.
		{"with bang", "a!b", `'a!b'`},
		{"with bang history-expand sequence", "echo !!", `'echo !!'`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := quoteForShell(tc.in); got != tc.want {
				t.Errorf("quoteForShell(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
