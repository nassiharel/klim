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
