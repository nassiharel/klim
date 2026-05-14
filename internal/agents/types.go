// Package agents discovers and manages the agent-tooling ecosystem across
// multiple agent CLIs (Claude Code, GitHub Copilot CLI, …). It surfaces
// five entity types — Marketplaces, Plugins, Skills, MCPs, and Sessions —
// behind a single Provider interface so klim's TUI and CLI can browse,
// search, mutate, and launch sessions uniformly.
//
// klim drives the underlying CLIs rather than re-implementing their
// install logic: read-only enumeration is filesystem-based, mutations
// shell out to `claude plugin install …` / `copilot mcp add …` etc.,
// and session launch is `tea.ExecProcess`-style (suspend klim, exec
// the agent CLI, resume on exit).
//
// See superpowers/specs/2026-05-14-agents-tab-design.md for the full
// design document.
package agents

import (
	"errors"
	"strings"
	"time"
)

// EntityType identifies one of the five top-level agent-ecosystem entities.
type EntityType string

// Entity type constants. AllEntityTypes lists them in display order.
const (
	EntityMarketplace EntityType = "marketplace"
	EntityPlugin      EntityType = "plugin"
	EntitySkill       EntityType = "skill"
	EntityMCP         EntityType = "mcp"
	EntitySession     EntityType = "session"
)

// AllEntityTypes lists the entity types in display order (matches sub-tab order).
var AllEntityTypes = []EntityType{
	EntityMarketplace,
	EntityPlugin,
	EntitySkill,
	EntityMCP,
	EntitySession,
}

// Scope describes where an entity is configured — user, project, plugin,
// or remote (a catalog entry not yet installed locally).
type Scope string

// Scope values.
const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
	ScopePlugin  Scope = "plugin"
	ScopeRemote  Scope = "remote"
)

// ProviderID identifies the source agent CLI for an entity.
type ProviderID string

// ProviderID values for the built-in providers.
const (
	ProviderClaudeCode  ProviderID = "claude-code"
	ProviderCopilotCLI  ProviderID = "copilot-cli"
	ProviderMCPRegistry ProviderID = "mcp-registry"
)

// Status reports whether a provider's binary is installed and detected.
type Status struct {
	Installed bool
	Version   string
	BinPath   string
	Error     error
}

// Source records where an entity record came from in the merged snapshot.
// Used to de-duplicate (installed plugin vs. marketplace listing) and as a
// provenance pill in the UI.
type Source string

// Source values.
const (
	SourceLocalClaude    Source = "local-claude"
	SourceLocalCopilot   Source = "local-copilot"
	SourceCatalogClaude  Source = "catalog-claude"
	SourceCatalogCopilot Source = "catalog-copilot"
	SourceCatalogMCP     Source = "catalog-mcp-registry"
	SourceConfig         Source = "config"
)

// Marketplace is a named registry of plugins/MCPs.
type Marketplace struct {
	ID          string     `yaml:"id"`
	Name        string     `yaml:"name"`
	DisplayName string     `yaml:"display_name,omitempty"`
	Description string     `yaml:"description,omitempty"`
	Provider    ProviderID `yaml:"provider"`
	URL         string     `yaml:"url,omitempty"`
	Owner       string     `yaml:"owner,omitempty"`
	PluginCount int        `yaml:"plugin_count,omitempty"`
	LastSynced  time.Time  `yaml:"last_synced,omitempty"`
	Source      Source     `yaml:"source"`
}

// Plugin is an installable bundle that may contain skills, MCP servers,
// agents, commands, hooks, and LSP servers.
type Plugin struct {
	ID          string     `yaml:"id"`
	Name        string     `yaml:"name"`
	DisplayName string     `yaml:"display_name,omitempty"`
	Description string     `yaml:"description,omitempty"`
	Version     string     `yaml:"version,omitempty"`
	Author      string     `yaml:"author,omitempty"`
	Homepage    string     `yaml:"homepage,omitempty"`
	Repository  string     `yaml:"repository,omitempty"`
	License     string     `yaml:"license,omitempty"`
	Keywords    []string   `yaml:"keywords,omitempty"`
	Provider    ProviderID `yaml:"provider"`
	Marketplace string     `yaml:"marketplace,omitempty"`
	Installed   bool       `yaml:"installed"`
	Enabled     bool       `yaml:"enabled"`
	InstallPath string     `yaml:"install_path,omitempty"`
	Scope       Scope      `yaml:"scope,omitempty"`
	SkillCount  int        `yaml:"skill_count,omitempty"`
	MCPCount    int        `yaml:"mcp_count,omitempty"`
	Source      Source     `yaml:"source"`
}

// Skill is an individual skill definition (typically a SKILL.md directory).
type Skill struct {
	ID                 string     `yaml:"id"`
	Name               string     `yaml:"name"`
	Description        string     `yaml:"description,omitempty"`
	WhenToUse          string     `yaml:"when_to_use,omitempty"`
	AllowedTools       string     `yaml:"allowed_tools,omitempty"`
	ArgumentHint       string     `yaml:"argument_hint,omitempty"`
	Model              string     `yaml:"model,omitempty"`
	DisableModelInvoke bool       `yaml:"disable_model_invocation,omitempty"`
	UserInvocable      bool       `yaml:"user_invocable,omitempty"`
	Provider           ProviderID `yaml:"provider"`
	SourcePlugin       string     `yaml:"source_plugin,omitempty"`
	Scope              Scope      `yaml:"scope"`
	Path               string     `yaml:"path,omitempty"`
	Enabled            bool       `yaml:"enabled"`
	Source             Source     `yaml:"source"`
}

// MCP describes a configured Model Context Protocol server.
type MCP struct {
	ID        string            `yaml:"id"`
	Name      string            `yaml:"name"`
	Provider  ProviderID        `yaml:"provider"`
	Transport string            `yaml:"transport,omitempty"` // "stdio" | "http" | "sse"
	Command   string            `yaml:"command,omitempty"`
	Args      []string          `yaml:"args,omitempty"`
	URL       string            `yaml:"url,omitempty"`
	EnvKeys   []string          `yaml:"env_keys,omitempty"` // keys only; values are not surfaced
	Headers   map[string]string `yaml:"headers,omitempty"`
	Tools     []string          `yaml:"tools,omitempty"`
	Scope     Scope             `yaml:"scope"`
	Enabled   bool              `yaml:"enabled"`
	Source    Source            `yaml:"source"`
}

// SessionStatus categorizes a session's lifecycle state.
type SessionStatus string

// Session status values.
const (
	SessionStatusActive    SessionStatus = "active"
	SessionStatusCompleted SessionStatus = "completed"
	SessionStatusStopped   SessionStatus = "stopped"
	SessionStatusUnknown   SessionStatus = ""
)

// Session is a saved or recent agent session.
type Session struct {
	ID             string        `yaml:"id"`
	Name           string        `yaml:"name,omitempty"`
	Provider       ProviderID    `yaml:"provider"`
	ProjectPath    string        `yaml:"project_path,omitempty"`
	Created        time.Time     `yaml:"created,omitempty"`       // first event (session.start)
	LastModified   time.Time     `yaml:"last_modified,omitempty"` // last event / dir mtime
	TurnCount      int           `yaml:"turn_count,omitempty"`
	Title          string        `yaml:"title,omitempty"`
	Type           string        `yaml:"type,omitempty"`   // e.g. "interactive", "background", "ado"
	Status         SessionStatus `yaml:"status,omitempty"` // active | completed | stopped | ""
	TranscriptPath string        `yaml:"transcript_path,omitempty"`
	Source         Source        `yaml:"source"`
}

// PluginRef identifies a plugin to install. Either a marketplace-qualified
// name (`workiq@copilot-plugins`) or a free-form source (`owner/repo`,
// `https://…`, local path).
type PluginRef struct {
	Name        string
	Marketplace string
	Source      string
	Scope       Scope
}

// MCPSpec captures the parameters needed to add an MCP server.
type MCPSpec struct {
	Name      string
	Transport string
	Command   string
	Args      []string
	URL       string
	Env       map[string]string
	Headers   map[string]string
	Scope     Scope
	Tools     []string
}

// LaunchSpec selects what to launch. Exactly one of SkillName, SessionID,
// PluginName, or NewSession should be set.
type LaunchSpec struct {
	Provider   ProviderID
	SkillName  string
	SessionID  string
	PluginName string
	NewSession bool
	Prompt     string
	Cwd        string
	ExtraArgs  []string
}

// ExecPlan describes a command to exec (used by both the TUI's
// tea.ExecProcess and the CLI launch path). The Note field is shown
// in the pre-launch confirmation modal.
type ExecPlan struct {
	Bin  string
	Args []string
	Env  []string // additional env vars beyond os.Environ
	Cwd  string
	Note string
}

// CommandLine renders the ExecPlan as a shell-style command for display.
func (p ExecPlan) CommandLine() string {
	parts := []string{p.Bin}
	for _, a := range p.Args {
		if needsQuote(a) {
			parts = append(parts, `"`+a+`"`)
		} else {
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}

func needsQuote(s string) bool {
	if s == "" {
		return true
	}
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '"' || r == '\'' {
			return true
		}
	}
	return false
}

// ErrNotSupported is returned by provider methods that don't apply to a
// given backend. Callers silently skip — these are not real errors.
var ErrNotSupported = errors.New("agents: operation not supported by this provider")

// ErrProviderNotInstalled is returned when a mutation requires the
// underlying agent CLI but the binary isn't on PATH.
var ErrProviderNotInstalled = errors.New("agents: provider binary not installed")
