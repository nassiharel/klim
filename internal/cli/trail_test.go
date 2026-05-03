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
