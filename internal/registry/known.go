package registry

import (
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

// gitHubSlugRE matches the "owner/repo" form of a GitHub repository slug.
// GitHub logins allow ASCII letters, digits and hyphens; repository names
// additionally allow `.` and `_`.
var gitHubSlugRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+$`)

// ValidGitHubSlug reports whether s is a well-formed "owner/repo" slug.
// Shared by marketplace validate/assemble so both enforce the same shape.
func ValidGitHubSlug(s string) bool { return gitHubSlugRE.MatchString(s) }

type toolsFile struct {
	Tools []ToolDef `yaml:"tools"`
	Packs []packDef `yaml:"packs,omitempty"`
}

// ToolDef is the YAML structure for a single tool definition.
// Exported so the catalog and marketplace packages can reuse it.
type ToolDef struct {
	Name        string     `yaml:"name"`
	DisplayName string     `yaml:"display_name"`
	Category    string     `yaml:"category"`
	Tags        []string   `yaml:"tags,omitempty"`
	BinaryNames []string   `yaml:"binary_names"`
	Packages    PackageDef `yaml:"packages"`
	// GitHub, if set, is the "owner/repo" slug of the project's GitHub
	// repository. It is hand-authored in marketplace/tools/*.yaml and drives
	// enrichment at assemble time (see GitHubInfo).
	GitHub string `yaml:"github,omitempty"`
	// GitHubInfo holds metadata fetched from the GitHub API at assemble
	// time. It is *not* meant to be hand-edited in source tool files — it
	// is populated by the marketplace assemble tool and appears in the
	// published marketplace.yaml so clients can use it without talking to
	// the GitHub API themselves.
	GitHubInfo *GitHubInfo `yaml:"github_info,omitempty"`
}

// GitHubInfo captures the subset of GitHub repository metadata that is
// interesting for display in clim (star count, description, homepage,
// license, topics, and recent activity timestamps).
type GitHubInfo struct {
	Stars       int      `yaml:"stars"`
	Forks       int      `yaml:"forks,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Homepage    string   `yaml:"homepage,omitempty"`
	License     string   `yaml:"license,omitempty"`
	Topics      []string `yaml:"topics,omitempty"`
	Archived    bool     `yaml:"archived,omitempty"`
	PushedAt    string   `yaml:"pushed_at,omitempty"`
	UpdatedAt   string   `yaml:"updated_at,omitempty"`
	FetchedAt   string   `yaml:"fetched_at,omitempty"`
}

// IsUseful reports whether the GitHubInfo contains meaningful data.
// A fully zero-valued struct is not useful; any populated metadata field is.
func (g *GitHubInfo) IsUseful() bool {
	if g == nil {
		return false
	}

	return g.Stars > 0 ||
		g.Forks > 0 ||
		g.Description != "" ||
		g.Homepage != "" ||
		g.License != "" ||
		len(g.Topics) > 0 ||
		g.Archived ||
		g.PushedAt != "" ||
		g.UpdatedAt != "" ||
		g.FetchedAt != ""
}

type packDef struct {
	Name        string   `yaml:"name"`
	DisplayName string   `yaml:"display_name"`
	Description string   `yaml:"description,omitempty"`
	Tools       []string `yaml:"tools"`
}

// PackageDef is the YAML structure for package manager identifiers.
// Exported so the catalog and marketplace packages can reuse it.
type PackageDef struct {
	Winget string `yaml:"winget,omitempty"`
	Choco  string `yaml:"choco,omitempty"`
	Scoop  string `yaml:"scoop,omitempty"`
	Brew   string `yaml:"brew,omitempty"`
	Apt    string `yaml:"apt,omitempty"`
	Snap   string `yaml:"snap,omitempty"`
	NPM    string `yaml:"npm,omitempty"`
}

// ToolsFromBytes parses catalog YAML bytes into a slice of Tools.
func ToolsFromBytes(data []byte) []Tool {
	defs := parseToolDefs(data)
	if defs == nil {
		return nil
	}
	return defsToTools(defs)
}

func parseToolDefs(data []byte) []ToolDef {
	var f toolsFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil
	}
	return f.Tools
}

// ParsePacksFromBytes parses the packs section from catalog YAML bytes.
// Returns an error if the YAML is unparsable. Returns an empty (non-nil)
// slice if the YAML is valid but contains no packs.
func ParsePacksFromBytes(data []byte) ([]Pack, error) {
	var f toolsFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing packs: %w", err)
	}
	packs := make([]Pack, 0, len(f.Packs))
	for _, pd := range f.Packs {
		p := Pack{
			Name:        pd.Name,
			DisplayName: pd.DisplayName,
			Description: pd.Description,
			ToolNames:   pd.Tools,
		}
		if p.DisplayName == "" {
			p.DisplayName = p.Name
		}
		packs = append(packs, p)
	}
	return packs, nil
}

func defsToTools(defs []ToolDef) []Tool {
	tools := make([]Tool, 0, len(defs))
	for _, d := range defs {
		t := Tool{
			Name:        d.Name,
			DisplayName: d.DisplayName,
			Category:    d.Category,
			Tags:        d.Tags,
			BinaryNames: d.BinaryNames,
			Packages: PackageIDs{
				Winget: d.Packages.Winget,
				Choco:  d.Packages.Choco,
				Scoop:  d.Packages.Scoop,
				Brew:   d.Packages.Brew,
				Apt:    d.Packages.Apt,
				Snap:   d.Packages.Snap,
				NPM:    d.Packages.NPM,
			},
			GitHubSlug: d.GitHub,
			GitHubInfo: d.GitHubInfo,
		}
		if t.DisplayName == "" {
			t.DisplayName = t.Name
		}
		if len(t.BinaryNames) == 0 {
			t.BinaryNames = []string{t.Name}
		}
		tools = append(tools, t)
	}
	return tools
}
