package copilotcli

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

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

// TestTokenSamples_CancelledContext pins that a cancelled context stops
// the scan and surfaces an error instead of scanning the whole corpus.
func TestTokenSamples_CancelledContext(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "session-state", "abc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "events.jsonl"),
		[]byte(`{"type":"model.response","data":{"sessionId":"abc","usage":{"inputTokens":1,"outputTokens":1}}}`+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := &Provider{HomeOverride: home}
	if _, err := p.TokenSamples(ctx, costs.ScanInput{}); err == nil {
		t.Errorf("expected an error from a cancelled context")
	}
}

// TestTokenSamples_SkipsUnchanged pins the incremental-scan skip path:
// a cold scan captures Seen["copilot:<id>"], and a warm scan with that
// mtime in Prior must skip re-parsing (no Samples) while still reporting
// the session in Seen (so it isn't pruned). A newer file re-parses.
func TestTokenSamples_SkipsUnchanged(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "session-state", "abc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	events := filepath.Join(dir, "events.jsonl")
	if err := os.WriteFile(events,
		[]byte(`{"type":"model.response","data":{"sessionId":"abc","usage":{"inputTokens":5,"outputTokens":7}}}`+"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	p := &Provider{HomeOverride: home}

	cold, err := p.TokenSamples(context.Background(), costs.ScanInput{})
	if err != nil || len(cold.Samples) != 1 {
		t.Fatalf("cold: samples=%d err=%v", len(cold.Samples), err)
	}
	mt, ok := cold.Seen["copilot:abc"]
	if !ok {
		t.Fatalf("cold scan should report Seen[copilot:abc]; got %v", cold.Seen)
	}

	warm, err := p.TokenSamples(context.Background(), costs.ScanInput{Prior: map[string]time.Time{"copilot:abc": mt}})
	if err != nil {
		t.Fatalf("warm: %v", err)
	}
	if len(warm.Samples) != 0 {
		t.Errorf("warm scan should skip unchanged file; got %d samples", len(warm.Samples))
	}
	if _, ok := warm.Seen["copilot:abc"]; !ok {
		t.Errorf("warm scan must still report the session in Seen; got %v", warm.Seen)
	}

	newer := mt.Add(2 * time.Second)
	if err := os.Chtimes(events, newer, newer); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	changed, err := p.TokenSamples(context.Background(), costs.ScanInput{Prior: map[string]time.Time{"copilot:abc": mt}})
	if err != nil {
		t.Fatalf("changed: %v", err)
	}
	if len(changed.Samples) != 1 {
		t.Errorf("a newer transcript must be re-parsed; got %d samples", len(changed.Samples))
	}
}
