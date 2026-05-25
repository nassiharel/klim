package catalog

import (
	"os"
	"sort"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/paths"
	"github.com/nassiharel/klim/internal/registry"
)

// sourceForProvider returns the canonical agents.Source pill for a
// provider when the YAML doesn't override it. Keeps catalog-XXX values
// consistent with what the providers themselves report.
func sourceForProvider(p agents.ProviderID) agents.Source {
	switch p {
	case agents.ProviderCopilotCLI:
		return agents.SourceCatalogCopilot
	default:
		return agents.SourceCatalogClaude
	}
}

// expandDefs converts parsed AgentMarketplaceDef records into
// agents.Marketplace records, expanding multi-provider entries into
// one record per provider. Returns the list sorted by Name then
// Provider for deterministic ordering.
func expandDefs(defs []registry.AgentMarketplaceDef) []agents.Marketplace {
	var out []agents.Marketplace
	for _, e := range defs {
		if e.Name == "" || len(e.Providers) == 0 {
			continue
		}
		if e.InstallSpec == "" && e.URL == "" {
			continue
		}
		for _, pStr := range e.Providers {
			pid := agents.ProviderID(pStr)
			id := e.ID
			if id == "" {
				id = e.Name
			}
			// When a single YAML entry lists multiple providers,
			// each expansion gets a provider-qualified ID so that
			// resolveDetailRow (which keys by ID alone) can
			// unambiguously identify each Marketplace record.
			if len(e.Providers) > 1 {
				id = id + ":" + string(pid)
			}
			src := agents.Source(e.Source)
			if src == "" {
				src = sourceForProvider(pid)
			}
			out = append(out, agents.Marketplace{
				ID:          id,
				Name:        e.Name,
				DisplayName: e.DisplayName,
				Description: e.Description,
				Provider:    pid,
				Owner:       e.Owner,
				URL:         e.URL,
				InstallSpec: e.InstallSpec,
				Source:      src,
				Installed:   false,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Provider < out[j].Provider
	})
	return out
}

// DiscoverableMarketplaces returns the curated list of discoverable
// agent marketplaces parsed from the agent_marketplaces section of
// the cached marketplace.yaml (the same file that carries tool and
// pack definitions). Each entry's Installed field is false; the
// snapshot merge in agents.Service flips it to true (or drops the
// discoverable copy entirely) once the same marketplace is also
// reported by a provider.
//
// Returns nil when the catalog cache is missing (first run before a
// catalog fetch) or unparsable — callers treat this the same way
// they treat an empty catalog.
func DiscoverableMarketplaces() []agents.Marketplace {
	cachePath, err := paths.CatalogCache()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil
	}
	defs, err := registry.ParseAgentMarketplaceDefsFromBytes(data)
	if err != nil || len(defs) == 0 {
		return nil
	}
	return expandDefs(defs)
}
