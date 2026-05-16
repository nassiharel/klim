// Package promote implements cross-provider sync of agent entities.
// klim treats Claude Code, Copilot CLI, and other agent CLIs as one
// ecosystem: a skill, MCP server, or plugin defined for one provider
// can be promoted to another so the user doesn't have to hand-copy
// YAML/JSON between provider config directories.
//
// The package is pure logic: it inspects the current Snapshot,
// validates the requested promotion, and produces a Plan describing
// the file copies and provider calls needed to carry it out. The TUI
// (or future CLI) is responsible for actually executing the Plan
// against a service.Service.
package promote

import (
	"errors"
	"fmt"
	"strings"
)

// EntityKind names the supported kinds. Sessions and marketplaces are
// not promotable in v1.
type EntityKind string

// EntityKind values.
const (
	KindSkill  EntityKind = "skill"
	KindMCP    EntityKind = "mcp"
	KindPlugin EntityKind = "plugin"
)

// Spec describes a single promote request.
//
//   - Kind / SubjectID identify the source row in the snapshot.
//   - TargetProvider is the provider to copy into.
//   - TargetScope narrows the target to a specific scope (user / project
//     for skills + MCPs; ignored for plugins).
//   - Force, when true, allows overwriting an existing target entry.
type Spec struct {
	Kind           EntityKind
	SubjectID      string
	SourceProvider string
	TargetProvider string
	TargetScope    string
	Force          bool
}

// Validate returns an error explaining why a Spec can't be carried
// out (missing fields, same-provider request, unsupported kind).
func (s Spec) Validate() error {
	if s.SubjectID == "" {
		return errors.New("promote: SubjectID is required")
	}
	if s.SourceProvider == "" || s.TargetProvider == "" {
		return errors.New("promote: source and target providers are required")
	}
	if s.SourceProvider == s.TargetProvider && s.Kind != KindSkill {
		// Same-provider scope changes are only meaningful for skills
		// (user <-> project). MCPs and plugins always target a
		// different provider.
		return errors.New("promote: source and target providers must differ for this kind")
	}
	switch s.Kind {
	case KindSkill, KindMCP, KindPlugin:
		return nil
	}
	return fmt.Errorf("promote: kind %q is not promotable", s.Kind)
}

// ConflictKind explains why a Plan was rejected.
type ConflictKind int

// ConflictKind values.
const (
	ConflictNone ConflictKind = iota
	ConflictDuplicate
	ConflictUnsupported
	ConflictMissing
)

// String returns a short label.
func (c ConflictKind) String() string {
	switch c {
	case ConflictDuplicate:
		return "duplicate"
	case ConflictUnsupported:
		return "unsupported"
	case ConflictMissing:
		return "missing"
	}
	return "ok"
}

// Plan is the materialised description of a promotion. The TUI
// dispatches Plan.Apply(svc) — separated so the Plan is also useful
// for "preview" UIs (we don't ship one yet but we will).
//
// FileCopies are absolute source→destination paths. ProviderOps are
// callable steps the executor must run via the target provider.
type Plan struct {
	Spec          Spec
	Conflict      ConflictKind
	ConflictMsg   string
	FileCopies    []FileCopy
	ProviderOps   []ProviderOp
	SourceSummary string // human label of what's being moved
	TargetSummary string // human label of where it's going
}

// FileCopy is one file we need to write at the target.
//
// Body, when non-nil, replaces source content (e.g. for SKILL.md
// frontmatter conversion); empty Body means "copy bytes verbatim".
// ModeUser is the desired file mode (default 0o644).
type FileCopy struct {
	Src     string
	Dst     string
	Body    []byte
	Mode    uint32
	MkdirOK bool
}

// ProviderOp is one mutating provider call the executor must run.
// Kind identifies the call site; the Spec field carries kind-specific
// arguments (we use a tagged union of small ad-hoc structs rather
// than reflection).
type ProviderOp struct {
	Kind          ProviderOpKind
	MCPName       string
	MCPTransport  string
	MCPCommand    string
	MCPArgs       []string
	MCPURL        string
	MCPEnv        map[string]string
	MCPScope      string
	PluginRefName string
	PluginRefMP   string
}

// ProviderOpKind names the executor steps.
type ProviderOpKind int

// ProviderOpKind values.
const (
	OpAddMCP ProviderOpKind = iota
	OpInstallPlugin
)

// SkillRef is the minimal view of a skill the planner needs.
type SkillRef struct {
	Name         string
	Provider     string
	Scope        string
	SourcePlugin string
	Path         string
	Description  string
	WhenToUse    string
	AllowedTools string
	Model        string
}

// MCPRef is the minimal MCP view.
type MCPRef struct {
	Name      string
	Provider  string
	Scope     string
	Transport string
	Command   string
	Args      []string
	URL       string
	EnvKeys   []string
}

// PluginRef is the minimal plugin view.
type PluginRef struct {
	Name        string
	Provider    string
	Marketplace string
	Installed   bool
}

// Snapshot bundles the rows the planner reads from. The TUI converts
// agents.Snapshot into this shape via Builder methods so we avoid an
// import cycle.
type Snapshot struct {
	Skills  []SkillRef
	MCPs    []MCPRef
	Plugins []PluginRef
}

// HasSkill returns true if the snapshot already has a skill with
// (name, provider, scope) matching `s`.
func (snap Snapshot) HasSkill(name, provider, scope string) bool {
	name = strings.ToLower(name)
	for _, sk := range snap.Skills {
		if strings.ToLower(sk.Name) == name && sk.Provider == provider && (scope == "" || sk.Scope == scope) {
			return true
		}
	}
	return false
}

// HasMCP returns true if the snapshot already has an MCP with
// (name, provider) matching the args (scope is informational only).
func (snap Snapshot) HasMCP(name, provider string) bool {
	name = strings.ToLower(name)
	for _, m := range snap.MCPs {
		if strings.ToLower(m.Name) == name && m.Provider == provider {
			return true
		}
	}
	return false
}

// HasPlugin returns true if the plugin is already installed at the
// target provider.
func (snap Snapshot) HasPlugin(name, provider string) bool {
	name = strings.ToLower(name)
	for _, p := range snap.Plugins {
		if strings.ToLower(p.Name) == name && p.Provider == provider && p.Installed {
			return true
		}
	}
	return false
}

// FindSkill returns the skill matching the spec's SubjectID; ok=false
// when the source is missing from the snapshot.
func (snap Snapshot) FindSkill(id, provider string) (SkillRef, bool) {
	for _, sk := range snap.Skills {
		if sk.Name == id && sk.Provider == provider {
			return sk, true
		}
	}
	return SkillRef{}, false
}

// FindMCP returns the MCP matching (name, provider).
func (snap Snapshot) FindMCP(name, provider string) (MCPRef, bool) {
	for _, m := range snap.MCPs {
		if m.Name == name && m.Provider == provider {
			return m, true
		}
	}
	return MCPRef{}, false
}

// FindPlugin returns the plugin matching (name, provider).
func (snap Snapshot) FindPlugin(name, provider string) (PluginRef, bool) {
	for _, p := range snap.Plugins {
		if p.Name == name && p.Provider == provider {
			return p, true
		}
	}
	return PluginRef{}, false
}
