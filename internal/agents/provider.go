package agents

import "context"

// Provider is the abstraction over a single agent CLI's ecosystem.
// All read methods are expected to be fast (filesystem reads + caching);
// mutating methods may shell out to the provider's CLI.
//
// Methods that don't apply to a given backend return ErrNotSupported.
// The Service layer silently skips these, so callers never have to
// special-case capability detection.
type Provider interface {
	// ID is the stable identifier used in CLI flags and config keys
	// (e.g. "claude-code", "copilot-cli", "mcp-registry").
	ID() ProviderID

	// DisplayName is the human-readable name shown in the TUI.
	DisplayName() string

	// Detect reports whether the provider's binary is installed and
	// returns its version if discoverable.
	Detect(ctx context.Context) Status

	// Read-only enumeration. Each may return ErrNotSupported.
	Marketplaces(ctx context.Context) ([]Marketplace, error)
	Plugins(ctx context.Context) ([]Plugin, error)
	Skills(ctx context.Context) ([]Skill, error)
	MCPs(ctx context.Context) ([]MCP, error)
	Sessions(ctx context.Context) ([]Session, error)

	// Mutations. Each may return ErrNotSupported or ErrProviderNotInstalled.
	AddMarketplace(ctx context.Context, spec string) error
	RemoveMarketplace(ctx context.Context, name string) error
	InstallPlugin(ctx context.Context, ref PluginRef) error
	UninstallPlugin(ctx context.Context, id string) error
	EnablePlugin(ctx context.Context, id string, enabled bool) error
	AddMCP(ctx context.Context, spec MCPSpec) error
	RemoveMCP(ctx context.Context, name string) error
	EnableMCP(ctx context.Context, name string, enabled bool) error
	DeleteSession(ctx context.Context, id string) error

	// BuildLaunch constructs the command to exec for a LaunchSpec.
	// klim never spawns the agent itself — it returns the plan,
	// shows it in a confirmation modal, then hands the terminal over
	// via tea.ExecProcess (or os.Exec from the CLI).
	BuildLaunch(spec LaunchSpec) (ExecPlan, error)
}

// Registry is an ordered collection of Providers. Iteration order is
// stable and matches the order providers are registered.
type Registry struct {
	providers []Provider
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds a provider. Duplicates (same ID) are ignored.
func (r *Registry) Register(p Provider) {
	for _, existing := range r.providers {
		if existing.ID() == p.ID() {
			return
		}
	}
	r.providers = append(r.providers, p)
}

// Providers returns the registered providers in registration order.
func (r *Registry) Providers() []Provider {
	out := make([]Provider, len(r.providers))
	copy(out, r.providers)
	return out
}

// Get returns the provider with the given ID, or nil.
func (r *Registry) Get(id ProviderID) Provider {
	for _, p := range r.providers {
		if p.ID() == id {
			return p
		}
	}
	return nil
}

// Snapshot is the merged in-memory view of every entity across every
// provider. The Service produces a Snapshot per scan and feeds it to
// TUI and CLI consumers.
type Snapshot struct {
	Marketplaces   []Marketplace
	Plugins        []Plugin
	Skills         []Skill
	MCPs           []MCP
	Sessions       []Session
	ProviderStatus map[ProviderID]Status
}

// Count returns the count of entities of the given type.
func (s *Snapshot) Count(typ EntityType) int {
	switch typ {
	case EntityMarketplace:
		return len(s.Marketplaces)
	case EntityPlugin:
		return len(s.Plugins)
	case EntitySkill:
		return len(s.Skills)
	case EntityMCP:
		return len(s.MCPs)
	case EntitySession:
		return len(s.Sessions)
	}
	return 0
}
