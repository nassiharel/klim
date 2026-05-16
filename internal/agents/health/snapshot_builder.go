package health

import "time"

// SnapshotBuilder is a small helper for callers (the TUI, future
// CLI) that need to build a health.Snapshot from the rich
// agents.Snapshot without dragging the agents package into the
// health package (which would create an import cycle).
type SnapshotBuilder struct {
	snap Snapshot
}

// NewSnapshotBuilder returns an empty builder.
func NewSnapshotBuilder() *SnapshotBuilder { return &SnapshotBuilder{} }

// Build returns the accumulated Snapshot.
func (b *SnapshotBuilder) Build() Snapshot { return b.snap }

// AddMarketplace appends a marketplace ref.
func (b *SnapshotBuilder) AddMarketplace(name, provider, url, source string, lastSynced time.Time) {
	b.snap.Marketplaces = append(b.snap.Marketplaces, MarketplaceRef{
		Name: name, Provider: provider, URL: url, Source: source, LastSynced: lastSynced,
	})
}

// AddPlugin appends a plugin ref.
func (b *SnapshotBuilder) AddPlugin(name, provider, marketplace string, installed, enabled bool, installPath, version string) {
	b.snap.Plugins = append(b.snap.Plugins, PluginRef{
		Name: name, Provider: provider, Marketplace: marketplace,
		Installed: installed, Enabled: enabled,
		InstallPath: installPath, Version: version,
	})
}

// AddSkill appends a skill ref.
func (b *SnapshotBuilder) AddSkill(name, provider, scope, sourcePlugin, path string) {
	b.snap.Skills = append(b.snap.Skills, SkillRef{
		Name: name, Provider: provider, Scope: scope, SourcePlugin: sourcePlugin, Path: path,
	})
}

// AddMCP appends an MCP ref.
func (b *SnapshotBuilder) AddMCP(name, provider, scope, transport, command string, args []string, url string) {
	b.snap.MCPs = append(b.snap.MCPs, MCPRef{
		Name: name, Provider: provider, Scope: scope,
		Transport: transport, Command: command, Args: args, URL: url,
	})
}

// AddProvider appends a provider ref.
func (b *SnapshotBuilder) AddProvider(id string, installed bool, version string) {
	b.snap.Providers = append(b.snap.Providers, ProviderRef{
		ID: id, Installed: installed, Version: version,
	})
}

// AddConfigFile registers a JSON config file the broken-JSON check
// should validate.
func (b *SnapshotBuilder) AddConfigFile(path, provider string) {
	b.snap.ConfigFiles = append(b.snap.ConfigFiles, ConfigFileRef{Path: path, Provider: provider})
}
