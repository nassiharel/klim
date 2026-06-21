package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShowFirstRunWelcome(t *testing.T) {
	t.Setenv("KLIM_HOME", t.TempDir())
	t.Setenv("NO_COLOR", "1")

	var buf bytes.Buffer

	// First call: prints the welcome and returns true.
	if got := showFirstRunWelcome(&buf); !got {
		t.Fatalf("first call returned false, want true")
	}
	out := buf.String()
	for _, want := range []string{"Welcome to klim", "klim onboard", "klim --help"} {
		if !strings.Contains(out, want) {
			t.Errorf("welcome output missing %q\n---\n%s", want, out)
		}
	}

	// Marker should now exist.
	marker, err := firstRunMarkerPath(t)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("marker not written: %v", err)
	}

	// Second call: no welcome, returns false.
	buf.Reset()
	if got := showFirstRunWelcome(&buf); got {
		t.Errorf("second call returned true, want false")
	}
	if buf.Len() != 0 {
		t.Errorf("second call printed output: %q", buf.String())
	}
}

func firstRunMarkerPath(t *testing.T) (string, error) {
	t.Helper()
	return filepath.Join(os.Getenv("KLIM_HOME"), ".welcomed"), nil
}
