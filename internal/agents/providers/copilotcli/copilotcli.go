// Package copilotcli implements the GitHub Copilot CLI agent provider.
//
// Filesystem layout it understands:
//
//	~/.copilot/mcp-config.json                  User-scope MCP servers
//	~/.copilot/installed-plugins/<mp>/<plugin>/ Plugin install cache
//	~/.copilot/skills/<name>/                   Personal skills
//	~/.copilot/agents/<name>.md                 Personal subagent definitions
//	<project>/.github/skills/                   Project skills
//	<project>/.github/mcp.json                  Project MCP servers
//	<project>/.mcp.json                         Project MCP servers (fallback)
//
// All read methods are filesystem-based and best-effort. Mutations
// shell out to `copilot plugin …` / `copilot mcp …`.
//
// COPILOT_HOME, when set, overrides the default ~/.copilot location.
package copilotcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/agents/enrich"
	"gopkg.in/yaml.v3"
)

const binaryName = "copilot"

// Provider implements agents.Provider for GitHub Copilot CLI.
type Provider struct {
	// HomeOverride lets tests point at a fixture filesystem instead of
	// $HOME / $COPILOT_HOME. Empty means honor COPILOT_HOME, then HOME.
	HomeOverride string
	// BinaryOverride lets tests stub the `copilot` binary lookup.
	BinaryOverride string
	// CwdOverride lets tests inject a project root for scope=project scans.
	CwdOverride string
}

// New constructs a default Provider.
func New() *Provider { return &Provider{} }

// ID returns the stable provider identifier.
func (p *Provider) ID() agents.ProviderID { return agents.ProviderCopilotCLI }

// DisplayName returns the human-readable provider name.
func (p *Provider) DisplayName() string { return "GitHub Copilot CLI" }

// Detect locates the `copilot` binary and runs `copilot --version`.
func (p *Provider) Detect(ctx context.Context) agents.Status {
	bin := p.binary()
	if bin == "" {
		return agents.Status{Installed: false}
	}
	out, err := exec.CommandContext(ctx, bin, "--version").Output()
	if err != nil {
		return agents.Status{Installed: true, BinPath: bin, Error: err.Error()}
	}
	return agents.Status{
		Installed: true,
		BinPath:   bin,
		Version:   firstLine(string(out)),
	}
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func (p *Provider) copilotHome() string {
	if p.HomeOverride != "" {
		return p.HomeOverride
	}
	if env := os.Getenv("COPILOT_HOME"); env != "" {
		return env
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(h, ".copilot")
}

func (p *Provider) cwd() string {
	if p.CwdOverride != "" {
		return p.CwdOverride
	}
	if d, err := os.Getwd(); err == nil {
		return d
	}
	return ""
}

func (p *Provider) binary() string {
	if p.BinaryOverride != "" {
		return p.BinaryOverride
	}
	bin, err := exec.LookPath(binaryName)
	if err != nil {
		return ""
	}
	return bin
}

// ---------- Marketplaces ----------

// Marketplaces returns the marketplaces Copilot CLI knows about. The
// two default marketplaces (`copilot-plugins`, `awesome-copilot`) are
// always surfaced; any extra ones come from subdirectories of
// `installed-plugins/`.
func (p *Provider) Marketplaces(ctx context.Context) ([]agents.Marketplace, error) {
	ms := []agents.Marketplace{
		{
			ID:          "copilot-plugins",
			Name:        "copilot-plugins",
			DisplayName: "GitHub Copilot Plugins",
			Description: "GitHub's official Copilot CLI plugin marketplace",
			Provider:    p.ID(),
			Owner:       "github",
			URL:         "https://github.com/github/copilot-plugins",
			Source:      agents.SourceCatalogCopilot,
			Installed:   true,
		},
		{
			ID:          "awesome-copilot",
			Name:        "awesome-copilot",
			DisplayName: "Awesome Copilot",
			Description: "Community-curated Copilot plugins and skills",
			Provider:    p.ID(),
			Owner:       "github",
			URL:         "https://github.com/github/awesome-copilot",
			Source:      agents.SourceCatalogCopilot,
			Installed:   true,
		},
	}
	root := filepath.Join(p.copilotHome(), "installed-plugins")
	entries, err := os.ReadDir(root)
	if err == nil {
		known := map[string]bool{"copilot-plugins": true, "awesome-copilot": true, "_direct": true}
		for _, e := range entries {
			if !e.IsDir() || known[e.Name()] {
				continue
			}
			ms = append(ms, agents.Marketplace{
				ID:        e.Name(),
				Name:      e.Name(),
				Provider:  p.ID(),
				Source:    agents.SourceLocalCopilot,
				Installed: true,
			})
		}
	}
	return ms, nil
}

// ---------- Plugins ----------

// pluginManifest mirrors Copilot's `plugin.json` schema. Copilot accepts
// the manifest in several locations; we use whichever is present.
type pluginManifest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
	Author      struct {
		Name  string `json:"name,omitempty"`
		Email string `json:"email,omitempty"`
		URL   string `json:"url,omitempty"`
	} `json:"author,omitempty"`
	Homepage   string   `json:"homepage,omitempty"`
	Repository string   `json:"repository,omitempty"`
	License    string   `json:"license,omitempty"`
	Keywords   []string `json:"keywords,omitempty"`
	Category   string   `json:"category,omitempty"`
}

// Plugins scans ~/.copilot/installed-plugins/<marketplace>/<plugin>/.
// The manifest is checked at four canonical locations (`.plugin/plugin.json`,
// `plugin.json`, `.github/plugin/plugin.json`, `.claude-plugin/plugin.json`).
func (p *Provider) Plugins(ctx context.Context) ([]agents.Plugin, error) {
	root := filepath.Join(p.copilotHome(), "installed-plugins")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil
	}
	var plugins []agents.Plugin
	for _, mp := range entries {
		if !mp.IsDir() {
			continue
		}
		mpName := mp.Name()
		mpDir := filepath.Join(root, mpName)
		pluginEntries, err := os.ReadDir(mpDir)
		if err != nil {
			continue
		}
		for _, pe := range pluginEntries {
			if !pe.IsDir() {
				continue
			}
			pluginDir := filepath.Join(mpDir, pe.Name())
			m, _ := findPluginManifest(pluginDir)
			if m == nil {
				continue
			}
			plugins = append(plugins, agents.Plugin{
				ID:          mpName + "/" + m.Name,
				Name:        m.Name,
				Description: m.Description,
				Version:     m.Version,
				Author:      m.Author.Name,
				Homepage:    m.Homepage,
				Repository:  m.Repository,
				License:     m.License,
				Keywords:    m.Keywords,
				Provider:    p.ID(),
				Marketplace: mpName,
				Installed:   true,
				Enabled:     true,
				InstallPath: pluginDir,
				Scope:       agents.ScopeUser,
				Source:      agents.SourceLocalCopilot,
			})
		}
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].Name < plugins[j].Name })
	return plugins, nil
}

func findPluginManifest(pluginDir string) (*pluginManifest, error) {
	candidates := []string{
		filepath.Join(pluginDir, ".plugin", "plugin.json"),
		filepath.Join(pluginDir, "plugin.json"),
		filepath.Join(pluginDir, ".github", "plugin", "plugin.json"),
		filepath.Join(pluginDir, ".claude-plugin", "plugin.json"),
	}
	for _, c := range candidates {
		data, err := os.ReadFile(c)
		if err != nil {
			continue
		}
		var m pluginManifest
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		if m.Name == "" {
			continue
		}
		return &m, nil
	}
	return nil, errors.New("no plugin manifest found")
}

// ---------- Skills ----------

type skillFrontmatter struct {
	Name               string `yaml:"name"`
	Description        string `yaml:"description"`
	WhenToUse          string `yaml:"when_to_use"`
	AllowedTools       string `yaml:"allowed-tools"`
	ArgumentHint       string `yaml:"argument-hint"`
	Model              string `yaml:"model"`
	DisableModelInvoke bool   `yaml:"disable-model-invocation"`
	UserInvocable      *bool  `yaml:"user-invocable"`
}

// Skills scans personal (`~/.copilot/skills/`, also `~/.agents/skills/`),
// project (`<cwd>/.github/skills/`, `<cwd>/.agents/skills/`,
// `<cwd>/.claude/skills/`), and plugin (`installed-plugins/.../skills/`).
func (p *Provider) Skills(ctx context.Context) ([]agents.Skill, error) {
	var skills []agents.Skill

	for _, root := range p.personalSkillRoots() {
		skills = append(skills, scanSkillDir(root, p.ID(), agents.ScopeUser, "")...)
	}
	for _, root := range p.projectSkillRoots() {
		skills = append(skills, scanSkillDir(root, p.ID(), agents.ScopeProject, "")...)
	}

	// Plugin-bundled skills.
	root := filepath.Join(p.copilotHome(), "installed-plugins")
	if entries, err := os.ReadDir(root); err == nil {
		for _, mp := range entries {
			if !mp.IsDir() {
				continue
			}
			pluginEntries, err := os.ReadDir(filepath.Join(root, mp.Name()))
			if err != nil {
				continue
			}
			for _, pe := range pluginEntries {
				if !pe.IsDir() {
					continue
				}
				skillsDir := filepath.Join(root, mp.Name(), pe.Name(), "skills")
				skills = append(skills, scanSkillDir(skillsDir, p.ID(), agents.ScopePlugin, pe.Name())...)
			}
		}
	}
	return skills, nil
}

func (p *Provider) personalSkillRoots() []string {
	h, err := os.UserHomeDir()
	roots := []string{filepath.Join(p.copilotHome(), "skills")}
	if err == nil {
		roots = append(roots, filepath.Join(h, ".agents", "skills"))
	}
	return roots
}

func (p *Provider) projectSkillRoots() []string {
	cw := p.cwd()
	if cw == "" {
		return nil
	}
	return []string{
		filepath.Join(cw, ".github", "skills"),
		filepath.Join(cw, ".agents", "skills"),
		filepath.Join(cw, ".claude", "skills"),
	}
}

func scanSkillDir(root string, provider agents.ProviderID, scope agents.Scope, sourcePlugin string) []agents.Skill {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []agents.Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillPath := filepath.Join(root, e.Name(), "SKILL.md")
		fm, err := readSkillFrontmatter(skillPath)
		if err != nil {
			continue
		}
		name := fm.Name
		if name == "" {
			name = e.Name()
		}
		userInv := true
		if fm.UserInvocable != nil {
			userInv = *fm.UserInvocable
		}
		out = append(out, agents.Skill{
			ID:                 string(provider) + ":" + string(scope) + ":" + name,
			Name:               name,
			Description:        fm.Description,
			WhenToUse:          fm.WhenToUse,
			AllowedTools:       fm.AllowedTools,
			ArgumentHint:       fm.ArgumentHint,
			Model:              fm.Model,
			DisableModelInvoke: fm.DisableModelInvoke,
			UserInvocable:      userInv,
			Provider:           provider,
			SourcePlugin:       sourcePlugin,
			Scope:              scope,
			Path:               skillPath,
			Enabled:            true,
			Source:             agents.SourceLocalCopilot,
		})
	}
	return out
}

func readSkillFrontmatter(path string) (*skillFrontmatter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	const fence = "---"
	lines := bytes.SplitN(data, []byte("\n"), 2)
	if len(lines) < 2 || strings.TrimSpace(string(lines[0])) != fence {
		return &skillFrontmatter{}, nil
	}
	rest := lines[1]
	end := bytes.Index(rest, []byte("\n"+fence))
	if end < 0 {
		return &skillFrontmatter{}, nil
	}
	yamlBlock := rest[:end]
	var fm skillFrontmatter
	if err := yaml.Unmarshal(yamlBlock, &fm); err != nil {
		return nil, err
	}
	return &fm, nil
}

// ---------- MCPs ----------

type mcpServerConfig struct {
	Type    string            `json:"type,omitempty"` // "local" = stdio | "http" | "sse"
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	URL     string            `json:"url,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Tools   []string          `json:"tools,omitempty"`
}

// MCPs reads `~/.copilot/mcp-config.json` (user scope) and
// `<cwd>/.github/mcp.json` / `<cwd>/.mcp.json` (project scope).
func (p *Provider) MCPs(ctx context.Context) ([]agents.MCP, error) {
	var mcps []agents.MCP

	userCfg := filepath.Join(p.copilotHome(), "mcp-config.json")
	mcps = append(mcps, parseMCPFile(userCfg, agents.ScopeUser, p.ID())...)

	if cw := p.cwd(); cw != "" {
		for _, projFile := range []string{
			filepath.Join(cw, ".github", "mcp.json"),
			filepath.Join(cw, ".mcp.json"),
		} {
			mcps = append(mcps, parseMCPFile(projFile, agents.ScopeProject, p.ID())...)
		}
	}
	return mcps, nil
}

func parseMCPFile(path string, scope agents.Scope, provider agents.ProviderID) []agents.MCP {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var doc struct {
		MCPServers map[string]mcpServerConfig `json:"mcpServers"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return nil
	}
	var out []agents.MCP
	for name, cfg := range doc.MCPServers {
		t := cfg.Type
		switch t {
		case "local":
			t = "stdio"
		case "":
			t = "stdio"
		}
		envKeys := make([]string, 0, len(cfg.Env))
		for k := range cfg.Env {
			envKeys = append(envKeys, k)
		}
		sort.Strings(envKeys)
		out = append(out, agents.MCP{
			ID:        string(provider) + ":" + name,
			Name:      name,
			Provider:  provider,
			Transport: t,
			Command:   cfg.Command,
			Args:      cfg.Args,
			URL:       cfg.URL,
			EnvKeys:   envKeys,
			Headers:   redactHeaders(cfg.Headers),
			Tools:     cfg.Tools,
			Scope:     scope,
			Enabled:   true,
			Source:    agents.SourceLocalCopilot,
		})
	}
	return out
}

func redactHeaders(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k := range in {
		out[k] = "***"
	}
	return out
}

// ---------- Sessions ----------

// Sessions enumerates session transcripts. Real layout used by Copilot
// CLI 1.0+:
//
//	~/.copilot/session-state/<uuid>/events.jsonl    transcript
//	~/.copilot/session-state/<uuid>/session.db      (optional)
//	~/.copilot/session-store.db                     SQLite index
//
// We scan the directories under `session-state/`, peek at the first
// couple of lines of `events.jsonl` to lift the session id + cwd, and
// fall back to two pre-1.0 layout candidates so older installs keep
// working. Sessions are returned recent-first.
//
// Each session is enriched (live state, recent activity, tool counts,
// MCP servers, branch, restart command) by [enrichFromEventsJSONL]
// running our generic state machine over the event log.
func (p *Provider) Sessions(ctx context.Context) ([]agents.Session, error) {
	root := p.copilotHome()
	candidates := []string{
		filepath.Join(root, "session-state"),
		filepath.Join(root, "sessions"),
		filepath.Join(root, "state"),
	}
	now := time.Now()
	bin := p.binary()
	var sessions []agents.Session
	seen := make(map[string]bool)
	for _, c := range candidates {
		entries, err := os.ReadDir(c)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			if seen[e.Name()] {
				continue
			}
			seen[e.Name()] = true
			dir := filepath.Join(c, e.Name())
			s := agents.Session{
				ID:             "copilot:" + e.Name(),
				Provider:       p.ID(),
				TranscriptPath: dir,
				Source:         agents.SourceLocalCopilot,
			}
			if fi, err := e.Info(); err == nil {
				s.LastModified = fi.ModTime()
			}
			enrichFromEventsJSONL(filepath.Join(dir, "events.jsonl"), &s, now)

			// Repository fallback — derive from cwd when the
			// transcript didn't carry an explicit repository tag.
			if s.Repository == "" && s.ProjectPath != "" {
				s.Repository = filepath.Base(filepath.Clean(s.ProjectPath))
			}

			// Restart command (paste-ready).
			cli := bin
			if cli == "" {
				cli = "copilot"
			}
			idForResume := strings.TrimPrefix(s.ID, "copilot:")
			if s.ProjectPath != "" {
				s.RestartCommand = "cd " + quoteForShell(s.ProjectPath) + " && " + cli + " --resume " + idForResume
			} else {
				s.RestartCommand = cli + " --resume " + idForResume
			}

			sessions = append(sessions, s)
		}
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastModified.After(sessions[j].LastModified)
	})
	if len(sessions) > 100 {
		sessions = sessions[:100]
	}
	return sessions, nil
}

// quoteForShell renders `s` so it can be pasted into a POSIX shell
// without further interpretation. Uses single-quote escaping: wraps
// the whole string in `'...'` and escapes any interior `'` as
// `'\''` (close-quote, escaped-quote, re-open-quote). Inside single
// quotes, nothing is expanded — no `$VAR`, no `$(...)`, no
// backticks, no globs — so this is the safe choice for a copy/paste
// snippet that must survive arbitrary metacharacters in ProjectPath.
//
// IMPORTANT — POSIX shells only:
//
//   - PowerShell uses `""` (doubled) or a backtick to escape an
//     embedded single quote, NOT `'\''`. Users on Windows are
//     expected to copy the cwd separately or use the dashboard's
//     resume action (which goes through Provider.BuildLaunch for a
//     no-shell exec, no quoting needed at all).
//   - For non-POSIX shells the safer path is to display the cwd as
//     a separate field; we keep the snippet to ease the common
//     POSIX terminal case.
//
// The RestartCommand field is intended for copy-to-clipboard only;
// the dashboard's resume action no longer pipes it through /bin/sh.
// If a future caller wants to actually execute this snippet, use
// the provider's BuildLaunch instead — it returns (bin, args, cwd)
// for a no-shell exec.
func quoteForShell(s string) string {
	if s == "" {
		return `''`
	}
	// Fast path: no shell-meaningful characters → no quoting needed.
	// We still wrap if the string contains a single quote, since the
	// caller pastes it into a sentence where unquoted whitespace
	// matters.
	safe := true
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\'' || r == '"' ||
			r == '$' || r == '`' || r == '\\' || r == '|' || r == '&' ||
			r == ';' || r == '<' || r == '>' || r == '(' || r == ')' ||
			r == '*' || r == '?' || r == '[' || r == ']' || r == '{' ||
			r == '}' || r == '#' || r == '~' {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	// Single-quote wrap. Interior `'` becomes `'\''`.
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// enrichFromEventsJSONL streams the full transcript to populate
// Created/Modified/Type/Status/TurnCount/ProjectPath/Title plus the
// dashboard-friendly enrichment fields (live state, recent activity,
// tool counts, MCP servers). It bails out gracefully on parse error
// or missing file — sessions stay usable with just their id + dir
// mtime when the transcript can't be parsed.
//
// Heuristics:
//   - Created   = data.startTime of the first session.start event.
//   - Modified  = timestamp of the most recent event.
//   - Type      = data.context.hostType (or "interactive" by default).
//   - Status    = derived from the last event type: session.end →
//     completed; session.stopped → stopped; otherwise active.
//   - TurnCount = number of `turn.start` / `user.message` events.
//
// `now` is used only for the live-state staleness threshold.
func enrichFromEventsJSONL(path string, s *agents.Session, now time.Time) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	dec := json.NewDecoder(f)

	var lastType string
	var firstStart, lastTS time.Time
	turns := 0
	var events []enrich.TimedEvent

	for {
		var ev struct {
			Type      string `json:"type"`
			Timestamp string `json:"timestamp"`
			Data      struct {
				SessionID string `json:"sessionId"`
				StartTime string `json:"startTime"`
				Context   struct {
					Cwd        string `json:"cwd"`
					Repository string `json:"repository"`
					HostType   string `json:"hostType"`
				} `json:"context"`
				Tool struct {
					Name string `json:"name"`
				} `json:"tool"`
				MCP struct {
					Server string `json:"server"`
				} `json:"mcp"`
				Subagent struct {
					Name string `json:"name"`
				} `json:"subagent"`
				Message struct {
					Text    string   `json:"text"`
					Content string   `json:"content"`
					Choices []string `json:"choices"`
				} `json:"message"`
			} `json:"data"`
		}
		if err := dec.Decode(&ev); err != nil {
			break
		}
		lastType = ev.Type
		var ts time.Time
		if t, err := time.Parse(time.RFC3339Nano, ev.Timestamp); err == nil {
			ts = t
			if firstStart.IsZero() {
				firstStart = ts
			}
			lastTS = ts
		}
		if ev.Type == "session.start" && ev.Data.StartTime != "" {
			if t, err := time.Parse(time.RFC3339Nano, ev.Data.StartTime); err == nil {
				if firstStart.IsZero() || t.Before(firstStart) {
					firstStart = t
				}
			}
		}
		if ev.Data.Context.Cwd != "" && s.ProjectPath == "" {
			s.ProjectPath = ev.Data.Context.Cwd
		}
		if ev.Data.Context.Repository != "" && s.Title == "" {
			s.Title = ev.Data.Context.Repository
			s.Repository = ev.Data.Context.Repository
		}
		if ev.Data.Context.HostType != "" && s.Type == "" {
			s.Type = ev.Data.Context.HostType
		}
		switch ev.Type {
		case "turn.start", "user.message":
			turns++
		}

		// Translate to generic events for the state machine.
		if kind := translateCopilotKind(ev.Type); kind != enrich.KindOther {
			te := enrich.TimedEvent{
				Event:     enrich.Event{Kind: kind},
				Timestamp: ts,
			}
			switch kind {
			case enrich.KindToolStarted, enrich.KindToolCompleted:
				te.Name = ev.Data.Tool.Name
			case enrich.KindMCPUsed:
				te.Name = ev.Data.MCP.Server
			case enrich.KindSubagentStarted, enrich.KindSubagentCompleted:
				te.Name = ev.Data.Subagent.Name
			case enrich.KindAskUser, enrich.KindAskPermission:
				te.Text = ev.Data.Message.Text
				if te.Text == "" {
					te.Text = ev.Data.Message.Content
				}
				te.Choices = ev.Data.Message.Choices
			case enrich.KindAssistantMessage:
				te.Text = ev.Data.Message.Text
				if te.Text == "" {
					te.Text = ev.Data.Message.Content
				}
			}
			events = append(events, te)
		}
	}

	if !firstStart.IsZero() {
		s.Created = firstStart
	}
	if !lastTS.IsZero() {
		s.LastModified = lastTS
	}
	if turns > 0 {
		s.TurnCount = turns
	}
	if s.Type == "" {
		s.Type = "interactive"
	}
	switch lastType {
	case "session.end", "session.close", "session.completed":
		s.Status = agents.SessionStatusCompleted
	case "session.stopped":
		s.Status = agents.SessionStatusStopped
	case "":
		s.Status = agents.SessionStatusUnknown
	default:
		s.Status = agents.SessionStatusActive
	}

	// Run the state machine over the translated events.
	if len(events) > 0 {
		result := enrich.DeriveState(events, now)
		agents.ApplyEnrichment(s, result)
	}
}

// translateCopilotKind maps a Copilot CLI event type string to the
// generic [enrich.EventKind] vocabulary. Returns KindOther for types
// that don't influence the live state.
func translateCopilotKind(t string) enrich.EventKind {
	switch t {
	case "tool.start", "tool.started", "tool_use", "tool_use.start":
		return enrich.KindToolStarted
	case "tool.end", "tool.completed", "tool.complete", "tool_use.end":
		return enrich.KindToolCompleted
	case "ask_user", "user.ask", "prompt.required":
		return enrich.KindAskUser
	case "ask_permission", "permission.required":
		return enrich.KindAskPermission
	case "user.answered", "permission.granted", "permission.denied":
		return enrich.KindAnswered
	case "subagent.started", "subagent.start":
		return enrich.KindSubagentStarted
	case "subagent.completed", "subagent.end", "subagent.complete":
		return enrich.KindSubagentCompleted
	case "mcp.use", "mcp.used", "mcp.invoke":
		return enrich.KindMCPUsed
	case "user.message", "user.msg":
		return enrich.KindUserMessage
	case "assistant.message", "assistant.msg":
		return enrich.KindAssistantMessage
	case "session.start":
		return enrich.KindSessionStart
	case "session.end", "session.close", "session.completed":
		return enrich.KindSessionEnd
	case "session.stopped":
		return enrich.KindSessionStopped
	}
	return enrich.KindOther
}

// ---------- Mutations ----------

// AddMarketplace registers a new marketplace with this provider.
func (p *Provider) AddMarketplace(ctx context.Context, spec string) error {
	return p.runCLI(ctx, "plugin", "marketplace", "add", spec)
}

// RemoveMarketplace unregisters a marketplace from this provider.
func (p *Provider) RemoveMarketplace(ctx context.Context, name string) error {
	return p.runCLI(ctx, "plugin", "marketplace", "remove", name)
}

// InstallPlugin installs a plugin via the provider CLI.
func (p *Provider) InstallPlugin(ctx context.Context, ref agents.PluginRef) error {
	var arg string
	switch {
	case ref.Source != "":
		arg = ref.Source
	case ref.Marketplace != "" && ref.Name != "":
		arg = ref.Name + "@" + ref.Marketplace
	case ref.Name != "":
		arg = ref.Name
	default:
		return errors.New("install: PluginRef must specify Name or Source")
	}
	return p.runCLI(ctx, "plugin", "install", arg)
}

// UninstallPlugin uninstalls a plugin via the provider CLI.
func (p *Provider) UninstallPlugin(ctx context.Context, id string) error {
	return p.runCLI(ctx, "plugin", "uninstall", id)
}

// EnablePlugin returns ErrNotSupported — Copilot CLI doesn't expose
// plugin enable/disable as separate CLI verbs in v1; the UI hides
// the action when this is returned. (PR #77 review #7: doc/code matched.)
func (p *Provider) EnablePlugin(ctx context.Context, id string, enabled bool) error {
	return agents.ErrNotSupported
}

// UpdatePlugin upgrades a plugin via `copilot plugin update <id>`. The
// subcommand is probed via `copilot plugin --help`; if `update` is
// absent the caller gets a clear error rather than a silent
// uninstall+install fallback.
func (p *Provider) UpdatePlugin(ctx context.Context, id string) error {
	bin := p.binary()
	if bin == "" {
		return agents.ErrProviderNotInstalled
	}
	if !p.pluginUpdateSupported(ctx) {
		return errors.New("update not supported by copilot-cli")
	}
	return p.runCLI(ctx, "plugin", "update", id)
}

// PluginUpdateProbe lets tests stub the probe step.
var PluginUpdateProbe func(ctx context.Context, bin string) bool

func (p *Provider) pluginUpdateSupported(ctx context.Context) bool {
	if PluginUpdateProbe != nil {
		return PluginUpdateProbe(ctx, p.binary())
	}
	out, err := exec.CommandContext(ctx, p.binary(), "plugin", "--help").CombinedOutput()
	if err != nil {
		return false
	}
	return bytes.Contains(bytes.ToLower(out), []byte("update"))
}

// AddMCP delegates to `copilot mcp add`. v1 only supports stdio + http;
// remote sse needs to be added via the in-session `/mcp add` form.
func (p *Provider) AddMCP(ctx context.Context, spec agents.MCPSpec) error {
	if spec.Name == "" {
		return errors.New("add mcp: Name is required")
	}
	// Copilot's non-interactive `mcp add` surface is unstable [TBV];
	// when we can't construct a known-good invocation, return a clear
	// error so the UI can fall back to suggesting `/mcp add`.
	args := []string{"mcp", "add", spec.Name}
	switch spec.Transport {
	case "http", "sse":
		args = append(args, "--url", spec.URL)
	default:
		if spec.Command == "" {
			return errors.New("add mcp: stdio transport requires Command")
		}
		args = append(args, "--command", spec.Command)
		for _, a := range spec.Args {
			args = append(args, "--arg", a)
		}
	}
	for k, v := range spec.Env {
		args = append(args, "--env", fmt.Sprintf("%s=%s", k, v))
	}
	return p.runCLI(ctx, args...)
}

// RemoveMCP removes an MCP server via the provider CLI.
func (p *Provider) RemoveMCP(ctx context.Context, name string) error {
	return p.runCLI(ctx, "mcp", "delete", name)
}

// EnableMCP toggles an MCP server enabled/disabled via the provider CLI.
func (p *Provider) EnableMCP(ctx context.Context, name string, enabled bool) error {
	if enabled {
		return p.runCLI(ctx, "mcp", "enable", name)
	}
	return p.runCLI(ctx, "mcp", "disable", name)
}

// DeleteSession removes a Copilot session's on-disk state.
//
// Copilot CLI as of 1.x does NOT expose a `session delete` subcommand —
// running `copilot session delete <id>` returns exit 1 with
// `error: Invalid command format. Did you mean: copilot -i "session
// delete <id>"?` on stderr. The previous implementation shelled out
// to that nonexistent command via runCLI which inherits stdio; in the
// TUI those 191 bytes of error text were written straight onto the
// rendered screen, corrupting the layout, AND the session wasn't
// actually deleted.
//
// We remove the session directory directly instead. Layouts checked
// (matching Sessions() — keep in sync if one changes):
//
//	~/.copilot/session-state/<uuid>/   (1.x default)
//	~/.copilot/sessions/<uuid>/        (pre-1.0)
//	~/.copilot/state/<uuid>/           (older fallback)
//
// The first existing dir is removed. Returns nil when at least one
// dir was found and removed; an error otherwise. The orphan row in
// ~/.copilot/session-store.db (the SQLite index Copilot keeps) is
// NOT cleaned up — Sessions() reads the dirs directly so the entry
// disappears from our UI either way, and we deliberately stay free
// of an SQLite dependency.
func (p *Provider) DeleteSession(ctx context.Context, id string) error {
	_ = ctx // no IO that can be cancelled; kept for interface conformance
	id = strings.TrimPrefix(id, "copilot:")
	if id == "" {
		return errors.New("delete: session id is required")
	}
	// Reject anything that isn't a plain directory name: path
	// separators (`/`, `\`), drive letters, parent refs (`..`), or
	// the current-dir ref (`.`) would let a crafted id escape the
	// Copilot home root and let os.RemoveAll wipe an arbitrary
	// directory. filepath.Base + Clean catches the common cases
	// across both POSIX and Windows path semantics.
	if id == "." || id == ".." ||
		strings.ContainsAny(id, `/\`) ||
		filepath.Base(id) != id ||
		filepath.Clean(id) != id {
		return fmt.Errorf("delete: invalid session id %q", id)
	}
	root := p.copilotHome()
	var lastErr error
	removed := false
	for _, sub := range []string{"session-state", "sessions", "state"} {
		dir := filepath.Join(root, sub, id)
		info, err := os.Stat(dir)
		if err != nil {
			if !os.IsNotExist(err) {
				lastErr = err
			}
			continue
		}
		if !info.IsDir() {
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			lastErr = err
			continue
		}
		removed = true
	}
	if removed {
		return nil
	}
	if lastErr != nil {
		return fmt.Errorf("delete: %w", lastErr)
	}
	return fmt.Errorf("delete: session %q not found under %s", id, root)
}

func (p *Provider) runCLI(ctx context.Context, args ...string) error {
	bin := p.binary()
	if bin == "" {
		return agents.ErrProviderNotInstalled
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// BuildLaunch constructs the exec plan for a LaunchSpec.
func (p *Provider) BuildLaunch(spec agents.LaunchSpec) (agents.ExecPlan, error) {
	bin := p.binary()
	if bin == "" {
		return agents.ExecPlan{}, agents.ErrProviderNotInstalled
	}
	var args []string
	note := "Start a Copilot CLI session"
	switch {
	case spec.SessionID != "":
		id := strings.TrimPrefix(spec.SessionID, "copilot:")
		args = append(args, "--resume="+id)
		note = "Resume Copilot CLI session"
	case spec.SkillName != "":
		note = "Start a Copilot CLI session (invoke /" + spec.SkillName + " when ready)"
	case spec.PluginName != "":
		note = "Start a Copilot CLI session (plugin " + spec.PluginName + " is active)"
	case spec.Prompt != "":
		args = append(args, "-p", spec.Prompt)
		note = "Run Copilot CLI with the given prompt"
	}
	args = append(args, spec.ExtraArgs...)
	return agents.ExecPlan{
		Bin:  bin,
		Args: args,
		Cwd:  spec.Cwd,
		Note: note,
	}, nil
}

var _ agents.Provider = (*Provider)(nil)
