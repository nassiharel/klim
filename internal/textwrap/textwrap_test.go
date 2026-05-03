package textwrap

import (
	"reflect"
	"testing"
)

func TestWrap_ASCII(t *testing.T) {
	got := Wrap("the quick brown fox jumps over the lazy dog", 12)
	want := []string{"the quick", "brown fox", "jumps over", "the lazy dog"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestWrap_EmptyInput(t *testing.T) {
	if got := Wrap("", 10); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestWrap_NoLimit(t *testing.T) {
	if got := Wrap("hello", 0); len(got) != 1 || got[0] != "hello" {
		t.Errorf("expected [%q], got %v", "hello", got)
	}
}

// TestWrap_DisplayWidth guards against the regression where wrap was
// measured by raw byte count. CJK glyphs occupy 2 display columns
// each, so 4 of them in a row already fill 8 columns even though
// they're 12 bytes.
func TestWrap_DisplayWidth(t *testing.T) {
	got := Wrap("中文中文 abcd", 6)
	want := []string{"中文中文", "abcd"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestWrap_OverlongWordEmittedAlone(t *testing.T) {
	got := Wrap("a verylongunbreakableword b", 6)
	want := []string{"a", "verylongunbreakableword", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}
