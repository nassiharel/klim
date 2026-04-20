// Package custompacks manages user-created pack definitions stored
// in ~/.config/clim/marketplace/custom-packs.yaml. Custom packs use the same
// registry.Pack schema as marketplace packs but are persisted locally.
package custompacks

import (
	"github.com/nassiharel/clim/internal/fileutil"
	"github.com/nassiharel/clim/internal/paths"
	"github.com/nassiharel/clim/internal/registry"
)

// packFile is the on-disk YAML structure.
type packFile struct {
	Packs []packDef `yaml:"packs"`
}

// packDef is the YAML representation of a single custom pack.
type packDef struct {
	Name        string   `yaml:"name"`
	DisplayName string   `yaml:"display_name,omitempty"`
	Description string   `yaml:"description,omitempty"`
	Tools       []string `yaml:"tools"`
}

const yamlHeader = "# clim — Custom Packs\n# User-created packs. Managed by clim; safe to edit manually.\n\n"

// StoragePath returns the path to the custom packs file.
func StoragePath() (string, error) {
	return paths.CustomPacks()
}

// Load reads all custom packs from disk. Returns an empty (non-nil) slice
// if the file doesn't exist yet.
func Load() ([]registry.Pack, error) {
	path, err := paths.CustomPacks()
	if err != nil {
		return nil, err
	}

	var f packFile
	found, err := fileutil.ReadYAML(path, &f)
	if err != nil {
		return nil, err
	}
	if !found {
		return []registry.Pack{}, nil
	}

	packs := make([]registry.Pack, 0, len(f.Packs))
	for _, pd := range f.Packs {
		p := registry.Pack{
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

// Save writes all custom packs to disk atomically.
func Save(packs []registry.Pack) error {
	path, err := paths.CustomPacks()
	if err != nil {
		return err
	}

	var f packFile
	for _, p := range packs {
		f.Packs = append(f.Packs, packDef{
			Name:        p.Name,
			DisplayName: p.DisplayName,
			Description: p.Description,
			Tools:       p.ToolNames,
		})
	}

	return fileutil.WriteYAML(path, &f, yamlHeader)
}

// Add appends a pack (or replaces one with the same name) and saves.
func Add(pack registry.Pack) error {
	packs, err := Load()
	if err != nil {
		return err
	}

	// Replace existing with same name.
	found := false
	for i, p := range packs {
		if p.Name == pack.Name {
			packs[i] = pack
			found = true
			break
		}
	}
	if !found {
		packs = append(packs, pack)
	}

	return Save(packs)
}

// Delete removes a pack by name and saves.
func Delete(name string) error {
	packs, err := Load()
	if err != nil {
		return err
	}

	filtered := packs[:0]
	for _, p := range packs {
		if p.Name != name {
			filtered = append(filtered, p)
		}
	}

	return Save(filtered)
}

// Exists checks whether a pack with the given name is already stored.
func Exists(name string) (bool, error) {
	packs, err := Load()
	if err != nil {
		return false, err
	}
	for _, p := range packs {
		if p.Name == name {
			return true, nil
		}
	}
	return false, nil
}
