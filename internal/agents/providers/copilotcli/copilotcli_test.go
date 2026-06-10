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
