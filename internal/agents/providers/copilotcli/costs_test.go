package copilotcli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestTokenSamples_ParsesUsageBlocks(t *testing.T) {
	home := t.TempDir()
	sessDir := filepath.Join(home, "sessions", "abc-123")
	if err := os.MkdirAll(sessDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	events := `{"type":"session.start","timestamp":"2026-05-15T08:00:00Z","data":{"sessionId":"abc-123","title":"klim build"}}
{"type":"model.response","timestamp":"2026-05-15T08:01:00Z","data":{"sessionId":"abc-123","model":"gpt-5.4","usage":{"inputTokens":500,"outputTokens":50}}}
{"type":"model.response","timestamp":"2026-05-15T08:02:00Z","data":{"sessionId":"abc-123","model":"gpt-5.4","usage":{"input_tokens":10,"output_tokens":5}}}
`
	if err := os.WriteFile(filepath.Join(sessDir, "events.jsonl"), []byte(events), 0o644); err != nil {
		t.Fatalf("write events: %v", err)
	}

	p := &Provider{HomeOverride: home}
	samples, err := p.TokenSamples(context.Background())
	if err != nil {
		t.Fatalf("TokenSamples: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("expected 2 samples, got %d: %+v", len(samples), samples)
	}
	totalIn, totalOut := 0, 0
	for _, s := range samples {
		totalIn += s.Input
		totalOut += s.Output
		if s.SessionID != "copilot:abc-123" {
			t.Errorf("session id = %q", s.SessionID)
		}
		if s.Provider != "copilot-cli" {
			t.Errorf("provider = %q", s.Provider)
		}
		if s.Title != "klim build" {
			t.Errorf("title = %q", s.Title)
		}
	}
	if totalIn != 510 || totalOut != 55 {
		t.Errorf("totals input/output = %d/%d, want 510/55", totalIn, totalOut)
	}
}

func TestTokenSamples_MissingSessionsDir(t *testing.T) {
	p := &Provider{HomeOverride: t.TempDir()}
	samples, err := p.TokenSamples(context.Background())
	if err != nil {
		t.Errorf("err = %v", err)
	}
	if len(samples) != 0 {
		t.Errorf("expected zero, got %d", len(samples))
	}
}
