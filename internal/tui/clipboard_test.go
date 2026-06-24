package tui

// fakeClipboard is a test Clipboard that records the last text written
// instead of touching the OS clipboard. Lets clipboard-writing actions
// be exercised on headless CI, which has no system clipboard.
type fakeClipboard struct {
	text string
	err  error
}

func (f *fakeClipboard) WriteAll(text string) error {
	if f.err != nil {
		return f.err
	}
	f.text = text
	return nil
}

// swapClipboard installs c as the package clipboard and returns a
// restore func (call via defer) that puts the previous one back.
func swapClipboard(c Clipboard) func() {
	prev := defaultClipboard
	defaultClipboard = c
	return func() { defaultClipboard = prev }
}
