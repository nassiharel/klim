package agents

import (
	"context"
	"testing"
)

func TestSnapshotCount(t *testing.T) {
	s := &Snapshot{
		Marketplaces: []Marketplace{{}},
		Plugins:      []Plugin{{}, {}},
		Skills:       []Skill{{}, {}, {}},
		MCPs:         []MCP{{}, {}, {}, {}},
		Sessions:     []Session{{}, {}, {}, {}, {}},
	}
	tests := map[EntityType]int{
		EntityMarketplace:  1,
		EntityPlugin:       2,
		EntitySkill:        3,
		EntityMCP:          4,
		EntitySession:      5,
		EntityType("nope"): 0,
	}
	for typ, want := range tests {
		if got := s.Count(typ); got != want {
			t.Errorf("Count(%q)=%d want %d", typ, got, want)
		}
	}
}

func TestExecPlan_CommandLine(t *testing.T) {
	tests := []struct {
		plan ExecPlan
		want string
	}{
		{ExecPlan{Bin: "claude", Args: []string{"-r", "abc"}}, "claude -r abc"},
		{ExecPlan{Bin: "copilot", Args: []string{"-p", "fix the bug"}}, `copilot -p "fix the bug"`},
		{ExecPlan{Bin: "claude"}, "claude"},
	}
	for _, tt := range tests {
		if got := tt.plan.CommandLine(); got != tt.want {
			t.Errorf("CommandLine()=%q want %q", got, tt.want)
		}
	}
}

func TestRegistry_DeduplicatesByID(t *testing.T) {
	r := NewRegistry()
	r.Register(&stubProvider{id: "claude-code"})
	r.Register(&stubProvider{id: "claude-code"})
	if got := len(r.Providers()); got != 1 {
		t.Errorf("expected dedup, got %d providers", got)
	}
}

func TestService_SearchAndLaunch(t *testing.T) {
	stub := &stubProvider{
		id: "claude-code",
		skills: []Skill{
			{ID: "claude-code:user:react-helper", Name: "react-helper", Description: "React helper", Provider: "claude-code", Source: SourceLocalClaude},
			{ID: "claude-code:user:postgres", Name: "postgres", Description: "DB", Provider: "claude-code", Source: SourceLocalClaude},
		},
	}
	svc := NewService(2, stub)
	if _, err := svc.LoadAll(context.Background(), LoadOpts{Refresh: true}); err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	results := svc.Search("react", "")
	if len(results) == 0 {
		t.Fatal("expected at least one search hit for 'react'")
	}
	if results[0].Name != "react-helper" {
		t.Errorf("top hit = %q, want react-helper", results[0].Name)
	}
	results = svc.Search("plugin:react", "")
	for _, r := range results {
		if r.Type != EntityPlugin {
			t.Errorf("scoped query returned non-plugin type %q", r.Type)
		}
	}
}

type stubProvider struct {
	id     ProviderID
	skills []Skill
}

func (s *stubProvider) ID() ProviderID                                        { return s.id }
func (s *stubProvider) DisplayName() string                                   { return string(s.id) }
func (s *stubProvider) Detect(_ context.Context) Status                       { return Status{Installed: true} }
func (s *stubProvider) Marketplaces(_ context.Context) ([]Marketplace, error) { return nil, nil }
func (s *stubProvider) Plugins(_ context.Context) ([]Plugin, error)           { return nil, nil }
func (s *stubProvider) Skills(_ context.Context) ([]Skill, error)             { return s.skills, nil }
func (s *stubProvider) MCPs(_ context.Context) ([]MCP, error)                 { return nil, nil }
func (s *stubProvider) Sessions(_ context.Context) ([]Session, error)         { return nil, nil }

func (s *stubProvider) AddMarketplace(_ context.Context, _ string) error       { return nil }
func (s *stubProvider) RemoveMarketplace(_ context.Context, _ string) error    { return nil }
func (s *stubProvider) InstallPlugin(_ context.Context, _ PluginRef) error     { return nil }
func (s *stubProvider) UninstallPlugin(_ context.Context, _ string) error      { return nil }
func (s *stubProvider) EnablePlugin(_ context.Context, _ string, _ bool) error { return nil }
func (s *stubProvider) AddMCP(_ context.Context, _ MCPSpec) error              { return nil }
func (s *stubProvider) RemoveMCP(_ context.Context, _ string) error            { return nil }
func (s *stubProvider) EnableMCP(_ context.Context, _ string, _ bool) error    { return nil }
func (s *stubProvider) DeleteSession(_ context.Context, _ string) error        { return nil }
func (s *stubProvider) BuildLaunch(_ LaunchSpec) (ExecPlan, error)             { return ExecPlan{Bin: "stub"}, nil }
