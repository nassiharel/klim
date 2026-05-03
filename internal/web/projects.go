package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"

	"github.com/nassiharel/clim/internal/teamfile"
)

// projectsView powers /projects. Each row pre-loads the .clim.yaml
// summary so the user can see the health of every project at a glance.
type projectsView struct {
	Projects []projectRow
}

type projectRow struct {
	Entry teamfile.ProjectEntry
	// PathURL is the URL-safe version of Entry.Path used in the
	// /projects/<path...> link target. We pre-escape here so the
	// template doesn't need a custom func and so spaces / # / % in
	// real project paths round-trip through url.PathUnescape on the
	// detail handler.
	PathURL string
	// Summary is best-effort: we Parse() the file but don't run a full
	// PATH check (that's per-project on the detail page) so the
	// landing page stays cheap.
	HasFile     bool
	ToolsCount  int
	ParseErrMsg string
}

// pageProjects renders the Projects landing page.
func (s *Server) pageProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := teamfile.LoadProjects()
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	rows := make([]projectRow, 0, len(projects))
	for _, p := range projects {
		row := projectRow{Entry: p, PathURL: url.PathEscape(p.Path)}
		filePath := p.Path + string(os.PathSeparator) + teamfile.FileName
		if _, err := os.Stat(filePath); err == nil {
			row.HasFile = true
			tf, perr := teamfile.Parse(filePath)
			if perr != nil {
				row.ParseErrMsg = perr.Error()
			} else if tf != nil {
				row.ToolsCount = len(tf.Tools) + len(tf.Optional)
			}
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Entry.Name < rows[j].Entry.Name })
	s.renderPage(w, r, "projects.html", pageData{
		Title:     "Projects",
		ActiveTab: "projects",
		Data:      projectsView{Projects: rows},
	})
}

// projectDetailView is the per-project check page.
type projectDetailView struct {
	Entry     teamfile.ProjectEntry
	HasFile   bool
	ParseErr  string
	TeamFile  *teamfile.TeamFile
	Results   []teamfile.CheckResult
	OK        int
	Missing   int
	Outdated  int
	Unknown   int
	Satisfied bool
}

// pageProject renders the per-project page with check results.
func (s *Server) pageProject(w http.ResponseWriter, r *http.Request) {
	rawPath := r.PathValue("path")
	projectPath, err := url.PathUnescape(rawPath)
	if err != nil {
		s.serveError(w, r, fmt.Errorf("invalid project path: %w", err), http.StatusBadRequest)
		return
	}
	projects, err := teamfile.LoadProjects()
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}
	entry, ok := findProject(projects, projectPath)
	if !ok {
		s.serveError(w, r, notFoundError{Name: projectPath}, http.StatusNotFound)
		return
	}

	view := projectDetailView{Entry: entry}
	filePath := entry.Path + string(os.PathSeparator) + teamfile.FileName
	if _, statErr := os.Stat(filePath); statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			view.HasFile = false
		} else {
			s.serveError(w, r, statErr, http.StatusInternalServerError)
			return
		}
	} else {
		view.HasFile = true
		tf, perr := teamfile.Parse(filePath)
		if perr != nil {
			view.ParseErr = perr.Error()
		} else {
			view.TeamFile = tf
			tools, _, err := s.loader.LoadInstalled(r.Context())
			if err != nil {
				s.serveError(w, r, err, http.StatusInternalServerError)
				return
			}
			view.Results = teamfile.Check(tf, tools)
			view.OK, view.Missing, view.Outdated, view.Unknown = teamfile.Summary(view.Results)
			view.Satisfied = teamfile.AllSatisfied(view.Results)
		}
	}
	s.renderPage(w, r, "project.html", pageData{
		Title:     entry.Name,
		ActiveTab: "projects",
		Data:      view,
	})
}

func findProject(projects []teamfile.ProjectEntry, path string) (teamfile.ProjectEntry, bool) {
	for _, p := range projects {
		if p.Path == path {
			return p, true
		}
	}
	return teamfile.ProjectEntry{}, false
}

// projectCheckStatusName maps the int CheckStatus to a string suitable
// for templates / JSON. Centralised so neither lives inside the
// template.
func projectCheckStatusName(s teamfile.CheckStatus) string {
	switch s {
	case teamfile.StatusOK:
		return "ok"
	case teamfile.StatusMissing:
		return "missing"
	case teamfile.StatusOutdated:
		return "outdated"
	case teamfile.StatusUnknown:
		return "unknown"
	}
	return "—"
}
