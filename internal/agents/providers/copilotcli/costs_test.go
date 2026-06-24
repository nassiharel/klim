package copilotcli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/nassiharel/klim/internal/agents/costs"
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
	res, err := p.TokenSamples(context.Background(), costs.ScanInput{})
	if err != nil {
		t.Fatalf("TokenSamples: %v", err)
	}
	samples := res.Samples
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
	res, err := p.TokenSamples(context.Background(), costs.ScanInput{})
	if err != nil {
		t.Errorf("err = %v", err)
	}
	if len(res.Samples) != 0 {
		t.Errorf("expected zero, got %d", len(res.Samples))
	}
}

// TestTokenSamples_SessionStateLayout pins the 1.x on-disk layout
// (session-state/<uuid>/events.jsonl). The original implementation
// walked sessions/ — a pre-1.0 path that no current install populates —
// so cost accounting silently returned zero samples for every Copilot
// user.
func TestTokenSamples_SessionStateLayout(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "session-state", "abc-1.x")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	events := `{"type":"session.start","timestamp":"2026-06-11T15:00:00Z","data":{"sessionId":"abc-1.x","title":"klim build"}}
{"type":"model.response","timestamp":"2026-06-11T15:00:01Z","data":{"sessionId":"abc-1.x","model":"gpt-5.4","usage":{"inputTokens":200,"outputTokens":40}}}
`
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), []byte(events), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	p := &Provider{HomeOverride: home}
	res, err := p.TokenSamples(context.Background(), costs.ScanInput{})
	if err != nil {
		t.Fatalf("TokenSamples: %v", err)
	}
	samples := res.Samples
	if len(samples) != 1 {
		t.Fatalf("expected 1 sample under session-state/, got %d", len(samples))
	}
	if samples[0].Input != 200 || samples[0].Output != 40 {
		t.Errorf("got input/output %d/%d, want 200/40", samples[0].Input, samples[0].Output)
	}
}
