// Package clim embeds the marketplace catalog for use by internal packages.
package clim

import _ "embed"

//go:embed marketplace.yaml
var MarketplaceYAML []byte
