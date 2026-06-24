package claudecode

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/agents/costs"
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
	res, err := p.TokenSamples(context.Background(), costs.ScanInput{})
	if err != nil {
		t.Fatalf("TokenSamples: %v", err)
	}
	samples := res.Samples
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
	// Cost samples + the Seen/cache key are keyed by the project DIR
	// name (matching the session list's ID), not the in-file sessionId.
	if s.SessionID != "claude:home%2Fuser%2Frepo" {
		t.Errorf("session id = %q, want claude:home%%2Fuser%%2Frepo", s.SessionID)
	}
	if _, ok := res.Seen["claude:home%2Fuser%2Frepo"]; !ok {
		t.Errorf("Seen should record the session by project dir; got %v", res.Seen)
	}
}

func TestTokenSamples_MissingProjectsDir_NoError(t *testing.T) {
	p := &Provider{HomeOverride: t.TempDir()}
	res, err := p.TokenSamples(context.Background(), costs.ScanInput{})
	if err != nil {
		t.Errorf("err = %v", err)
	}
	if len(res.Samples) != 0 {
		t.Errorf("expected zero samples, got %d", len(res.Samples))
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
	res, err := p.TokenSamples(context.Background(), costs.ScanInput{})
	if err != nil || len(res.Samples) != 1 {
		t.Fatalf("samples=%d err=%v", len(res.Samples), err)
	}
	if res.Samples[0].SessionID != "claude:myproj" {
		t.Errorf("session id = %q, want claude:myproj", res.Samples[0].SessionID)
	}
}

// TestTokenSamples_SkipsUnchangedTranscript pins the incremental
// behavior: a transcript whose mtime is already in ScanInput.Prior is
// reported in Seen but NOT re-parsed (no fresh samples). This is the
// fix for the slow Costs tab.
func TestTokenSamples_SkipsUnchangedTranscript(t *testing.T) {
	home := t.TempDir()
	projDir := filepath.Join(home, ".claude", "projects", "proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	line := `{"type":"assistant","timestamp":"2026-05-15T08:00:01Z","sessionId":"sess","message":{"role":"assistant","model":"m","usage":{"input_tokens":5,"output_tokens":7}}}` + "\n"
	tr := filepath.Join(projDir, "sess.jsonl")
	if err := os.WriteFile(tr, []byte(line), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	p := &Provider{HomeOverride: home}

	// Cold scan parses the file. The session is keyed by the project
	// dir ("proj"), not the file/in-file id.
	cold, err := p.TokenSamples(context.Background(), costs.ScanInput{})
	if err != nil || len(cold.Samples) != 1 {
		t.Fatalf("cold: samples=%d err=%v", len(cold.Samples), err)
	}
	mt, ok := cold.Seen["claude:proj"]
	if !ok {
		t.Fatalf("cold scan should report Seen[claude:proj]; got %v", cold.Seen)
	}

	// Warm scan with the prior mtime must skip parsing (no samples)
	// but still report the session in Seen so it isn't pruned.
	warm, err := p.TokenSamples(context.Background(), costs.ScanInput{Prior: map[string]time.Time{"claude:proj": mt}})
	if err != nil {
		t.Fatalf("warm: %v", err)
	}
	if len(warm.Samples) != 0 {
		t.Errorf("warm scan should skip unchanged file; got %d samples", len(warm.Samples))
	}
	if _, ok := warm.Seen["claude:proj"]; !ok {
		t.Errorf("warm scan must still report the session in Seen; got %v", warm.Seen)
	}

	// Touch the file forward → it must be re-parsed.
	newer := mt.Add(2 * time.Second)
	if err := os.Chtimes(tr, newer, newer); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	changed, err := p.TokenSamples(context.Background(), costs.ScanInput{Prior: map[string]time.Time{"claude:proj": mt}})
	if err != nil {
		t.Fatalf("changed: %v", err)
	}
	if len(changed.Samples) != 1 {
		t.Errorf("a newer transcript must be re-parsed; got %d samples", len(changed.Samples))
	}
}

// TestTokenSamples_MultiFileDirAggregatesUnderDirKey covers a project
// dir holding several transcript files (the real Claude layout — one
// file per session UUID). All of their token usage must aggregate
// under a single dir-keyed session id, and changing any one file must
// re-parse the whole dir.
func TestTokenSamples_MultiFileDirAggregatesUnderDirKey(t *testing.T) {
	home := t.TempDir()
	projDir := filepath.Join(home, ".claude", "projects", "C--dev-klim")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	mk := func(name, sid string, in, out int) string {
		line := `{"type":"assistant","timestamp":"2026-05-15T08:00:01Z","sessionId":"` + sid +
			`","message":{"role":"assistant","model":"m","usage":{"input_tokens":` +
			strconv.Itoa(in) + `,"output_tokens":` + strconv.Itoa(out) + `}}}` + "\n"
		path := filepath.Join(projDir, name)
		if err := os.WriteFile(path, []byte(line), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return path
	}
	mk("11111111.jsonl", "uuid-1", 100, 10)
	f2 := mk("22222222.jsonl", "uuid-2", 200, 20)

	p := &Provider{HomeOverride: home}
	cold, err := p.TokenSamples(context.Background(), costs.ScanInput{})
	if err != nil {
		t.Fatalf("cold: %v", err)
	}
	// Both files aggregate under the single dir-keyed session id.
	if len(cold.Seen) != 1 {
		t.Fatalf("expected 1 session key, got %d: %v", len(cold.Seen), cold.Seen)
	}
	mt, ok := cold.Seen["claude:C--dev-klim"]
	if !ok {
		t.Fatalf("expected Seen[claude:C--dev-klim]; got %v", cold.Seen)
	}
	if len(cold.Samples) != 2 {
		t.Fatalf("expected 2 samples (one per file), got %d", len(cold.Samples))
	}
	for _, s := range cold.Samples {
		if s.SessionID != "claude:C--dev-klim" {
			t.Errorf("sample keyed by %q, want claude:C--dev-klim", s.SessionID)
		}
	}

	// Touching one file re-parses the WHOLE dir (both files).
	newer := mt.Add(2 * time.Second)
	if err := os.Chtimes(f2, newer, newer); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	changed, err := p.TokenSamples(context.Background(), costs.ScanInput{Prior: map[string]time.Time{"claude:C--dev-klim": mt}})
	if err != nil {
		t.Fatalf("changed: %v", err)
	}
	if len(changed.Samples) != 2 {
		t.Errorf("changing one file must re-parse the whole dir (2 samples); got %d", len(changed.Samples))
	}
}

var _ = agents.ProviderClaudeCode
