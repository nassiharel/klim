package claudecode

// Regression test for the polymorphic message.content bug in
// enrichSessionFromJSONL. Real Claude transcripts emit user
// messages as a plain string (`"content":"hi there"`) AND as an
// array of typed blocks (`"content":[{"type":"text",...}]`). The
// original code declared Content as `[]struct{...}`, so any user
// line with string content failed json.Unmarshal and was silently
// dropped — taking TurnCount, first-user-message Title, and other
// enrichment with it. Same bug shape as the TUI's
// renderTranscriptLine, but here it affects the persisted
// session record, not just the viewer.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnrichSessionFromJSONL_HandlesPolymorphicUserContent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "polymorphic.jsonl")

	// Two user messages: first uses the string form (the dominant
	// real-world case for user-typed prompts), second uses the
	// array form (Claude's "block" content for IDE-attached or
	// programmatic payloads). Both must be visible in the result.
	lines := []string{
		`{"type":"user","sessionId":"abc","cwd":"/dev/klim","gitBranch":"main","timestamp":"2026-06-10T08:54:49.851Z","message":{"role":"user","content":"please refactor the tile renderer"}}`,
		`{"type":"assistant","sessionId":"abc","timestamp":"2026-06-10T08:54:55.000Z","message":{"role":"assistant","content":[{"type":"text","text":"on it"}]}}`,
		`{"type":"user","sessionId":"abc","timestamp":"2026-06-10T08:55:00.000Z","message":{"role":"user","content":[{"type":"text","text":"second turn"}]}}`,
	}
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	got := enrichSessionFromJSONL(path, now)

	if got.SessionID != "abc" {
		t.Errorf("SessionID = %q, want %q", got.SessionID, "abc")
	}
	if got.Cwd != "/dev/klim" {
		t.Errorf("Cwd = %q, want %q", got.Cwd, "/dev/klim")
	}
	if got.Branch != "main" {
		t.Errorf("Branch = %q, want %q", got.Branch, "main")
	}
	// FirstUserMsg must come from the STRING-form first user
	// message — that's the regression: pre-fix it was empty
	// because the line was dropped.
	if got.FirstUserMsg != "please refactor the tile renderer" {
		t.Errorf("FirstUserMsg = %q, want %q — string-form user content was dropped",
			got.FirstUserMsg, "please refactor the tile renderer")
	}
	if got.Created.IsZero() {
		t.Error("Created not set — first event timestamp should populate it")
	}
}

func TestEnrichSessionFromJSONL_AssistantArrayContentStillWorks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "assistant.jsonl")
	body := `{"type":"assistant","sessionId":"x","timestamp":"2026-06-10T08:00:00.000Z","message":{"role":"assistant","content":[{"type":"text","text":"hi"},{"type":"tool_use","name":"Bash"}]}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got := enrichSessionFromJSONL(path, time.Now())
	if got.SessionID != "x" {
		t.Errorf("SessionID = %q, want x", got.SessionID)
	}
}

// TestEnrichSessionFromJSONL_Invocations covers the per-kind
// invocation extraction from a Claude transcript. Fixture shapes
// mirror real on-disk transcripts (verified at
// C:/Users/nassiharel/.claude/projects/ before this code was
// written — see the plan file's "verified signal inventory" table):
//
//   - Skill: tool_use with `name=="Skill"`, `input.skill` = id
//   - Sub-agent: tool_use with `name=="Agent"`, `input.subagent_type` = type
//   - MCP tool: tool_use with `name="mcp__<server>__<tool>"`
//   - Hook: top-level `type=="attachment"`, nested
//     `attachment.hookName` (already encodes event+slot)
//   - Slash command: STRING-form user content beginning with
//     `<command-name>/...</command-name>` (array-form content with
//     the literal string inside a tool_result must NOT match — that
//     would surface every transcript that QUOTES a slash-command as
//     having used one)
func TestEnrichSessionFromJSONL_Invocations(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "invocations.jsonl")

	lines := []string{
		// Real Skill tool_use shape.
		`{"type":"assistant","sessionId":"abc","timestamp":"2026-06-11T15:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"Skill","input":{"skill":"superpowers:systematic-debugging"}}]}}`,
		// Two more invocations of the same skill.
		`{"type":"assistant","sessionId":"abc","timestamp":"2026-06-11T15:00:01Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"Skill","input":{"skill":"superpowers:systematic-debugging"}}]}}`,
		`{"type":"assistant","sessionId":"abc","timestamp":"2026-06-11T15:00:02Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"Skill","input":{"skill":"superpowers:brainstorming"}}]}}`,
		// Sub-agent dispatch — name="Agent" with input.subagent_type.
		`{"type":"assistant","sessionId":"abc","timestamp":"2026-06-11T15:00:03Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"Agent","input":{"subagent_type":"Explore"}}]}}`,
		`{"type":"assistant","sessionId":"abc","timestamp":"2026-06-11T15:00:04Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"Agent","input":{"subagent_type":"Explore"}}]}}`,
		// MCP tool — name pattern mcp__<server>__<tool>.
		`{"type":"assistant","sessionId":"abc","timestamp":"2026-06-11T15:00:05Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"mcp__ado-tools__repo_pull_request","input":{}}]}}`,
		// Hook attachment record — top-level type=="attachment".
		`{"type":"attachment","sessionId":"abc","timestamp":"2026-06-11T15:00:06Z","attachment":{"type":"hook_success","hookName":"SessionStart:startup","hookEvent":"SessionStart"}}`,
		`{"type":"attachment","sessionId":"abc","timestamp":"2026-06-11T15:00:07Z","attachment":{"type":"hook_additional_context","hookName":"SessionStart:startup","hookEvent":"SessionStart"}}`,
		// Real slash-command shape — STRING-form user content
		// starting with <command-name>/.
		`{"type":"user","sessionId":"abc","timestamp":"2026-06-11T15:00:08Z","message":{"role":"user","content":"<command-name>/exit</command-name>\n<command-message>exit</command-message>\n<command-args></command-args>"}}`,
		// FALSE-POSITIVE GUARD: a tool_result whose text payload
		// happens to QUOTE the slash-command marker (e.g. an Explore
		// agent report that documents the format). This is an
		// array-form user message, not a string. MUST NOT count as
		// /transcribe being used.
		`{"type":"user","sessionId":"abc","timestamp":"2026-06-11T15:00:09Z","message":{"role":"user","content":[{"tool_use_id":"x","type":"tool_result","content":[{"type":"text","text":"quoted example: <command-name>/transcribe</command-name>"}]}]}}`,
	}
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got := enrichSessionFromJSONL(path, time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC))

	if got.Result.Invocations.Skills["superpowers:systematic-debugging"] != 2 {
		t.Errorf("Skills[systematic-debugging] = %d, want 2 (got map = %+v)",
			got.Result.Invocations.Skills["superpowers:systematic-debugging"],
			got.Result.Invocations.Skills)
	}
	if got.Result.Invocations.Skills["superpowers:brainstorming"] != 1 {
		t.Errorf("Skills[brainstorming] = %d, want 1", got.Result.Invocations.Skills["superpowers:brainstorming"])
	}
	if got.Result.Invocations.Subagents["Explore"] != 2 {
		t.Errorf("Subagents[Explore] = %d, want 2 (got map = %+v)",
			got.Result.Invocations.Subagents["Explore"],
			got.Result.Invocations.Subagents)
	}
	if got.Result.Invocations.MCPTools["ado-tools::repo_pull_request"] != 1 {
		t.Errorf("MCPTools[ado-tools::repo_pull_request] = %d, want 1 (got map = %+v)",
			got.Result.Invocations.MCPTools["ado-tools::repo_pull_request"],
			got.Result.Invocations.MCPTools)
	}
	if got.Result.Invocations.Hooks["SessionStart:startup"] != 2 {
		t.Errorf("Hooks[SessionStart:startup] = %d, want 2 (got map = %+v)",
			got.Result.Invocations.Hooks["SessionStart:startup"],
			got.Result.Invocations.Hooks)
	}
	if got.Result.Invocations.SlashCommands["/exit"] != 1 {
		t.Errorf("SlashCommands[/exit] = %d, want 1 (got map = %+v)",
			got.Result.Invocations.SlashCommands["/exit"],
			got.Result.Invocations.SlashCommands)
	}
	// FALSE-POSITIVE GUARD: the array-form quoted-marker line must
	// NOT be counted. If this fires the regex is too eager.
	if _, leaked := got.Result.Invocations.SlashCommands["/transcribe"]; leaked {
		t.Errorf("SlashCommands[/transcribe] leaked from quoted tool_result text; "+
			"slash-command regex must only run on string-form user content. Got map = %+v",
			got.Result.Invocations.SlashCommands)
	}
}

// TestEnrichSessionFromJSONL_NamedKindsAlsoIncrementToolCounts pins
// the dual-emit contract: Skill, Agent/Task, and MCP tool_use blocks
// MUST also bump ToolCounts under their tool name (e.g. "Skill",
// "Agent", "mcp__server__tool"). Pre-Invocations behavior had every
// tool_use increment ToolCounts; the Invocations feature added the
// per-kind maps as ADDITIONAL signal, not a replacement. Dropping
// the ToolCounts increment would silently undercount in:
//
//   - sessionstui/dashboard.go totalTools aggregator
//   - cli/agents_sessions.go per-session "tools" histogram
//   - any downstream sum-of-ToolCounts metric
//
// Verifies both Claude's Skill/Agent forms and the MCP dual-emit.
func TestEnrichSessionFromJSONL_NamedKindsAlsoIncrementToolCounts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "named.jsonl")
	body := strings.Join([]string{
		`{"type":"assistant","sessionId":"z","timestamp":"2026-06-11T15:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"Skill","input":{"skill":"superpowers:tdd"}}]}}`,
		`{"type":"assistant","sessionId":"z","timestamp":"2026-06-11T15:00:01Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"Agent","input":{"subagent_type":"Explore"}}]}}`,
		`{"type":"assistant","sessionId":"z","timestamp":"2026-06-11T15:00:02Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"Task","input":{"subagent_type":"general-purpose"}}]}}`,
		`{"type":"assistant","sessionId":"z","timestamp":"2026-06-11T15:00:03Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"mcp__ado-tools__repo_pull_request","input":{}}]}}`,
		`{"type":"assistant","sessionId":"z","timestamp":"2026-06-11T15:00:04Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := enrichSessionFromJSONL(path, time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC))

	// Each named-kind tool_use must ALSO appear in ToolCounts
	// under its tool name. The Invocations maps are additional
	// signal on top of, not a replacement for, the existing
	// per-tool histogram.
	for _, want := range []string{"Skill", "Agent", "Task", "mcp__ado-tools__repo_pull_request", "Bash"} {
		if got.Result.ToolCounts[want] != 1 {
			t.Errorf("ToolCounts[%q] = %d, want 1 (named kinds must dual-emit so the existing dashboard aggregator doesn't undercount)",
				want, got.Result.ToolCounts[want])
		}
	}
	// And the per-kind maps still populate.
	if got.Result.Invocations.Skills["superpowers:tdd"] != 1 {
		t.Errorf("Skills[superpowers:tdd] missing: %+v", got.Result.Invocations.Skills)
	}
	if got.Result.Invocations.Subagents["Explore"] != 1 {
		t.Errorf("Subagents[Explore] missing: %+v", got.Result.Invocations.Subagents)
	}
	if got.Result.Invocations.Subagents["general-purpose"] != 1 {
		t.Errorf("Subagents[general-purpose] missing: %+v", got.Result.Invocations.Subagents)
	}
	if got.Result.Invocations.MCPTools["ado-tools::repo_pull_request"] != 1 {
		t.Errorf("MCPTools[ado-tools::repo_pull_request] missing: %+v", got.Result.Invocations.MCPTools)
	}
}

// TestEnrichSessionFromJSONL_SlashCommandFalsePositiveFromQuotedText
// pins the false-positive guard for the SECOND quoting case:
// a STRING-form user message that mentions the marker in prose
// (not as the leading invocation), e.g. "what does
// <command-name>/inject</command-name> do?". The marker is only a
// genuine slash command when it appears at the START of the user's
// content string — real Claude transcripts always emit it at offset
// 0 of the content string (verified empirically). Mid-text quotes
// must NOT be counted, otherwise any meta-discussion of slash
// commands corrupts the SlashCommands counts.
//
// Existing TestEnrichSessionFromJSONL_Invocations covers the
// ARRAY-form quote case (tool_result text); this test covers the
// STRING-form mid-text case which is a separate attack surface.
func TestEnrichSessionFromJSONL_SlashCommandFalsePositiveFromQuotedText(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "quoted.jsonl")
	lines := []string{
		// User asks about the marker mid-sentence (string form).
		// MUST NOT count as having invoked /inject.
		`{"type":"user","sessionId":"q","timestamp":"2026-06-11T15:00:00Z","message":{"role":"user","content":"explain what <command-name>/inject</command-name> does"}}`,
		// A genuine slash invocation right after — MUST count.
		`{"type":"user","sessionId":"q","timestamp":"2026-06-11T15:00:01Z","message":{"role":"user","content":"<command-name>/exit</command-name>\n<command-message>exit</command-message>"}}`,
	}
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := enrichSessionFromJSONL(path, time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC))

	if _, leaked := got.Result.Invocations.SlashCommands["/inject"]; leaked {
		t.Errorf("SlashCommands[/inject] leaked from a mid-text mention; the marker must only be "+
			"recognised at the START of the user content string. Got map = %+v",
			got.Result.Invocations.SlashCommands)
	}
	if got.Result.Invocations.SlashCommands["/exit"] != 1 {
		t.Errorf("SlashCommands[/exit] = %d, want 1 (genuine leading marker)",
			got.Result.Invocations.SlashCommands["/exit"])
	}
}

// TestEnrichSessionFromJSONL_HookPrefixAcceptsAllHookTypes pins the
// hook attachment filter to use a `hook_` prefix match instead of a
// strict allow-list. Real Claude 2.x transcripts emit two types
// today (`hook_success`, `hook_additional_context`), but a future
// release adding e.g. `hook_failure` (a natural feature for hooks
// that can already block tool use, per the SessionStart and
// PostToolUse hook contracts) would be silently invisible under the
// strict allow-list. Prefix matching makes the schema-drift surface
// honest: any hook_*-typed attachment with a non-empty HookName
// counts.
func TestEnrichSessionFromJSONL_HookPrefixAcceptsAllHookTypes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "hooks.jsonl")
	lines := []string{
		// All three should count (the two real types + a
		// hypothetical future type).
		`{"type":"attachment","sessionId":"h","timestamp":"2026-06-11T15:00:00Z","attachment":{"type":"hook_success","hookName":"SessionStart:startup"}}`,
		`{"type":"attachment","sessionId":"h","timestamp":"2026-06-11T15:00:01Z","attachment":{"type":"hook_additional_context","hookName":"SessionStart:startup"}}`,
		`{"type":"attachment","sessionId":"h","timestamp":"2026-06-11T15:00:02Z","attachment":{"type":"hook_failure","hookName":"PostToolUse:format"}}`,
		// Non-hook attachment types must still be ignored — the
		// `hook_` prefix is the discriminator.
		`{"type":"attachment","sessionId":"h","timestamp":"2026-06-11T15:00:03Z","attachment":{"type":"skill_listing","hookName":"NotReallyAHook"}}`,
		`{"type":"attachment","sessionId":"h","timestamp":"2026-06-11T15:00:04Z","attachment":{"type":"file-history-snapshot","hookName":"AlsoNotAHook"}}`,
	}
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := enrichSessionFromJSONL(path, time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC))

	if got.Result.Invocations.Hooks["SessionStart:startup"] != 2 {
		t.Errorf("Hooks[SessionStart:startup] = %d, want 2 (hook_success + hook_additional_context)",
			got.Result.Invocations.Hooks["SessionStart:startup"])
	}
	if got.Result.Invocations.Hooks["PostToolUse:format"] != 1 {
		t.Errorf("Hooks[PostToolUse:format] = %d, want 1 (hook_failure must count under the hook_ prefix policy)",
			got.Result.Invocations.Hooks["PostToolUse:format"])
	}
	if _, leaked := got.Result.Invocations.Hooks["NotReallyAHook"]; leaked {
		t.Errorf("Hooks[NotReallyAHook] leaked from skill_listing (no hook_ prefix); got map = %+v",
			got.Result.Invocations.Hooks)
	}
	if _, leaked := got.Result.Invocations.Hooks["AlsoNotAHook"]; leaked {
		t.Errorf("Hooks[AlsoNotAHook] leaked from file-history-snapshot (no hook_ prefix); got map = %+v",
			got.Result.Invocations.Hooks)
	}
}

// TestEnrichSessionFromJSONL_AnonymousToolsDontDoubleCountAsMCPOrSkill
// pins that a regular `Bash` tool_use does NOT slip into any of the
// per-kind invocation maps. The dashboard's "tools" row comes from
// ToolCounts; Skills/Subagents/MCPTools should each stay empty.
func TestEnrichSessionFromJSONL_AnonymousToolsDontDoubleCountAsMCPOrSkill(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "anon.jsonl")
	body := `{"type":"assistant","sessionId":"y","timestamp":"2026-06-11T15:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := enrichSessionFromJSONL(path, time.Date(2026, 6, 11, 16, 0, 0, 0, time.UTC))

	if got.Result.ToolCounts["Bash"] != 1 {
		t.Errorf("ToolCounts[Bash] = %d, want 1 (existing behaviour)", got.Result.ToolCounts["Bash"])
	}
	if len(got.Result.Invocations.Skills) != 0 {
		t.Errorf("plain Bash tool_use leaked into Skills: %+v", got.Result.Invocations.Skills)
	}
	if len(got.Result.Invocations.Subagents) != 0 {
		t.Errorf("plain Bash tool_use leaked into Subagents: %+v", got.Result.Invocations.Subagents)
	}
	if len(got.Result.Invocations.MCPTools) != 0 {
		t.Errorf("plain Bash tool_use leaked into MCPTools: %+v", got.Result.Invocations.MCPTools)
	}
}
