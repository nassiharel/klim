package web

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/trail"
)

// writeJSON marshals payload as indented JSON. We always write a JSON
// content-type, even on error responses, so clients written against the
// API don't have to special-case error shapes.
func (s *Server) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		s.opts.Logger.Error("web: encoding json", "err", err)
	}
}

func (s *Server) jsonError(w http.ResponseWriter, status int, msg string) {
	s.writeJSON(w, status, map[string]string{"error": msg})
}

// apiTools returns the resolved tool list. Same shape `clim list
// --output json` emits.
func (s *Server) apiTools(w http.ResponseWriter, r *http.Request) {
	tools, info, err := s.loader.LoadInstalled(r.Context())
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"tools":   toAPIToolList(tools),
		"count":   len(tools),
		"catalog": info,
	})
}

// apiTool returns one tool. Same shape `clim info --output json` emits.
func (s *Server) apiTool(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	t, err := s.loader.LoadTool(r.Context(), name)
	if err != nil {
		status := http.StatusInternalServerError
		if isNotFound(err) {
			status = http.StatusNotFound
		}
		s.jsonError(w, status, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, toAPITool(t))
}

// apiDashboard returns aggregate stats.
func (s *Server) apiDashboard(w http.ResponseWriter, r *http.Request) {
	tools, _, err := s.loader.LoadInstalled(r.Context())
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, buildDashboard(tools))
}

// apiTrail returns trail entries.
func (s *Server) apiTrail(w http.ResponseWriter, _ *http.Request) {
	entries, err := trail.Log(trail.LogOptions{})
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if entries == nil {
		entries = []trail.Entry{}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"entries": entries,
		"count":   len(entries),
	})
}

// apiTrailShow returns the snapshot at ref.
func (s *Server) apiTrailShow(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("ref")
	if ref == "" {
		s.jsonError(w, http.StatusBadRequest, "missing ref")
		return
	}
	snap, entry, err := trail.Show(ref)
	if err != nil {
		s.jsonError(w, http.StatusNotFound, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"entry":    entry,
		"snapshot": snap,
	})
}

// --- API DTOs ---
//
// The DTOs intentionally mirror the existing CLI JSON output rather
// than the registry struct directly. Two reasons: (1) registry.Tool
// has fields that aren't useful over the wire (e.g. internal cache
// state), and (2) the CLI JSON shape is what existing scripts already
// consume, so the browser API keeps that contract stable.

type apiTool struct {
	Name            string         `json:"name"`
	DisplayName     string         `json:"display_name,omitempty"`
	Category        string         `json:"category,omitempty"`
	Tags            []string       `json:"tags"`
	Installed       bool           `json:"installed"`
	UpdateAvailable bool           `json:"update_available"`
	Latest          string         `json:"latest,omitempty"`
	LatestFrom      string         `json:"latest_from,omitempty"`
	Instances       []apiInstance  `json:"instances"`
	GitHubSlug      string         `json:"github_slug,omitempty"`
	GitHubInfo      *apiGitHubInfo `json:"github,omitempty"`
}

type apiInstance struct {
	Path    string `json:"path"`
	Version string `json:"version,omitempty"`
	Source  string `json:"source"`
}

type apiGitHubInfo struct {
	Slug        string   `json:"slug,omitempty"`
	URL         string   `json:"url,omitempty"`
	Description string   `json:"description,omitempty"`
	Stars       int      `json:"stars"`
	Forks       int      `json:"forks"`
	License     string   `json:"license,omitempty"`
	Topics      []string `json:"topics"`
	PushedAt    string   `json:"pushed_at,omitempty"`
}

func toAPITool(t registry.Tool) apiTool {
	tags := append([]string(nil), t.Tags...)
	if tags == nil {
		tags = []string{}
	}
	out := apiTool{
		Name:        t.Name,
		DisplayName: t.DisplayName,
		Category:    t.Category,
		Tags:        tags,
		Installed:   t.IsInstalled(),
		Latest:      t.Latest,
		LatestFrom:  t.LatestFrom,
		Instances:   toAPIInstances(t.Instances),
		GitHubSlug:  t.GitHubSlug,
	}
	if pi := t.PrimaryInstance(); pi != nil && t.Latest != "" {
		out.UpdateAvailable = registry.CompareVersions(pi.Version, t.Latest) < 0
	}
	if t.GitHubInfo != nil {
		gi := apiGitHubInfo{
			Slug:        t.GitHubSlug,
			Description: t.GitHubInfo.Description,
			Stars:       t.GitHubInfo.Stars,
			Forks:       t.GitHubInfo.Forks,
			License:     t.GitHubInfo.License,
			Topics:      append([]string(nil), t.GitHubInfo.Topics...),
		}
		if gi.Topics == nil {
			gi.Topics = []string{}
		}
		if t.GitHubInfo.PushedAt != "" {
			gi.PushedAt = t.GitHubInfo.PushedAt
		}
		out.GitHubInfo = &gi
	}
	return out
}

func toAPIToolList(tools []registry.Tool) []apiTool {
	out := make([]apiTool, 0, len(tools))
	for _, t := range tools {
		out = append(out, toAPITool(t))
	}
	return out
}

func toAPIInstances(insts []registry.Instance) []apiInstance {
	out := make([]apiInstance, 0, len(insts))
	for _, i := range insts {
		out = append(out, apiInstance{
			Path:    i.Path,
			Version: i.Version,
			Source:  string(i.Source),
		})
	}
	return out
}

// --- dashboard payload ---

type dashboardView struct {
	TotalTools      int            `json:"total_tools"`
	InstalledTools  int            `json:"installed_tools"`
	UpdatesAvail    int            `json:"updates_available"`
	BySource        map[string]int `json:"by_source"`
	ByCategory      map[string]int `json:"by_category"`
	TopCategories   []labelCount   `json:"top_categories"`
	UpdatableSample []string       `json:"updatable_sample,omitempty"`
}

type labelCount struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

func buildDashboard(tools []registry.Tool) dashboardView {
	v := dashboardView{
		TotalTools: len(tools),
		BySource:   map[string]int{},
		ByCategory: map[string]int{},
	}
	for _, t := range tools {
		if !t.IsInstalled() {
			continue
		}
		v.InstalledTools++
		pi := t.PrimaryInstance()
		if pi != nil {
			v.BySource[string(pi.Source)]++
			if t.Latest != "" && registry.CompareVersions(pi.Version, t.Latest) < 0 {
				v.UpdatesAvail++
				if len(v.UpdatableSample) < 5 {
					v.UpdatableSample = append(v.UpdatableSample, t.Name)
				}
			}
		}
		if t.Category != "" {
			v.ByCategory[t.Category]++
		}
	}
	v.TopCategories = topN(v.ByCategory, 5)
	return v
}

func topN(m map[string]int, n int) []labelCount {
	out := make([]labelCount, 0, len(m))
	for k, v := range m {
		out = append(out, labelCount{Label: k, Count: v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Label < out[j].Label
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}
