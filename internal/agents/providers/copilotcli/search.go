package copilotcli

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

// copilotMessageEvent is the subset of an events.jsonl record we
// need for search. type tells us which event we have; data carries
// the text. Field names cover both the camelCase and snake_case
// shapes seen across CLI versions.
type copilotMessageEvent struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Data      struct {
		SessionID string `json:"sessionId"`
		Title     string `json:"title"`
		Role      string `json:"role"`
		Text      string `json:"text"`
		Content   string `json:"content"`
		Message   string `json:"message"`
		Prompt    string `json:"prompt"`
		Response  string `json:"response"`
	} `json:"data"`
}

// SessionTexts walks every Copilot session directory (see
// [Provider.sessionDirs] for the on-disk layouts honored) and returns
// one SessionText per session whose events.jsonl contains at least one
// extractable message.
func (p *Provider) SessionTexts(ctx context.Context) ([]search.SessionText, error) {
	var out []search.SessionText
	for _, d := range p.sessionDirs() {
		path := filepath.Join(d.Path, "events.jsonl")
		if st, ok := parseCopilotSessionText(path, d.ID); ok {
			out = append(out, st)
		}
	}
	return out, nil
}

func parseCopilotSessionText(path, sessionDir string) (search.SessionText, bool) {
	f, err := os.Open(path)
	if err != nil {
		return search.SessionText{}, false
	}
	defer func() { _ = f.Close() }()
	info, _ := f.Stat()
	br := bufio.NewReaderSize(f, 64*1024)
	st := search.SessionText{
		SessionID:      "copilot:" + sessionDir,
		Provider:       "copilot-cli",
		TranscriptPath: path,
	}
	if info != nil {
		st.TranscriptMtime = info.ModTime()
	}
	lineNo := 0
	for {
		line, readErr := br.ReadBytes('\n')
		lineNo++
		if len(line) > 0 {
			var ev copilotMessageEvent
			if json.Unmarshal(line, &ev) == nil {
				if st.Title == "" && ev.Data.Title != "" {
					st.Title = ev.Data.Title
				}
				if text, role, ok := copilotExtractMessage(ev); ok {
					ts, _ := time.Parse(time.RFC3339, ev.Timestamp)
					st.Lines = append(st.Lines, search.SessionLine{
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
	if st.Title == "" {
		st.Title = sessionDir
	}
	return st, len(st.Lines) > 0
}

func copilotExtractMessage(ev copilotMessageEvent) (string, string, bool) {
	role := strings.ToLower(ev.Data.Role)
	switch {
	case strings.Contains(ev.Type, "user"):
		role = "user"
	case strings.Contains(ev.Type, "assistant"), strings.Contains(ev.Type, "model.response"):
		role = "assistant"
	case strings.Contains(ev.Type, "tool"):
		role = "tool"
	case strings.Contains(ev.Type, "system"):
		role = "system"
	}
	text := firstNonEmpty(
		ev.Data.Text,
		ev.Data.Content,
		ev.Data.Message,
		ev.Data.Prompt,
		ev.Data.Response,
	)
	text = strings.TrimSpace(text)
	if text == "" || role == "" {
		return "", "", false
	}
	if role != "user" && role != "assistant" && role != "tool" && role != "system" {
		return "", "", false
	}
	return text, role, true
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
