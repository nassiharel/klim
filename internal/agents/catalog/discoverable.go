package catalog

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/marketplace/marketplaces"
)

// discoverableEntry is the on-disk YAML schema for a single
// well-known agent marketplace. The catalog package loads every
// `marketplace/marketplaces/*.yaml` file into a copy of this struct
// and then expands it into one agents.Marketplace per provider.
//
// See marketplace/marketplaces/README.md for the full schema
// documentation.
type discoverableEntry struct {
	ID          string              `yaml:"id"`
	Name        string              `yaml:"name"`
	DisplayName string              `yaml:"display_name,omitempty"`
	Description string              `yaml:"description,omitempty"`
	Providers   []agents.ProviderID `yaml:"providers"`
	Owner       string              `yaml:"owner,omitempty"`
	URL         string              `yaml:"url,omitempty"`
	InstallSpec string              `yaml:"install_spec,omitempty"`
	Source      agents.Source       `yaml:"source,omitempty"`
}

// sourceForProvider returns the canonical agents.Source pill for a
// provider when the YAML doesn't override it. Keeps catalog-XXX values
// consistent with what the providers themselves report.
func sourceForProvider(p agents.ProviderID) agents.Source {
	switch p {
	case agents.ProviderCopilotCLI:
		return agents.SourceCatalogCopilot
	case agents.ProviderMCPRegistry:
		return agents.SourceCatalogMCP
	default:
		return agents.SourceCatalogClaude
	}
}

// loadDiscoverable parses every `discoverable/*.yaml` file embedded in
// the binary and expands each entry into one agents.Marketplace per
// provider listed under `providers:`. Returns the merged list sorted
// by Name for deterministic ordering.
//
// Errors are aggregated and returned to callers so unit tests can fail
// loudly when a file is malformed; production callers
// (DiscoverableMarketplaces) ignore the error and use the entries that
// did parse, so a single bad YAML never blocks the whole UI.
func loadDiscoverable() ([]agents.Marketplace, error) {
	files, err := fs.ReadDir(marketplaces.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("catalog: read marketplaces dir: %w", err)
	}
	var (
		out  []agents.Marketplace
		errs []string
	)
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".yaml") {
			continue
		}
		body, err := fs.ReadFile(marketplaces.FS, f.Name())
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", f.Name(), err))
			continue
		}
		var e discoverableEntry
		if err := yaml.Unmarshal(body, &e); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", f.Name(), err))
			continue
		}
		if e.Name == "" {
			errs = append(errs, fmt.Sprintf("%s: missing required field 'name'", f.Name()))
			continue
		}
		if len(e.Providers) == 0 {
			errs = append(errs, fmt.Sprintf("%s: missing required field 'providers'", f.Name()))
			continue
		}
		for _, pid := range e.Providers {
			id := e.ID
			if id == "" {
				id = e.Name
			}
			src := e.Source
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
	if len(errs) > 0 {
		return out, fmt.Errorf("catalog: %d discoverable file(s) had errors: %s", len(errs), strings.Join(errs, "; "))
	}
	return out, nil
}

// DiscoverableMarketplaces returns the curated list of discoverable
// agent marketplaces loaded from marketplace/marketplaces/.
// Each entry's Installed field is false; the snapshot merge in
// agents.Service flips it to true (or drops the discoverable copy
// entirely) once the same marketplace is also reported by a provider.
//
// A malformed YAML file in the discoverable directory surfaces as an
// error from loadDiscoverable but never blocks the caller — every
// well-formed entry still surfaces in the UI.
func DiscoverableMarketplaces() []agents.Marketplace {
	out, _ := loadDiscoverable()
	return out
}
