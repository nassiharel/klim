package web

import (
	"context"
	"net/http"

	"github.com/nassiharel/clim/internal/audit"
	"github.com/nassiharel/clim/internal/vuln"
)

// securityView is the data shape for the /security page.
type securityView struct {
	AuditWarnings        int
	AuditInfos           int
	AuditFindings        []audit.Finding
	VulnRiskCount        int
	VulnMatches          []vuln.Match
	VulnCacheLoaded      bool
	VulnSource           string
	SkippedTools         []vuln.Skip
	ComplianceLoaded     bool
	ComplianceViolations int
}

// pageSecurity renders the umbrella Security page. It aggregates:
//   - audit findings (in-memory; cheap)
//   - cached vulnerability scan results (no network — tells the user
//     to run `clim security vuln` if no cache exists)
//   - compliance state when a policy is loaded
//
// We deliberately don't fetch fresh vuln data here — the web view
// shouldn't block its render on a 30s OSV.dev round-trip. The user
// triggers a refresh from the CLI; the page picks it up next reload.
func (s *Server) pageSecurity(w http.ResponseWriter, r *http.Request) {
	tools, _, _, err := s.opts.Service.LoadAndResolveCached(context.Background(), false)
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
	cfg := s.snapshotConfig()
	vulnURL := cfg.Vuln.URL
	if vulnURL == "" {
		vulnURL = vuln.DefaultOSVURL
	}
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
