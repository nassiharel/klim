package bookmarks

import (
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
