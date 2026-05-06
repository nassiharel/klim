package vuln

import (
	"strings"

	"github.com/nassiharel/klim/internal/registry"
)

// Map returns the OSV coordinates we can query for a given installed
// tool, plus a non-empty reason string when the tool was *not*
// mappable (e.g. no version detected, no ecosystem-mappable package
// id). The reason becomes the Skip entry in Report.
//
// Mapping rules:
//
//   - tool.Packages.NPM     → npm
//
// We deliberately limit ourselves to OSV ecosystems that respond
// successfully to /v1/query without authentication. Notably:
//
//   - "Homebrew" is NOT a valid OSV ecosystem (OSV returns HTTP 400
//     "Invalid ecosystem"). GHSA mirrors brew formulas through the
//     underlying language ecosystems instead, which we already cover
//     when the tool also has an npm id.
//   - "GitHub" by repo slug is not a query path either; OSV models
//     advisories per-ecosystem, not per-repo.
//
// Adding Go modules / PyPI / crates / RubyGems would slot in cleanly
// once the marketplace catalog grows package ids for them.
func Map(t registry.Tool) (coords []Coord, skipReason string) {
	if !t.IsInstalled() {
		return nil, "not installed"
	}
	primary := t.PrimaryInstance()
	if primary == nil || primary.Version == "" {
		return nil, "no version detected"
	}
	v := strings.TrimSpace(primary.Version)

	if t.Packages.NPM != "" {
		coords = append(coords, Coord{
			Ecosystem: EcosystemNPM,
			Package:   t.Packages.NPM,
			Version:   v,
		})
	}

	if len(coords) == 0 {
		return nil, "no OSV-queryable ecosystem (only npm packages currently supported)"
	}
	return coords, ""
}
