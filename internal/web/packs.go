package web

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"github.com/nassiharel/clim/internal/custompacks"
	"github.com/nassiharel/clim/internal/registry"
)

// packsView powers the /packs landing page. We split marketplace and
// user-created packs visually because they have different lifecycles
// (marketplace packs are catalog-driven, custom packs live in the
// user's config dir).
type packsView struct {
	Marketplace []packRow
	Custom      []packRow
}

type packRow struct {
	Pack      registry.Pack
	ToolCount int
	Installed int
	Missing   int
	IsCustom  bool
}

// packDetailView is the per-pack page. We resolve every tool name in
// the pack against the catalog so the user can see installed status
// up-front instead of clicking each one.
type packDetailView struct {
	Pack     registry.Pack
	IsCustom bool
	Tools    []packToolRow
	// MissingCount counts pack entries that are in the catalog but
	// not installed locally — i.e. the ones the user could click
	// "Install" on. UnknownCount tracks entries the catalog doesn't
	// know about (typo in the pack file or a tool that's been
	// removed since the pack was authored). They're displayed
	// separately so "5 missing" never quietly includes 2 garbage
	// entries.
	MissingCount int
	UnknownCount int
}

type packToolRow struct {
	Name      string
	Tool      *registry.Tool // nil if the pack references an unknown tool
	Installed bool
}

// pagePacks renders the Packs landing page.
func (s *Server) pagePacks(w http.ResponseWriter, r *http.Request) {
	mp, custom, err := s.loadAllPacks(r.Context())
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	tools, _, err := s.loader.LoadInstalled(r.Context())
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	installed := registry.InstalledSet(tools)
	view := packsView{
		Marketplace: buildPackRows(mp, installed, false),
		Custom:      buildPackRows(custom, installed, true),
	}
	s.renderPage(w, r, "packs.html", pageData{
		Title:     "Packs",
		ActiveTab: "packs",
		Data:      view,
	})
}

// pagePack renders the per-pack detail page.
func (s *Server) pagePack(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	mp, custom, err := s.loadAllPacks(r.Context())
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	pack, isCustom, ok := findPack(name, mp, custom)
	if !ok {
		s.serveError(w, r, notFoundError{Name: name}, http.StatusNotFound)
		return
	}
	tools, _, err := s.loader.LoadInstalled(r.Context())
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	byName := registry.ToolMap(tools)
	rows := make([]packToolRow, 0, len(pack.ToolNames))
	missing, unknown := 0, 0
	for _, n := range pack.ToolNames {
		row := packToolRow{Name: n}
		if t, ok := byName[n]; ok {
			row.Tool = t
			row.Installed = t.IsInstalled()
			if !row.Installed {
				missing++
			}
		} else {
			// Catalog doesn't know this tool — count it under "unknown"
			// so the header doesn't conflate "missing but installable"
			// with "garbage entry".
			unknown++
		}
		rows = append(rows, row)
	}
	s.renderPage(w, r, "pack.html", pageData{
		Title:     pack.Name,
		ActiveTab: "packs",
		Data: packDetailView{
			Pack:         pack,
			IsCustom:     isCustom,
			Tools:        rows,
			MissingCount: missing,
			UnknownCount: unknown,
		},
	})
}

// loadAllPacks returns marketplace and custom packs separately. We
// don't merge them — the UI cares about which list a pack came from
// because custom packs can be deleted from disk.
func (s *Server) loadAllPacks(ctx context.Context) (marketplace, custom []registry.Pack, err error) {
	mp, err := s.loader.LoadPacks(ctx)
	if err != nil {
		return nil, nil, err
	}
	customAll, cerr := custompacks.Load()
	// Missing custom packs file is fine — it's optional; surface other errors.
	if cerr != nil {
		return mp, nil, cerr
	}
	// Custom packs may include built-in marketplace packs the user
	// duplicated; we still show them under "custom" because that's
	// where they actually live.
	sort.Slice(mp, func(i, j int) bool { return mp[i].Name < mp[j].Name })
	sort.Slice(customAll, func(i, j int) bool { return customAll[i].Name < customAll[j].Name })
	return mp, customAll, nil
}

func buildPackRows(packs []registry.Pack, installed map[string]bool, isCustom bool) []packRow {
	out := make([]packRow, 0, len(packs))
	for _, p := range packs {
		row := packRow{Pack: p, ToolCount: len(p.ToolNames), IsCustom: isCustom}
		for _, n := range p.ToolNames {
			if installed[n] {
				row.Installed++
			} else {
				row.Missing++
			}
		}
		out = append(out, row)
	}
	return out
}

func findPack(name string, lists ...[]registry.Pack) (registry.Pack, bool, bool) {
	for i, list := range lists {
		for _, p := range list {
			if strings.EqualFold(p.Name, name) {
				return p, i == 1, true // second list = custom
			}
		}
	}
	return registry.Pack{}, false, false
}
