package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nassiharel/clim/internal/custompacks"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/teamfile"
)

// samePath reports whether a and b refer to the same filesystem path.
// On Windows the comparison is case-insensitive (the OS treats
// `C:\Users\me` and `c:\users\me` as identical). Other platforms keep
// the standard byte-wise comparison since macOS/Linux file systems
// are routinely case-sensitive even when the kernel is not.
func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

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

// FormatReference renders a Reference as a single human-readable line
// for the text output of `clim info` and `clim why`. Both surfaces
// consume this directly so the surrounding wording — and any new
// Reference.Kind that gets added — stays in lockstep across the two
// commands. Required + optional refs both preserve their version
// constraint via roleWithConstraint.
func FormatReference(ref Reference) string {
	switch ref.Kind {
	case "teamfile":
		role := "optional"
		if ref.Required {
			role = "required"
		}
		return fmt.Sprintf(".clim.yaml (%s) — %s", roleWithConstraint(role, ref.Constraint), ref.Path)
	case "project":
		role := "optional"
		if ref.Required {
			role = "required"
		}
		return fmt.Sprintf("Project %q (%s) — %s", ref.Name, roleWithConstraint(role, ref.Constraint), ref.Path)
	case "pack":
		if ref.DisplayName != "" {
			return fmt.Sprintf("Pack %q (%s)", ref.DisplayName, ref.Name)
		}
		return fmt.Sprintf("Pack %q", ref.Name)
	case "custom_pack":
		if ref.DisplayName != "" {
			return fmt.Sprintf("Custom pack %q (%s)", ref.DisplayName, ref.Name)
		}
		return fmt.Sprintf("Custom pack %q", ref.Name)
	}
	return ref.Kind + " " + ref.Name
}

// roleWithConstraint joins the required/optional role label with an
// optional version constraint so a teamfile or project pin like
// `>=1.28` is preserved in the human output. Without this, an
// optional-but-pinned reference would render as just "(optional)" and
// silently drop the pin.
func roleWithConstraint(role, constraint string) string {
	if constraint == "" {
		return role
	}
	return role + " " + constraint
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

	// 1) Local .clim.yaml in or above CWD. If we can't resolve the
	// CWD (deleted/inaccessible directory, weird FS state) we skip
	// the local-teamfile branch but record a warning so callers
	// don't silently report fewer references than actually exist.
	cwd, cwdErr := os.Getwd()
	if cwdErr != nil {
		warnings = append(warnings, fmt.Sprintf("could not determine working directory: %v (skipping local .clim.yaml lookup)", cwdErr))
	}
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
		if seenTeamPath != "" && samePath(climPath, seenTeamPath) {
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
