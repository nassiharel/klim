package tui

import (
	"strings"
	"testing"

	"github.com/nassiharel/klim/internal/doctor"
)

func TestBuildFixOptions_CopyCommandHasRunAndCopy(t *testing.T) {
	issue := doctor.Issue{
		Action: doctor.Action{
			Kind:    doctor.ActionCopyCommand,
			Command: "echo hi",
			Label:   "Test",
		},
	}
	opts := buildFixOptions(issue)
	if len(opts) != 3 {
		t.Fatalf("want 3 options (Run, Copy, Cancel), got %d", len(opts))
	}
	if opts[0].Key != "run" || opts[1].Key != "copy" || opts[2].Key != "cancel" {
		t.Errorf("unexpected option keys: %q %q %q", opts[0].Key, opts[1].Key, opts[2].Key)
	}
}

func TestBuildFixOptions_JumpKindsSingleConfirmPlusCancel(t *testing.T) {
	for _, kind := range []doctor.ActionKind{doctor.ActionJumpPathView, doctor.ActionRescan, doctor.ActionJumpUpdates} {
		opts := buildFixOptions(doctor.Issue{Action: doctor.Action{Kind: kind, Target: "node"}})
		if len(opts) != 2 {
			t.Fatalf("kind %q: want 2 options, got %d", kind, len(opts))
		}
		if opts[1].Key != "cancel" {
			t.Errorf("kind %q: last option should be cancel, got %q", kind, opts[1].Key)
		}
	}
}

func TestBuildFixOptions_NoneReturnsNothing(t *testing.T) {
	opts := buildFixOptions(doctor.Issue{})
	if len(opts) != 0 {
		t.Errorf("ActionNone should yield no options, got %d", len(opts))
	}
}

func TestSoftWrap_WrapsLongLineAtWhitespace(t *testing.T) {
	in := "this is a fairly long sentence that should wrap across several lines"
	out := softWrap(in, 20, "  ", nil)
	for _, line := range strings.Split(out, "\n") {
		if len(line) > 24 { // 20 + a bit of slack for the indent
			t.Errorf("line exceeds width: %q", line)
		}
	}
	if !strings.HasPrefix(out, "  this") {
		t.Errorf("indent not applied to first line: %q", out)
	}
}

func TestSoftWrap_HardBreaksOverlongToken(t *testing.T) {
	in := strings.Repeat("x", 30)
	out := softWrap(in, 10, "", nil)
	parts := strings.Split(out, "\n")
	if len(parts) < 3 {
		t.Errorf("long token should hard-break into multiple lines, got %d", len(parts))
	}
}
