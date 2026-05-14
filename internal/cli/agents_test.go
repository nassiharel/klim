package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/agents"
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
func (f *fakeProvider) AddMCP(_ context.Context, _ agents.MCPSpec) error             { return nil }
func (f *fakeProvider) RemoveMCP(_ context.Context, _ string) error                  { return nil }
func (f *fakeProvider) EnableMCP(_ context.Context, _ string, _ bool) error          { return nil }
func (f *fakeProvider) DeleteSession(_ context.Context, _ string) error              { return nil }
func (f *fakeProvider) BuildLaunch(_ agents.LaunchSpec) (agents.ExecPlan, error) {
	return agents.ExecPlan{Bin: "fake"}, nil
}

var _ = errors.New
