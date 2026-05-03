package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/custompacks"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/teamfile"
)

// Reference describes a place where a tool is mentioned: the local
// .clim.yaml, a registered project's .clim.yaml, a marketplace pack, or
// a custom pack. It's the shared shape used by both `clim why` and
// `clim info` so the two commands cannot drift out of sync.
type Reference struct {
	Kind        string `yaml:"kind"                            json:"kind"`
	Name        string `yaml:"name,omitempty"                  json:"name,omitempty"`
	DisplayName string `yaml:"display_name,omitempty"          json:"display_name,omitempty"`
	Path        string `yaml:"path,omitempty"                  json:"path,omitempty"`
	Required    bool   `yaml:"required,omitempty"              json:"required,omitempty"`
	Constraint  string `yaml:"version_constraint,omitempty"    json:"version_constraint,omitempty"`
}

// PackageEntry is one populated package-manager ID for a tool. Shared
// between `clim why` (AvailableVia) and `clim info` (Packages) so the
// list of supported sources cannot drift between the two commands the
// next time a package manager is added or renamed.
type PackageEntry struct {
	Source string `json:"source"`
	ID     string `json:"id"`
}

// CollectPackageEntries returns the populated PackageEntries for pkgs in
// canonical display order. Empty IDs are skipped. Both `clim why` and
// `clim info` consume this so they list the same sources every time.
func CollectPackageEntries(pkgs registry.PackageIDs) []PackageEntry {
	all := []PackageEntry{
		{Source: "winget", ID: pkgs.Winget},
		{Source: "choco", ID: pkgs.Choco},
		{Source: "scoop", ID: pkgs.Scoop},
		{Source: "brew", ID: pkgs.Brew},
		{Source: "apt", ID: pkgs.Apt},
		{Source: "snap", ID: pkgs.Snap},
		{Source: "npm", ID: pkgs.NPM},
	}
	out := make([]PackageEntry, 0, len(all))
	for _, e := range all {
		if e.ID != "" {
			out = append(out, e)
		}
	}
	return out
}

// CollectReferences scans the four sources where a tool name can appear
// (CWD-or-ancestor .clim.yaml, registered projects, marketplace packs,
// custom packs) and returns matched references plus any non-fatal
// warnings encountered along the way.
//
// Both teamfile parses (local + registered) accumulate parse errors
// into warnings rather than silently dropping them — otherwise a
// malformed file could mislead the caller into reporting "no
// references found" when there really are some.
//
// Pack lookups go through cliCtx via svcFrom(cmd) so tests can stub
// the service.
func CollectReferences(cmd *cobra.Command, toolName string) ([]Reference, []string) {
	var refs []Reference
	var warnings []string

	// 1) Local .clim.yaml in or above CWD.
	cwd, _ := os.Getwd()
	var seenTeamPath string
	if cwd != "" {
		path := teamfile.Find(cwd)
		if path != "" {
			seenTeamPath = path
			tf, err := teamfile.Parse(path)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("could not parse %s: %v", path, err))
			} else {
				for _, req := range tf.Tools {
					if req.Name == toolName {
						refs = append(refs, Reference{
							Kind:       "teamfile",
							Path:       path,
							Required:   true,
							Constraint: req.Version,
						})
					}
				}
				for _, opt := range tf.Optional {
					if opt.Name == toolName {
						refs = append(refs, Reference{
							Kind:       "teamfile",
							Path:       path,
							Required:   false,
							Constraint: opt.Version,
						})
					}
				}
			}
		}
	}

	// 2) Registered projects.
	projects, projErr := teamfile.LoadProjects()
	if projErr != nil {
		warnings = append(warnings, fmt.Sprintf("could not load project registry: %v", projErr))
	}
	for _, proj := range projects {
		climPath := filepath.Join(proj.Path, ".clim.yaml")
		if seenTeamPath != "" && filepath.Clean(climPath) == filepath.Clean(seenTeamPath) {
			continue
		}
		tf, err := teamfile.Parse(climPath)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("could not parse %s: %v", climPath, err))
			continue
		}
		for _, req := range tf.Tools {
			if req.Name == toolName {
				refs = append(refs, Reference{
					Kind:       "project",
					Name:       proj.Name,
					Path:       climPath,
					Required:   true,
					Constraint: req.Version,
				})
			}
		}
		for _, opt := range tf.Optional {
			if opt.Name == toolName {
				refs = append(refs, Reference{
					Kind:       "project",
					Name:       proj.Name,
					Path:       climPath,
					Required:   false,
					Constraint: opt.Version,
				})
			}
		}
	}

	// 3) Marketplace packs.
	packs, packErr := svcFrom(cmd).LoadPacks(cmd.Context())
	if packErr != nil {
		warnings = append(warnings, fmt.Sprintf("could not load packs: %v", packErr))
	}
	for _, pack := range packs {
		for _, pToolName := range pack.ToolNames {
			if pToolName == toolName {
				refs = append(refs, Reference{
					Kind:        "pack",
					Name:        pack.Name,
					DisplayName: pack.DisplayName,
				})
			}
		}
	}

	// 4) Custom packs.
	if cp, cpErr := custompacks.Load(); cpErr != nil {
		warnings = append(warnings, fmt.Sprintf("could not load custom packs: %v", cpErr))
	} else {
		for _, pack := range cp {
			for _, pToolName := range pack.ToolNames {
				if pToolName == toolName {
					refs = append(refs, Reference{
						Kind:        "custom_pack",
						Name:        pack.Name,
						DisplayName: pack.DisplayName,
					})
				}
			}
		}
	}

	return refs, warnings
}
