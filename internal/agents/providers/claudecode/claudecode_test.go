package claudecode

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nassiharel/klim/internal/agents"
)

// fixture builds a fake ~/.claude tree under t.TempDir() so we can exercise
// the filesystem-based read methods without needing a real Claude install.
func fixture(t *testing.T) (home, cwd string) {
	t.Helper()
	root := t.TempDir()
	home = filepath.Join(root, "home")
	cwd = filepath.Join(root, "proj")
	for _, d := range []string{home, cwd, filepath.Join(home, ".claude")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}

	// User skill.
	writeFile(t, filepath.Join(home, ".claude", "skills", "summarize", "SKILL.md"),
		"---\nname: summarize\ndescription: Summarize a diff\n---\nbody\n")
	// Project skill.
	writeFile(t, filepath.Join(cwd, ".claude", "skills", "ship-it", "SKILL.md"),
		"---\nname: ship-it\ndescription: Ship the change\n---\n")
	// User MCP via ~/.claude.json.
	writeFile(t, filepath.Join(home, ".claude.json"), `{
  "mcpServers": {
    "notion": {"type": "http", "url": "https://mcp.notion.com/mcp"}
  },
  "projects": {}
}`)
	// Project MCP via .mcp.json.
	writeFile(t, filepath.Join(cwd, ".mcp.json"), `{
  "mcpServers": {
    "github": {"command": "npx", "args": ["-y", "@modelcontextprotocol/server-github"]}
  }
}`)
	// Plugin install cache.
	pluginDir := filepath.Join(home, ".claude", "plugins", "claude-plugins-official", "commit-commands")
	writeFile(t, filepath.Join(pluginDir, ".claude-plugin", "plugin.json"), `{
  "name": "commit-commands",
  "description": "Git commit helpers",
  "version": "1.0.0",
  "author": {"name": "Anthropic"}
}`)
	// Plugin-bundled skill.
	writeFile(t, filepath.Join(pluginDir, "skills", "commit", "SKILL.md"),
		"---\nname: commit\ndescription: Create a git commit\n---\n")
	// Session project dir.
	if err := os.MkdirAll(filepath.Join(home, ".claude", "projects", "home%2Fuser%2Frepo"), 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
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

func TestProvider_Skills_ReadsUserAndProject(t *testing.T) {
	home, cwd := fixture(t)
	p := &Provider{HomeOverride: home, CwdOverride: cwd, BinaryOverride: "/bin/echo"}

	skills, err := p.Skills(context.Background())
	if err != nil {
		t.Fatalf("Skills: %v", err)
	}
	if len(skills) < 3 {
		t.Fatalf("expected >=3 skills (user/project/plugin), got %d: %+v", len(skills), skills)
	}
	names := map[string]bool{}
	for _, s := range skills {
		names[s.Name] = true
	}
	for _, want := range []string{"summarize", "ship-it", "commit"} {
		if !names[want] {
			t.Errorf("missing skill %q in %v", want, names)
		}
	}
}

func TestProvider_MCPs_UserAndProject(t *testing.T) {
	home, cwd := fixture(t)
	p := &Provider{HomeOverride: home, CwdOverride: cwd}
	mcps, err := p.MCPs(context.Background())
	if err != nil {
		t.Fatalf("MCPs: %v", err)
	}
	if len(mcps) != 2 {
		t.Fatalf("expected 2 MCPs (user notion + project github), got %d: %+v", len(mcps), mcps)
	}
	var sawNotion, sawGitHub bool
	for _, m := range mcps {
		if m.Name == "notion" {
			sawNotion = true
			if m.Transport != "http" {
				t.Errorf("notion transport = %q, want http", m.Transport)
			}
			if m.Scope != agents.ScopeUser {
				t.Errorf("notion scope = %q, want user", m.Scope)
			}
		}
		if m.Name == "github" {
			sawGitHub = true
			if m.Transport != "stdio" {
				t.Errorf("github transport = %q, want stdio", m.Transport)
			}
			if m.Scope != agents.ScopeProject {
				t.Errorf("github scope = %q, want project", m.Scope)
			}
		}
	}
	if !sawNotion || !sawGitHub {
		t.Errorf("missing expected MCP names")
	}
}

func TestProvider_Plugins(t *testing.T) {
	home, cwd := fixture(t)
	p := &Provider{HomeOverride: home, CwdOverride: cwd}
	plugins, err := p.Plugins(context.Background())
	if err != nil {
		t.Fatalf("Plugins: %v", err)
	}
	if len(plugins) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(plugins))
	}
	pl := plugins[0]
	if pl.Name != "commit-commands" {
		t.Errorf("name = %q, want commit-commands", pl.Name)
	}
	if pl.Marketplace != "claude-plugins-official" {
		t.Errorf("marketplace = %q", pl.Marketplace)
	}
	if !pl.Installed {
		t.Error("expected Installed=true")
	}
}

func TestProvider_Sessions(t *testing.T) {
	home, cwd := fixture(t)
	p := &Provider{HomeOverride: home, CwdOverride: cwd}
	sessions, err := p.Sessions(context.Background())
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].ProjectPath != "home/user/repo" {
		t.Errorf("ProjectPath = %q, want home/user/repo", sessions[0].ProjectPath)
	}
}

func TestProvider_BuildLaunch_Variants(t *testing.T) {
	p := &Provider{BinaryOverride: "/usr/bin/claude"}
	cases := []struct {
		spec     agents.LaunchSpec
		wantArgs []string
	}{
		{agents.LaunchSpec{}, nil},
		{agents.LaunchSpec{SessionID: "claude:home%2Fuser%2Frepo"}, []string{"-r", "home%2Fuser%2Frepo"}},
		{agents.LaunchSpec{Prompt: "hi"}, []string{"-p", "hi"}},
		{agents.LaunchSpec{ExtraArgs: []string{"--model", "sonnet"}}, []string{"--model", "sonnet"}},
	}
	for _, c := range cases {
		got, err := p.BuildLaunch(c.spec)
		if err != nil {
			t.Errorf("BuildLaunch(%+v) err=%v", c.spec, err)
			continue
		}
		if !sliceEqual(got.Args, c.wantArgs) {
			t.Errorf("BuildLaunch(%+v).Args = %v, want %v", c.spec, got.Args, c.wantArgs)
		}
		if got.Bin != "/usr/bin/claude" {
			t.Errorf("Bin = %q", got.Bin)
		}
	}
}

func sliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
