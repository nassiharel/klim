package catalog

import "github.com/nassiharel/klim/internal/agents"

// DefaultDiscoverableMarketplaces is the curated, statically-shipped
// list of well-known third-party agent marketplaces klim surfaces in
// the Marketplaces sub-tab. They appear with Installed=false until the
// user adds them via the provider CLI (e.g. `claude /plugin marketplace
// add <repo>`); the Marketplaces tab's "Add to library" action drives
// that. Entries already registered locally (in known_marketplaces.json
// or scanned from disk) are de-duplicated by name during the snapshot
// merge in agents.Service.
//
// Add new entries here when you want to expose a community marketplace
// to every klim user out of the box. Keep the list short and curated;
// per-user picks belong in user config, not here.
var DefaultDiscoverableMarketplaces = []agents.Marketplace{
	{
		ID:          "claude-plugins-official",
		Name:        "claude-plugins-official",
		DisplayName: "Anthropic Official",
		Description: "Anthropic's curated Claude Code plugin marketplace",
		Provider:    agents.ProviderClaudeCode,
		Owner:       "anthropics",
		URL:         "https://github.com/anthropics/claude-plugins-official",
		InstallSpec: "anthropics/claude-plugins-official",
		Source:      agents.SourceCatalogClaude,
	},
	{
		ID:          "openai-codex-plugin-cc",
		Name:        "openai-codex-plugin-cc",
		DisplayName: "OpenAI Codex Plugin",
		Description: "OpenAI's Codex plugin marketplace for Claude Code",
		Provider:    agents.ProviderClaudeCode,
		Owner:       "openai",
		URL:         "https://github.com/openai/codex-plugin-cc",
		InstallSpec: "openai/codex-plugin-cc",
		Source:      agents.SourceCatalogClaude,
	},
	{
		ID:          "superpowers",
		Name:        "superpowers",
		DisplayName: "Superpowers",
		Description: "Community productivity plugins for Claude Code",
		Provider:    agents.ProviderClaudeCode,
		Owner:       "obra",
		URL:         "https://github.com/obra/superpowers-marketplace",
		InstallSpec: "obra/superpowers-marketplace",
		Source:      agents.SourceCatalogClaude,
	},
	{
		ID:          "copilot-plugins",
		Name:        "copilot-plugins",
		DisplayName: "GitHub Copilot Plugins",
		Description: "GitHub's official Copilot CLI plugin marketplace",
		Provider:    agents.ProviderCopilotCLI,
		Owner:       "github",
		URL:         "https://github.com/github/copilot-plugins",
		InstallSpec: "github/copilot-plugins",
		Source:      agents.SourceCatalogCopilot,
	},
	{
		ID:          "awesome-copilot",
		Name:        "awesome-copilot",
		DisplayName: "Awesome Copilot",
		Description: "Community-curated Copilot plugins and skills",
		Provider:    agents.ProviderCopilotCLI,
		Owner:       "github",
		URL:         "https://github.com/github/awesome-copilot",
		InstallSpec: "github/awesome-copilot",
		Source:      agents.SourceCatalogCopilot,
	},
}

// DiscoverableMarketplaces returns a copy of the curated discoverable
// list with Installed=false. Returning a copy keeps callers from
// accidentally mutating the package-level slice when post-processing
// (e.g., marking entries installed during snapshot merge).
func DiscoverableMarketplaces() []agents.Marketplace {
	out := make([]agents.Marketplace, len(DefaultDiscoverableMarketplaces))
	for i, m := range DefaultDiscoverableMarketplaces {
		m.Installed = false
		out[i] = m
	}
	return out
}
