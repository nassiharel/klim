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
package agents

import (
	"encoding/json"
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

// String satisfies fmt.Stringer.
func (p ProviderID) String() string { return string(p) }

// ProviderID values for the built-in providers.
const (
	ProviderClaudeCode ProviderID = "claude-code"
	ProviderCopilotCLI ProviderID = "copilot-cli"
)

// Status reports whether a provider's binary is installed and detected.
// Error carries the *message* (not the underlying error value) so the
// JSON / YAML schema is a useful string instead of '{}' — encoding/json
// renders interface error values as their concrete type, which for
// standard errors is an empty struct.
type Status struct {
	Installed bool   `yaml:"installed"`
	Version   string `yaml:"version,omitempty"`
	BinPath   string `yaml:"bin_path,omitempty"`
	Error     string `yaml:"error,omitempty"`
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
	// InstallSpec is the argument to pass to the provider's
	// AddMarketplace verb (e.g. `owner/repo`, an https URL, or a
	// local path). For installed marketplaces it may be empty.
	InstallSpec string    `yaml:"install_spec,omitempty"`
	PluginCount int       `yaml:"plugin_count,omitempty"`
	LastSynced  time.Time `yaml:"last_synced,omitempty"`
	Source      Source    `yaml:"source"`
	// Installed reports whether this marketplace is currently
	// registered with the provider (true) or is a discoverable
	// catalog entry the user could add (false).
	Installed bool `yaml:"installed"`
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

// LiveState is the derived run-state of a session, computed from the
// tail of its event log. It complements the persisted Status field
// (which only flips on a terminal event): LiveState reflects "what is
// this session doing right now?" and is updated on every scan.
//
// The state machine follows ghcpCliDashboard's model:
//
//   - StateWorking   — at least one tool execution is still pending.
//   - StateThinking  — a turn is in progress with no pending tool calls.
//   - StateWaiting   — the agent is blocked on user input (ask_user /
//     ask_permission), with the question in WaitingContext.
//   - StateIdle      — no events for ≥60s; the CLI is probably idle or
//     has crashed.
//   - StateUnknown   — no event log or unparseable.
type LiveState string

// LiveState values.
const (
	StateUnknown  LiveState = ""
	StateWorking  LiveState = "working"
	StateThinking LiveState = "thinking"
	StateWaiting  LiveState = "waiting"
	StateIdle     LiveState = "idle"
)

// Session is a saved or recent agent session.
//
// Fields after Source are optional enrichment derived from the
// per-session event log (where available). They are populated by the
// provider's enrich pass and used by `klim agents sessions` to render
// a glanceable dashboard. All such fields use `omitempty` so JSON /
// YAML output stays compact for providers that can't fill them.
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

	// ---------- Enrichment fields (optional) ----------

	// LiveState is the derived running state at scan time.
	LiveState LiveState `json:"live_state,omitempty" yaml:"live_state,omitempty"`

	// WaitingContext carries the active ask_user prompt text when
	// LiveState == StateWaiting. Truncated to ~200 chars.
	WaitingContext string `json:"waiting_context,omitempty" yaml:"waiting_context,omitempty"`

	// RecentActivity is a short, one-line description of the most
	// recent event (tool call name, last assistant message snippet,
	// etc.). Capped at ~120 chars for terminal rendering.
	RecentActivity string `json:"recent_activity,omitempty" yaml:"recent_activity,omitempty"`

	// Branch is the active git branch at the session's cwd, read
	// live from .git/HEAD. Empty when the cwd isn't a git repo.
	Branch string `json:"branch,omitempty" yaml:"branch,omitempty"`

	// Repository is the derived repo name (last segment of the
	// remote URL, or the directory basename when no remote).
	Repository string `json:"repository,omitempty" yaml:"repository,omitempty"`

	// Group is the smart project group resolved via grouping.Resolve.
	// Used as the section header in the grouped CLI / TUI list.
	Group string `json:"group,omitempty" yaml:"group,omitempty"`

	// RestartCommand is a shell-paste-ready snippet that resumes
	// this session in a new terminal: `cd "<cwd>" && <cli> --resume <id>`.
	RestartCommand string `json:"restart_command,omitempty" yaml:"restart_command,omitempty"`

	// ToolCounts maps tool names to call counts (e.g. {"Bash": 4}).
	// Used by the Stats tab and the tools-used bar chart.
	ToolCounts map[string]int `json:"tool_counts,omitempty" yaml:"tool_counts,omitempty"`

	// MCPServers lists MCP server names that participated in this
	// session.
	MCPServers []string `json:"mcp_servers,omitempty" yaml:"mcp_servers,omitempty"`

	// SubagentRuns counts subagent invocations seen across the log.
	SubagentRuns int `json:"subagent_runs,omitempty" yaml:"subagent_runs,omitempty"`

	// BackgroundTasks is the number of subagents still running at
	// scan time (started events minus completed events).
	BackgroundTasks int `json:"background_tasks,omitempty" yaml:"background_tasks,omitempty"`

	// Starred is true when the session is in the bookmarks store.
	// Hydrated by Service.LoadAll, not by providers.
	Starred bool `json:"starred,omitempty" yaml:"starred,omitempty"`
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

// ---------- structured-output helpers ----------
//
// encoding/json's `omitempty` does NOT recognise a zero-value
// time.Time as empty (only nil pointers / zero scalars trigger it).
// Without these custom marshalers a Marketplace with no LastSynced
// would emit '"LastSynced": "0001-01-01T00:00:00Z"' — visually
// broken in JSON output. (The same issue applied to YAML when
// printYAML routed through JSON. After the PR-12 schema-revert,
// agents YAML uses direct yaml.Marshal via printYAMLDirect, so it
// honours the yaml: tags here and emits snake_case with omitempty
// behaviour appropriate for those tags.)

// MarshalJSON omits a zero LastSynced. Other fields use Go field
// names — agents list / search emitted Go names for JSON since
// before this PR; the JSON schema is intentionally unchanged.
func (m Marketplace) MarshalJSON() ([]byte, error) {
	type alias Marketplace
	a := alias(m)
	raw, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, err
	}
	if m.LastSynced.IsZero() {
		delete(generic, "LastSynced")
	}
	return json.Marshal(generic)
}

// MarshalJSON omits zero Created / LastModified on Sessions.
//
// New enrichment fields (LiveState, RecentActivity, ToolCounts, etc.)
// carry json:"…,omitempty" tags directly, so they're stripped by the
// standard encoder before we even see them here. We only need custom
// handling for time.Time (which omitempty doesn't recognise) and for
// the nil-vs-empty distinction on ToolCounts: json.Marshal will emit
// `"tool_counts": null` for a nil map even with omitempty, so we delete
// it explicitly when the map ended up empty.
func (s Session) MarshalJSON() ([]byte, error) {
	type alias Session
	a := alias(s)
	raw, err := json.Marshal(a)
	if err != nil {
		return nil, err
	}
	var generic map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, err
	}
	if s.Created.IsZero() {
		delete(generic, "Created")
	}
	if s.LastModified.IsZero() {
		delete(generic, "LastModified")
	}
	if len(s.ToolCounts) == 0 {
		delete(generic, "tool_counts")
	}
	if len(s.MCPServers) == 0 {
		delete(generic, "mcp_servers")
	}
	return json.Marshal(generic)
}
