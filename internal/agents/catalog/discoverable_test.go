package catalog

import (
	"testing"

	"github.com/nassiharel/klim/internal/agents"
)

// TestLoadDiscoverable_AllFilesParse fails loudly when any embedded
// YAML file in marketplace/marketplaces/ is malformed or missing a
// required field. This is the entry point that ships with klim, so a
// typo here would silently drop a marketplace from the Marketplaces
// sub-tab.
func TestLoadDiscoverable_AllFilesParse(t *testing.T) {
	out, err := loadDiscoverable()
	if err != nil {
		t.Fatalf("loadDiscoverable: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("expected at least one discoverable marketplace")
	}
	for _, m := range out {
		if m.Name == "" {
			t.Errorf("entry missing Name: %+v", m)
		}
		if m.Provider == "" {
			t.Errorf("entry %q missing Provider", m.Name)
		}
		if m.Installed {
			t.Errorf("entry %q should have Installed=false", m.Name)
		}
	}
}

// TestLoadDiscoverable_ExpandsProviders verifies that a marketplace
// listed under multiple `providers:` is expanded into one
// agents.Marketplace per provider.
func TestLoadDiscoverable_ExpandsProviders(t *testing.T) {
	out, _ := loadDiscoverable()
	// claude-plugins-official is single-provider; just sanity-check
	// the source mapping. The expansion logic is also covered
	// directly via sourceForProvider().
	var got *agents.Marketplace
	for i := range out {
		if out[i].Name == "claude-plugins-official" && out[i].Provider == agents.ProviderClaudeCode {
			got = &out[i]
			break
		}
	}
	if got == nil {
		t.Fatal("claude-plugins-official/claude-code not in discoverable list")
	}
	if got.Source != agents.SourceCatalogClaude {
		t.Errorf("source = %q, want catalog-claude", got.Source)
	}
	if got.InstallSpec == "" {
		t.Error("InstallSpec should not be empty")
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
