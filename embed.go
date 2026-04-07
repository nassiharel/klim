// Package clim embeds the marketplace catalog for use by internal packages.
package clim

import _ "embed"

// MarketplaceYAML contains the embedded marketplace catalog YAML data.
//
//go:embed marketplace.yaml
var MarketplaceYAML []byte
