package costs

import (
	"time"

	"github.com/nassiharel/klim/internal/fileutil"
	"github.com/nassiharel/klim/internal/paths"
)

// Cache is the on-disk per-session token-count cache. Keyed by session
// ID, it remembers each transcript's mtime so the parsers can skip
// re-reading files that haven't changed since the last scan.
type Cache struct {
	Version  int                    `yaml:"version"`
	Sessions map[string]CachedEntry `yaml:"sessions"`
}

// CachedEntry is one session's worth of cached token data.
type CachedEntry struct {
	Provider        ProviderID        `yaml:"provider"`
	Title           string            `yaml:"title,omitempty"`
	Model           string            `yaml:"model,omitempty"`
	TranscriptMtime time.Time         `yaml:"transcript_mtime"`
	Days            map[string]Totals `yaml:"days,omitempty"`
}

// NewCache returns an empty Cache ready to be populated.
func NewCache() *Cache {
	return &Cache{Version: 1, Sessions: map[string]CachedEntry{}}
}

// LoadCache reads the on-disk cache. Missing file returns an empty
// cache without error so callers treat that as a cold start.
func LoadCache() (*Cache, error) {
	path, err := paths.AgentCostsCache()
	if err != nil {
		return NewCache(), err
	}
	c := NewCache()
	found, err := fileutil.ReadYAML(path, c)
	if err != nil {
		return NewCache(), err
	}
	if !found || c.Sessions == nil {
		return NewCache(), nil
	}
	if c.Version == 0 {
		c.Version = 1
	}
	return c, nil
}

// Save writes the cache atomically. A nil receiver is a no-op.
func (c *Cache) Save() error {
	if c == nil {
		return nil
	}
	path, err := paths.AgentCostsCache()
	if err != nil {
		return err
	}
	return fileutil.WriteYAML(path, c, "# klim agent-costs cache - auto-generated\n")
}

// SessionTotal sums the cached input/output token totals for one
// session id across its daily buckets, plus whether the session is
// present in the cache. Lets callers (the session detail page) read a
// session's cost instantly instead of re-parsing its transcripts.
func (c *Cache) SessionTotal(id string) (Totals, bool) {
	if c == nil {
		return Totals{}, false
	}
	e, ok := c.Sessions[id]
	if !ok {
		return Totals{}, false
	}
	var total Totals
	for _, t := range e.Days {
		total.Input += t.Input
		total.Output += t.Output
	}
	return total, true
}

// Samples flattens every cached entry into a TokenSample slice, one
// sample per (session, day) bucket. Used by Build() so the Costs
// report sees the full historical record.
func (c *Cache) Samples() []TokenSample {
	if c == nil {
		return nil
	}
	var out []TokenSample
	for sessionID, e := range c.Sessions {
		for day, t := range e.Days {
			d, err := time.ParseInLocation("2006-01-02", day, time.Local)
			if err != nil {
				continue
			}
			out = append(out, TokenSample{
				Provider:  e.Provider,
				SessionID: sessionID,
				Title:     e.Title,
				Model:     e.Model,
				Day:       d,
				Input:     t.Input,
				Output:    t.Output,
			})
		}
	}
	return out
}

// AggregateSession folds the supplied samples for one session into
// a CachedEntry, recording the transcript's mtime so subsequent
// loads can skip reparsing unchanged files.
func AggregateSession(samples []TokenSample, transcriptMtime time.Time) CachedEntry {
	if len(samples) == 0 {
		return CachedEntry{TranscriptMtime: transcriptMtime, Days: map[string]Totals{}}
	}
	entry := CachedEntry{
		Provider:        samples[0].Provider,
		Title:           samples[0].Title,
		Model:           samples[0].Model,
		TranscriptMtime: transcriptMtime,
		Days:            map[string]Totals{},
	}
	for _, s := range samples {
		key := s.Day.Format("2006-01-02")
		t := entry.Days[key]
		t.Input += s.Input
		t.Output += s.Output
		entry.Days[key] = t
		if entry.Title == "" {
			entry.Title = s.Title
		}
		if entry.Model == "" {
			entry.Model = s.Model
		}
	}
	return entry
}

// PruneMissing drops cache entries whose session IDs are absent
// from the present set. Used after every scan to keep the cache
// from growing forever as the user deletes sessions.
func (c *Cache) PruneMissing(present map[string]bool) {
	if c == nil {
		return
	}
	for id := range c.Sessions {
		if !present[id] {
			delete(c.Sessions, id)
		}
	}
}
