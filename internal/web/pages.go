package web

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"

	"github.com/nassiharel/clim/internal/githubfmt"
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
	s.renderPage(w, r, "tool.html", pageData{
		Title:     tool.Name,
		ActiveTab: "installed",
		Data: toolView{
			Tool:      tool,
			Installed: tool.IsInstalled(),
			Latest:    tool.Latest,
			GitHub:    formatGitHub(tool),
		},
	})
}

// pageDashboard renders aggregate stats.
func (s *Server) pageDashboard(w http.ResponseWriter, r *http.Request) {
	tools, _, err := s.loader.LoadInstalled(r.Context())
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	s.renderPage(w, r, "dashboard.html", pageData{
		Title:     "Dashboard",
		ActiveTab: "dashboard",
		Data:      buildDashboard(tools),
	})
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

// pageStub renders a "Coming soon" placeholder for tabs that aren't
// fully implemented yet. Returns a closure so the handler can carry
// the tab name without parsing the path.
func (s *Server) pageStub(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.renderPage(w, r, "stub.html", pageData{
			Title:     name,
			ActiveTab: strings.ToLower(name),
			Data:      stubView{Name: name},
		})
	}
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

type toolView struct {
	Tool      registry.Tool
	Installed bool
	Latest    string
	GitHub    githubView
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

type stubView struct{ Name string }

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
	}
}
