package tui

import "github.com/atotto/clipboard"

// Clipboard abstracts system clipboard access for testability.
type Clipboard interface {
	WriteAll(text string) error
}

// systemClipboard is the default Clipboard using the OS clipboard.
type systemClipboard struct{}

// WriteAll copies text to the system clipboard.
func (systemClipboard) WriteAll(text string) error {
	return clipboard.WriteAll(text)
}

// defaultClipboard is the Clipboard used by clipboard-writing actions
// that need to be exercised in tests (e.g. the transcript viewer's
// copy key). It is swappable so headless CI — which has no system
// clipboard — can inject a fake instead of failing the real
// clipboard.WriteAll call. Mirrors the swappable agentsService factory.
var defaultClipboard Clipboard = systemClipboard{}
