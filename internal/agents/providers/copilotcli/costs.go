package copilotcli

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nassiharel/klim/internal/agents"
	"github.com/nassiharel/klim/internal/agents/costs"
)

// copilotEventLine is the minimal subset of a Copilot CLI events.jsonl
// entry that we need for token accounting. Field names follow the
// observed format; unknown shapes are silently skipped.
type copilotEventLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Data      struct {
		SessionID string `json:"sessionId"`
		Model     string `json:"model"`
		Title     string `json:"title"`
		Usage     struct {
			InputTokens  int `json:"inputTokens"`
			OutputTokens int `json:"outputTokens"`
			// Some builds use snake_case; cover both.
			InputTokensSnake  int `json:"input_tokens"`
			OutputTokensSnake int `json:"output_tokens"`
			PromptTokens      int `json:"promptTokens"`
			CompletionTokens  int `json:"completionTokens"`
		} `json:"usage"`
	} `json:"data"`
}

// TokenSamples walks every Copilot session directory (see
// [Provider.sessionDirs] for the on-disk layouts honored) and emits
// per-event samples wherever a usage block is present.
//
// The Copilot CLI event format is reasonably stable for the
// `model.response` / `model.usage` event types, but we don't filter
// by type — any line with usage tokens counts. Missing files /
// missing usage are tolerated silently.
func (p *Provider) TokenSamples(ctx context.Context, in costs.ScanInput) (costs.ScanResult, error) {
	res := costs.ScanResult{Seen: map[string]time.Time{}}
	for _, d := range p.sessionDirs() {
		path := filepath.Join(d.Path, "events.jsonl")
		info, statErr := os.Stat(path)
		if statErr != nil {
			continue
		}
		mtime := info.ModTime().Truncate(time.Second)
		// The session dir name is the session id in the common case
		// (copilotSampleFromLine falls back to it when the in-file id
		// is absent). Use it as the skip/cache key without parsing.
		sessionKey := "copilot:" + d.ID
		if res.Seen[sessionKey].Before(mtime) {
			res.Seen[sessionKey] = mtime
		}
		if prior, ok := in.Prior[sessionKey]; ok && !mtime.After(prior) {
			continue
		}
		samples, err := parseCopilotTranscript(path, d.ID, p.ID())
		if err == nil {
			res.Samples = append(res.Samples, samples...)
		}
	}
	return res, nil
}

func parseCopilotTranscript(path, sessionDir string, providerID agents.ProviderID) ([]costs.TokenSample, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	br := bufio.NewReaderSize(f, 64*1024)
	var samples []costs.TokenSample
	var title string
	for {
		line, readErr := br.ReadBytes('\n')
		if len(line) > 0 {
			var l copilotEventLine
			if json.Unmarshal(line, &l) == nil {
				if l.Data.Title != "" && title == "" {
					title = l.Data.Title
				}
				if s, ok := copilotSampleFromLine(l, sessionDir, providerID, title); ok {
					samples = append(samples, s)
				}
			}
		}
		if readErr != nil {
			break
		}
	}
	return samples, nil
}

func copilotSampleFromLine(l copilotEventLine, sessionDir string, providerID agents.ProviderID, title string) (costs.TokenSample, bool) {
	in := pickFirstNonZero(
		l.Data.Usage.InputTokens,
		l.Data.Usage.InputTokensSnake,
		l.Data.Usage.PromptTokens,
	)
	out := pickFirstNonZero(
		l.Data.Usage.OutputTokens,
		l.Data.Usage.OutputTokensSnake,
		l.Data.Usage.CompletionTokens,
	)
	if in == 0 && out == 0 {
		return costs.TokenSample{}, false
	}
	sessionID := l.Data.SessionID
	if sessionID == "" {
		sessionID = sessionDir
	}
	ts, _ := time.Parse(time.RFC3339, l.Timestamp)
	if ts.IsZero() {
		ts = time.Now()
	}
	day := time.Date(ts.Year(), ts.Month(), ts.Day(), 0, 0, 0, 0, ts.Location())
	return costs.TokenSample{
		Provider:  costs.ProviderID(providerID),
		SessionID: "copilot:" + sessionID,
		Title:     strings.TrimSpace(title),
		Model:     l.Data.Model,
		Day:       day,
		Input:     in,
		Output:    out,
	}, true
}

func pickFirstNonZero(values ...int) int {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}
