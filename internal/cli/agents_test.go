package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/agents/costs"
	"github.com/nassiharel/klim/internal/agents/search"
)

func TestInferLaunchProvider_SessionPrefix(t *testing.T) {
	svc := agents.NewService(2)
	prov, _, err := inferLaunchProvider(context.Background(), svc, "", "claude:abc", "")
	if err != nil || prov != agents.ProviderClaudeCode {
		t.Errorf("claude prefix: got (%q, %v)", prov, err)
	}
	prov, _, err = inferLaunchProvider(context.Background(), svc, "", "copilot:xyz", "")
	if err != nil || prov != agents.ProviderCopilotCLI {
		t.Errorf("copilot prefix: got (%q, %v)", prov, err)
	}
}

func TestInferLaunchProvider_NoHint(t *testing.T) {
	svc := agents.NewService(2)
	_, _, err := inferLaunchProvider(context.Background(), svc, "", "", "")
	if err == nil || !strings.Contains(err.Error(), "--provider is required") {
		t.Errorf("expected helpful error; got %v", err)
	}
}

func TestInferLaunchProvider_UniqueSkill(t *testing.T) {
	stub := &fakeProvider{
		id:     agents.ProviderClaudeCode,
		skills: []agents.Skill{{Name: "review", Provider: agents.ProviderClaudeCode}},
	}
	svc := agents.NewService(2, stub)
	if _, err := svc.LoadAll(context.Background(), agents.LoadOpts{Refresh: true}); err != nil {
		t.Fatalf("load: %v", err)
	}
	prov, hint, err := inferLaunchProvider(context.Background(), svc, "review", "", "")
	if err != nil {
		t.Fatalf("infer: %v", err)
	}
	if prov != agents.ProviderClaudeCode {
		t.Errorf("provider = %q, want claude-code", prov)
	}
	if !strings.Contains(hint, "claude-code") {
		t.Errorf("hint should mention provider, got %q", hint)
	}
}

func TestInferLaunchProvider_AmbiguousSkill(t *testing.T) {
	a := &fakeProvider{
		id:     agents.ProviderClaudeCode,
		skills: []agents.Skill{{Name: "review", Provider: agents.ProviderClaudeCode}},
	}
	b := &fakeProvider{
		id:     agents.ProviderCopilotCLI,
		skills: []agents.Skill{{Name: "review", Provider: agents.ProviderCopilotCLI}},
	}
	svc := agents.NewService(2, a, b)
	if _, err := svc.LoadAll(context.Background(), agents.LoadOpts{Refresh: true}); err != nil {
		t.Fatalf("load: %v", err)
	}
	_, _, err := inferLaunchProvider(context.Background(), svc, "review", "", "")
	if err == nil || !strings.Contains(err.Error(), "multiple providers") {
		t.Errorf("expected ambiguity error; got %v", err)
	}
}

type fakeProvider struct {
	id     agents.ProviderID
	skills []agents.Skill
}

func (f *fakeProvider) ID() agents.ProviderID                                        { return f.id }
func (f *fakeProvider) DisplayName() string                                          { return string(f.id) }
func (f *fakeProvider) Detect(_ context.Context) agents.Status                       { return agents.Status{Installed: true} }
func (f *fakeProvider) Marketplaces(_ context.Context) ([]agents.Marketplace, error) { return nil, nil }
func (f *fakeProvider) Plugins(_ context.Context) ([]agents.Plugin, error)           { return nil, nil }
func (f *fakeProvider) Skills(_ context.Context) ([]agents.Skill, error)             { return f.skills, nil }
func (f *fakeProvider) MCPs(_ context.Context) ([]agents.MCP, error)                 { return nil, nil }
func (f *fakeProvider) Sessions(_ context.Context) ([]agents.Session, error)         { return nil, nil }
func (f *fakeProvider) AddMarketplace(_ context.Context, _ string) error             { return nil }
func (f *fakeProvider) RemoveMarketplace(_ context.Context, _ string) error          { return nil }
func (f *fakeProvider) InstallPlugin(_ context.Context, _ agents.PluginRef) error    { return nil }
func (f *fakeProvider) UninstallPlugin(_ context.Context, _ string) error            { return nil }
func (f *fakeProvider) EnablePlugin(_ context.Context, _ string, _ bool) error       { return nil }
func (f *fakeProvider) UpdatePlugin(_ context.Context, _ string) error               { return nil }
func (f *fakeProvider) TokenSamples(_ context.Context, _ costs.ScanInput) (costs.ScanResult, error) {
	return costs.ScanResult{}, nil
}
func (f *fakeProvider) SessionTexts(_ context.Context) ([]search.SessionText, error) {
	return nil, nil
}
func (f *fakeProvider) AddMCP(_ context.Context, _ agents.MCPSpec) error    { return nil }
func (f *fakeProvider) RemoveMCP(_ context.Context, _ string) error         { return nil }
func (f *fakeProvider) EnableMCP(_ context.Context, _ string, _ bool) error { return nil }
func (f *fakeProvider) DeleteSession(_ context.Context, _ string) error     { return nil }
func (f *fakeProvider) BuildLaunch(_ agents.LaunchSpec) (agents.ExecPlan, error) {
	return agents.ExecPlan{Bin: "fake"}, nil
}

var _ = errors.New

func TestInferLaunchProvider_BareSessionMissingPrefix(t *testing.T) {
	// PR #77 review #6: a session id without a provider prefix used
	// to return the generic "no provider owns the requested entity"
	// error from the snapshot-scan fallthrough. We now surface a
	// specific hint.
	svc := agents.NewService(2)
	_, _, err := inferLaunchProvider(context.Background(), svc, "", "abc-123", "")
	if err == nil {
		t.Fatal("expected error for bare session id")
	}
	if !strings.Contains(err.Error(), "missing a provider prefix") {
		t.Errorf("expected specific error; got %v", err)
	}
}

func TestForEachProvider_HonoursProviderFlag(t *testing.T) {
	// PR #77 review #4: forEachProvider must respect the --provider
	// flag so mutations target the right backend.
	defer func(prev string) { agentsListProvider = prev }(agentsListProvider)

	// Restore the production service factory after the test even on
	// failure.
	origFactory := newAgentsService
	defer func() { newAgentsService = origFactory }()

	claude := &fakeProvider{id: agents.ProviderClaudeCode}
	copilot := &fakeProvider{id: agents.ProviderCopilotCLI}
	newAgentsService = func() *agents.Service {
		return agents.NewService(2, claude, copilot)
	}

	agentsListProvider = "copilot-cli"
	var hits []agents.ProviderID
	err := forEachProvider(context.Background(), func(p agents.Provider) error {
		hits = append(hits, p.ID())
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hits) != 1 || hits[0] != agents.ProviderCopilotCLI {
		t.Errorf("expected only copilot-cli hit, got %v", hits)
	}

	// Unknown provider returns a useful error.
	agentsListProvider = "windsurf"
	if err := forEachProvider(context.Background(), func(p agents.Provider) error { return nil }); err == nil || !strings.Contains(err.Error(), "not registered") {
		t.Errorf("expected 'not registered' error for unknown provider, got %v", err)
	}

	// Empty filter falls back to walking every provider.
	agentsListProvider = ""
	hits = nil
	_ = forEachProvider(context.Background(), func(p agents.Provider) error {
		hits = append(hits, p.ID())
		return nil
	})
	if len(hits) == 0 {
		t.Error("empty filter should walk providers")
	}
}

// TestAgentsResume_BadArgsReturnUsageError pins the convention that
// argument/flag validation failures from `klim agent session resume`
// surface as *UsageError so Run() maps them to exit code 2 (per
// CLI-CONVENTIONS.md). Prior to this they were returned as plain
// errors.New(...) which exited with code 1 — indistinguishable from
// a genuine runtime failure.
func TestAgentsResume_BadArgsReturnUsageError(t *testing.T) {
	t.Parallel()

	mkCmd := func() *cobra.Command {
		c := &cobra.Command{Use: "resume"}
		c.Flags().Bool("last", false, "")
		return c
	}

	t.Run("no args, no --last", func(t *testing.T) {
		err := agentsResumeSession(mkCmd(), nil)
		var ue *UsageError
		if !errors.As(err, &ue) {
			t.Fatalf("err = %v (%T), want *UsageError", err, err)
		}
		if !strings.Contains(err.Error(), "session id") {
			t.Errorf("unexpected message: %v", err)
		}
	})

	t.Run("--last with explicit id", func(t *testing.T) {
		c := mkCmd()
		_ = c.Flags().Set("last", "true")
		err := agentsResumeSession(c, []string{"claude:abc"})
		var ue *UsageError
		if !errors.As(err, &ue) {
			t.Fatalf("err = %v (%T), want *UsageError", err, err)
		}
		if !strings.Contains(err.Error(), "--last cannot be combined") {
			t.Errorf("unexpected message: %v", err)
		}
	})
}

// TestAgentsResume_TooManyArgsReturnsUsageError pins the >1-args
// path through Cobra's Args validator. Without the usageArgs wrap,
// cobra.MaximumNArgs(1) returns a plain error whose message
// ("accepts at most 1 arg(s), received N") does NOT match
// isCobraUsageError's prefix list, so Run() would exit 1 instead of
// 2 (violates CLI-CONVENTIONS.md). The wrap ensures the error type
// is *UsageError regardless of message wording.
//
// We exercise the validator directly to avoid spinning up the full
// rootCmd graph.
func TestAgentsResume_TooManyArgsReturnsUsageError(t *testing.T) {
	t.Parallel()

	// Reconstruct the same Args validator the production command
	// uses so the test stays decoupled from how the command is
	// wired together.
	validator := usageArgs(cobra.MaximumNArgs(1))
	err := validator(&cobra.Command{Use: "resume"}, []string{"a", "b"})
	if err == nil {
		t.Fatal("expected error from >1 args, got nil")
	}
	var ue *UsageError
	if !errors.As(err, &ue) {
		t.Fatalf("err = %v (%T), want *UsageError", err, err)
	}
}
