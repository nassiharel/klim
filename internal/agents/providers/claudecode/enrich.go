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
	Type      string        `json:"type"`
	Timestamp string        `json:"timestamp"`
	SessionID string        `json:"sessionId"`
	Cwd       string        `json:"cwd"`
	GitBranch string        `json:"gitBranch"`
	Message   claudeMessage `json:"message"`
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
	pendingTools := map[string]string{} // toolUseID → name, for matched completion

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
					// Claude treats every assistant tool_use as a
					// fresh start; the matching result arrives in a
					// later user message of type tool_result.
					events = append(events, enrich.TimedEvent{
						Event:     enrich.Event{Kind: enrich.KindToolStarted, Name: c.Name},
						Timestamp: ts,
					})
					pendingTools[c.Name] = c.Name
				}
			}
			if lastText != "" {
				events = append(events, enrich.TimedEvent{
					Event:     enrich.Event{Kind: enrich.KindAssistantMessage, Text: lastText},
					Timestamp: ts,
				})
			}
		default:
			// Skip queue-operation, hook_success, etc. They don't
			// move the state machine.
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
