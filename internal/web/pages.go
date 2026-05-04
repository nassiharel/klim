package web

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"

	"github.com/nassiharel/clim/internal/favorites"
	"github.com/nassiharel/clim/internal/githubfmt"
	"github.com/nassiharel/clim/internal/recommend"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/service"
	"github.com/nassiharel/clim/internal/trail"
)

// pageData is the universal context every layout-using template
// receives. Per-page payload lives in Data.
type pageData struct {
	Title       string
	ActiveTab   string
	URL         string
	Data        any
	Error       string
	BuildBanner string
}

// renderPage executes the layout for the named page. It buffers the
// rendering so a template error doesn't leave a partial response on
// the wire.
func (s *Server) renderPage(w http.ResponseWriter, r *http.Request, name string, data pageData) {
	data.URL = r.URL.String()
	tpl, ok := s.tpls[name]
	if !ok {
		s.serveError(w, r, fmt.Errorf("template %q not found", name), http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, "layout", data); err != nil {
		s.serveError(w, r, fmt.Errorf("rendering %s: %w", name, err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(buf.Bytes())
}

// serveError writes a minimal error page. We keep this dependency-free
// (no template lookup) so the error path can't itself fail.
func (s *Server) serveError(w http.ResponseWriter, _ *http.Request, err error, status int) {
	s.opts.Logger.Error("web error", "err", err.Error(), "status", status)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><meta charset="utf-8"><title>clim — error</title>`+
		`<body style="font-family:system-ui;padding:2rem"><h1>Error</h1><p>%s</p>`+
		`<p><a href="/">Back to home</a></p></body>`, template.HTMLEscapeString(err.Error()))
}

// pageInstalled renders the "Installed" tab.
func (s *Server) pageInstalled(w http.ResponseWriter, r *http.Request) {
	tools, info, err := s.loader.LoadInstalled(r.Context())
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	cat := strings.TrimSpace(r.URL.Query().Get("category"))
	src := strings.TrimSpace(r.URL.Query().Get("source"))
	filtered := filterTools(tools, q, cat, src)
	s.renderPage(w, r, "installed.html", pageData{
		Title:     "Installed",
		ActiveTab: "installed",
		Data: installedView{
			Tools:      filtered,
			Total:      countInstalled(tools),
			Filtered:   len(filtered),
			Categories: distinctCategories(tools),
			Sources:    distinctSources(tools),
			Query:      q,
			Category:   cat,
			Source:     src,
			Catalog:    info,
		},
	})
}

// pageTool renders the per-tool detail page (CLI-info equivalent).
func (s *Server) pageTool(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tool, err := s.loader.LoadTool(r.Context(), name)
	if err != nil {
		status := http.StatusInternalServerError
		if isNotFound(err) {
			status = http.StatusNotFound
		}
		s.serveError(w, r, err, status)
		return
	}

	// Pull catalog + favorites once so the related-tools section and
	// the favorite-button can render without re-fetching.
	tools, _, _ := s.loader.LoadInstalled(r.Context())
	favs, _ := s.loader.Favorites()
	if favs == nil {
		favs = map[string]bool{}
	}

	view := toolView{
		Tool:        tool,
		Installed:   tool.IsInstalled(),
		Latest:      tool.Latest,
		IsFavorite:  favs[tool.Name],
		PMRows:      buildPMRows(tool),
		Tags:        mergedTagsAndTopics(tool),
		BinaryNames: tool.BinaryNames,
		Related:     buildRelatedTools(tool, tools, favs),
		GitHub:      formatGitHub(tool),
	}
	s.renderPage(w, r, "tool.html", pageData{
		Title:     tool.Name,
		ActiveTab: "installed",
		Data:      view,
	})
}

// pageDashboard renders aggregate stats. Mirrors the TUI's dashboard
// sections so users see the same overview either way.
func (s *Server) pageDashboard(w http.ResponseWriter, r *http.Request) {
	view, err := s.collectDashboard(r.Context())
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	s.renderPage(w, r, "dashboard.html", pageData{
		Title:     "Dashboard",
		ActiveTab: "dashboard",
		Data:      view,
	})
}

// collectDashboard pulls together every input the dashboard renders:
// the resolved tool list, favorites, marketplace packs, recommendations,
// and the saved-backup count. Each piece is best-effort — a failure in
// one (e.g. catalog packs not loaded yet) shouldn't blank the whole page.
func (s *Server) collectDashboard(ctx context.Context) (dashboardView, error) {
	tools, _, err := s.loader.LoadInstalled(ctx)
	if err != nil {
		return dashboardView{}, err
	}
	favs, _ := s.loader.Favorites()
	if favs == nil {
		favs = map[string]bool{}
	}
	packs, _ := s.loader.LoadPacks(ctx)
	recs := recommend.Compute(tools)
	backupCount := 0
	if list, lerr := loadSavedBackups(); lerr == nil {
		backupCount = len(list)
	}
	return buildDashboard(tools, favs, packs, recs, backupCount), nil
}

// pageUpdates renders the list of installed tools that have a newer
// version available. This is the second-most-common landing page so
// the user can act on the actual deltas rather than scanning the
// whole installed list.
func (s *Server) pageUpdates(w http.ResponseWriter, r *http.Request) {
	tools, _, err := s.loader.LoadInstalled(r.Context())
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	favs, _ := s.loader.Favorites()
	if favs == nil {
		favs = map[string]bool{}
	}
	updates := make([]updateRow, 0, len(tools))
	for _, t := range tools {
		pi := t.PrimaryInstance()
		if pi == nil || t.Latest == "" {
			continue
		}
		if registry.CompareVersions(pi.Version, t.Latest) >= 0 {
			continue
		}
		updates = append(updates, updateRow{
			Tool:     t,
			Current:  pi.Version,
			Latest:   t.Latest,
			Source:   string(pi.Source),
			Favorite: favs[t.Name],
		})
	}
	s.renderPage(w, r, "updates.html", pageData{
		Title:     "Updates",
		ActiveTab: "updates",
		Data: updatesView{
			Rows:  updates,
			Total: len(updates),
		},
	})
}

// pageDiscover renders the full marketplace catalog with optional
// category, tag, and free-text filters. Unlike Installed, this page
// includes tools that aren't installed locally — it's the entry point
// for finding new tools.
func (s *Server) pageDiscover(w http.ResponseWriter, r *http.Request) {
	tools, _, err := s.loader.LoadInstalled(r.Context())
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	cat := strings.TrimSpace(r.URL.Query().Get("category"))
	tag := strings.TrimSpace(r.URL.Query().Get("tag"))
	filtered := filterDiscover(tools, q, cat, tag)
	favs, _ := s.loader.Favorites()
	if favs == nil {
		favs = map[string]bool{}
	}
	s.renderPage(w, r, "discover.html", pageData{
		Title:     "Discover",
		ActiveTab: "discover",
		Data: discoverView{
			Tools:      filtered,
			Total:      len(tools),
			Filtered:   len(filtered),
			Categories: distinctCatalogCategories(tools),
			Tags:       distinctCatalogTags(tools),
			Query:      q,
			Category:   cat,
			Tag:        tag,
			Favorites:  favs,
		},
	})
}

// pageFavorites lists the user's favorited tools and exposes a toggle
// action on each row.
func (s *Server) pageFavorites(w http.ResponseWriter, r *http.Request) {
	favs, err := s.loader.Favorites()
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	tools, _, err := s.loader.LoadInstalled(r.Context())
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	rows := make([]registry.Tool, 0, len(favs))
	for _, t := range tools {
		if favs[t.Name] {
			rows = append(rows, t)
		}
	}
	s.renderPage(w, r, "favorites.html", pageData{
		Title:     "Favorites",
		ActiveTab: "favorites",
		Data: favoritesView{
			Tools: rows,
			Total: len(rows),
		},
	})
}

// pageFavoritesToggle handles the form-submission flow from the
// favorite/unfavorite buttons. It toggles the favorite and redirects
// back to the page the user came from (Referer) so the toggle UX is
// "click and the row updates".
func (s *Server) pageFavoritesToggle(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		s.serveError(w, r, fmt.Errorf("missing tool name"), http.StatusBadRequest)
		return
	}
	if _, err := s.loader.ToggleFavorite(name); err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	dest := r.Referer()
	if dest == "" {
		dest = "/favorites"
	}
	http.Redirect(w, r, dest, http.StatusSeeOther)
}

// pageTrail renders the trail entry list.
func (s *Server) pageTrail(w http.ResponseWriter, r *http.Request) {
	entries, err := trail.Log(trail.LogOptions{})
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	s.renderPage(w, r, "trail.html", pageData{
		Title:     "Trail",
		ActiveTab: "trail",
		Data:      trailView{Entries: entries},
	})
}

// pageTrailShow renders a single snapshot.
func (s *Server) pageTrailShow(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	if ref == "" {
		s.serveError(w, r, fmt.Errorf("missing ref"), http.StatusBadRequest)
		return
	}
	snap, entry, err := trail.Show(ref)
	if err != nil {
		s.serveError(w, r, err, http.StatusNotFound)
		return
	}
	s.renderPage(w, r, "snapshot.html", pageData{
		Title:     "Snapshot " + entry.Object.Short(),
		ActiveTab: "trail",
		Data: snapshotView{
			Entry:    *entry,
			Snapshot: snap,
		},
	})
}

// --- view models ---

type installedView struct {
	Tools      []registry.Tool
	Total      int
	Filtered   int
	Categories []string
	Sources    []string
	Query      string
	Category   string
	Source     string
	Catalog    catalogSummary
}

type updateRow struct {
	Tool     registry.Tool
	Current  string
	Latest   string
	Source   string
	Favorite bool
}

type updatesView struct {
	Rows  []updateRow
	Total int
}

type discoverView struct {
	Tools      []registry.Tool
	Total      int
	Filtered   int
	Categories []string
	Tags       []string
	Query      string
	Category   string
	Tag        string
	Favorites  map[string]bool
}

type favoritesView struct {
	Tools []registry.Tool
	Total int
}

type toolView struct {
	Tool        registry.Tool
	Installed   bool
	Latest      string
	IsFavorite  bool
	PMRows      []pmRow
	Tags        []string // tags + GitHub topics, deduped
	BinaryNames []string
	Related     []relatedTool
	GitHub      githubView
}

// pmRow is one row in the per-tool Package Managers table. We list
// every source clim has a package id for, plus a flag for whether the
// PM binary is on PATH so the user can tell which "Install" button
// would actually work.
type pmRow struct {
	Source     string
	PackageID  string
	Available  bool
	InstallCmd string
}

// relatedTool is one entry in the per-tool "You might also like"
// list. We compute it from the global Recommend logic but limited to
// the tools that share at least one tag/topic with this one.
type relatedTool struct {
	Name        string
	DisplayName string
	Category    string
	Description string
	Stars       int
	MatchPct    int
	Reason      string
	IsFavorite  bool
}

type githubView struct {
	Slug    string
	URL     string
	Stars   string
	Forks   int
	License string
	Topics  []string
	Push    string
}

type trailView struct {
	Entries []trail.Entry
}

type snapshotView struct {
	Entry    trail.Entry
	Snapshot *trail.Snapshot
}

type catalogSummary struct {
	Source string
	Count  int
}

// --- data loaders (centralised so HTML and JSON share one path) ---

// loader abstracts where tool data comes from. The production
// implementation wraps service.ToolService; tests inject a fixture so
// they don't depend on the host machine's PATH or package managers.
type loader interface {
	LoadInstalled(ctx context.Context) ([]registry.Tool, catalogSummary, error)
	LoadTool(ctx context.Context, name string) (registry.Tool, error)
	Favorites() (map[string]bool, error)
	ToggleFavorite(name string) (bool, error)
	LoadPacks(ctx context.Context) ([]registry.Pack, error)
}

type serviceLoader struct{ svc *service.ToolService }

func newServiceLoader(svc *service.ToolService) loader {
	return &serviceLoader{svc: svc}
}

func (l *serviceLoader) LoadInstalled(ctx context.Context) ([]registry.Tool, catalogSummary, error) {
	tools, info, scan, err := l.svc.LoadAndResolveCached(ctx, false)
	if err != nil {
		return nil, catalogSummary{}, err
	}
	registry.SortByName(tools)
	src := "fresh"
	if scan != nil && string(scan.Source) != "" {
		src = string(scan.Source)
	}
	count := 0
	if info != nil {
		count = info.Tools
	}
	return tools, catalogSummary{Source: src, Count: count}, nil
}

func (l *serviceLoader) LoadTool(ctx context.Context, name string) (registry.Tool, error) {
	tools, _, err := l.svc.ScanOnly(ctx)
	if err != nil {
		return registry.Tool{}, err
	}
	for i := range tools {
		if strings.EqualFold(tools[i].Name, name) {
			refreshed := l.svc.RefreshTool(ctx, tools[i])
			return refreshed, nil
		}
	}
	return registry.Tool{}, notFoundError{Name: name}
}

func (l *serviceLoader) Favorites() (map[string]bool, error) {
	return favorites.Set()
}

func (l *serviceLoader) ToggleFavorite(name string) (bool, error) {
	return favorites.Toggle(name)
}

func (l *serviceLoader) LoadPacks(ctx context.Context) ([]registry.Pack, error) {
	return l.svc.LoadPacks(ctx)
}

type notFoundError struct{ Name string }

func (e notFoundError) Error() string { return fmt.Sprintf("tool %q not found in catalog", e.Name) }

func isNotFound(err error) bool {
	_, ok := err.(notFoundError) //nolint:errorlint // sentinel type, not wrapped
	return ok
}

func countInstalled(tools []registry.Tool) int {
	n := 0
	for _, t := range tools {
		if t.IsInstalled() {
			n++
		}
	}
	return n
}

// --- helpers ---

func filterTools(tools []registry.Tool, q, cat, src string) []registry.Tool {
	if q == "" && cat == "" && src == "" {
		out := make([]registry.Tool, 0, len(tools))
		for _, t := range tools {
			if t.IsInstalled() {
				out = append(out, t)
			}
		}
		return out
	}
	q = strings.ToLower(q)
	cat = strings.ToLower(cat)
	src = strings.ToLower(src)
	out := make([]registry.Tool, 0, len(tools))
	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(t.Name), q) &&
			!strings.Contains(strings.ToLower(t.DisplayName), q) {
			continue
		}
		if cat != "" && !strings.EqualFold(t.Category, cat) {
			continue
		}
		if src != "" {
			pi := t.PrimaryInstance()
			if pi == nil || !strings.EqualFold(string(pi.Source), src) {
				continue
			}
		}
		out = append(out, t)
	}
	return out
}

func distinctCategories(tools []registry.Tool) []string {
	seen := map[string]struct{}{}
	for _, t := range tools {
		if !t.IsInstalled() || t.Category == "" {
			continue
		}
		seen[t.Category] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// distinctCatalogCategories is like distinctCategories but for the
// Discover page, which shows non-installed tools too.
func distinctCatalogCategories(tools []registry.Tool) []string {
	seen := map[string]struct{}{}
	for _, t := range tools {
		if t.Category == "" {
			continue
		}
		seen[t.Category] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

func distinctCatalogTags(tools []registry.Tool) []string {
	seen := map[string]struct{}{}
	for _, t := range tools {
		for _, tag := range t.Tags {
			if tag == "" {
				continue
			}
			seen[tag] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// filterDiscover filters the catalog (NOT just installed tools) by
// search term, category, and tag.
func filterDiscover(tools []registry.Tool, q, cat, tag string) []registry.Tool {
	q = strings.ToLower(strings.TrimSpace(q))
	cat = strings.TrimSpace(cat)
	tag = strings.TrimSpace(tag)
	out := make([]registry.Tool, 0, len(tools))
	for _, t := range tools {
		if cat != "" && !strings.EqualFold(t.Category, cat) {
			continue
		}
		if tag != "" && !hasTag(t, tag) {
			continue
		}
		if q != "" && !matchesQuery(t, q) {
			continue
		}
		out = append(out, t)
	}
	return out
}

func hasTag(t registry.Tool, tag string) bool {
	for _, x := range t.Tags {
		if strings.EqualFold(x, tag) {
			return true
		}
	}
	return false
}

func matchesQuery(t registry.Tool, q string) bool {
	if strings.Contains(strings.ToLower(t.Name), q) {
		return true
	}
	if strings.Contains(strings.ToLower(t.DisplayName), q) {
		return true
	}
	if t.GitHubInfo != nil && strings.Contains(strings.ToLower(t.GitHubInfo.Description), q) {
		return true
	}
	return false
}

func distinctSources(tools []registry.Tool) []string {
	seen := map[string]struct{}{}
	for _, t := range tools {
		pi := t.PrimaryInstance()
		if pi == nil {
			continue
		}
		seen[string(pi.Source)] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

func formatGitHub(t registry.Tool) githubView {
	if t.GitHubInfo == nil {
		return githubView{Slug: t.GitHubSlug}
	}
	gi := t.GitHubInfo
	view := githubView{
		Slug:    t.GitHubSlug,
		URL:     githubfmt.RepoURL(t.GitHubSlug),
		Stars:   githubfmt.FormatStars(gi.Stars),
		Forks:   gi.Forks,
		License: gi.License,
		Topics:  gi.Topics,
	}
	if gi.PushedAt != "" {
		view.Push = githubfmt.FormatDate(gi.PushedAt)
	}
	return view
}

// templateFuncs are exposed to all templates.
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"primarySource": func(t registry.Tool) string {
			pi := t.PrimaryInstance()
			if pi == nil {
				return "—"
			}
			return string(pi.Source)
		},
		"primaryVersion": func(t registry.Tool) string {
			pi := t.PrimaryInstance()
			if pi == nil || pi.Version == "" {
				return "—"
			}
			return pi.Version
		},
		"updateAvailable": func(t registry.Tool) bool {
			pi := t.PrimaryInstance()
			if pi == nil || t.Latest == "" {
				return false
			}
			return registry.CompareVersions(pi.Version, t.Latest) < 0
		},
		"dash": func(s string) string {
			if s == "" {
				return "—"
			}
			return s
		},
		"join": strings.Join,
		// barPct turns a count + max into a percentage capped at 100.
		// Used by /dashboard's bar-chart rows. Pure helper so it can
		// live in the package without any HTML logic.
		"barPct": func(count, max int) int {
			if max <= 0 {
				return 0
			}
			pct := count * 100 / max
			if pct > 100 {
				return 100
			}
			if count > 0 && pct == 0 {
				return 1
			}
			return pct
		},
		// first returns the first element of a string slice (or empty
		// string when the slice is empty / nil). Useful for reading
		// url.Values entries from the config form, which are always
		// []string but always single-valued in our usage.
		"first": func(xs []string) string {
			if len(xs) == 0 {
				return ""
			}
			return xs[0]
		},
	}
}
