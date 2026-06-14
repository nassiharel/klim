package claudecode

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nassiharel/klim/internal/agents/search"
)

// claudeMessageBlock captures the parts of a Claude transcript line we
// can render as searchable text. Message.Content may be a plain
// string OR an array of typed content blocks {type, text}, depending
// on which CLI version produced the transcript — both shapes are
// handled.
type claudeMessageBlock struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	SessionID string          `json:"sessionId"`
	Message   json.RawMessage `json:"message"`
}

type claudeMessageEnvelope struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type claudeContentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`  // tool_use.name; only present for tool_use blocks
	Input json.RawMessage `json:"input"` // tool_use.input; opaque payload, parsed lazily by callers that need specific fields like .skill / .subagent_type
}

// SessionTexts walks every `~/.claude/projects/<encoded>/*.jsonl` and
// returns one SessionText per session ID. Missing dirs are not an
// error; malformed lines are tolerated and skipped.
func (p *Provider) SessionTexts(ctx context.Context) ([]search.SessionText, error) {
	root := filepath.Join(p.claudeDir(), "projects")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil
	}
	// Group lines by session ID; one project dir may contain multiple
	// sessions across multiple files.
	bySession := map[string]*search.SessionText{}
	mtimeBySession := map[string]time.Time{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(d.Name()), ".jsonl") {
				return nil
			}
			info, err := d.Info()
			if err == nil {
				if t := info.ModTime(); t.After(mtimeBySession["__current__"]) {
					mtimeBySession["__current__"] = t
				}
			}
			parseClaudeSessionLines(path, e.Name(), bySession, mtimeBySession)
			return nil
		})
	}
	out := make([]search.SessionText, 0, len(bySession))
	for _, s := range bySession {
		out = append(out, *s)
	}
	return out, nil
}

func parseClaudeSessionLines(path, projectName string, bySession map[string]*search.SessionText, mtimes map[string]time.Time) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	info, _ := f.Stat()
	br := bufio.NewReaderSize(f, 64*1024)
	lineNo := 0
	for {
		line, readErr := br.ReadBytes('\n')
		lineNo++
		if len(line) > 0 {
			var rec claudeMessageBlock
			if json.Unmarshal(line, &rec) == nil {
				if text, role, ok := claudeExtractMessage(rec); ok {
					sessionID := rec.SessionID
					if sessionID == "" {
						sessionID = projectName
					}
					id := "claude:" + sessionID
					s, exists := bySession[id]
					if !exists {
						s = &search.SessionText{
							SessionID:      id,
							Provider:       "claude-code",
							Title:          projectName,
							TranscriptPath: path,
						}
						if info != nil {
							s.TranscriptMtime = info.ModTime()
						}
						bySession[id] = s
					}
					if info != nil && info.ModTime().After(s.TranscriptMtime) {
						s.TranscriptMtime = info.ModTime()
					}
					ts, _ := time.Parse(time.RFC3339, rec.Timestamp)
					s.Lines = append(s.Lines, search.SessionLine{
						Role:      role,
						Text:      text,
						Timestamp: ts,
						LineNo:    lineNo,
					})
				}
			}
		}
		if readErr != nil {
			break
		}
	}
}

// claudeExtractMessage pulls (text, role, ok) out of one transcript
// record. Returns ok=false for non-message events (tool_use, system
// announcements, etc.) so the indexer skips them.
func claudeExtractMessage(rec claudeMessageBlock) (string, string, bool) {
	if rec.Type == "" {
		return "", "", false
	}
	var env claudeMessageEnvelope
	if len(rec.Message) == 0 || json.Unmarshal(rec.Message, &env) != nil {
		return "", "", false
	}
	role := env.Role
	if role == "" {
		// `type` carries the role on some lines (user/assistant/system).
		role = rec.Type
	}
	if role != "user" && role != "assistant" && role != "system" {
		return "", "", false
	}
	// Content may be a string or an array of blocks. Try string first.
	if len(env.Content) > 0 && env.Content[0] == '"' {
		var s string
		if json.Unmarshal(env.Content, &s) == nil {
			return strings.TrimSpace(s), role, s != ""
		}
	}
	// Try array of blocks.
	var blocks []claudeContentBlock
	if json.Unmarshal(env.Content, &blocks) == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" && b.Text != "" {
				parts = append(parts, b.Text)
			}
		}
		joined := strings.TrimSpace(strings.Join(parts, " "))
		return joined, role, joined != ""
	}
	return "", "", false
}
