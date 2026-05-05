package web

import (
	"net/http"
	"strings"

	"github.com/nassiharel/clim/internal/audit"
	"github.com/nassiharel/clim/internal/registry"
	"github.com/nassiharel/clim/internal/security"
	"github.com/nassiharel/clim/internal/vuln"
)

// securityView is the data shape for the /security page. We
// deliberately don't surface compliance here today — wiring up the
// web compliance loader is a separate piece of work, and showing a
// hard-coded "0 violations" was actively misleading.
type securityView struct {
	AuditWarnings   int
	AuditInfos      int
	AuditFindings   []audit.Finding
	VulnRiskCount   int
	VulnMatches     []vuln.Match
	VulnCacheLoaded bool
	VulnSource      string
	SkippedTools    []vuln.Skip
}

// pageSecurity renders the umbrella Security page. It aggregates:
//   - audit findings (in-memory; cheap)
//   - cached vulnerability scan results (no network — tells the user
//     to run `clim security vuln` if no cache exists)
//
// We deliberately don't fetch fresh vuln data here — the web view
// shouldn't block its render on a 30s OSV.dev round-trip. The user
// triggers a refresh from the CLI; the page picks it up next reload.
func (s *Server) pageSecurity(w http.ResponseWriter, r *http.Request) {
	tools, _, err := s.loader.LoadInstalled(r.Context())
	if err != nil {
		s.serveError(w, r, err, http.StatusInternalServerError)
		return
	}

	findings, _ := audit.Analyze(tools)
	auditW, auditI := audit.CountBySeverity(findings)

	view := securityView{
		AuditWarnings: auditW,
		AuditInfos:    auditI,
		AuditFindings: findings,
	}

	// Cached vuln data — passive read keyed by the configured OSV URL
	// so we hit the same cache file the CLI populates.
	vulnURL := s.resolveVulnSourceKey()
	view.VulnSource = vulnURL
	if rep, ok := vuln.ReadCache(vulnURL); ok {
		view.VulnCacheLoaded = true
		view.VulnMatches = rep.Matches
		view.SkippedTools = rep.Skipped
		for _, m := range rep.Matches {
			if len(m.Vulnerabilities) > 0 {
				view.VulnRiskCount++
			}
		}
	}

	s.renderPage(w, r, "security.html", pageData{
		Title:     "Security",
		ActiveTab: "security",
		Data:      view,
	})
}

// resolveVulnSourceKey returns the cache key the CLI's `clim security
// vuln` writes under, given the server's loaded config. Surfaces that
// read passively (tool detail, /security) must match this key or
// they'll look at the wrong cache file.
func (s *Server) resolveVulnSourceKey() string {
	cfg := s.snapshotConfig()
	if u := strings.TrimSpace(cfg.Vuln.URL); u != "" {
		return u
	}
	return vuln.DefaultOSVURL
}

// buildToolSecurity computes the per-tool security panel for the
// /tools/{name} page. Audit findings are computed from the in-memory
// tool list (no I/O); vuln data is read from cache only — page
// renders never block on a live OSV.dev call. Returns nil when the
// tool isn't installed (a verdict only makes sense for installed
// tools).
func buildToolSecurity(tool registry.Tool, allTools []registry.Tool, s *Server) *toolSecurityView {
	if !tool.IsInstalled() {
		return nil
	}
	findings, _ := audit.Analyze(allTools)

	var match *vuln.Match
	cacheLoaded := false
	if rep, ok := vuln.ReadCache(s.resolveVulnSourceKey()); ok {
		cacheLoaded = true
		for i := range rep.Matches {
			if rep.Matches[i].Tool == tool.Name {
				match = &rep.Matches[i]
				break
			}
		}
	}

	verdict := security.Score(tool, findings, match)
	out := &toolSecurityView{
		Status:           strings.ToLower(verdict.Status.String()),
		Reasons:          verdict.Reasons,
		VulnsCacheLoaded: cacheLoaded,
	}
	if match != nil {
		out.Vulns = match.Vulnerabilities
	}
	return out
}
