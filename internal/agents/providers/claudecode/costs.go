package claudecode

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

// claudeTranscriptLine captures the fields of a Claude Code transcript
// JSONL entry that we care about for token accounting. Unknown fields
// are ignored. The schema is undocumented and varies across versions,
// so the parser is intentionally permissive — missing fields produce
// a zero-token sample (which Build() then ignores).
type claudeTranscriptLine struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	SessionID string `json:"sessionId"`
	// SessionIDAlt covers the snake_case variant some builds emit.
	SessionIDAlt string `json:"session_id"`
	Message      struct {
		Model string `json:"model"`
		Usage struct {
			InputTokens        int `json:"input_tokens"`
			OutputTokens       int `json:"output_tokens"`
			CacheCreationInput int `json:"cache_creation_input_tokens"`
			CacheReadInput     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// TokenSamples walks ~/.claude/projects/<encoded>/*.jsonl and emits
// one sample per assistant message that has a usage block. Sessions
// without parsable transcripts contribute zero samples; missing dirs
// are not an error.
//
// Scanning is incremental: a transcript whose mtime matches the value
// in.Prior already recorded for its session is NOT re-read — only its
// session id + mtime are reported in Seen so the caller keeps the
// cached entry. This is the hot path: a heavy user can have thousands
// of multi-MB transcripts, and re-parsing all of them on every Costs
// scan is what made the tab slow.
//
// The transcript layout is best-effort: Claude Code's session
// transcript format is undocumented, so the parser scans every .jsonl
// file under each project directory and looks for the common usage
// shape used by recent CLI builds.
func (p *Provider) TokenSamples(ctx context.Context, in costs.ScanInput) (costs.ScanResult, error) {
	projects := filepath.Join(p.claudeDir(), "projects")
	entries, err := os.ReadDir(projects)
	if err != nil {
		// No transcripts yet — that's fine.
		return costs.ScanResult{}, nil
	}
	res := costs.ScanResult{Seen: map[string]time.Time{}}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(projects, e.Name())
		// Each session's transcript may be one .jsonl file or many;
		// walk them all so we don't miss the right one.
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(d.Name()), ".jsonl") {
				return nil
			}
			info, statErr := d.Info()
			if statErr != nil {
				return nil
			}
			mtime := info.ModTime().Truncate(time.Second)

			// The Claude transcript filename is the session UUID, which
			// is exactly the in-file sessionId — so we can identify the
			// session (and thus its cache key) without parsing. Fall
			// back to the project dir name to match claudeSampleFromLine.
			sid := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
			if sid == "" {
				sid = e.Name()
			}
			sessionKey := "claude:" + sid

			// Record the newest mtime per session for Seen.
			if res.Seen[sessionKey].Before(mtime) {
				res.Seen[sessionKey] = mtime
			}

			// Skip re-parsing when the file hasn't changed since the
			// cached scan. !mtime.After(prior) means mtime <= prior:
			// equal mtimes (the common case) skip; a strictly-newer
			// file forces a fresh parse.
			if prior, ok := in.Prior[sessionKey]; ok && !mtime.After(prior) {
				return nil
			}

			samples, err := parseClaudeTranscript(path, e.Name(), p.ID())
			if err == nil {
				res.Samples = append(res.Samples, samples...)
			}
			return nil
		})
	}
	return res, nil
}

func parseClaudeTranscript(path, projectName string, providerID agents.ProviderID) ([]costs.TokenSample, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	br := bufio.NewReaderSize(f, 64*1024)
	var samples []costs.TokenSample
	for {
		line, readErr := br.ReadBytes('\n')
		if len(line) > 0 {
			var l claudeTranscriptLine
			if json.Unmarshal(line, &l) == nil {
				if s, ok := claudeSampleFromLine(l, projectName, providerID); ok {
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

func claudeSampleFromLine(l claudeTranscriptLine, projectName string, providerID agents.ProviderID) (costs.TokenSample, bool) {
	in := l.Message.Usage.InputTokens + l.Message.Usage.CacheCreationInput + l.Message.Usage.CacheReadInput
	out := l.Message.Usage.OutputTokens
	if in == 0 && out == 0 {
		return costs.TokenSample{}, false
	}
	sessionID := l.SessionID
	if sessionID == "" {
		sessionID = l.SessionIDAlt
	}
	if sessionID == "" {
		// Fall back to project dir name so we still get a unique key.
		sessionID = projectName
	}
	ts, _ := time.Parse(time.RFC3339, l.Timestamp)
	if ts.IsZero() {
		ts = time.Now()
	}
	day := time.Date(ts.Year(), ts.Month(), ts.Day(), 0, 0, 0, 0, ts.Location())
	return costs.TokenSample{
		Provider:  costs.ProviderID(providerID),
		SessionID: "claude:" + sessionID,
		Title:     projectName,
		Model:     l.Message.Model,
		Day:       day,
		Input:     in,
		Output:    out,
	}, true
}
