package claudecode

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
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
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(projects, e.Name())

		// The session list (Sessions()) shows one row per PROJECT DIR
		// with ID "claude:"+<dir>, and the cost cache keys by the same
		// id. A dir can hold many transcript files, so we key the whole
		// dir by its NEWEST .jsonl mtime: collect the files + newest
		// mtime first, then skip or parse the dir as a unit. Keying
		// per-file (by UUID) would split one project's cost across
		// thousands of non-existent session ids and diverge from the
		// skip key.
		var files []string
		var newest time.Time
		walkErr := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(d.Name()), ".jsonl") {
				return nil
			}
			info, statErr := d.Info()
			if statErr != nil {
				return nil
			}
			files = append(files, path)
			if mt := info.ModTime().Truncate(time.Second); newest.Before(mt) {
				newest = mt
			}
			return nil
		})
		if walkErr != nil {
			return res, walkErr
		}
		if len(files) == 0 {
			continue
		}

		sessionKey := "claude:" + e.Name()
		res.Seen[sessionKey] = newest

		// Skip re-parsing the whole dir when its newest file is no newer
		// than the cached scan. !newest.After(prior) means newest <=
		// prior: equal mtimes (the common case) skip; a strictly-newer
		// file forces a fresh parse of the dir.
		if prior, ok := in.Prior[sessionKey]; ok && !newest.After(prior) {
			continue
		}

		for _, path := range files {
			if ctx.Err() != nil {
				return res, ctx.Err()
			}
			samples, err := parseClaudeTranscript(path, e.Name(), p.ID())
			if err == nil {
				res.Samples = append(res.Samples, samples...)
			}
		}
	}
	return res, nil
}

// SessionTokens sums the input/output token usage for ONE session by
// parsing only that session's transcripts — used by the session detail
// page so it doesn't have to scan every project. The id is the session
// list id ("claude:"+<dir>); a Claude session is a project directory,
// so we parse every .jsonl under ~/.claude/projects/<dir>.
//
// The dir component is validated as a plain directory name (no path
// separators / volume / parent refs) before joining, so a crafted id
// can't escape the projects root.
func (p *Provider) SessionTokens(ctx context.Context, id string) (costs.Totals, error) {
	dir := strings.TrimPrefix(id, "claude:")
	if dir == "" || dir == "." || dir == ".." ||
		strings.ContainsAny(dir, `/\`) ||
		filepath.VolumeName(dir) != "" ||
		filepath.Base(dir) != dir ||
		filepath.Clean(dir) != dir {
		return costs.Totals{}, fmt.Errorf("session tokens: invalid session id %q", id)
	}
	projectDir := filepath.Join(p.claudeDir(), "projects", dir)
	if _, err := os.Stat(projectDir); err != nil {
		return costs.Totals{}, err
	}
	var total costs.Totals
	walkErr := filepath.WalkDir(projectDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".jsonl") {
			return nil
		}
		samples, perr := parseClaudeTranscript(path, dir, p.ID())
		if perr != nil {
			return nil
		}
		for _, s := range samples {
			total.Input += s.Input
			total.Output += s.Output
		}
		return nil
	})
	if walkErr != nil {
		return costs.Totals{}, walkErr
	}
	return total, nil
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
	// Key cost samples by the project DIRECTORY name, not the in-file
	// sessionId. The session list (Sessions()) shows one row per project
	// dir with ID "claude:"+<dir>, and a single dir holds many
	// transcript files; keying by the per-file UUID would (a) split one
	// project's cost across thousands of ids that don't exist in the
	// session list (breaking Enter-to-open) and (b) diverge from the
	// dir-derived Seen/Prior skip key, breaking incremental skipping.
	sessionID := projectName
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
