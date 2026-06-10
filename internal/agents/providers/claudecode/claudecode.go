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

// installedPluginRecord mirrors one entry in the array under each
// plugin key in ~/.claude/plugins/installed_plugins.json.
type installedPluginRecord struct {
	Scope        string `json:"scope"`
	InstallPath  string `json:"installPath"`
	Version      string `json:"version"`
	InstalledAt  string `json:"installedAt"`
	LastUpdated  string `json:"lastUpdated"`
	GitCommitSha string `json:"gitCommitSha"`
}

// installedPluginsFile mirrors ~/.claude/plugins/installed_plugins.json.
type installedPluginsFile struct {
	Version int                                `json:"version"`
	Plugins map[string][]installedPluginRecord `json:"plugins"`
}

// readInstalledPlugins reads ~/.claude/plugins/installed_plugins.json
// and returns a map keyed by "<name>@<marketplace>" -> first install
// record (the array supports multi-scope but in practice has one entry).
//
// Returns nil if the file is missing or unreadable; callers should treat
// nil as "no info — fall back to scanning disk" so legacy fixtures and
// fresh installs without the registry file continue to surface plugins.
func (p *Provider) readInstalledPlugins() map[string]installedPluginRecord {
	cd := p.claudeDir()
	if cd == "" {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(cd, "plugins", "installed_plugins.json"))
	if err != nil {
		return nil
	}
	var f installedPluginsFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil
	}
	out := make(map[string]installedPluginRecord, len(f.Plugins))
	for key, records := range f.Plugins {
		if len(records) == 0 {
			// Present but empty array — still record key presence so the
			// caller treats it as installed (even without metadata).
			out[key] = installedPluginRecord{}
			continue
		}
		out[key] = records[0]
	}
	return out
}

// readEnabledPlugins merges enabledPlugins maps from ~/.claude/settings.json
// and ~/.claude/settings.local.json (local takes precedence per key).
// Returns nil when neither file yields any entries; callers should
// default a plugin's Enabled field to true in that case.
//
// TODO: settings files may technically contain JSONC (// comments).
// We use plain json.Unmarshal here — if that becomes an issue, swap in
// a JSONC-tolerant parser.
func (p *Provider) readEnabledPlugins() map[string]bool {
	cd := p.claudeDir()
	if cd == "" {
		return nil
	}
	out := map[string]bool{}
	// Order matters: local is read after global so it overrides per-key.
	for _, name := range []string{"settings.json", "settings.local.json"} {
		data, err := os.ReadFile(filepath.Join(cd, name))
		if err != nil {
			continue
		}
		var doc struct {
			EnabledPlugins map[string]bool `json:"enabledPlugins"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			continue
		}
		for k, v := range doc.EnabledPlugins {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Plugins discovers installed plugins by scanning the marketplace
// clones. Real Claude Code layout nests plugins under
// `<mp>/plugins/<plugin>` and `<mp>/external_plugins/<plugin>`; older
// test fixtures put them at `<mp>/<plugin>` directly. We accept all
// three.
//
// Claude itself only considers a plugin "installed" if it's recorded in
// ~/.claude/plugins/installed_plugins.json (keyed by `name@marketplace`).
// When that file exists we filter the on-disk scan to only those
// entries — otherwise `klim agents plugins uninstall X` would shell out
// to `claude plugin uninstall X` and fail with "not found" for plugins
// that merely happen to sit in a cloned marketplace. When the file is
// absent (fresh install, fixture-based tests) we fall back to the
// legacy behavior of treating every on-disk manifest as installed.
func (p *Provider) Plugins(ctx context.Context) ([]agents.Plugin, error) {
	var plugins []agents.Plugin
	seen := make(map[string]bool)

	installed := p.readInstalledPlugins()
	enabled := p.readEnabledPlugins()

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

					// installed_plugins.json keys are name@marketplace,
					// distinct from our ID format (mp/name) which other
					// code depends on — keep ID untouched.
					key := m.Name + "@" + mpName
					if installed != nil {
						if _, ok := installed[key]; !ok {
							continue
						}
					}

					isEnabled := true
					if enabled != nil {
						if v, ok := enabled[key]; ok {
							isEnabled = v
						}
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
						Enabled:     isEnabled,
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
// Each project dir contains one or more `<sessionId>.jsonl` transcripts.
// We pick the most recently modified transcript per project as that
// project's "current" session, then parse it (via enrichSessionFromJSONL)
// to populate dashboard fields: title, created/last-modified times,
// turn count, live state, recent activity, tool counts, MCP servers,
// branch, group, and a paste-ready restart command.
//
// Identifier convention (important — read before changing):
//
//   - s.ID stays `"claude:" + e.Name()`, where e.Name() is the
//     URL-encoded project directory name (e.g. `C--dev-klim`). This
//     is the form `claude project purge <decoded-path>` understands
//     and what the TUI's Delete action passes through to
//     [Provider.DeleteSession].
//   - s.Name carries the in-file session UUID (when discoverable).
//     This is what `claude -r <uuid>` expects for resume. Both the
//     CLI and TUI launch paths read it via [Provider.BuildLaunch].
//   - s.TranscriptPath remains the project directory, so the TUI's
//     "Open Dir" action lands in the right folder. View Transcript
//     handles the directory case by looking for the newest `*.jsonl`
//     inside.
//
// Sessions are returned sorted by LastModified descending.
func (p *Provider) Sessions(ctx context.Context) ([]agents.Session, error) {
	dir := filepath.Join(p.claudeDir(), "projects")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil // no projects yet — empty is fine
	}

	now := time.Now()
	var sessions []agents.Session
	bin := p.binary() // empty when claude isn't on PATH — RestartCommand falls back to "claude"

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
		s := agents.Session{
			ID:             "claude:" + e.Name(),
			Provider:       p.ID(),
			ProjectPath:    decoded,
			LastModified:   fi.ModTime(),
			TranscriptPath: full,
			Source:         agents.SourceLocalClaude,
		}

		// Parse the most recent transcript for enrichment. When no
		// .jsonl exists (fresh project dir, or pre-1.0 layout) we
		// fall back to the dir-mtime baseline above.
		if tr := latestTranscript(full); tr != "" {
			scan := enrichSessionFromJSONL(tr, now)
			// Surface the in-file UUID via s.Name so BuildLaunch
			// (and any other UUID-needing consumer) can find it
			// without re-parsing the transcript. s.ID stays the
			// URL-encoded dir name — see the Sessions godoc above.
			if scan.SessionID != "" {
				s.Name = scan.SessionID
			}
			if scan.Cwd != "" {
				s.ProjectPath = scan.Cwd
			}
			if scan.Branch != "" {
				s.Branch = scan.Branch
			}
			if scan.FirstUserMsg != "" {
				s.Title = scan.FirstUserMsg
			}
			if !scan.Created.IsZero() {
				s.Created = scan.Created
			}
			if !scan.LastSeen.IsZero() {
				s.LastModified = scan.LastSeen
			}
			agents.ApplyEnrichment(&s, scan.Result)
		}

		// Status fallback: when no terminal event fired, treat the
		// session as active so the dashboard's --status=active filter
		// surfaces it. Sessions without any transcript stay
		// SessionStatusUnknown.
		if s.Status == agents.SessionStatusUnknown && (s.LiveState != agents.StateUnknown || !s.LastModified.IsZero()) {
			s.Status = agents.SessionStatusActive
		}

		if s.Type == "" {
			s.Type = "interactive"
		}

		// Build the copy-paste resume command. Prefer the in-file
		// UUID (s.Name) since that's what `claude -r` expects; fall
		// back to the dir-name form if no UUID was parsed (very old
		// transcripts).
		resumeID := s.Name
		if resumeID == "" {
			resumeID = strings.TrimPrefix(s.ID, "claude:")
		}
		cli := bin
		if cli == "" {
			cli = "claude"
		}
		if s.ProjectPath != "" {
			s.RestartCommand = "cd " + quoteForShell(s.ProjectPath) + " && " + cli + " --resume " + resumeID
		} else {
			s.RestartCommand = cli + " --resume " + resumeID
		}

		// Repository name (best-effort).
		if s.Repository == "" && s.ProjectPath != "" {
			s.Repository = filepath.Base(filepath.Clean(s.ProjectPath))
		}

		sessions = append(sessions, s)
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].LastModified.After(sessions[j].LastModified)
	})
	return sessions, nil
}

// quoteForShell wraps `s` in double quotes when it contains a space
// or other shell-significant character. Keeps the rendered restart
// command paste-safe across both POSIX shells and PowerShell, both
// of which honour double-quoted strings for cd targets.
func quoteForShell(s string) string {
	if s == "" {
		return `""`
	}
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '"' || r == '$' || r == '`' {
			return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
		}
	}
	return s
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
//
// For session resumes, Claude's `-r` flag expects the session UUID
// (e.g. `3b4dc369-3956-…`), not the URL-encoded project directory
// name we use for s.ID. When the SessionID we receive looks like a
// directory name (contains `--` or `%2F`), we look up the most
// recently modified `.jsonl` in that project directory and pull the
// UUID out of it. Falls back to the raw trimmed string for legacy
// callers passing a bare UUID.
func (p *Provider) BuildLaunch(spec agents.LaunchSpec) (agents.ExecPlan, error) {
	bin := p.binary()
	if bin == "" {
		return agents.ExecPlan{}, agents.ErrProviderNotInstalled
	}
	var args []string
	note := "Start a Claude Code session"
	switch {
	case spec.SessionID != "":
		raw := strings.TrimPrefix(spec.SessionID, "claude:")
		uuid := p.resolveSessionUUID(raw)
		if uuid == "" {
			uuid = raw
		}
		args = append(args, "-r", uuid)
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

// resolveSessionUUID maps a Claude session ID (which may be either
// a bare UUID or the URL-encoded directory name like `C--dev-klim`)
// to the in-file session UUID by reading the directory's most recent
// transcript. Returns the empty string when the lookup fails — the
// caller should fall back to using the raw input.
//
// Best-effort: any I/O or parse error returns "" and the caller
// proceeds with whatever it had. The cost is one stat + one small
// JSON decode of the first event line, so this stays cheap even when
// called from interactive paths.
func (p *Provider) resolveSessionUUID(idOrDir string) string {
	// Already a UUID? UUIDs have 4 dashes and no double-dash, so a
	// `--` substring is a strong dirname signal.
	if !strings.Contains(idOrDir, "--") && !strings.Contains(idOrDir, "%2F") {
		return idOrDir
	}
	projectDir := filepath.Join(p.claudeDir(), "projects", idOrDir)
	tr := latestTranscript(projectDir)
	if tr == "" {
		return ""
	}
	f, err := os.Open(tr)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	dec := json.NewDecoder(f)
	// Walk a handful of events: some early events (queue-operation
	// telemetry) carry the sessionId before any user/assistant
	// message arrives. Cap the search so a transcript with no
	// sessionId at all doesn't read the whole file.
	for i := 0; i < 20; i++ {
		var ev struct {
			SessionID string `json:"sessionId"`
		}
		if err := dec.Decode(&ev); err != nil {
			return ""
		}
		if ev.SessionID != "" {
			return ev.SessionID
		}
	}
	return ""
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
