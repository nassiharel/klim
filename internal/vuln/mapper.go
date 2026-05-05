package vuln

import (
	"strings"

	"github.com/nassiharel/clim/internal/registry"
)

// Map returns the OSV coordinates we can query for a given installed
// tool, plus a non-empty reason string when the tool was *not*
// mappable (e.g. no version detected, no ecosystem-mappable package
// id, no GitHub slug). The reason becomes the Skip entry in Report.
//
// A single tool may produce multiple coords: a tool installed via
// brew that is also a node package (e.g. yarn) may legitimately
// match advisories under both ecosystems. Callers query each coord
// and de-duplicate by Vulnerability.ID.
//
// Mapping rules (in order of preference):
//
//   - tool.Packages.NPM     → npm
//   - tool.Packages.Brew    → Homebrew
//   - tool.GitHubInfo + slug→ GitHub (synthetic ecosystem; see types.go)
//
// Additional ecosystems (Go modules via go.mod, PyPI, crates.io)
// would slot in cleanly once we have catalog metadata for them.
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
	if t.Packages.Brew != "" {
		coords = append(coords, Coord{
			Ecosystem: EcosystemHomebrew,
			Package:   t.Packages.Brew,
			Version:   v,
		})
	}
	if slug := strings.TrimSpace(t.GitHubSlug); slug != "" {
		coords = append(coords, Coord{
			Ecosystem: EcosystemGitHub,
			Package:   slug,
			Version:   v,
		})
	}

	if len(coords) == 0 {
		return nil, "no ecosystem mapping (no npm/brew package id and no GitHub slug)"
	}
	return coords, ""
}
