// Package bookmarks persists user-marked agent sessions. A bookmark
// is keyed by session ID and carries an optional free-form note; the
// store is loaded from / saved to disk atomically so a crash mid-
// edit can't corrupt the file.
//
// The store has no in-memory caching beyond the lifetime of a Store
// value — callers typically load once at TUI startup and keep the
// pointer around.
package bookmarks

import (
	"os"
	"sort"
	"sync"
	"time"

	"github.com/nassiharel/klim/internal/fileutil"
	"github.com/nassiharel/klim/internal/paths"
)

// Entry is one bookmarked session.
type Entry struct {
	SessionID string    `yaml:"session_id"`
	Note      string    `yaml:"note,omitempty"`
	Created   time.Time `yaml:"created,omitempty"`
}

// Store is the persisted collection of session bookmarks. Methods
// are goroutine-safe for the single-process klim TUI; the file is
// re-written on every mutation via atomic rename so partial writes
// can't corrupt it.
type Store struct {
	mu        sync.RWMutex
	Version   int              `yaml:"version"`
	Bookmarks map[string]Entry `yaml:"bookmarks"`
}

// New returns an empty initialised Store.
func New() *Store {
	return &Store{Version: 1, Bookmarks: map[string]Entry{}}
}

// Load reads the persistent bookmarks file. Missing file returns an
// empty (but valid) store — callers treat that as a cold cache.
//
// Klim < 0.1.4 wrote the file directly to ~/.klim/agent-bookmarks.yaml.
// On first read in a newer klim we migrate that legacy file to the
// new location (~/.klim/agents/bookmarks.yaml) and remove the old
// one. The migration is best-effort: a failure to delete the legacy
// file is logged-by-being-silent — the new file is authoritative.
func Load() (*Store, error) {
	s := New()
	path, err := paths.AgentBookmarks()
	if err != nil {
		return s, err
	}
	found, err := fileutil.ReadYAML(path, s)
	if err != nil {
		return New(), err
	}
	if found {
		if s.Bookmarks == nil {
			return New(), nil
		}
		if s.Version == 0 {
			s.Version = 1
		}
		return s, nil
	}
	// New-location miss: try the legacy path and migrate on hit.
	if migrated, ok := migrateFromLegacy(path); ok {
		return migrated, nil
	}
	return New(), nil
}

// migrateFromLegacy reads the pre-0.1.4 bookmarks file (if present)
// and rewrites it at `newPath`, then removes the legacy file. The
// returned bool is true when a legacy file was found AND
// successfully read into the returned Store. The new-location write
// is best-effort; on failure we still return the loaded data so a
// read-only volume doesn't lose the user's bookmarks.
func migrateFromLegacy(newPath string) (*Store, bool) {
	legacy, err := paths.AgentBookmarksLegacy()
	if err != nil {
		return nil, false
	}
	s := New()
	found, err := fileutil.ReadYAML(legacy, s)
	if err != nil || !found {
		return nil, false
	}
	if s.Bookmarks == nil {
		s.Bookmarks = map[string]Entry{}
	}
	if s.Version == 0 {
		s.Version = 1
	}
	// Write to the new location atomically, then remove the legacy
	// file. Ignore errors on the delete — having both files is a
	// harmless transient state since Load reads new-path first.
	if err := fileutil.WriteYAML(newPath, s, "# klim agent session bookmarks - auto-generated\n"); err == nil {
		_ = os.Remove(legacy)
	}
	return s, true
}

// Save writes the store atomically.
func (s *Store) Save() error {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	path, err := paths.AgentBookmarks()
	if err != nil {
		return err
	}
	return fileutil.WriteYAML(path, s, "# klim agent session bookmarks - auto-generated\n")
}

// Add bookmarks the session if it isn't already; returns true when a
// new entry was added.
func (s *Store) Add(sessionID, note string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Bookmarks[sessionID]; ok {
		// Already bookmarked — update the note if the caller provided
		// one but keep the existing entry's Created time.
		if note != "" {
			e := s.Bookmarks[sessionID]
			e.Note = note
			s.Bookmarks[sessionID] = e
		}
		return false
	}
	s.Bookmarks[sessionID] = Entry{
		SessionID: sessionID,
		Note:      note,
		Created:   time.Now(),
	}
	return true
}

// Remove unbookmarks the session. Returns true when an entry was
// actually deleted (so callers can show a confirmation only on real
// state changes).
func (s *Store) Remove(sessionID string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Bookmarks[sessionID]; !ok {
		return false
	}
	delete(s.Bookmarks, sessionID)
	return true
}

// Toggle adds the bookmark when it's absent and removes it when it's
// present. Returns the new state (true = bookmarked).
func (s *Store) Toggle(sessionID string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Bookmarks[sessionID]; ok {
		delete(s.Bookmarks, sessionID)
		return false
	}
	s.Bookmarks[sessionID] = Entry{SessionID: sessionID, Created: time.Now()}
	return true
}

// SetNote attaches (or clears) the free-form note for a bookmark.
// When the session isn't bookmarked yet, it is created.
func (s *Store) SetNote(sessionID, note string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.Bookmarks[sessionID]
	if !ok {
		e = Entry{SessionID: sessionID, Created: time.Now()}
	}
	e.Note = note
	s.Bookmarks[sessionID] = e
}

// Contains reports whether the session is bookmarked.
func (s *Store) Contains(sessionID string) bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.Bookmarks[sessionID]
	return ok
}

// Get returns the bookmark entry for a session (or zero, false).
func (s *Store) Get(sessionID string) (Entry, bool) {
	if s == nil {
		return Entry{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.Bookmarks[sessionID]
	return e, ok
}

// All returns every entry sorted by creation time descending. The
// caller may freely mutate the returned slice.
func (s *Store) All() []Entry {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Entry, 0, len(s.Bookmarks))
	for _, e := range s.Bookmarks {
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out
}

// Count returns the number of bookmarked sessions.
func (s *Store) Count() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.Bookmarks)
}
