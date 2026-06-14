package claudecode

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nassiharel/klim/internal/agents/enrich"
)

// claudeEvent is the subset of fields we read from each .jsonl line.
// Claude's transcript schema is not formally published; the fields
// below are derived from inspecting real session files. Unknown lines
// (different `type`) are tolerated — only the structure we recognise
// influences the enrichment.
//
// Message.Content is intentionally json.RawMessage: in real Claude
// transcripts it appears in TWO shapes — a plain string
// (`"content":"hi there"`, the dominant case for user-typed messages)
// AND an array of typed blocks (`"content":[{"type":"text",...}]`,
// used for assistant turns and tool calls). Declaring it as
// `[]struct{...}` makes json.Unmarshal fail with a type mismatch on
// every string-form line — the line gets skipped, TurnCount comes
// out short, and the session Title is wrong (because the first user
// message is missed). Decoding manually keeps both shapes.
type claudeEvent struct {
	Type       string           `json:"type"`
	Timestamp  string           `json:"timestamp"`
	SessionID  string           `json:"sessionId"`
	Cwd        string           `json:"cwd"`
	GitBranch  string           `json:"gitBranch"`
	Message    claudeMessage    `json:"message"`
	Attachment claudeAttachment `json:"attachment"`
}

// claudeAttachment is the nested payload Claude uses for hook
// records (top-level `type=="attachment"`). Only the fields we need
// for invocation attribution are declared; the bulky stdout/stderr
// blobs and tool-result content are ignored.
//
// `hookName` already encodes event+slot (e.g. "SessionStart:startup"),
// so the enricher uses it verbatim. Combining with `hookEvent` would
// produce duplicative names like "SessionStart:startup:SessionStart".
type claudeAttachment struct {
	Type     string `json:"type"`     // "hook_success" | "hook_additional_context" | …
	HookName string `json:"hookName"` // e.g. "SessionStart:startup"
}

// claudeMessage carries the polymorphic content field. See
// claudeEvent docstring for why Content is a RawMessage. Defined
// here to keep enrichSessionFromJSONL self-contained; the parallel
// type in search.go (claudeMessageEnvelope) has the same shape but
// belongs to a separate code path.
type claudeMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// jsonlScanResult is what enrichSessionFromJSONL returns to its caller.
// We don't construct an agents.Session here to keep this file free of
// the agents-package import (the provider's Sessions() method does the
// final assembly).
type jsonlScanResult struct {
	SessionID    string
	Cwd          string
	Branch       string
	FirstUserMsg string
	Created      time.Time
	LastSeen     time.Time
	Result       enrich.Result
}

// enrichSessionFromJSONL parses a Claude transcript and derives both
// the persistent and the dashboard-friendly fields.
//
// The function is best-effort: any parse error short-circuits to
// whatever has been read so far, so a malformed tail line doesn't
// invalidate the rest of the data. The result's enrich.Result is the
// outcome of running our generic state machine over the events
// translated from Claude's vocabulary.
func enrichSessionFromJSONL(path string, now time.Time) jsonlScanResult {
	out := jsonlScanResult{}
	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer func() { _ = f.Close() }()

	// We accumulate every event into a slice for the state machine
	// (which needs them in order). Long transcripts can be MBs; the
	// state machine itself is O(N) and a Claude session log of a few
	// hundred turns is well under 50k events, so loading them all is
	// cheap on modern hardware. If profiling shows a regression we
	// can switch to a tail-only pass (see enrich.ReadTail) — but the
	// trade-off is losing accurate Created / TurnCount / RestartCmd
	// derivation, which the user values more than a few ms of scan time.
	scanner := bufio.NewScanner(f)
	// Claude transcripts contain very long lines (large hook outputs,
	// pasted code). Bump the buffer to 8 MiB.
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	var events []enrich.TimedEvent

	for scanner.Scan() {
		var ev claudeEvent
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			// Skip malformed lines but keep walking — partial data
			// is better than nothing.
			continue
		}
		ts := parseClaudeTime(ev.Timestamp)

		// Top-level metadata we want regardless of type.
		if out.SessionID == "" && ev.SessionID != "" {
			out.SessionID = ev.SessionID
		}
		if out.Cwd == "" && ev.Cwd != "" {
			out.Cwd = ev.Cwd
		}
		if out.Branch == "" && ev.GitBranch != "" && ev.GitBranch != "HEAD" {
			out.Branch = ev.GitBranch
		}
		if !ts.IsZero() {
			if out.Created.IsZero() {
				out.Created = ts
			}
			out.LastSeen = ts
		}

		// Translate Claude's event vocabulary into our generic shape.
		switch ev.Type {
		case "user":
			events = append(events, enrich.TimedEvent{
				Event:     enrich.Event{Kind: enrich.KindUserMessage},
				Timestamp: ts,
			})
			// Capture the first user text for the session title.
			// Walk both polymorphic content shapes; whichever has
			// the text wins.
			if out.FirstUserMsg == "" {
				if text := firstUserText(ev.Message.Content); text != "" && !looksLikeIDEAttachment(text) {
					out.FirstUserMsg = enrich.TruncateOneLine(text, 80)
				}
			}
			// Slash-command extraction: only the STRING form of
			// user content carries the `<command-name>/foo</command-name>`
			// marker that represents an actual user-initiated
			// slash command. The same characters can appear inside
			// the text of a tool_result block (e.g. a sub-agent
			// report quoting the marker shape) — those are array-
			// form content and MUST NOT count, or every transcript
			// that discusses the marker would falsely show "used".
			// Verified empirically against real .claude transcripts
			// before this code was written.
			if cmd := extractSlashCommandFromStringContent(ev.Message.Content); cmd != "" {
				events = append(events, enrich.TimedEvent{
					Event:     enrich.Event{Kind: enrich.KindSlashCommand, Name: cmd},
					Timestamp: ts,
				})
			}
		case "assistant":
			// Collect text + tool_use children. Assistant messages
			// only use the array form in practice, but the helper
			// gracefully returns "" if a future schema change emits
			// a plain string.
			var lastText string
			for _, c := range decodeContentBlocks(ev.Message.Content) {
				switch c.Type {
				case "text":
					if c.Text != "" {
						lastText = c.Text
					}
				case "tool_use":
					// Classify tool_use into one of the four
					// invocation kinds Claude expresses through
					// the tool surface: explicit Skill calls,
					// Agent/Task sub-agent dispatches, MCP tool
					// invocations (name pattern mcp__server__tool),
					// or a plain anonymous tool that the existing
					// state machine already counts via ToolCounts.
					// Verified empirically against real Claude
					// transcripts at C:/Users/nassiharel/.claude/projects/
					// before this code was written.
					switch c.Name {
					case "Skill":
						// Dual-emit: KindToolStarted preserves the
						// pre-Invocations dashboard aggregation
						// (sessionstui totalTools, CLI per-session
						// tool histogram), and KindSkillInvoked
						// powers the new per-skill row. The
						// Invocations feature is ADDITIONAL signal
						// on top of ToolCounts, not a replacement.
						emit := []enrich.TimedEvent{{
							Event:     enrich.Event{Kind: enrich.KindToolStarted, Name: c.Name},
							Timestamp: ts,
						}}
						if skill := extractToolInputString(c.Input, "skill"); skill != "" {
							emit = append(emit, enrich.TimedEvent{
								Event:     enrich.Event{Kind: enrich.KindSkillInvoked, Name: skill},
								Timestamp: ts,
							})
						}
						events = append(events, emit...)
					case "Agent", "Task":
						// Same dual-emit rationale as Skill above.
						emit := []enrich.TimedEvent{{
							Event:     enrich.Event{Kind: enrich.KindToolStarted, Name: c.Name},
							Timestamp: ts,
						}}
						if sub := extractToolInputString(c.Input, "subagent_type"); sub != "" {
							emit = append(emit, enrich.TimedEvent{
								Event:     enrich.Event{Kind: enrich.KindSubagentStarted, Name: sub},
								Timestamp: ts,
							})
						}
						events = append(events, emit...)
					default:
						if server, tool, ok := splitMCPToolName(c.Name); ok {
							// MCP tool calls ALSO count as
							// regular tool calls — the existing
							// dashboard already shows them in
							// ToolCounts as "mcp__foo__bar". Emit
							// both in one append so the per-tool
							// histograms in MCP and Tools rows
							// both work.
							events = append(events,
								enrich.TimedEvent{
									Event:     enrich.Event{Kind: enrich.KindMCPToolCall, Name: server + "::" + tool},
									Timestamp: ts,
								},
								enrich.TimedEvent{
									Event:     enrich.Event{Kind: enrich.KindToolStarted, Name: c.Name},
									Timestamp: ts,
								},
							)
						} else {
							events = append(events, enrich.TimedEvent{
								Event:     enrich.Event{Kind: enrich.KindToolStarted, Name: c.Name},
								Timestamp: ts,
							})
						}
					}
				}
			}
			if lastText != "" {
				events = append(events, enrich.TimedEvent{
					Event:     enrich.Event{Kind: enrich.KindAssistantMessage, Text: lastText},
					Timestamp: ts,
				})
			}
		case "attachment":
			// Hook records live at the top level as
			// type=="attachment" with the hook fields nested under
			// `attachment`. Any attachment whose nested type
			// starts with `hook_` carries a hook firing; this
			// covers today's `hook_success` / `hook_additional_context`
			// and any future `hook_failure` / `hook_blocked` /
			// similar without code changes. Other attachment types
			// (skill_listing, task_reminder, command_permissions,
			// file-history-snapshot, …) are ignored because they
			// represent passive session state, not an invocation.
			// Verified empirically against real transcripts.
			if ev.Attachment.HookName != "" &&
				strings.HasPrefix(ev.Attachment.Type, "hook_") {
				events = append(events, enrich.TimedEvent{
					Event:     enrich.Event{Kind: enrich.KindHookFired, Name: ev.Attachment.HookName},
					Timestamp: ts,
				})
			}
		default:
			// Skip queue-operation, etc. They don't move the
			// state machine.
		}
	}
	if err := scanner.Err(); err != nil {
		// Truncated tail is fine — we keep partial data.
		_ = err
	}

	// We don't have explicit tool_completed events in Claude's schema
	// (results are interleaved in user messages with unstructured
	// payloads). For state-machine purposes assume every tool call
	// completed except the very last one if it was a tool_use — this
	// matches the common case where the user closes Claude mid-tool.
	// We emit synthetic completions for all but the last.
	startedIdx := []int{}
	for i, e := range events {
		if e.Kind == enrich.KindToolStarted {
			startedIdx = append(startedIdx, i)
		}
	}
	if len(startedIdx) > 1 {
		// Insert synthetic completions after each started except the last.
		// To keep things simple, append them all to the end with the
		// same timestamp as the last event minus 1µs. The state
		// machine only cares about the running count.
		end := events[len(events)-1].Timestamp.Add(-time.Microsecond)
		for i := 0; i < len(startedIdx)-1; i++ {
			events = append(events, enrich.TimedEvent{
				Event:     enrich.Event{Kind: enrich.KindToolCompleted},
				Timestamp: end,
			})
		}
		// Resort by timestamp so the state machine sees them in order.
		sort.SliceStable(events, func(i, j int) bool {
			return events[i].Timestamp.Before(events[j].Timestamp)
		})
	}

	out.Result = enrich.DeriveState(events, now)
	return out
}

// firstUserText returns the first non-empty text of a Claude user
// message's polymorphic content. Real transcripts use a string for
// most user-typed messages and an array of typed blocks for IDE-
// attached payloads; this helper transparently handles both so the
// derived session Title doesn't depend on which shape Claude chose.
// Returns "" when no text can be extracted.
func firstUserText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// String form (`"content":"plain text from the user"`).
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return s
		}
		return ""
	}
	// Array form: walk blocks for the first text.
	if raw[0] == '[' {
		for _, c := range decodeContentBlocks(raw) {
			if c.Type == "text" && c.Text != "" {
				return c.Text
			}
		}
	}
	return ""
}

// decodeContentBlocks decodes the array-form Claude message content.
// Returns nil for string-form content or any decode error — the
// caller is expected to fall back to firstUserText when looking up
// text-only payloads.
func decodeContentBlocks(raw json.RawMessage) []claudeContentBlock {
	if len(raw) == 0 || raw[0] != '[' {
		return nil
	}
	var blocks []claudeContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil
	}
	return blocks
}

// parseClaudeTime parses Claude's RFC3339 / RFC3339Nano timestamps,
// returning the zero time on failure.
func parseClaudeTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}

// looksLikeIDEAttachment skips the noise that VSCode / Cursor inject
// at the head of user messages (the `<ide_opened_file>` block) so the
// derived title is the user's actual question.
func looksLikeIDEAttachment(s string) bool {
	t := strings.TrimSpace(s)
	return strings.HasPrefix(t, "<ide_") ||
		strings.HasPrefix(t, "<system-reminder>") ||
		strings.HasPrefix(t, "<command-")
}

// slashCommandOpen / slashCommandClose are the literal marker tags
// Claude wraps slash-command invocations in at the START of a
// string-form user message: `<command-name>/foo</command-name>`.
// Real transcripts always place the marker at offset 0 of the
// content string (verified empirically against on-disk transcripts);
// mid-text occurrences are quotations and MUST NOT be counted.
const (
	slashCommandOpen  = "<command-name>/"
	slashCommandClose = "</command-name>"
)

// extractSlashCommandFromStringContent returns the slash-command name
// (e.g. "/exit") from a user message whose `content` is the STRING
// form STARTING WITH the `<command-name>/` marker. Returns "" when
// either:
//
//   - the content is the array form (a tool_result whose text payload
//     happens to QUOTE the marker shape is NOT a real slash-command
//     invocation; counting it would surface every transcript that
//     documents the format as having used the command);
//   - the marker exists but doesn't begin at offset 0 (mid-text
//     quote, e.g. "what does <command-name>/inject</command-name>
//     do?" — same false-positive class as the array-form quote,
//     pinned by TestEnrichSessionFromJSONL_SlashCommandFalsePositiveFromQuotedText).
//
// Honest-signal contract: this is the only path that may emit
// KindSlashCommand for the claudecode provider.
func extractSlashCommandFromStringContent(raw json.RawMessage) string {
	if len(raw) == 0 || raw[0] != '"' {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	if !strings.HasPrefix(s, slashCommandOpen) {
		return ""
	}
	rest := s[len(slashCommandOpen)-1:] // keep the leading '/'
	end := strings.Index(rest, slashCommandClose)
	if end <= 1 {
		// No close tag, or only the '/' before the close tag.
		return ""
	}
	return rest[:end]
}

// extractToolInputString returns the string value of `key` from a
// tool_use's `input` payload. Returns "" when the key is missing, not
// a string, or the payload doesn't decode. Used to pull `skill` out of
// a Skill tool_use and `subagent_type` out of an Agent/Task tool_use
// without binding a separate struct per tool.
func extractToolInputString(raw json.RawMessage, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	v, ok := m[key]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return ""
	}
	return s
}

// splitMCPToolName splits a Claude MCP tool name of the form
// `mcp__<server>__<tool>` into its server and tool parts. The tool
// portion is allowed to contain further `__` sequences and is
// returned as-is (e.g. `mcp__ado-tools__repo_pull_request_thread_write`
// → "ado-tools", "repo_pull_request_thread_write"). Returns
// `("", "", false)` for any name that doesn't match the expected
// prefix structure. Verified empirically against real transcripts.
func splitMCPToolName(name string) (server, tool string, ok bool) {
	const prefix = "mcp__"
	if !strings.HasPrefix(name, prefix) {
		return "", "", false
	}
	rest := name[len(prefix):]
	idx := strings.Index(rest, "__")
	if idx <= 0 || idx == len(rest)-2 {
		// No tool suffix, or empty server / empty tool.
		return "", "", false
	}
	return rest[:idx], rest[idx+2:], true
}

// latestTranscript returns the path to the most recently modified
// `.jsonl` file in `projectDir`, or the empty string when none exist.
func latestTranscript(projectDir string) string {
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return ""
	}
	var newest string
	var newestTime time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		if newest == "" || fi.ModTime().After(newestTime) {
			newest = filepath.Join(projectDir, e.Name())
			newestTime = fi.ModTime()
		}
	}
	return newest
}
