package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/nassiharel/klim/internal/fileutil"
	"github.com/nassiharel/klim/internal/paths"
)

// showFirstRunWelcome prints a short getting-started message on klim's very
// first interactive launch and returns true if it did. On the first run it
// writes a marker file and returns true (the caller exits instead of launching
// the TUI), so a brand-new user lands on actionable guidance rather than a
// multi-tab interface. On every subsequent run it returns false immediately.
//
// If the marker can't be read or written (e.g. unwritable home), it fails
// open — returns false — so the TUI still launches normally.
func showFirstRunWelcome(w io.Writer) bool {
	marker, err := paths.FirstRunMarker()
	if err != nil {
		return false
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		return false // already welcomed
	} else if !os.IsNotExist(statErr) {
		// Some other Stat error (permission denied, I/O error). Fail open:
		// don't show the welcome and don't risk overwriting an existing
		// marker we simply couldn't read.
		return false
	}
	// Best-effort marker write; if it fails, don't trap the user in a loop —
	// just launch the TUI this time.
	if err := fileutil.AtomicWrite(marker, []byte("klim has greeted you.\n"), 0o644); err != nil {
		return false
	}

	color := os.Getenv("NO_COLOR") == ""
	b := func(s string) string {
		if color {
			return cBold + cWhite + s + cReset
		}
		return s
	}
	teal := func(s string) string {
		if color {
			return cTeal + s + cReset
		}
		return s
	}
	gray := func(s string) string {
		if color {
			return cGray + s + cReset
		}
		return s
	}

	_, _ = fmt.Fprintf(w, "\n  %s — set up any dev machine with one command.\n\n", b("Welcome to klim"))
	_, _ = fmt.Fprintln(w, "  Start here:")
	_, _ = fmt.Fprintf(w, "    %s   %s\n", teal("klim onboard"), gray("pick your role → klim installs the recommended tools"))
	_, _ = fmt.Fprintf(w, "    %s   %s\n", teal("klim install ripgrep fzf gh"), gray("install tools by name"))
	_, _ = fmt.Fprintf(w, "    %s   %s\n", teal("klim search kubernetes"), gray("browse the marketplace"))
	_, _ = fmt.Fprintf(w, "    %s   %s\n", teal("klim --help"), gray("every command"))
	_, _ = fmt.Fprintf(w, "\n  Run %s again to open the interactive TUI.\n\n", teal("klim"))
	return true
}
