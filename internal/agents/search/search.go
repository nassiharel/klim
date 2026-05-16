// Package search powers the full-text search overlay in the Agents
// tab. Each Provider that wants to participate implements a
// SessionTexts method returning a flat list of SessionText records;
// the package indexes them (mtime-keyed cache at
// ~/.klim/cache/agent-search-index.yaml) and serves substring queries
// against the cached lines.
package search

import (
	"sort"
	"strings"
	"time"

	"github.com/nassiharel/klim/internal/fileutil"
	"github.com/nassiharel/klim/internal/paths"
)

// ProviderID mirrors agents.ProviderID as a string so we avoid an
// import cycle.
type ProviderID string

// SessionLine is one extracted message from a transcript. Role is the
// speaker (user / assistant / system / tool). Timestamp may be zero
// when the provider didn't include one. LineNo is the 1-indexed
// position inside the source transcript so the viewer can jump there.
type SessionLine struct {
	Role      string    `yaml:"role"`
	Text      string    `yaml:"text"`
	Timestamp time.Time `yaml:"ts,omitempty"`
	LineNo    int       `yaml:"line"`
}

// SessionText is the searchable view of one session's transcript.
type SessionText struct {
	SessionID       string        `yaml:"session_id"`
	Provider        ProviderID    `yaml:"provider"`
	Title           string        `yaml:"title,omitempty"`
	TranscriptPath  string        `yaml:"path,omitempty"`
	TranscriptMtime time.Time     `yaml:"mtime,omitempty"`
	Lines           []SessionLine `yaml:"lines,omitempty"`
}

// Hit is one query result.
type Hit struct {
	SessionID string
	Provider  ProviderID
	Title     string
	Path      string
	LineNo    int
	Role      string
	Text      string // the full line text (so the viewer can highlight)
	Snippet   string // a short context window around the match
	Score     int    // higher is better (matches in title, exact case match, etc.)
}

// Index is the persisted, queryable collection of SessionText records.
type Index struct {
	Version  int                    `yaml:"version"`
	Sessions map[string]SessionText `yaml:"sessions"`
}

// NewIndex returns an empty initialised Index.
func NewIndex() *Index {
	return &Index{Version: 1, Sessions: map[string]SessionText{}}
}

// LoadIndex reads the persisted index. Missing file returns an empty
// (but valid) index — callers treat that as a cold cache.
func LoadIndex() (*Index, error) {
	path, err := paths.AgentSearchIndex()
	if err != nil {
		return NewIndex(), err
	}
	idx := NewIndex()
	found, err := fileutil.ReadYAML(path, idx)
	if err != nil {
		return NewIndex(), err
	}
	if !found || idx.Sessions == nil {
		return NewIndex(), nil
	}
	if idx.Version == 0 {
		idx.Version = 1
	}
	return idx, nil
}

// Save writes the index atomically.
func (idx *Index) Save() error {
	if idx == nil {
		return nil
	}
	path, err := paths.AgentSearchIndex()
	if err != nil {
		return err
	}
	return fileutil.WriteYAML(path, idx, "# klim agent-search index - auto-generated\n")
}

// PruneMissing drops sessions absent from the `present` set.
func (idx *Index) PruneMissing(present map[string]bool) {
	if idx == nil {
		return
	}
	for id := range idx.Sessions {
		if !present[id] {
			delete(idx.Sessions, id)
		}
	}
}

// Merge replaces the entry for each provided SessionText. Lines slices
// are stored by value so callers can mutate their copies afterwards.
func (idx *Index) Merge(texts []SessionText) {
	if idx == nil {
		return
	}
	for _, t := range texts {
		idx.Sessions[t.SessionID] = t
	}
}

// Query searches every indexed session for the given substring (case-
// insensitive) and returns the matching lines, sorted by relevance.
// At most `limit` hits are returned; pass 0 for "no limit".
func (idx *Index) Query(q string, limit int) []Hit {
	q = strings.TrimSpace(q)
	if q == "" || idx == nil {
		return nil
	}
	qLower := strings.ToLower(q)
	var hits []Hit
	for _, sess := range idx.Sessions {
		titleHit := strings.Contains(strings.ToLower(sess.Title), qLower)
		for _, line := range sess.Lines {
			textLower := strings.ToLower(line.Text)
			pos := strings.Index(textLower, qLower)
			if pos < 0 {
				continue
			}
			h := Hit{
				SessionID: sess.SessionID,
				Provider:  sess.Provider,
				Title:     sess.Title,
				Path:      sess.TranscriptPath,
				LineNo:    line.LineNo,
				Role:      line.Role,
				Text:      line.Text,
				Snippet:   snippetAround(line.Text, pos, len(q), 70),
				Score:     scoreHit(line, pos, q, titleHit),
			}
			hits = append(hits, h)
			if limit > 0 && len(hits) >= limit*4 {
				// Sort-and-truncate later; collect a bit more so we
				// don't miss higher-ranked matches.
				break
			}
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		// Newer sessions first when scores tie.
		return hits[i].SessionID > hits[j].SessionID
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// scoreHit returns a relevance score for ranking. Matches in the
// assistant role outrank user/system, exact-case matches outrank
// case-insensitive, and a title match boosts the line.
func scoreHit(line SessionLine, pos int, q string, titleHit bool) int {
	score := 100
	if line.Role == "assistant" {
		score += 20
	}
	if line.Role == "user" {
		score += 10
	}
	if strings.Contains(line.Text, q) {
		score += 15 // exact case match
	}
	if titleHit {
		score += 25
	}
	// Earlier matches in the line are slightly better — readers see
	// the match closer to the snippet start.
	score -= pos / 50
	return score
}

// snippetAround returns at most width characters centered on pos.
// The match itself is uppercased in the returned snippet so a viewer
// can render it bold/highlighted using ANSI later if it wants to.
func snippetAround(text string, pos, qLen, width int) string {
	if pos < 0 || pos >= len(text) {
		return text
	}
	start := pos - width/2
	if start < 0 {
		start = 0
	}
	end := start + width
	if end > len(text) {
		end = len(text)
		start = end - width
		if start < 0 {
			start = 0
		}
	}
	s := text[start:end]
	prefix := ""
	if start > 0 {
		prefix = "…"
	}
	suffix := ""
	if end < len(text) {
		suffix = "…"
	}
	return prefix + s + suffix
}

// Present returns the set of session IDs currently in the index, for
// PruneMissing comparisons.
func (idx *Index) Present() map[string]bool {
	out := make(map[string]bool, len(idx.Sessions))
	for id := range idx.Sessions {
		out[id] = true
	}
	return out
}
