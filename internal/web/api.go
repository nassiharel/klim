package web

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/nassiharel/klim/internal/recommend"
	"github.com/nassiharel/klim/internal/registry"
	"github.com/nassiharel/klim/internal/trail"
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

// apiTools returns the resolved tool list. Same shape `klim tool list
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

// apiTool returns one tool. Same shape `klim tool info --output json` emits.
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
	view, err := s.collectDashboard(r.Context())
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, view)
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

// apiFavoritesList returns the current favorite names sorted.
func (s *Server) apiFavoritesList(w http.ResponseWriter, _ *http.Request) {
	favs, err := s.loader.Favorites()
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	names := make([]string, 0, len(favs))
	for n := range favs {
		names = append(names, n)
	}
	sort.Strings(names)
	s.writeJSON(w, http.StatusOK, map[string]any{
		"favorites": names,
		"count":     len(names),
	})
}

// apiFavoritesToggle flips the favorite state of a tool. Returns the
// resulting state in the response so clients can re-render without
// re-fetching.
func (s *Server) apiFavoritesToggle(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		s.jsonError(w, http.StatusBadRequest, "missing tool name")
		return
	}
	added, err := s.loader.ToggleFavorite(name)
	if err != nil {
		s.jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.writeJSON(w, http.StatusOK, map[string]any{
		"name":     name,
		"favorite": added,
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

// dashboardView is the data the Dashboard page renders. It mirrors
// the TUI's dashboard sections so users get the same overview either
// way they look at it.
type dashboardView struct {
	// Stat cards.
	TotalTools     int `json:"total_tools"`
	InstalledTools int `json:"installed_tools"`
	UpdatesAvail   int `json:"updates_available"`
	NotInstalled   int `json:"not_installed"`
	FavoritesCount int `json:"favorites_count"`
	UpToDate       int `json:"up_to_date"`

	// Coverage percentages.
	PctInstalled int `json:"pct_installed"`
	PctUpToDate  int `json:"pct_up_to_date"`

	// Attention alerts.
	UpdatableSample []string `json:"updatable_sample,omitempty"`
	FavoriteUpdates int      `json:"favorite_updates"`
	NewMarketCount  int      `json:"new_marketplace_count"`

	// Breakdowns.
	BySource      []labelCount `json:"by_source"`
	ByCategory    []labelCount `json:"by_category"`
	TopCategories []labelCount `json:"top_categories"`
	TopTags       []labelCount `json:"top_tags"`

	// GitHub highlights — top starred installed tools.
	StarredHighlights []starredHighlight `json:"starred_highlights"`

	// Recommendation preview (top 3 from /foryou).
	TopPicks      []dashboardPick `json:"top_picks"`
	MoreRecsCount int             `json:"more_recs_count"`

	// Pack completion.
	PacksMarketplaceTotal   int `json:"packs_marketplace_total"`
	PacksMarketplaceFull    int `json:"packs_marketplace_full"`
	PacksMarketplacePartial int `json:"packs_marketplace_partial"`

	// Backup count (best-effort; zero on read errors).
	BackupCount int `json:"backup_count"`

	// Convenience for the template — total raw rows (used when we
	// build per-section "max" values for gauge widths).
	MaxSource   int `json:"-"`
	MaxCategory int `json:"-"`
	MaxTag      int `json:"-"`
}

type starredHighlight struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Stars       int    `json:"stars"`
	PushedAt    string `json:"pushed_at,omitempty"`
}

type dashboardPick struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name,omitempty"`
	Category    string `json:"category,omitempty"`
	MatchPct    int    `json:"match_pct"`
	Reason      string `json:"reason,omitempty"`
	Stars       int    `json:"stars"`
}

type labelCount struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

func buildDashboard(tools []registry.Tool, favs map[string]bool, packs []registry.Pack, recs []recommend.Recommendation, backupCount int) dashboardView {
	v := dashboardView{
		TotalTools:     len(tools),
		FavoritesCount: len(favs),
		BackupCount:    backupCount,
	}
	bySource := map[string]int{}
	byCategory := map[string]int{}
	tagCounts := map[string]int{}
	starred := []starredHighlight{}
	for _, t := range tools {
		if !t.IsInstalled() {
			v.NotInstalled++
			continue
		}
		v.InstalledTools++
		pi := t.PrimaryInstance()
		if pi != nil {
			bySource[string(pi.Source)]++
			if t.Latest != "" && registry.CompareVersions(pi.Version, t.Latest) < 0 {
				v.UpdatesAvail++
				if len(v.UpdatableSample) < 5 {
					v.UpdatableSample = append(v.UpdatableSample, t.Name)
				}
				if favs[t.Name] {
					v.FavoriteUpdates++
				}
			}
		}
		if t.Category != "" {
			byCategory[t.Category]++
		}
		for _, tag := range t.Tags {
			if tag != "" {
				tagCounts[tag]++
			}
		}
		if t.MarketplaceStatus == registry.StatusNew {
			v.NewMarketCount++
		}
		if t.GitHubInfo != nil && t.GitHubInfo.Stars > 0 {
			starred = append(starred, starredHighlight{
				Name:        t.Name,
				DisplayName: t.DisplayName,
				Stars:       t.GitHubInfo.Stars,
				PushedAt:    t.GitHubInfo.PushedAt,
			})
		}
	}
	v.UpToDate = v.InstalledTools - v.UpdatesAvail
	if v.TotalTools > 0 {
		v.PctInstalled = v.InstalledTools * 100 / v.TotalTools
	}
	if v.InstalledTools > 0 {
		v.PctUpToDate = v.UpToDate * 100 / v.InstalledTools
	}

	v.BySource = sortedLabelCounts(bySource)
	v.ByCategory = sortedLabelCounts(byCategory)
	v.TopCategories = topNLabelCounts(byCategory, 5)
	v.TopTags = topNLabelCounts(tagCounts, 12)
	v.MaxSource = maxLabelCount(v.BySource)
	v.MaxCategory = maxLabelCount(v.ByCategory)
	v.MaxTag = maxLabelCount(v.TopTags)

	// GitHub highlights — top 5 by stars.
	sort.Slice(starred, func(i, j int) bool { return starred[i].Stars > starred[j].Stars })
	if len(starred) > 5 {
		starred = starred[:5]
	}
	v.StarredHighlights = starred

	// Top picks for you (preview of /foryou).
	if len(recs) > 0 {
		shown := len(recs)
		if shown > 3 {
			shown = 3
		}
		for i := 0; i < shown; i++ {
			r := recs[i]
			if r.ToolIdx >= 0 && r.ToolIdx < len(tools) {
				t := tools[r.ToolIdx]
				v.TopPicks = append(v.TopPicks, dashboardPick{
					Name:        t.Name,
					DisplayName: t.DisplayName,
					Category:    t.Category,
					MatchPct:    r.MatchPct,
					Reason:      r.Reason,
					Stars:       r.Stars,
				})
			}
		}
		if len(recs) > 3 {
			v.MoreRecsCount = len(recs) - 3
		}
	}

	// Pack completion.
	installed := registry.InstalledSet(tools)
	v.PacksMarketplaceTotal = len(packs)
	for _, p := range packs {
		have := 0
		for _, n := range p.ToolNames {
			if installed[n] {
				have++
			}
		}
		switch {
		case len(p.ToolNames) > 0 && have == len(p.ToolNames):
			v.PacksMarketplaceFull++
		case have > 0:
			v.PacksMarketplacePartial++
		}
	}
	return v
}

func sortedLabelCounts(m map[string]int) []labelCount {
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
	return out
}

func topNLabelCounts(m map[string]int, n int) []labelCount {
	out := sortedLabelCounts(m)
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func maxLabelCount(rows []labelCount) int {
	m := 0
	for _, r := range rows {
		if r.Count > m {
			m = r.Count
		}
	}
	return m
}
