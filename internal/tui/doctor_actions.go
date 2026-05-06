package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/nassiharel/klim/internal/compliance"
	"github.com/nassiharel/klim/internal/config"
	"github.com/nassiharel/klim/internal/registry"
	"github.com/nassiharel/klim/internal/vuln"
)

// vulnScanResultMsg arrives after a forced OSV.dev scan completes.
type vulnScanResultMsg struct {
	report *vuln.Report
	err    error
}

// scanVulnsCmd kicks off a fresh OSV.dev scan for every installed tool
// and writes the result to the local cache. Equivalent to running
// `klim security vuln --force-refresh-vulns` from the CLI. The TUI's
// existing Security tab reads from that cache, so the next render
// after this command finishes will show the fresh data.
func scanVulnsCmd(tools []registry.Tool, cfg *config.Config) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		url := vuln.DefaultOSVURL
		if cfg != nil && cfg.Vuln.URL != "" {
			url = cfg.Vuln.URL
		}
		client := &vuln.OSVClient{URL: url}
		opts := vuln.LookupOptions{ForceRefresh: true}
		rep, err := vuln.Lookup(ctx, client, tools, url, opts)
		return vulnScanResultMsg{report: rep, err: err}
	}
}

// refreshCompliancePolicyCmd force-fetches the policy at
// cfg.Compliance.URL, bypassing the cache. Mirrors `klim security
// compliance refresh`. The result piggybacks on the existing
// complianceLoadedMsg pipeline so the TUI's apply-and-rebuild path
// (recomputeComplianceDerivedState) is the same after either an
// auto-refresh-driven fetch or a manual one.
func refreshCompliancePolicyCmd(cfg *config.Config) tea.Cmd {
	if cfg == nil || cfg.Compliance.URL == "" {
		return nil
	}
	url := cfg.Compliance.URL
	return func() tea.Msg {
		ctx := context.Background()
		fetcher := &compliance.HTTPFetcher{URL: url}
		policy, _, err := compliance.Refresh(ctx, fetcher)
		if err != nil {
			return complianceLoadedMsg{errorMsg: fmt.Sprintf("Failed to refresh policy from %s: %v", url, err)}
		}
		return complianceLoadedMsg{policy: policy}
	}
}
