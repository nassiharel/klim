// Package mcpregistry implements a read-only Provider that surfaces
// entries from the official MCP registry (registry.modelcontextprotocol.io).
//
// Servers from the registry appear as available-to-install MCP entries in
// the Agents tab. They have no local install state — `Source` is set to
// SourceCatalogMCP so the UI can distinguish them from configured local
// MCPs. Mutations on this provider return ErrNotSupported; installing
// an entry routes through one of the agent providers (Claude / Copilot)
// which actually own the MCP configuration files.
package mcpregistry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/agents/costs"
	"github.com/nassiharel/klim/internal/agents/search"
)

const defaultAPIURL = "https://registry.modelcontextprotocol.io/v0/servers"

// Provider implements agents.Provider for the official MCP registry.
type Provider struct {
	APIURL string       // override for tests
	Client *http.Client // override for tests
	Limit  int          // per-page limit (default 50)
	Pages  int          // max pages to fetch (default 1, keeps things snappy)
}

// New returns a Provider with sensible defaults.
func New() *Provider {
	return &Provider{
		APIURL: defaultAPIURL,
		Client: &http.Client{Timeout: 10 * time.Second},
		Limit:  50,
		Pages:  1,
	}
}

// ID returns the stable provider identifier.
func (p *Provider) ID() agents.ProviderID { return agents.ProviderMCPRegistry }

// DisplayName returns the human-readable provider name.
func (p *Provider) DisplayName() string { return "MCP Registry" }

// Detect reports whether the registry endpoint is reachable. PR #77
// review #8 called out the old behaviour (always Installed=true,
// regardless of network state), which made the doctor command lie.
// We now do a short HEAD probe; failure flips Installed=false and
// surfaces the error in agents.Status so the provider-health pill
// reflects reality. The probe shares a 4-second budget with the
// caller's context — overall scan latency stays low even when the
// registry is unreachable.
func (p *Provider) Detect(ctx context.Context) agents.Status {
	api := p.APIURL
	if api == "" {
		api = defaultAPIURL
	}
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 4 * time.Second}
	}
	probeCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodHead, api, nil)
	if err != nil {
		return agents.Status{Installed: false, Error: err}
	}
	resp, err := client.Do(req)
	if err != nil {
		return agents.Status{Installed: false, Error: err}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return agents.Status{Installed: true, Version: "v0"}
	}
	return agents.Status{Installed: false, Error: fmt.Errorf("HEAD %s -> %d", api, resp.StatusCode)}
}

// Marketplaces returns the single registry as a marketplace entry.
func (p *Provider) Marketplaces(_ context.Context) ([]agents.Marketplace, error) {
	return []agents.Marketplace{
		{
			ID:          "mcp-registry",
			Name:        "mcp-registry",
			DisplayName: "Official MCP Registry",
			Description: "registry.modelcontextprotocol.io — community MCP server catalog",
			Provider:    p.ID(),
			URL:         "https://registry.modelcontextprotocol.io",
			Source:      agents.SourceCatalogMCP,
		},
	}, nil
}

// Plugins is not supported; the MCP registry catalogs servers, not plugins.
func (p *Provider) Plugins(_ context.Context) ([]agents.Plugin, error) {
	return nil, agents.ErrNotSupported
}

// Skills is not supported.
func (p *Provider) Skills(_ context.Context) ([]agents.Skill, error) {
	return nil, agents.ErrNotSupported
}

// MCPs fetches the registry catalog and returns each server as a
// remote MCP entry. PR #77 reviews #10 + #9 reshaped error handling:
// previously every URL/HTTP/JSON failure returned (nil, nil), so a
// 5xx response was indistinguishable from "registry empty". We now
// propagate the first error so the Service-level scan can attach it
// to the provider's Status and the doctor command can surface it.
//
// On a per-page failure (build URL, HTTP request, non-2xx response,
// or JSON decode) any successfully-parsed earlier pages are returned
// alongside the error. The decoder is all-or-nothing within a single
// page — partially-decoded servers from the failing page itself are
// NOT preserved.
func (p *Provider) MCPs(ctx context.Context) ([]agents.MCP, error) {
	api := p.APIURL
	if api == "" {
		api = defaultAPIURL
	}
	limit := p.Limit
	if limit <= 0 {
		limit = 50
	}
	maxPages := p.Pages
	if maxPages <= 0 {
		maxPages = 1
	}
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	var out []agents.MCP
	cursor := ""
	for page := 0; page < maxPages; page++ {
		u, err := buildURL(api, limit, cursor)
		if err != nil {
			return out, fmt.Errorf("mcp-registry: build url: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return out, fmt.Errorf("mcp-registry: build request: %w", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return out, fmt.Errorf("mcp-registry: fetch: %w", err)
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_ = resp.Body.Close()
			return out, fmt.Errorf("mcp-registry: GET %s -> %d", u, resp.StatusCode)
		}
		var body registryResponse
		err = json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()
		if err != nil {
			return out, fmt.Errorf("mcp-registry: decode response: %w", err)
		}
		for _, s := range body.Servers {
			out = append(out, p.toMCP(s))
		}
		if body.Metadata.NextCursor == "" {
			break
		}
		cursor = body.Metadata.NextCursor
	}
	return out, nil
}

// Sessions is not supported.
func (p *Provider) Sessions(_ context.Context) ([]agents.Session, error) {
	return nil, agents.ErrNotSupported
}

// All mutations are unsupported — the registry is read-only. Installing
// an entry happens through one of the agent providers.

// AddMarketplace is unsupported.
func (p *Provider) AddMarketplace(context.Context, string) error { return agents.ErrNotSupported }

// RemoveMarketplace is unsupported.
func (p *Provider) RemoveMarketplace(context.Context, string) error {
	return agents.ErrNotSupported
}

// InstallPlugin is unsupported.
func (p *Provider) InstallPlugin(context.Context, agents.PluginRef) error {
	return agents.ErrNotSupported
}

// UninstallPlugin is unsupported.
func (p *Provider) UninstallPlugin(context.Context, string) error { return agents.ErrNotSupported }

// EnablePlugin is unsupported.
func (p *Provider) EnablePlugin(context.Context, string, bool) error {
	return agents.ErrNotSupported
}

// UpdatePlugin is unsupported — the MCP registry is read-only.
func (p *Provider) UpdatePlugin(context.Context, string) error {
	return agents.ErrNotSupported
}

// TokenSamples is unsupported — the registry has no sessions.
func (p *Provider) TokenSamples(context.Context) ([]costs.TokenSample, error) {
	return nil, agents.ErrNotSupported
}

// SessionTexts is unsupported — the registry has no transcripts.
func (p *Provider) SessionTexts(context.Context) ([]search.SessionText, error) {
	return nil, agents.ErrNotSupported
}

// AddMCP is unsupported.
func (p *Provider) AddMCP(context.Context, agents.MCPSpec) error { return agents.ErrNotSupported }

// RemoveMCP is unsupported.
func (p *Provider) RemoveMCP(context.Context, string) error { return agents.ErrNotSupported }

// EnableMCP is unsupported.
func (p *Provider) EnableMCP(context.Context, string, bool) error { return agents.ErrNotSupported }

// DeleteSession is unsupported.
func (p *Provider) DeleteSession(context.Context, string) error { return agents.ErrNotSupported }

// BuildLaunch is unsupported on the registry; route launches through a
// real agent provider (claude-code or copilot-cli).
func (p *Provider) BuildLaunch(_ agents.LaunchSpec) (agents.ExecPlan, error) {
	return agents.ExecPlan{}, errors.New("mcp-registry: launch not supported (route through claude-code or copilot-cli)")
}

func buildURL(base string, limit int, cursor string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	q := u.Query()
	if limit > 0 {
		q.Set("limit", itoa(limit))
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	negative := n < 0
	if negative {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

type registryResponse struct {
	Servers  []registryServerEntry `json:"servers"`
	Metadata struct {
		NextCursor string `json:"nextCursor,omitempty"`
		Count      int    `json:"count,omitempty"`
	} `json:"metadata,omitempty"`
}

// registryServerEntry is one element of registryResponse.Servers. PR
// #77 review: extracted as a named type so the JSON envelope shape
// is declared in one place — previously toMCP took an anonymous
// struct that re-declared the shape and would silently drift out of
// sync with registryResponse.Servers' element type.
type registryServerEntry struct {
	Server registryServer `json:"server"`
	Meta   any            `json:"_meta,omitempty"`
}

type registryServer struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
	WebsiteURL  string `json:"websiteUrl,omitempty"`
	Repository  struct {
		URL    string `json:"url,omitempty"`
		Source string `json:"source,omitempty"`
	} `json:"repository,omitempty"`
	Remotes []struct {
		Type string `json:"type,omitempty"`
		URL  string `json:"url,omitempty"`
	} `json:"remotes,omitempty"`
	Packages []struct {
		Registry string `json:"registry,omitempty"`
		Name     string `json:"name,omitempty"`
	} `json:"packages,omitempty"`
}

func (p *Provider) toMCP(in registryServerEntry) agents.MCP {
	s := in.Server
	transport := "stdio"
	// PR #77 review: local was named `url`, shadowing the imported
	// `net/url` package. Renamed to `remoteURL` so the package alias
	// stays usable in this function (and to make the variable's
	// purpose more obvious).
	remoteURL := ""
	if len(s.Remotes) > 0 {
		transport = s.Remotes[0].Type
		if transport == "streamable-http" {
			transport = "http"
		}
		remoteURL = s.Remotes[0].URL
	}
	name := s.Title
	if name == "" {
		name = s.Name
	}
	return agents.MCP{
		ID:        "mcp-registry:" + s.Name,
		Name:      name,
		Provider:  p.ID(),
		Transport: transport,
		URL:       remoteURL,
		Scope:     agents.ScopeRemote,
		Enabled:   false,
		Source:    agents.SourceCatalogMCP,
	}
}

// Compile-time check.
var _ agents.Provider = (*Provider)(nil)
