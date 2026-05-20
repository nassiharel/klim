// Package marketplaces embeds the discoverable agent-marketplace YAML
// definitions so they can be loaded at compile time by the catalog
// loader in internal/agents/catalog.
package marketplaces

import "embed"

// FS contains every *.yaml file in this directory. The catalog package
// reads and expands these into agents.Marketplace records.
//
//go:embed *.yaml
var FS embed.FS
