package cli

import (
	"errors"
	"testing"

	"github.com/nassiharel/clim/internal/trail"
)

// TestLabelInUseError_ImplementsError sanity-checks that the trail
// package's typed sentinel error for duplicate labels carries the
// information the CLI needs to format a clear message.
func TestLabelInUseError_ImplementsError(t *testing.T) {
	e := &trail.LabelInUseError{Label: "v1.0", Index: 7}
	msg := e.Error()
	if msg == "" || msg == e.Label {
		t.Fatalf("Error() returned %q", msg)
	}
	// errors.As must round-trip the typed value so the CLI can map it
	// to UsageError.
	var got *trail.LabelInUseError
	if !errors.As(error(e), &got) {
		t.Fatalf("errors.As did not recover *LabelInUseError")
	}
	if got.Label != "v1.0" || got.Index != 7 {
		t.Errorf("recovered fields wrong: %+v", got)
	}
}

// TestUniquePrefixLen verifies that the REF column widens its prefix
// when two entries' objects share a 7-char prefix, so the value users
// copy back into trail show / diff is always unambiguous.
func TestUniquePrefixLen(t *testing.T) {
	hex := func(s string) trail.ObjectID {
		// Pad to 64 chars with zeros so IsValid passes structurally.
		for len(s) < 64 {
			s += "0"
		}
		return trail.ObjectID(s)
	}
	// All distinct at 7 chars → returns 7.
	got := uniquePrefixLen([]trail.Entry{
		{Object: hex("abcdef0")},
		{Object: hex("9876543")},
	})
	if got != 7 {
		t.Errorf("distinct prefixes: want 7, got %d", got)
	}
	// Two entries pointing at SAME object: also return 7 (the
	// constraint is uniqueness across distinct objects).
	got = uniquePrefixLen([]trail.Entry{
		{Object: hex("abcdef0")},
		{Object: hex("abcdef0")},
	})
	if got != 7 {
		t.Errorf("identical objects: want 7, got %d", got)
	}
	// 7-char collision → widens to 8 since the 8th char differs.
	got = uniquePrefixLen([]trail.Entry{
		{Object: hex("abcdef00")},
		{Object: hex("abcdef01")},
	})
	if got != 8 {
		t.Errorf("collision at 7: want 8, got %d", got)
	}
}
