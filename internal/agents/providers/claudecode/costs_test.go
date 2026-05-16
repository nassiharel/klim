package claudecode

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nassiharel/klim/internal/agents"
)

func TestTokenSamples_ParsesUsageBlocks(t *testing.T) {
	home := t.TempDir()
	projDir := filepath.Join(home, ".claude", "projects", "home%2Fuser%2Frepo")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	transcript := `{"type":"user","timestamp":"2026-05-15T08:00:00Z","sessionId":"abc","message":{"role":"user"}}
{"type":"assistant","timestamp":"2026-05-15T08:00:01Z","sessionId":"abc","message":{"role":"assistant","model":"claude-sonnet-4.6","usage":{"input_tokens":120,"output_tokens":34,"cache_read_input_tokens":50}}}
{"type":"assistant","timestamp":"2026-05-15T08:00:02Z","sessionId":"abc","message":{"role":"assistant","model":"claude-sonnet-4.6","usage":{"input_tokens":0,"output_tokens":0}}}
`
	if err := os.WriteFile(filepath.Join(projDir, "session.jsonl"), []byte(transcript), 0o644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	p := &Provider{HomeOverride: home}
	samples, err := p.TokenSamples(context.Background())
	if err != nil {
		t.Fatalf("TokenSamples: %v", err)
	}
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample with non-zero usage, got %d: %+v", len(samples), samples)
	}
	s := samples[0]
	if s.Input != 170 || s.Output != 34 {
		t.Errorf("input/output = %d/%d, want 170/34", s.Input, s.Output)
	}
	if s.Provider != "claude-code" {
		t.Errorf("provider = %q", s.Provider)
	}
	if s.Model != "claude-sonnet-4.6" {
		t.Errorf("model = %q", s.Model)
	}
	if s.SessionID != "claude:abc" {
		t.Errorf("session id = %q", s.SessionID)
	}
}

func TestTokenSamples_MissingProjectsDir_NoError(t *testing.T) {
	p := &Provider{HomeOverride: t.TempDir()}
	samples, err := p.TokenSamples(context.Background())
	if err != nil {
		t.Errorf("err = %v", err)
	}
	if len(samples) != 0 {
		t.Errorf("expected zero samples, got %d", len(samples))
	}
}

func TestTokenSamples_FallsBackToProjectNameWhenSessionIDMissing(t *testing.T) {
	home := t.TempDir()
	projDir := filepath.Join(home, ".claude", "projects", "myproj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	transcript := `{"type":"assistant","timestamp":"2026-05-15T08:00:01Z","message":{"role":"assistant","model":"sonnet","usage":{"input_tokens":1,"output_tokens":2}}}` + "\n"
	if err := os.WriteFile(filepath.Join(projDir, "s.jsonl"), []byte(transcript), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	p := &Provider{HomeOverride: home}
	samples, err := p.TokenSamples(context.Background())
	if err != nil || len(samples) != 1 {
		t.Fatalf("samples=%d err=%v", len(samples), err)
	}
	if samples[0].SessionID != "claude:myproj" {
		t.Errorf("session id = %q, want claude:myproj", samples[0].SessionID)
	}
}

var _ = agents.ProviderClaudeCode
