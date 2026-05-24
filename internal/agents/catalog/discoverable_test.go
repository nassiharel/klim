package catalog

import (
	"testing"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/registry"
)

// TestExpandDefs verifies that AgentMarketplaceDef records are correctly
// expanded into agents.Marketplace records with proper provider mapping,
// ID generation, and sorting.
func TestExpandDefs(t *testing.T) {
	defs := []registry.AgentMarketplaceDef{
		{
			Name:        "test-marketplace",
			Providers:   []string{"claude-code"},
			URL:         "https://example.com",
			InstallSpec: "owner/repo",
		},
	}
	out := expandDefs(defs)
	if len(out) != 1 {
		t.Fatalf("expected 1 marketplace, got %d", len(out))
	}
	if out[0].Name != "test-marketplace" {
		t.Errorf("Name = %q, want test-marketplace", out[0].Name)
	}
	if out[0].Provider != agents.ProviderClaudeCode {
		t.Errorf("Provider = %q, want claude-code", out[0].Provider)
	}
	if out[0].Source != agents.SourceCatalogClaude {
		t.Errorf("Source = %q, want catalog-claude", out[0].Source)
	}
	if out[0].Installed {
		t.Error("Installed should be false")
	}
	if out[0].ID != "test-marketplace" {
		t.Errorf("ID = %q, want test-marketplace (defaulted from Name)", out[0].ID)
	}
}

// TestExpandDefs_MultiProvider verifies that a marketplace listed under
// multiple providers is expanded into one agents.Marketplace per
// provider with provider-qualified IDs.
func TestExpandDefs_MultiProvider(t *testing.T) {
	defs := []registry.AgentMarketplaceDef{
		{
			Name:        "multi-mp",
			Providers:   []string{"claude-code", "copilot-cli"},
			URL:         "https://example.com",
			InstallSpec: "owner/repo",
		},
	}
	out := expandDefs(defs)
	if len(out) != 2 {
		t.Fatalf("expected 2 marketplaces, got %d", len(out))
	}
	// Verify provider-qualified IDs.
	ids := map[string]bool{}
	for _, m := range out {
		ids[m.ID] = true
	}
	if !ids["multi-mp:claude-code"] {
		t.Error("missing provider-qualified ID multi-mp:claude-code")
	}
	if !ids["multi-mp:copilot-cli"] {
		t.Error("missing provider-qualified ID multi-mp:copilot-cli")
	}
}

// TestExpandDefs_SkipsInvalid verifies that entries missing required
// fields are silently skipped.
func TestExpandDefs_SkipsInvalid(t *testing.T) {
	defs := []registry.AgentMarketplaceDef{
		{Name: "", Providers: []string{"claude-code"}, URL: "https://example.com"},     // no name
		{Name: "no-providers", URL: "https://example.com"},                             // no providers
		{Name: "no-action", Providers: []string{"claude-code"}},                        // no url or install_spec
		{Name: "valid", Providers: []string{"claude-code"}, InstallSpec: "owner/repo"}, // valid
	}
	out := expandDefs(defs)
	if len(out) != 1 {
		t.Fatalf("expected 1 valid marketplace, got %d", len(out))
	}
	if out[0].Name != "valid" {
		t.Errorf("Name = %q, want valid", out[0].Name)
	}
}

func TestSourceForProvider(t *testing.T) {
	cases := map[agents.ProviderID]agents.Source{
		agents.ProviderClaudeCode:  agents.SourceCatalogClaude,
		agents.ProviderCopilotCLI:  agents.SourceCatalogCopilot,
		agents.ProviderMCPRegistry: agents.SourceCatalogMCP,
	}
	for in, want := range cases {
		if got := sourceForProvider(in); got != want {
			t.Errorf("sourceForProvider(%q) = %q, want %q", in, got, want)
		}
	}
}
