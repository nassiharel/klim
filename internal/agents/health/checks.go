package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ----- MCP reachability -----

type checkMCPReach struct {
	HTTPProbe func(ctx context.Context, url string) error
}

func (c *checkMCPReach) ID() string { return "mcp-reach" }

func (c *checkMCPReach) Run(ctx context.Context, snap Snapshot) []Issue {
	var issues []Issue
	for _, m := range snap.MCPs {
		// Skip catalog/remote-only entries — they're not configured
		// locally so reachability isn't ours to police.
		if m.Scope == "remote" || m.Scope == "" {
			continue
		}
		switch m.Transport {
		case "http", "sse":
			if m.URL == "" {
				issues = append(issues, Issue{
					CheckID:  c.ID(),
					Severity: SeverityError,
					Kind:     KindMCP,
					Subject:  m.Name,
					Provider: m.Provider,
					Title:    fmt.Sprintf("MCP %q has no URL", m.Name),
					Detail:   "Transport is http/sse but the URL field is empty — the MCP cannot be reached.",
					Hint:     "Set the url field in the provider's MCP config file.",
				})
				continue
			}
			pctx, cancel := context.WithTimeout(ctx, 4*time.Second)
			err := c.HTTPProbe(pctx, m.URL)
			cancel()
			if err != nil {
				issues = append(issues, Issue{
					CheckID:  c.ID(),
					Severity: SeverityError,
					Kind:     KindMCP,
					Subject:  m.Name,
					Provider: m.Provider,
					Title:    fmt.Sprintf("MCP %q endpoint unreachable", m.Name),
					Detail:   fmt.Sprintf("HTTP probe to %s failed: %v", m.URL, err),
					Hint:     "Check the server is running and your network can reach it (auth may also be wrong).",
				})
			}
		case "stdio", "":
			if m.Command == "" {
				continue
			}
			if !commandResolvable(m.Command) {
				issues = append(issues, Issue{
					CheckID:  c.ID(),
					Severity: SeverityError,
					Kind:     KindMCP,
					Subject:  m.Name,
					Provider: m.Provider,
					Title:    fmt.Sprintf("MCP %q command not found", m.Name),
					Detail:   fmt.Sprintf("`%s` is not on PATH and is not an existing absolute file.", m.Command),
					Hint:     "Install the binary or correct the command in the MCP config.",
				})
			}
		}
	}
	return issues
}

// commandResolvable returns true if `command` is either an existing
// file (with any path separator) or resolves via PATH.
func commandResolvable(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	if strings.ContainsAny(command, "/\\") {
		if _, err := os.Stat(command); err == nil {
			return true
		}
		return false
	}
	_, err := exec.LookPath(command)
	return err == nil
}

// DefaultHTTPProbe performs a HEAD/GET request against url with the
// caller's context. Any non-2xx/3xx response or transport error
// counts as unreachable. Used by the MCP reachability check in
// production; tests pass a stub.
func DefaultHTTPProbe(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 4 * time.Second}
	resp, err := client.Do(req)
	if err == nil {
		_ = resp.Body.Close()
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			return nil
		}
		return fmt.Errorf("HEAD %s -> %d", url, resp.StatusCode)
	}
	// Many MCP HTTP servers don't accept HEAD; retry with GET.
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	getResp, err := client.Do(getReq)
	if err != nil {
		return err
	}
	defer func() { _ = getResp.Body.Close() }()
	if getResp.StatusCode >= 200 && getResp.StatusCode < 400 {
		return nil
	}
	return fmt.Errorf("GET %s -> %d", url, getResp.StatusCode)
}

// ----- Duplicate MCP names -----

type checkDuplicateMCP struct{}

func (c *checkDuplicateMCP) ID() string { return "mcp-duplicate" }

func (c *checkDuplicateMCP) Run(_ context.Context, snap Snapshot) []Issue {
	type key struct{ name, provider string }
	seen := map[key][]MCPRef{}
	for _, m := range snap.MCPs {
		if m.Scope == "remote" {
			continue
		}
		k := key{name: strings.ToLower(m.Name), provider: m.Provider}
		seen[k] = append(seen[k], m)
	}
	var issues []Issue
	for k, list := range seen {
		if len(list) < 2 {
			continue
		}
		scopes := make([]string, 0, len(list))
		for _, m := range list {
			scopes = append(scopes, m.Scope)
		}
		issues = append(issues, Issue{
			CheckID:  c.ID(),
			Severity: SeverityWarn,
			Kind:     KindMCP,
			Subject:  list[0].Name,
			Provider: k.provider,
			Title:    fmt.Sprintf("MCP %q is configured %d times", list[0].Name, len(list)),
			Detail:   fmt.Sprintf("Scopes: %s. The provider may pick the wrong one.", strings.Join(scopes, ", ")),
			Hint:     "Remove the duplicate from one scope, or rename one of them.",
		})
	}
	return issues
}

// ----- Shadowed skill names -----

type checkShadowedSkill struct{}

func (c *checkShadowedSkill) ID() string { return "skill-shadowed" }

func (c *checkShadowedSkill) Run(_ context.Context, snap Snapshot) []Issue {
	type key struct{ name, provider string }
	hasUser := map[key]bool{}
	plugin := map[key][]SkillRef{}
	for _, s := range snap.Skills {
		k := key{name: strings.ToLower(s.Name), provider: s.Provider}
		switch s.Scope {
		case "user", "project":
			hasUser[k] = true
		case "plugin":
			plugin[k] = append(plugin[k], s)
		}
	}
	var issues []Issue
	for k, plist := range plugin {
		if !hasUser[k] {
			continue
		}
		ref := plist[0]
		issues = append(issues, Issue{
			CheckID:  c.ID(),
			Severity: SeverityWarn,
			Kind:     KindSkill,
			Subject:  ref.Name,
			Provider: ref.Provider,
			Title:    fmt.Sprintf("skill %q from plugin %q is shadowed", ref.Name, ref.SourcePlugin),
			Detail:   fmt.Sprintf("A user/project skill with the same name takes precedence; the plugin's version will not be invoked."),
			Hint:     "Rename one of the skills or remove the duplicate.",
		})
	}
	return issues
}

// ----- Plugin manifest validity -----

type checkPluginManifest struct{}

func (c *checkPluginManifest) ID() string { return "plugin-manifest" }

func (c *checkPluginManifest) Run(_ context.Context, snap Snapshot) []Issue {
	var issues []Issue
	for _, p := range snap.Plugins {
		if !p.Installed || p.InstallPath == "" {
			continue
		}
		if p.Name == "" {
			issues = append(issues, Issue{
				CheckID:  c.ID(),
				Severity: SeverityError,
				Kind:     KindPlugin,
				Subject:  p.InstallPath,
				Provider: p.Provider,
				Title:    "plugin manifest missing name",
				Hint:     "Add a `name` field to the plugin's manifest.",
			})
			continue
		}
		if p.Version == "" {
			issues = append(issues, Issue{
				CheckID:  c.ID(),
				Severity: SeverityWarn,
				Kind:     KindPlugin,
				Subject:  p.Name,
				Provider: p.Provider,
				Title:    fmt.Sprintf("plugin %q has no version", p.Name),
				Hint:     "Add a `version` field so update detection works.",
			})
		}
		if _, err := os.Stat(p.InstallPath); err != nil {
			issues = append(issues, Issue{
				CheckID:  c.ID(),
				Severity: SeverityError,
				Kind:     KindPlugin,
				Subject:  p.Name,
				Provider: p.Provider,
				Title:    fmt.Sprintf("plugin %q install path missing", p.Name),
				Detail:   fmt.Sprintf("`%s` does not exist on disk.", p.InstallPath),
				Hint:     "Reinstall the plugin, or remove its entry from the provider config.",
			})
		}
	}
	return issues
}

// ----- Broken JSON in agent config files -----

type checkBrokenJSON struct{}

func (c *checkBrokenJSON) ID() string { return "config-json" }

func (c *checkBrokenJSON) Run(_ context.Context, snap Snapshot) []Issue {
	var issues []Issue
	for _, f := range snap.ConfigFiles {
		data, err := os.ReadFile(f.Path)
		if err != nil {
			// Missing config files are fine — many users don't have them.
			continue
		}
		var any interface{}
		if err := json.Unmarshal(data, &any); err != nil {
			issues = append(issues, Issue{
				CheckID:  c.ID(),
				Severity: SeverityError,
				Kind:     KindConfigFile,
				Subject:  filepath.Base(f.Path),
				Provider: f.Provider,
				Title:    fmt.Sprintf("%s is not valid JSON", filepath.Base(f.Path)),
				Detail:   err.Error(),
				Hint:     "Open the file and fix the syntax — the provider may refuse to start otherwise.",
			})
		}
	}
	return issues
}

// ----- Stale marketplace catalog -----

type checkStaleCatalog struct {
	ThresholdDays int
	NowFunc       func() time.Time // override in tests
}

func (c *checkStaleCatalog) ID() string { return "catalog-stale" }

func (c *checkStaleCatalog) Run(_ context.Context, snap Snapshot) []Issue {
	threshold := c.ThresholdDays
	if threshold <= 0 {
		threshold = 14
	}
	now := time.Now
	if c.NowFunc != nil {
		now = c.NowFunc
	}
	cutoff := now().Add(-time.Duration(threshold) * 24 * time.Hour)
	var issues []Issue
	for _, mp := range snap.Marketplaces {
		if mp.LastSynced.IsZero() || !mp.LastSynced.Before(cutoff) {
			continue
		}
		issues = append(issues, Issue{
			CheckID:  c.ID(),
			Severity: SeverityInfo,
			Kind:     KindMarketplace,
			Subject:  mp.Name,
			Provider: mp.Provider,
			Title:    fmt.Sprintf("marketplace %q hasn't synced in %d days", mp.Name, int(now().Sub(mp.LastSynced).Hours()/24)),
			Hint:     "Press `r` in the Agents tab to refresh, or re-run `claude plugin marketplace add` to bump it.",
		})
	}
	return issues
}

// ----- Provider binary detection -----

type checkProviderInstalled struct{}

func (c *checkProviderInstalled) ID() string { return "provider-binary" }

func (c *checkProviderInstalled) Run(_ context.Context, snap Snapshot) []Issue {
	var issues []Issue
	for _, p := range snap.Providers {
		if p.Installed {
			continue
		}
		// mcp-registry is virtual — always "installed" conceptually,
		// but its Installed flag may be false if the provider didn't
		// set it. Filter that out so we don't nag.
		if p.ID == "mcp-registry" {
			continue
		}
		issues = append(issues, Issue{
			CheckID:  c.ID(),
			Severity: SeverityInfo,
			Kind:     KindProvider,
			Subject:  p.ID,
			Provider: p.ID,
			Title:    fmt.Sprintf("provider %q binary is not on PATH", p.ID),
			Detail:   "klim can still display cached state, but mutations and launch will fail until the CLI is installed.",
			Hint:     fmt.Sprintf("Install with `klim install %s` or follow the upstream docs.", providerInstallHint(p.ID)),
		})
	}
	return issues
}

func providerInstallHint(id string) string {
	switch id {
	case "claude-code":
		return "claude-code"
	case "copilot-cli":
		return "copilot-cli"
	}
	return id
}
