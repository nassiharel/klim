package tui

import (
	"context"
	"testing"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/agents/costs"
	"github.com/nassiharel/klim/internal/agents/providers/claudecode"
)

// erroringCostsProvider embeds a real provider (to satisfy the 21-method
// agents.Provider interface) but forces TokenSamples to fail, simulating
// a transient read error / timeout during a cost scan.
type erroringCostsProvider struct {
	*claudecode.Provider
}

func (erroringCostsProvider) TokenSamples(context.Context, costs.ScanInput) (costs.ScanResult, error) {
	return costs.ScanResult{}, context.DeadlineExceeded
}

// TestLoadCostSamples_DoesNotPruneOnProviderError pins the fix for the
// review finding: when a provider's TokenSamples fails, loadCostSamples
// must NOT prune the on-disk cache (the sessions still exist; the scan
// just couldn't read them this time). Pruning would wipe valid data and
// force a cold rescan next time.
func TestLoadCostSamples_DoesNotPruneOnProviderError(t *testing.T) {
	// Isolate the klim home so we touch a temp cache, not the real one.
	t.Setenv("KLIM_HOME", t.TempDir())

	// Seed a cache with an existing session.
	cache := costs.NewCache()
	cache.Sessions["claude:existing"] = costs.CachedEntry{
		Provider: "claude-code",
		Days:     map[string]costs.Totals{"2026-05-15": {Input: 100, Output: 20}},
	}
	if err := cache.Save(); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	// Swap in a service whose only provider errors on TokenSamples.
	prev := agentsService
	agentsService = func() *agents.Service {
		return agents.NewService(2, erroringCostsProvider{&claudecode.Provider{HomeOverride: t.TempDir()}})
	}
	defer func() { agentsService = prev }()

	if _, err := loadCostSamples(); err != nil {
		t.Fatalf("loadCostSamples: %v", err)
	}

	// The seeded session must survive — the failed scan must not prune it.
	reloaded, _ := costs.LoadCache()
	if _, ok := reloaded.Sessions["claude:existing"]; !ok {
		t.Errorf("a provider error must not prune the cache; got %v", reloaded.Sessions)
	}
}
