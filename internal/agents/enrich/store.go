package enrich

import (
	"sort"
	"sync"

	"github.com/nassiharel/klim/internal/fileutil"
	"github.com/nassiharel/klim/internal/paths"
)

// GroupingMappings is the persisted set of user-supplied cwd → group
// mappings. Stored as YAML at ~/.klim/agents/sessions-grouping.yaml.
//
// The file is hand-edited and rewritten by `klim agents sessions
// group set`; the store reads it on every Load and writes it
// atomically via fileutil.WriteYAML so an interrupted edit can't
// corrupt the file.
type GroupingMappings struct {
	mu       sync.RWMutex
	Version  int               `yaml:"version"`
	Mappings map[string]string `yaml:"mappings"`
}

// NewGroupingMappings returns an empty initialised store.
func NewGroupingMappings() *GroupingMappings {
	return &GroupingMappings{Version: 1, Mappings: map[string]string{}}
}

// LoadGroupingMappings reads the persisted mappings file. A missing
// file returns an empty store; a corrupt file returns the error so
// callers can surface it rather than silently replacing the user's
// mappings with an empty set.
func LoadGroupingMappings() (*GroupingMappings, error) {
	s := NewGroupingMappings()
	path, err := groupingPath()
	if err != nil {
		return s, err
	}
	found, err := fileutil.ReadYAML(path, s)
	if err != nil {
		return NewGroupingMappings(), err
	}
	if found {
		if s.Mappings == nil {
			s.Mappings = map[string]string{}
		}
		if s.Version == 0 {
			s.Version = 1
		}
	}
	return s, nil
}

// Save writes the store atomically.
func (s *GroupingMappings) Save() error {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	path, err := groupingPath()
	if err != nil {
		return err
	}
	return fileutil.WriteYAML(path, s, "# klim sessions grouping mappings - auto-generated\n")
}

// Set adds or replaces a mapping. The needle is a cwd substring; the
// group is the resolved group name.
func (s *GroupingMappings) Set(needle, group string) {
	if s == nil || needle == "" || group == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Mappings == nil {
		s.Mappings = map[string]string{}
	}
	s.Mappings[needle] = group
}

// Delete removes a mapping. Returns true when an entry was actually
// removed.
func (s *GroupingMappings) Delete(needle string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.Mappings[needle]; !ok {
		return false
	}
	delete(s.Mappings, needle)
	return true
}

// All returns a copy of the mappings as a plain map suitable for
// passing to Resolve.
func (s *GroupingMappings) All() map[string]string {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.Mappings))
	for k, v := range s.Mappings {
		out[k] = v
	}
	return out
}

// GroupingEntry is one (pattern, group) mapping in the persisted
// store, used by `klim agents sessions group list`.
type GroupingEntry struct {
	Pattern string
	Group   string
}

// Entries returns the mappings sorted by pattern for deterministic
// CLI output.
func (s *GroupingMappings) Entries() []GroupingEntry {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]GroupingEntry, 0, len(s.Mappings))
	for k, v := range s.Mappings {
		out = append(out, GroupingEntry{Pattern: k, Group: v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Pattern < out[j].Pattern })
	return out
}

// groupingPath returns the on-disk YAML location for the mappings.
func groupingPath() (string, error) {
	return paths.Join("agents", "sessions-grouping.yaml")
}
