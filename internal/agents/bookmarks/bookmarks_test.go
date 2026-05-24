package bookmarks

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestToggleAndPersist(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("KLIM_HOME", tmp)

	s, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Count() != 0 {
		t.Errorf("cold load count = %d, want 0", s.Count())
	}

	if got := s.Toggle("claude:abc"); !got {
		t.Errorf("first Toggle should turn on, got %v", got)
	}
	if !s.Contains("claude:abc") {
		t.Error("Contains should be true after toggle on")
	}
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Reload and verify persistence.
	loaded, err := Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !loaded.Contains("claude:abc") {
		t.Error("bookmark did not persist across save/load")
	}
	if loaded.Count() != 1 {
		t.Errorf("count after reload = %d, want 1", loaded.Count())
	}

	// Toggle off and persist again.
	if got := loaded.Toggle("claude:abc"); got {
		t.Errorf("second Toggle should turn off, got %v", got)
	}
	if err := loaded.Save(); err != nil {
		t.Fatalf("Save 2: %v", err)
	}
	again, _ := Load()
	if again.Contains("claude:abc") {
		t.Error("toggle-off did not persist")
	}

	// Sanity: the file lives under our temp KLIM_HOME.
	t.Logf("bookmarks dir: %s", filepath.Join(tmp, ".klim"))
}

func TestAdd_PreservesCreatedTime_UpdatesNote(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("KLIM_HOME", tmp)
	s := New()
	s.Add("s", "first")
	first := s.Bookmarks["s"].Created
	// Second Add with a different note should NOT create a new entry
	// but should update the note.
	if added := s.Add("s", "second"); added {
		t.Error("re-Add should return false")
	}
	got := s.Bookmarks["s"]
	if !got.Created.Equal(first) {
		t.Error("Created time changed on re-Add")
	}
	if got.Note != "second" {
		t.Errorf("note = %q, want second", got.Note)
	}
}

func TestSetNote_CreatesIfMissing(t *testing.T) {
	s := New()
	s.SetNote("s", "hello")
	e, ok := s.Get("s")
	if !ok {
		t.Fatal("SetNote should have created an entry")
	}
	if e.Note != "hello" {
		t.Errorf("note = %q", e.Note)
	}
}

func TestAll_SortedNewestFirst(t *testing.T) {
	s := New()
	s.Add("a", "")
	// Force a measurable Created delta so the sort is deterministic.
	time.Sleep(2 * time.Millisecond)
	s.Add("b", "")
	all := s.All()
	if len(all) != 2 {
		t.Fatalf("len = %d", len(all))
	}
	if all[0].SessionID != "b" || all[1].SessionID != "a" {
		t.Errorf("order = %v, want [b,a]", []string{all[0].SessionID, all[1].SessionID})
	}
}

func TestRemove_ReportsTruth(t *testing.T) {
	s := New()
	s.Add("s", "")
	if !s.Remove("s") {
		t.Error("Remove(existing) should return true")
	}
	if s.Remove("nonexistent") {
		t.Error("Remove(missing) should return false")
	}
}

func TestLoad_MigratesLegacyLocation(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("KLIM_HOME", tmp)

	// Plant a legacy bookmarks file at ~/.klim/agent-bookmarks.yaml.
	legacy := New()
	legacy.Bookmarks["claude:legacy"] = Entry{SessionID: "claude:legacy", Note: "from old layout", Created: time.Now()}
	if err := saveLegacyForTest(legacy); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}

	// Load should find the legacy file, migrate it, and return it.
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !got.Contains("claude:legacy") {
		t.Error("migrated store missing legacy bookmark")
	}

	// New-location file should now exist…
	newPath := filepath.Join(tmp, "agents", "bookmarks.yaml")
	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("new-location file not written: %v", err)
	}
	// …and the legacy file should be gone.
	legacyPath := filepath.Join(tmp, "agent-bookmarks.yaml")
	if _, err := os.Stat(legacyPath); err == nil {
		t.Error("legacy file should have been removed after migration")
	}
}

// saveLegacyForTest writes a Store at the pre-0.1.4 location.
func saveLegacyForTest(s *Store) error {
	path, err := pathsAgentBookmarksLegacy()
	if err != nil {
		return err
	}
	return fileutilWriteYAML(path, s)
}
