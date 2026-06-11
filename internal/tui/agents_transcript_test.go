package tui

// Regression tests for renderTranscriptLine against the REAL shapes
// produced by Claude Code and Copilot CLI. The earlier version
// silently dropped most lines because the json.Unmarshal struct
// declared Content as `[]struct{...}` — which fails to parse when
// the JSON has `"content":"plain string"` (the common case for user
// messages typed at the prompt). When Unmarshal fails the renderer
// returns "" and the line vanishes from the viewer.

import (
	"strings"
	"testing"
)

func TestRenderTranscriptLine_RealClaudeUserStringContent(t *testing.T) {
	t.Parallel()
	// Verbatim shape from a real ~/.claude/projects/*.jsonl line:
	// a user message whose `content` field is a plain string.
	raw := []byte(`{"type":"user","message":{"role":"user","content":"improve klim->agents->sessions"},"uuid":"a","timestamp":"2026-06-10T08:54:49.851Z"}`)
	got := renderTranscriptLine(raw)
	if got == "" {
		t.Fatal("renderTranscriptLine returned empty for a real Claude user string-content message — viewer will be blank")
	}
	if !strings.Contains(got, "[user]") {
		t.Errorf("expected [user] prefix, got %q", got)
	}
	if !strings.Contains(got, "improve klim") {
		t.Errorf("expected user text to be rendered, got %q", got)
	}
}

func TestRenderTranscriptLine_RealClaudeAssistantArrayContent(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hello there"}]},"uuid":"b"}`)
	got := renderTranscriptLine(raw)
	if !strings.Contains(got, "[assistant]") || !strings.Contains(got, "hello there") {
		t.Errorf("assistant array-content render wrong: %q", got)
	}
}

func TestRenderTranscriptLine_RealClaudeToolUse(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","id":"x","name":"Bash","input":{"command":"ls -la"}}]},"uuid":"c"}`)
	got := renderTranscriptLine(raw)
	if !strings.Contains(got, "[tool]") || !strings.Contains(got, "Bash") {
		t.Errorf("tool_use render wrong: %q", got)
	}
}

func TestRenderTranscriptLine_SkipsNoiseEvents(t *testing.T) {
	t.Parallel()
	noisy := [][]byte{
		[]byte(`{"type":"last-prompt","leafUuid":"x"}`),
		[]byte(`{"type":"mode","mode":"normal"}`),
		[]byte(`{"type":"permission-mode","permissionMode":"bypass"}`),
		[]byte(`{"type":"summary","summary":"x"}`),
	}
	for _, raw := range noisy {
		if got := renderTranscriptLine(raw); got != "" {
			t.Errorf("noise event leaked: %s → %q", raw, got)
		}
	}
}

// TestRenderTranscriptLine_MalformedJSONFallsThrough covers the
// reviewer-flagged silent-drop bug: when a line looks like JSON
// (starts with `{`) but json.Unmarshal fails — typically a
// partially-written tail line at the end of an in-progress session
// — the old code returned "" and the user saw nothing. The doc
// contract for this function says "lines that don't parse fall
// through unchanged at 4 KiB"; this test pins that contract.
func TestRenderTranscriptLine_MalformedJSONFallsThrough(t *testing.T) {
	t.Parallel()
	// A truncated assistant event — valid JSON shape, invalid
	// because the array is unterminated. Real-world cause: a
	// session in mid-write when the user opened the viewer.
	raw := []byte(`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"unfini`)
	got := renderTranscriptLine(raw)
	if got == "" {
		t.Fatal("malformed JSON line silently dropped — doc says it should fall through unchanged")
	}
	if !strings.Contains(got, "unfini") {
		t.Errorf("expected the raw fragment to surface, got %q", got)
	}
}
