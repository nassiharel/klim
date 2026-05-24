// Package claudecode implements the Claude Code agent provider.
//
// Filesystem layout it understands:
//
//	~/.claude.json                  OAuth + user-level MCP servers + per-project trust state
//	~/.claude/skills/<name>/        Personal skill (directory with SKILL.md)
//	~/.claude/agents/<name>.md      Personal subagent definitions
//	~/.claude/projects/<repo>/      Auto-memory + session transcripts
//	~/.claude/plugins/marketplaces/<name>/   Installed marketplace clones
//	~/.claude/plugins/known_marketplaces.json Registry of `claude plugin marketplace add` entries
//	<project>/.claude/skills/       Project skills
//	<project>/.mcp.json             Project MCP servers
//
// CLI shell-outs used for mutations:
//
//	claude plugin install / uninstall / list / update
//	claude plugin marketplace add / remove / list
//	claude mcp add / remove / list
//	claude project purge
//
// All read methods are filesystem-based and best-effort: a missing
// directory yields zero entities, not an error.
package claudecode

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
	"gopkg.in/yaml.v3"
)

const binaryName = "claude"

// Provider implements agents.Provider for Claude Code.
type Provider struct {
	// HomeOverride lets tests point at a fixture filesystem instead of
	// $HOME. Empty means use the real home dir.
	HomeOverride string
	// BinaryOverride lets tests stub the `claude` binary lookup.
	BinaryOverride string
	// CwdOverride lets tests inject a project root for scope=project scans.
	CwdOverride string
}

// New constructs a default Provider.
func New() *Provider { return &Provider{} }

// ID returns the stable provider identifier.
func (p *Provider) ID() agents.ProviderID { return agents.ProviderClaudeCode }

// DisplayName returns the human-readable provider name.
func (p *Provider) DisplayName() string { return "Claude Code" }

// Detect locates the `claude` binary and runs `claude --version`.
func (p *Provider) Detect(ctx context.Context) agents.Status {
	bin := p.binary()
	if bin == "" {
		return agents.Status{Installed: false}
	}
	cmd := exec.CommandContext(ctx, bin, "--version")
	out, err := cmd.Output()
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

// home returns the effective home directory (honoring the override).
func (p *Provider) home() string {
	if p.HomeOverride != "" {
		return p.HomeOverride
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
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

// claudeDir returns ~/.claude (using the home override when set).
func (p *Provider) claudeDir() string {
	h := p.home()
	if h == "" {
		return ""
	}
	return filepath.Join(h, ".claude")
}

// ---------- Marketplaces ----------

// Marketplaces returns the marketplaces Claude Code knows about.
//
// The authoritative source is ~/.claude/plugins/known_marketplaces.json,
// which `claude plugin marketplace add` writes when the user registers a
// new marketplace (e.g. `/plugin marketplace add openai/codex-plugin-cc`
// inside a Claude session). We parse that file for repo + install path +
// last-updated metadata, then also list ~/.claude/plugins/marketplaces/
// to surface any cloned-but-not-registered marketplaces. The canonical
// `claude-plugins-official` is always included.
func (p *Provider) Marketplaces(ctx context.Context) ([]agents.Marketplace, error) {
	seen := make(map[string]bool)
	var ms []agents.Marketplace

	add := func(m agents.Marketplace) {
		if m.Name == "" || seen[m.Name] {
			return
		}
		seen[m.Name] = true
		if m.ID == "" {
			m.ID = m.Name
		}
		if m.Provider == "" {
			m.Provider = p.ID()
		}
		m.Installed = true
		ms = append(ms, m)
	}

	add(agents.Marketplace{
		ID:          "claude-plugins-official",
		Name:        "claude-plugins-official",
		DisplayName: "Anthropic Official",
		Description: "Anthropic's curated Claude Code plugin marketplace",
		Owner:       "anthropics",
		URL:         "https://github.com/anthropics/claude-plugins-official",
		Source:      agents.SourceCatalogClaude,
	})

	for _, km := range p.readKnownMarketplaces() {
		add(km)
	}

	for _, dir := range p.marketplaceRoots() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			// Skip the marketplaces/ parent itself and other known
			// non-marketplace directories that live under plugins/.
			name := e.Name()
			if name == "marketplaces" || strings.HasPrefix(name, ".") {
				continue
			}
			add(agents.Marketplace{
				Name:   name,
				Source: agents.SourceLocalClaude,
			})
		}
	}

	return ms, nil
}

// knownMarketplaceEntry mirrors the schema of
// ~/.claude/plugins/known_marketplaces.json.
type knownMarketplaceEntry struct {
	Source struct {
		Source string `json:"source"`
		Repo   string `json:"repo,omitempty"`
		URL    string `json:"url,omitempty"`
	} `json:"source"`
	InstallLocation string `json:"installLocation,omitempty"`
	LastUpdated     string `json:"lastUpdated,omitempty"`
}

// readKnownMarketplaces returns marketplaces registered via
// `claude plugin marketplace add`. Returns nil on any read/parse error.
func (p *Provider) readKnownMarketplaces() []agents.Marketplace {
	cd := p.claudeDir()
	if cd == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(cd, "plugins", "known_marketplaces.json"))
	if err != nil {
		return nil
	}
	var raw map[string]knownMarketplaceEntry
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	out := make([]agents.Marketplace, 0, len(raw))
	for name, e := range raw {
		m := agents.Marketplace{
			ID:     name,
			Name:   name,
			Source: agents.SourceLocalClaude,
		}
		switch e.Source.Source {
		case "github":
			if e.Source.Repo != "" {
				if i := strings.IndexByte(e.Source.Repo, '/'); i > 0 {
					m.Owner = e.Source.Repo[:i]
				}
				m.URL = "https://github.com/" + e.Source.Repo
			}
		case "url":
			m.URL = e.Source.URL
		}
		if t, err := time.Parse(time.RFC3339, e.LastUpdated); err == nil {
			m.LastSynced = t
		}
		out = append(out, m)
	}
	return out
}

// marketplaceRoots returns directories that may contain installed
// marketplace clones. The real path used by Claude Code is
// ~/.claude/plugins/marketplaces; the bare ~/.claude/plugins path is
// kept for backward compatibility with older fixtures.
func (p *Provider) marketplaceRoots() []string {
	cd := p.claudeDir()
	if cd == "" {
		return nil
	}
	return []string{
		filepath.Join(cd, "plugins", "marketplaces"),
		filepath.Join(cd, "plugins"),
	}
}

// ---------- Plugins ----------

// pluginManifest mirrors the Claude `.claude-plugin/plugin.json` schema.
type pluginManifest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
	Author      struct {
		Name  string `json:"name,omitempty"`
		Email string `json:"email,omitempty"`
	} `json:"author,omitempty"`
	Homepage   string   `json:"homepage,omitempty"`
	Repository string   `json:"repository,omitempty"`
	License    string   `json:"license,omitempty"`
	Keywords   []string `json:"keywords,omitempty"`
}

// Plugins discovers installed plugins by scanning the marketplace
// clones. Real Claude Code layout nests plugins under
// `<mp>/plugins/<plugin>` and `<mp>/external_plugins/<plugin>`; older
// test fixtures put them at `<mp>/<plugin>` directly. We accept all
// three.
func (p *Provider) Plugins(ctx context.Context) ([]agents.Plugin, error) {
	var plugins []agents.Plugin
	seen := make(map[string]bool)

	for _, root := range p.marketplaceRoots() {
		mEntries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, mp := range mEntries {
			if !mp.IsDir() {
				continue
			}
			mpName := mp.Name()
			mpDir := filepath.Join(root, mpName)

			// Candidate plugin parent dirs within a marketplace clone.
			candidates := []string{
				mpDir,                                    // legacy: <mp>/<plugin>
				filepath.Join(mpDir, "plugins"),          // real: first-party plugins
				filepath.Join(mpDir, "external_plugins"), // real: third-party plugins
			}
			for _, parent := range candidates {
				pluginEntries, err := os.ReadDir(parent)
				if err != nil {
					continue
				}
				for _, pe := range pluginEntries {
					if !pe.IsDir() {
						continue
					}
					pluginDir := filepath.Join(parent, pe.Name())
					manifestPath := filepath.Join(pluginDir, ".claude-plugin", "plugin.json")
					m, err := readPluginManifest(manifestPath)
					if err != nil {
						continue
					}
					id := mpName + "/" + m.Name
					if seen[id] {
						continue
					}
					seen[id] = true
					plugins = append(plugins, agents.Plugin{
						ID:          id,
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
						Source:      agents.SourceLocalClaude,
					})
				}
			}
		}
	}
	sort.Slice(plugins, func(i, j int) bool { return plugins[i].Name < plugins[j].Name })
	return plugins, nil
}

// pluginCacheRoots returns parent dirs that may contain marketplace
// clones for the plugin / skill scanners. Kept for Skills which walks
// the same structure.
func (p *Provider) pluginCacheRoots() []string {
	return p.marketplaceRoots()
}

func readPluginManifest(path string) (*pluginManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m pluginManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	if m.Name == "" {
		return nil, errors.New("plugin manifest missing name")
	}
	return &m, nil
}

// ---------- Skills ----------

// Skills scans personal (~/.claude/skills/) and project (.claude/skills/)
// skill directories. Each subdir with a SKILL.md is a skill.
func (p *Provider) Skills(ctx context.Context) ([]agents.Skill, error) {
	var skills []agents.Skill
	dir := p.claudeDir()
	if dir != "" {
		skills = append(skills, scanSkillDir(filepath.Join(dir, "skills"), p.ID(), agents.ScopeUser, "")...)
	}
	if cw := p.cwd(); cw != "" {
		skills = append(skills, scanSkillDir(filepath.Join(cw, ".claude", "skills"), p.ID(), agents.ScopeProject, "")...)
	}
	// Plugin-shipped skills: scan plugin installs.
	for _, root := range p.pluginCacheRoots() {
		mEntries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, mp := range mEntries {
			if !mp.IsDir() {
				continue
			}
			mpDir := filepath.Join(root, mp.Name())
			pluginParents := []string{
				mpDir,
				filepath.Join(mpDir, "plugins"),
				filepath.Join(mpDir, "external_plugins"),
			}
			for _, parent := range pluginParents {
				plugins, err := os.ReadDir(parent)
				if err != nil {
					continue
				}
				for _, pe := range plugins {
					if !pe.IsDir() {
						continue
					}
					skillsDir := filepath.Join(parent, pe.Name(), "skills")
					skills = append(skills, scanSkillDir(skillsDir, p.ID(), agents.ScopePlugin, pe.Name())...)
				}
			}
		}
	}
	return skills, nil
}

// skillFrontmatter is the subset of SKILL.md frontmatter we parse.
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
			ID:                 string(provider) + ":" + scopeKey(scope) + ":" + name,
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
			Source:             agents.SourceLocalClaude,
		})
	}
	return out
}

func scopeKey(s agents.Scope) string { return string(s) }

func readSkillFrontmatter(path string) (*skillFrontmatter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Frontmatter delimited by '---' on its own line.
	const fence = "---"
	lines := bytes.SplitN(data, []byte("\n"), 2)
	if len(lines) < 2 || strings.TrimSpace(string(lines[0])) != fence {
		return &skillFrontmatter{}, nil // no frontmatter — empty struct is OK
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

// claudeRoot mirrors the relevant subset of ~/.claude.json.
type claudeRoot struct {
	MCPServers map[string]mcpServerConfig `json:"mcpServers,omitempty"`
	Projects   map[string]projectEntry    `json:"projects,omitempty"`
}

type projectEntry struct {
	MCPServers map[string]mcpServerConfig `json:"mcpServers,omitempty"`
}

type mcpServerConfig struct {
	Type    string            `json:"type,omitempty"` // streamable-http | http | sse | stdio
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	URL     string            `json:"url,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// MCPs reads ~/.claude.json (user + per-project local MCPs) and the
// current project's .mcp.json (project-shared MCPs).
func (p *Provider) MCPs(ctx context.Context) ([]agents.MCP, error) {
	var mcps []agents.MCP
	if h := p.home(); h != "" {
		root, err := readClaudeJSON(filepath.Join(h, ".claude.json"))
		if err == nil {
			for name, cfg := range root.MCPServers {
				mcps = append(mcps, mcpFromConfig(name, cfg, agents.ScopeUser, p.ID()))
			}
			// Local-scope per-project MCPs.
			cw := p.cwd()
			if cw != "" {
				if pe, ok := root.Projects[cw]; ok {
					for name, cfg := range pe.MCPServers {
						mcps = append(mcps, mcpFromConfig(name, cfg, agents.ScopeProject, p.ID()))
					}
				}
			}
		}
	}
	if cw := p.cwd(); cw != "" {
		projectMCP := filepath.Join(cw, ".mcp.json")
		var doc struct {
			MCPServers map[string]mcpServerConfig `json:"mcpServers"`
		}
		if data, err := os.ReadFile(projectMCP); err == nil {
			if json.Unmarshal(data, &doc) == nil {
				for name, cfg := range doc.MCPServers {
					mcps = append(mcps, mcpFromConfig(name, cfg, agents.ScopeProject, p.ID()))
				}
			}
		}
	}
	return mcps, nil
}

func readClaudeJSON(path string) (*claudeRoot, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c claudeRoot
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func mcpFromConfig(name string, cfg mcpServerConfig, scope agents.Scope, provider agents.ProviderID) agents.MCP {
	t := cfg.Type
	if t == "streamable-http" {
		t = "http"
	}
	if t == "" {
		t = "stdio"
	}
	envKeys := make([]string, 0, len(cfg.Env))
	for k := range cfg.Env {
		envKeys = append(envKeys, k)
	}
	sort.Strings(envKeys)
	return agents.MCP{
		ID:        string(provider) + ":" + name,
		Name:      name,
		Provider:  provider,
		Transport: t,
		Command:   cfg.Command,
		Args:      cfg.Args,
		URL:       cfg.URL,
		EnvKeys:   envKeys,
		Headers:   redactHeaders(cfg.Headers),
		Scope:     scope,
		Enabled:   true,
		Source:    agents.SourceLocalClaude,
	}
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

// Sessions enumerates per-project directories under ~/.claude/projects/.
// Exact session-transcript filename format is undocumented [TBV]; we
// expose each project as one "latest session" entry with last-modified
// times pulled from the project dir mtime.
func (p *Provider) Sessions(ctx context.Context) ([]agents.Session, error) {
	dir := filepath.Join(p.claudeDir(), "projects")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil // no projects yet — empty is fine
	}
	var sessions []agents.Session
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		full := filepath.Join(dir, e.Name())
		fi, err := e.Info()
		if err != nil {
			continue
		}
		decoded := decodeProjectPath(e.Name())
		sessions = append(sessions, agents.Session{
			ID:             "claude:" + e.Name(),
			Provider:       p.ID(),
			ProjectPath:    decoded,
			LastModified:   fi.ModTime(),
			TranscriptPath: full,
			Source:         agents.SourceLocalClaude,
		})
	}
	return sessions, nil
}

// decodeProjectPath converts Claude's URL-encoded project directory
// name (`home%2Fuser%2Fwork%2Frepo`) back to a readable path. The
// exact encoding is [TBV] — we handle both %2F and a dash-separated
// form just in case.
func decodeProjectPath(name string) string {
	// Most reliable signal: %2F-encoded slashes.
	if strings.Contains(name, "%2F") {
		return strings.ReplaceAll(name, "%2F", "/")
	}
	return name
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

// EnablePlugin returns ErrNotSupported. Claude Code doesn't expose
// enable/disable as separate CLI verbs in v1 of this integration; the
// UI hides the action when this is returned.
func (p *Provider) EnablePlugin(ctx context.Context, id string, enabled bool) error {
	return agents.ErrNotSupported
}

// UpdatePlugin upgrades a plugin via `claude plugin update <id>`. The
// subcommand surface is verified via `claude plugin --help`; when
// `update` is missing the user gets a clear error rather than a
// silent uninstall+install fallback.
func (p *Provider) UpdatePlugin(ctx context.Context, id string) error {
	bin := p.binary()
	if bin == "" {
		return agents.ErrProviderNotInstalled
	}
	if !p.pluginUpdateSupported(ctx) {
		return errors.New("update not supported by claude-code")
	}
	return p.runCLI(ctx, "plugin", "update", id)
}

// PluginUpdateProbe lets tests assert UpdatePlugin without depending
// on a real `claude` binary. Returning true means the probe step is
// skipped and the shell-out is invoked directly.
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

// AddMCP adds an MCP server via the provider CLI.
func (p *Provider) AddMCP(ctx context.Context, spec agents.MCPSpec) error {
	args := []string{"mcp", "add"}
	if spec.Scope != "" && spec.Scope != agents.ScopeUser {
		args = append(args, "--scope", string(spec.Scope))
	}
	if spec.Transport != "" {
		args = append(args, "--transport", spec.Transport)
	}
	for k, v := range spec.Env {
		args = append(args, "--env", fmt.Sprintf("%s=%s", k, v))
	}
	args = append(args, spec.Name)
	switch spec.Transport {
	case "http", "sse":
		args = append(args, spec.URL)
	default:
		if spec.Command != "" {
			args = append(append(append(args, "--"), spec.Command), spec.Args...)
		}
	}
	return p.runCLI(ctx, args...)
}

// RemoveMCP removes an MCP server via the provider CLI.
func (p *Provider) RemoveMCP(ctx context.Context, name string) error {
	return p.runCLI(ctx, "mcp", "remove", name)
}

// EnableMCP returns ErrNotSupported — Claude Code doesn't expose
// MCP enable/disable as separate CLI verbs in v1; the UI hides the
// action when this is returned. (PR #77 review #7: doc/code matched.)
func (p *Provider) EnableMCP(ctx context.Context, name string, enabled bool) error {
	return agents.ErrNotSupported
}

// DeleteSession deletes a session via the provider CLI.
func (p *Provider) DeleteSession(ctx context.Context, id string) error {
	// Claude exposes session purge via `claude project purge <path>`.
	// id here is the URL-encoded directory name from Sessions().
	id = strings.TrimPrefix(id, "claude:")
	decoded := decodeProjectPath(id)
	return p.runCLI(ctx, "project", "purge", decoded)
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
	note := "Start a Claude Code session"
	switch {
	case spec.SessionID != "":
		id := strings.TrimPrefix(spec.SessionID, "claude:")
		args = append(args, "-r", id)
		note = "Resume Claude Code session"
	case spec.SkillName != "":
		// Claude has no `--skill` flag; we open a new session and the
		// user invokes the skill via `/<name>` at the prompt.
		note = "Start a Claude Code session (invoke /" + spec.SkillName + " when ready)"
	case spec.PluginName != "":
		note = "Start a Claude Code session (plugin " + spec.PluginName + " is active)"
	case spec.Prompt != "":
		args = append(args, "-p", spec.Prompt)
		note = "Run Claude Code with the given prompt"
	}
	args = append(args, spec.ExtraArgs...)
	return agents.ExecPlan{
		Bin:  bin,
		Args: args,
		Cwd:  spec.Cwd,
		Note: note,
	}, nil
}

// Provider compile-time check.
var _ agents.Provider = (*Provider)(nil)

// timeOrZero is used in tests to clamp generation times.
func timeOrZero(t time.Time) time.Time {
	if t.IsZero() {
		return time.Unix(0, 0).UTC()
	}
	return t
}

var _ = timeOrZero
